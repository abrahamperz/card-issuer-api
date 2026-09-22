package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/novopayment/card-issuer-api/internal/domain"
)

type batchRepository struct {
	db DBTX
}

// NewBatchRepository creates a new BatchRepository.
func NewBatchRepository(db DBTX) BatchRepository {
	return &batchRepository{db: db}
}

func (r *batchRepository) Create(ctx context.Context, op *domain.BatchOperation) error {
	resultsJSON, err := json.Marshal(op.Results)
	if err != nil {
		return fmt.Errorf("failed to marshal batch results: %w", err)
	}

	var completedAt sql.NullTime
	if op.CompletedAt != nil {
		completedAt.Time = *op.CompletedAt
		completedAt.Valid = true
	}

	query := `
		INSERT INTO batch_operations (
			id, tenant_id, action, mode, status, total_count,
			success_count, failure_count, results, idempotency_key,
			created_at, completed_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
		)
	`
	_, err = r.db.ExecContext(ctx, query,
		op.ID, op.TenantID, op.Action, op.Mode, op.Status, op.TotalCount,
		op.SuccessCount, op.FailureCount, resultsJSON, op.IdempotencyKey,
		op.CreatedAt, completedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create batch operation: %w", err)
	}
	return nil
}

func (r *batchRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*domain.BatchOperation, error) {
	query := `
		SELECT
			id, tenant_id, action, mode, status, total_count,
			success_count, failure_count, results, idempotency_key,
			created_at, completed_at
		FROM batch_operations
		WHERE tenant_id = $1 AND id = $2
	`
	var op domain.BatchOperation
	var resultsJSON []byte
	var completedAt sql.NullTime
	var idempotencyKey sql.NullString

	err := r.db.QueryRowContext(ctx, query, tenantID, id).Scan(
		&op.ID, &op.TenantID, &op.Action, &op.Mode, &op.Status, &op.TotalCount,
		&op.SuccessCount, &op.FailureCount, &resultsJSON, &idempotencyKey,
		&op.CreatedAt, &completedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get batch operation by id: %w", err)
	}

	if completedAt.Valid {
		op.CompletedAt = &completedAt.Time
	}
	if idempotencyKey.Valid {
		op.IdempotencyKey = idempotencyKey.String
	}

	if len(resultsJSON) > 0 && string(resultsJSON) != "null" {
		if err := json.Unmarshal(resultsJSON, &op.Results); err != nil {
			return nil, fmt.Errorf("failed to unmarshal batch results: %w", err)
		}
	}

	return &op, nil
}

func (r *batchRepository) Update(ctx context.Context, op *domain.BatchOperation) error {
	resultsJSON, err := json.Marshal(op.Results)
	if err != nil {
		return fmt.Errorf("failed to marshal batch results: %w", err)
	}

	var completedAt sql.NullTime
	if op.CompletedAt != nil {
		completedAt.Time = *op.CompletedAt
		completedAt.Valid = true
	}

	query := `
		UPDATE batch_operations
		SET
			status = $1, success_count = $2, failure_count = $3,
			results = $4, completed_at = $5
		WHERE tenant_id = $6 AND id = $7
	`
	res, err := r.db.ExecContext(ctx, query,
		op.Status, op.SuccessCount, op.FailureCount,
		resultsJSON, completedAt,
		op.TenantID, op.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update batch operation: %w", err)
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
