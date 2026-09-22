package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/novopayment/card-issuer-api/internal/domain"
	"github.com/novopayment/card-issuer-api/internal/repository"
)

// CardholderService manages cardholder CRUD operations.
type CardholderService struct {
	cardholders repository.CardholderRepository
}

// NewCardholderService creates a new CardholderService.
func NewCardholderService(cardholders repository.CardholderRepository) *CardholderService {
	return &CardholderService{
		cardholders: cardholders,
	}
}

// CreateCardholder creates a new cardholder for the authenticated tenant.
func (s *CardholderService) CreateCardholder(ctx context.Context, firstName, lastName, email, phone string) (*domain.Cardholder, error) {
	tenantID := domain.MustTenantID(ctx)

	ch := domain.NewCardholder(tenantID, firstName, lastName)
	ch.Email = email
	ch.Phone = phone

	if err := ch.Validate(); err != nil {
		return nil, err
	}

	if err := s.cardholders.Create(ctx, ch); err != nil {
		return nil, fmt.Errorf("creating cardholder: %w", err)
	}
	return ch, nil
}

// GetCardholder retrieves a cardholder by ID, scoped to the authenticated tenant.
func (s *CardholderService) GetCardholder(ctx context.Context, id uuid.UUID) (*domain.Cardholder, error) {
	tenantID := domain.MustTenantID(ctx)
	return s.cardholders.GetByID(ctx, tenantID, id)
}

// ListCardholders returns a paginated list of cardholders.
func (s *CardholderService) ListCardholders(ctx context.Context, limit, offset int) ([]*domain.Cardholder, int, error) {
	tenantID := domain.MustTenantID(ctx)
	return s.cardholders.List(ctx, tenantID, limit, offset)
}

// UpdateCardholder updates an existing cardholder's information.
func (s *CardholderService) UpdateCardholder(ctx context.Context, id uuid.UUID, firstName, lastName, email, phone string) (*domain.Cardholder, error) {
	tenantID := domain.MustTenantID(ctx)

	ch, err := s.cardholders.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("getting cardholder: %w", err)
	}

	ch.FirstName = firstName
	ch.LastName = lastName
	ch.Email = email
	ch.Phone = phone

	if err := s.cardholders.Update(ctx, ch); err != nil {
		return nil, fmt.Errorf("updating cardholder: %w", err)
	}

	return ch, nil
}
