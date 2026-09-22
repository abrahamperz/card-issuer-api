package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/novopayment/card-issuer-api/internal/domain"
)

type cardRepository struct {
	db DBTX
}

// NewCardRepository creates a new CardRepository.
func NewCardRepository(db DBTX) CardRepository {
	return &cardRepository{db: db}
}

func (r *cardRepository) Create(ctx context.Context, card *domain.Card) error {
	query := `
		INSERT INTO cards (
			id, tenant_id, cardholder_id, pan_encrypted, pan_blind_index,
			last_four_digits, expiry_month, expiry_year, status, version,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
		)
	`
	_, err := r.db.ExecContext(ctx, query,
		card.ID, card.TenantID, card.CardholderID, card.PANEncrypted,
		card.PANBlindIndex, card.LastFourDigits, card.ExpiryMonth,
		card.ExpiryYear, card.Status, card.Version, card.CreatedAt, card.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create card: %w", err)
	}
	return nil
}

func (r *cardRepository) GetByID(ctx context.Context, tenantID, cardID uuid.UUID) (*domain.Card, error) {
	query := `
		SELECT
			id, tenant_id, cardholder_id, pan_encrypted, pan_blind_index,
			last_four_digits, expiry_month, expiry_year, status, version,
			created_at, updated_at
		FROM cards
		WHERE tenant_id = $1 AND id = $2
	`
	var card domain.Card
	err := r.db.QueryRowContext(ctx, query, tenantID, cardID).Scan(
		&card.ID, &card.TenantID, &card.CardholderID, &card.PANEncrypted,
		&card.PANBlindIndex, &card.LastFourDigits, &card.ExpiryMonth,
		&card.ExpiryYear, &card.Status, &card.Version, &card.CreatedAt, &card.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get card by id: %w", err)
	}
	return &card, nil
}

func (r *cardRepository) GetByBlindIndex(ctx context.Context, tenantID uuid.UUID, blindIndex string) (*domain.Card, error) {
	query := `
		SELECT
			id, tenant_id, cardholder_id, pan_encrypted, pan_blind_index,
			last_four_digits, expiry_month, expiry_year, status, version,
			created_at, updated_at
		FROM cards
		WHERE tenant_id = $1 AND pan_blind_index = $2
	`
	var card domain.Card
	err := r.db.QueryRowContext(ctx, query, tenantID, blindIndex).Scan(
		&card.ID, &card.TenantID, &card.CardholderID, &card.PANEncrypted,
		&card.PANBlindIndex, &card.LastFourDigits, &card.ExpiryMonth,
		&card.ExpiryYear, &card.Status, &card.Version, &card.CreatedAt, &card.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get card by blind index: %w", err)
	}
	return &card, nil
}

func (r *cardRepository) List(ctx context.Context, tenantID uuid.UUID, status *domain.CardStatus, limit, offset int) ([]*domain.Card, int, error) {
	countQuery := `SELECT COUNT(*) FROM cards WHERE tenant_id = $1`
	var args []any
	args = append(args, tenantID)

	if status != nil {
		countQuery += ` AND status = $2`
		args = append(args, *status)
	}

	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count cards: %w", err)
	}

	query := `
		SELECT
			id, tenant_id, cardholder_id, pan_encrypted, pan_blind_index,
			last_four_digits, expiry_month, expiry_year, status, version,
			created_at, updated_at
		FROM cards
		WHERE tenant_id = $1
	`

	if status != nil {
		query += ` AND status = $2`
	}

	query += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2)
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list cards: %w", err)
	}
	defer rows.Close()

	var cards []*domain.Card
	for rows.Next() {
		var card domain.Card
		if err := rows.Scan(
			&card.ID, &card.TenantID, &card.CardholderID, &card.PANEncrypted,
			&card.PANBlindIndex, &card.LastFourDigits, &card.ExpiryMonth,
			&card.ExpiryYear, &card.Status, &card.Version, &card.CreatedAt, &card.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan card: %w", err)
		}
		cards = append(cards, &card)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows error: %w", err)
	}

	return cards, total, nil
}

func (r *cardRepository) ListByCardholder(ctx context.Context, tenantID, cardholderID uuid.UUID) ([]*domain.Card, error) {
	query := `
		SELECT
			id, tenant_id, cardholder_id, pan_encrypted, pan_blind_index,
			last_four_digits, expiry_month, expiry_year, status, version,
			created_at, updated_at
		FROM cards
		WHERE tenant_id = $1 AND cardholder_id = $2
		ORDER BY created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, tenantID, cardholderID)
	if err != nil {
		return nil, fmt.Errorf("failed to list cards by cardholder: %w", err)
	}
	defer rows.Close()

	var cards []*domain.Card
	for rows.Next() {
		var card domain.Card
		if err := rows.Scan(
			&card.ID, &card.TenantID, &card.CardholderID, &card.PANEncrypted,
			&card.PANBlindIndex, &card.LastFourDigits, &card.ExpiryMonth,
			&card.ExpiryYear, &card.Status, &card.Version, &card.CreatedAt, &card.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan card: %w", err)
		}
		cards = append(cards, &card)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return cards, nil
}

func (r *cardRepository) Update(ctx context.Context, card *domain.Card) error {
	query := `
		UPDATE cards
		SET
			cardholder_id = $1, pan_encrypted = $2, pan_blind_index = $3,
			last_four_digits = $4, expiry_month = $5, expiry_year = $6,
			status = $7, version = version + 1, updated_at = $8
		WHERE tenant_id = $9 AND id = $10 AND version = $11
	`
	res, err := r.db.ExecContext(ctx, query,
		card.CardholderID, card.PANEncrypted, card.PANBlindIndex,
		card.LastFourDigits, card.ExpiryMonth, card.ExpiryYear,
		card.Status, card.UpdatedAt,
		card.TenantID, card.ID, card.Version,
	)
	if err != nil {
		return fmt.Errorf("failed to update card: %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return domain.ErrConflict
	}

	card.Version++
	return nil
}

func (r *cardRepository) GetForUpdateByIDs(ctx context.Context, tenantID uuid.UUID, cardIDs []uuid.UUID) ([]*domain.Card, error) {
	if len(cardIDs) == 0 {
		return nil, nil
	}

	// Convert cardIDs slice to string array for pq.Array
	var ids []string
	for _, id := range cardIDs {
		ids = append(ids, id.String())
	}

	query := `
		SELECT
			id, tenant_id, cardholder_id, pan_encrypted, pan_blind_index,
			last_four_digits, expiry_month, expiry_year, status, version,
			created_at, updated_at
		FROM cards
		WHERE tenant_id = $1 AND id = ANY($2)
		ORDER BY id ASC
		FOR UPDATE
	`

	rows, err := r.db.QueryContext(ctx, query, tenantID, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("failed to get cards for update: %w", err)
	}
	defer rows.Close()

	var cards []*domain.Card
	for rows.Next() {
		var card domain.Card
		if err := rows.Scan(
			&card.ID, &card.TenantID, &card.CardholderID, &card.PANEncrypted,
			&card.PANBlindIndex, &card.LastFourDigits, &card.ExpiryMonth,
			&card.ExpiryYear, &card.Status, &card.Version, &card.CreatedAt, &card.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan card: %w", err)
		}
		cards = append(cards, &card)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return cards, nil
}

func (r *cardRepository) BatchUpdateStatus(ctx context.Context, tenantID uuid.UUID, cards []*domain.Card) error {
	for _, card := range cards {
		if err := r.Update(ctx, card); err != nil {
			return err
		}
	}
	return nil
}
