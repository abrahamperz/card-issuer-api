package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/novopayment/card-issuer-api/internal/domain"
)

type idempotencyRepository struct {
	db DBTX
}

// NewIdempotencyRepository creates a new IdempotencyRepository.
func NewIdempotencyRepository(db DBTX) IdempotencyRepository {
	return &idempotencyRepository{db: db}
}

func (r *idempotencyRepository) Get(ctx context.Context, tenantID uuid.UUID, key string) (*IdempotencyRecord, error) {
	query := `
		SELECT
			tenant_id, key, status, response_code, response_body
		FROM idempotency_keys
		WHERE tenant_id = $1 AND key = $2
	`
	var record IdempotencyRecord
	err := r.db.QueryRowContext(ctx, query, tenantID, key).Scan(
		&record.TenantID, &record.Key, &record.Status,
		&record.ResponseCode, &record.ResponseBody,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get idempotency record: %w", err)
	}
	return &record, nil
}

func (r *idempotencyRepository) Create(ctx context.Context, tenantID uuid.UUID, key string) error {
	query := `
		INSERT INTO idempotency_keys (
			tenant_id, key, status, response_code, response_body, created_at
		) VALUES (
			$1, $2, 'PROCESSING', 0, NULL, NOW()
		)
	`
	_, err := r.db.ExecContext(ctx, query, tenantID, key)
	if err != nil {
		return fmt.Errorf("failed to create idempotency record: %w", err)
	}
	return nil
}

func (r *idempotencyRepository) Complete(ctx context.Context, tenantID uuid.UUID, key string, responseCode int, responseBody []byte) error {
	query := `
		UPDATE idempotency_keys
		SET
			status = 'COMPLETED',
			response_code = $1,
			response_body = $2
		WHERE tenant_id = $3 AND key = $4
	`
	res, err := r.db.ExecContext(ctx, query, responseCode, responseBody, tenantID, key)
	if err != nil {
		return fmt.Errorf("failed to complete idempotency record: %w", err)
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

func (r *idempotencyRepository) Cleanup(ctx context.Context) error {
	query := `
		DELETE FROM idempotency_keys
		WHERE created_at < NOW() - INTERVAL '24 HOURS'
	`
	_, err := r.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to cleanup idempotency records: %w", err)
	}
	return nil
}
