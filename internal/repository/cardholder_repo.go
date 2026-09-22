package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/novopayment/card-issuer-api/internal/domain"
)

type cardholderRepository struct {
	db DBTX
}

// NewCardholderRepository creates a new CardholderRepository.
func NewCardholderRepository(db DBTX) CardholderRepository {
	return &cardholderRepository{db: db}
}

func (r *cardholderRepository) Create(ctx context.Context, ch *domain.Cardholder) error {
	query := `
		INSERT INTO cardholders (
			id, tenant_id, first_name, last_name, email, phone,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8
		)
	`
	_, err := r.db.ExecContext(ctx, query,
		ch.ID, ch.TenantID, ch.FirstName, ch.LastName, ch.Email, ch.Phone,
		ch.CreatedAt, ch.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create cardholder: %w", err)
	}
	return nil
}

func (r *cardholderRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*domain.Cardholder, error) {
	query := `
		SELECT
			id, tenant_id, first_name, last_name, email, phone,
			created_at, updated_at
		FROM cardholders
		WHERE tenant_id = $1 AND id = $2
	`
	var ch domain.Cardholder
	err := r.db.QueryRowContext(ctx, query, tenantID, id).Scan(
		&ch.ID, &ch.TenantID, &ch.FirstName, &ch.LastName, &ch.Email, &ch.Phone,
		&ch.CreatedAt, &ch.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get cardholder by id: %w", err)
	}
	return &ch, nil
}

func (r *cardholderRepository) List(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]*domain.Cardholder, int, error) {
	countQuery := `SELECT COUNT(*) FROM cardholders WHERE tenant_id = $1`
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, tenantID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count cardholders: %w", err)
	}

	query := `
		SELECT
			id, tenant_id, first_name, last_name, email, phone,
			created_at, updated_at
		FROM cardholders
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.db.QueryContext(ctx, query, tenantID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list cardholders: %w", err)
	}
	defer rows.Close()

	var cardholders []*domain.Cardholder
	for rows.Next() {
		var ch domain.Cardholder
		if err := rows.Scan(
			&ch.ID, &ch.TenantID, &ch.FirstName, &ch.LastName, &ch.Email, &ch.Phone,
			&ch.CreatedAt, &ch.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan cardholder: %w", err)
		}
		cardholders = append(cardholders, &ch)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows error: %w", err)
	}

	return cardholders, total, nil
}

func (r *cardholderRepository) Update(ctx context.Context, ch *domain.Cardholder) error {
	query := `
		UPDATE cardholders
		SET
			first_name = $1, last_name = $2, email = $3, phone = $4,
			updated_at = $5
		WHERE tenant_id = $6 AND id = $7
	`
	res, err := r.db.ExecContext(ctx, query,
		ch.FirstName, ch.LastName, ch.Email, ch.Phone,
		ch.UpdatedAt,
		ch.TenantID, ch.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update cardholder: %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return domain.ErrNotFound
	}

	return nil
}
