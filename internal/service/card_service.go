// Package service provides business logic orchestration for the Card Issuer API.
// Services coordinate between domain entities, repositories, and cryptographic
// operations. They enforce business rules and ensure atomicity through transactions.
package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/novopayment/card-issuer-api/internal/crypto"
	"github.com/novopayment/card-issuer-api/internal/domain"
	"github.com/novopayment/card-issuer-api/internal/repository"
)

// CardService orchestrates card lifecycle operations.
// It coordinates encryption, blind indexing, state transitions, and audit logging.
type CardService struct {
	cards     repository.CardRepository
	audit     repository.AuditRepository
	encryptor crypto.Encryptor
	indexer   crypto.BlindIndexer
	txManager repository.TxManager
	logger    *slog.Logger
}

// NewCardService creates a new CardService with all required dependencies.
func NewCardService(
	cards repository.CardRepository,
	audit repository.AuditRepository,
	encryptor crypto.Encryptor,
	indexer crypto.BlindIndexer,
	txManager repository.TxManager,
	logger *slog.Logger,
) *CardService {
	return &CardService{
		cards:     cards,
		audit:     audit,
		encryptor: encryptor,
		indexer:   indexer,
		txManager: txManager,
		logger:    logger,
	}
}

// CreateCard creates a new card in PENDING status for the given cardholder.
func (s *CardService) CreateCard(ctx context.Context, cardholderID uuid.UUID) (*domain.Card, error) {
	tenantID := domain.MustTenantID(ctx)

	card := domain.NewCard(tenantID, cardholderID)

	err := s.txManager.WithTransaction(ctx, func(ctx context.Context, repos repository.TxRepositories) error {
		if err := repos.Cards().Create(ctx, card); err != nil {
			return err
		}

		auditEvent := domain.NewAuditEvent(tenantID, "card", card.ID, "CREATED", "", string(card.Status), "operator")
		return repos.Audit().Create(ctx, auditEvent)
	})

	if err != nil {
		return nil, fmt.Errorf("creating card: %w", err)
	}

	return card, nil
}

// GetCard retrieves a card by ID, scoped to the authenticated tenant.
func (s *CardService) GetCard(ctx context.Context, cardID uuid.UUID) (*domain.Card, error) {
	tenantID := domain.MustTenantID(ctx)
	return s.cards.GetByID(ctx, tenantID, cardID)
}

// ListCards returns a paginated list of cards with optional status filter.
func (s *CardService) ListCards(ctx context.Context, status *domain.CardStatus, limit, offset int) ([]*domain.Card, int, error) {
	tenantID := domain.MustTenantID(ctx)
	return s.cards.List(ctx, tenantID, status, limit, offset)
}

// IssueCard transitions a card from PENDING to ACTIVE.
// This generates a PAN, encrypts it with AES-256-GCM (using tenant_id as AAD),
// computes a blind index for duplicate detection, and persists everything atomically.
//
// SECURITY:
//   - PAN plaintext exists only in memory during this function call
//   - PAN bytes are zeroized after encryption
//   - Audit log records status change but NEVER PAN values
func (s *CardService) IssueCard(ctx context.Context, cardID uuid.UUID) (*domain.Card, error) {
	tenantID := domain.MustTenantID(ctx)

	var updatedCard *domain.Card

	err := s.txManager.WithTransaction(ctx, func(ctx context.Context, repos repository.TxRepositories) error {
		card, err := repos.Cards().GetByID(ctx, tenantID, cardID)
		if err != nil {
			return err
		}

		oldStatus := card.Status

		// Generate a Luhn-valid PAN
		pan := domain.GeneratePAN()
		panBytes := []byte(pan.RawValue())
		defer crypto.Zeroize(panBytes) // Best-effort memory clearing

		// Encrypt PAN with tenant_id as AAD — prevents ciphertext migration between tenants
		aad := []byte(tenantID.String())
		encryptedPAN, err := s.encryptor.Encrypt(panBytes, aad)
		if err != nil {
			return fmt.Errorf("encrypting PAN: %w", err)
		}

		// Compute blind index for exact-match lookups without decryption
		blindIndex := s.indexer.ComputeIndex(panBytes)

		// Check for duplicate PAN (collision detection via blind index)
		existing, err := repos.Cards().GetByBlindIndex(ctx, tenantID, blindIndex)
		if err != nil && err != domain.ErrNotFound {
			return fmt.Errorf("checking for duplicate PAN: %w", err)
		}
		if existing != nil {
			return domain.ErrDuplicatePAN
		}

		// Apply domain state transition
		if err := card.Issue(pan, int(pan[len(pan.RawValue())-2]-'0')+1, 2029); err != nil {
			return err
		}

		// Set encrypted fields
		card.PANEncrypted = encryptedPAN
		card.PANBlindIndex = blindIndex

		if err := repos.Cards().Update(ctx, card); err != nil {
			return err
		}

		// Audit: record status change, NEVER PAN data
		auditEvent := domain.NewAuditEvent(tenantID, "card", card.ID, "ISSUED", string(oldStatus), string(card.Status), "operator")
		if err := repos.Audit().Create(ctx, auditEvent); err != nil {
			return err
		}

		updatedCard = card
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("issuing card: %w", err)
	}

	return updatedCard, nil
}

// SuspendCard transitions a card from ACTIVE to SUSPENDED.
func (s *CardService) SuspendCard(ctx context.Context, cardID uuid.UUID) (*domain.Card, error) {
	return s.changeCardStatus(ctx, cardID, "SUSPENDED", func(c *domain.Card) error {
		return c.Suspend()
	})
}

// ReactivateCard transitions a card from SUSPENDED to ACTIVE.
func (s *CardService) ReactivateCard(ctx context.Context, cardID uuid.UUID) (*domain.Card, error) {
	return s.changeCardStatus(ctx, cardID, "REACTIVATED", func(c *domain.Card) error {
		return c.Reactivate()
	})
}

// CloseCard transitions a card to CLOSED (terminal state — irreversible).
func (s *CardService) CloseCard(ctx context.Context, cardID uuid.UUID) (*domain.Card, error) {
	return s.changeCardStatus(ctx, cardID, "CLOSED", func(c *domain.Card) error {
		return c.Close()
	})
}

// changeCardStatus is a generic helper for card state transitions.
// It handles the common pattern of: fetch → validate transition → update → audit.
func (s *CardService) changeCardStatus(ctx context.Context, cardID uuid.UUID, action string, transition func(*domain.Card) error) (*domain.Card, error) {
	tenantID := domain.MustTenantID(ctx)

	var updatedCard *domain.Card
	err := s.txManager.WithTransaction(ctx, func(ctx context.Context, repos repository.TxRepositories) error {
		card, err := repos.Cards().GetByID(ctx, tenantID, cardID)
		if err != nil {
			return err
		}
		oldStatus := card.Status

		if err := transition(card); err != nil {
			return err
		}

		if err := repos.Cards().Update(ctx, card); err != nil {
			return err
		}

		auditEvent := domain.NewAuditEvent(tenantID, "card", card.ID, action, string(oldStatus), string(card.Status), "operator")
		if err := repos.Audit().Create(ctx, auditEvent); err != nil {
			return err
		}

		updatedCard = card
		return nil
	})

	return updatedCard, err
}
