package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/novopayment/card-issuer-api/internal/domain"
)

type auditRepository struct {
	db DBTX
}

// NewAuditRepository creates a new AuditRepository.
func NewAuditRepository(db DBTX) AuditRepository {
	return &auditRepository{db: db}
}

func (r *auditRepository) Create(ctx context.Context, event *domain.AuditEvent) error {
	oldValueJSON, err := json.Marshal(event.OldValue)
	if err != nil {
		return fmt.Errorf("failed to marshal old value: %w", err)
	}

	newValueJSON, err := json.Marshal(event.NewValue)
	if err != nil {
		return fmt.Errorf("failed to marshal new value: %w", err)
	}

	query := `
		INSERT INTO audit_events (
			id, tenant_id, entity_type, entity_id, action,
			old_value, new_value, performed_by, ip_address,
			request_id, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		)
	`
	_, err = r.db.ExecContext(ctx, query,
		event.ID, event.TenantID, event.EntityType, event.EntityID, event.Action,
		oldValueJSON, newValueJSON, event.PerformedBy, event.IPAddress,
		event.RequestID, event.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create audit event: %w", err)
	}
	return nil
}

func (r *auditRepository) ListByEntity(ctx context.Context, tenantID uuid.UUID, entityType string, entityID uuid.UUID) ([]*domain.AuditEvent, error) {
	query := `
		SELECT
			id, tenant_id, entity_type, entity_id, action,
			old_value, new_value, performed_by, ip_address,
			request_id, created_at
		FROM audit_events
		WHERE tenant_id = $1 AND entity_type = $2 AND entity_id = $3
		ORDER BY created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, tenantID, entityType, entityID)
	if err != nil {
		return nil, fmt.Errorf("failed to list audit events by entity: %w", err)
	}
	defer rows.Close()

	var events []*domain.AuditEvent
	for rows.Next() {
		var event domain.AuditEvent
		var oldValueJSON, newValueJSON []byte
		if err := rows.Scan(
			&event.ID, &event.TenantID, &event.EntityType, &event.EntityID, &event.Action,
			&oldValueJSON, &newValueJSON, &event.PerformedBy, &event.IPAddress,
			&event.RequestID, &event.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan audit event: %w", err)
		}

		if len(oldValueJSON) > 0 && string(oldValueJSON) != "null" {
			if err := json.Unmarshal(oldValueJSON, &event.OldValue); err != nil {
				return nil, fmt.Errorf("failed to unmarshal old value: %w", err)
			}
		}

		if len(newValueJSON) > 0 && string(newValueJSON) != "null" {
			if err := json.Unmarshal(newValueJSON, &event.NewValue); err != nil {
				return nil, fmt.Errorf("failed to unmarshal new value: %w", err)
			}
		}

		events = append(events, &event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return events, nil
}
