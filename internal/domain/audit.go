package domain

import (
	"time"

	"github.com/google/uuid"
)

// AuditEvent represents an immutable audit log entry.
// Once created, audit events MUST NOT be modified or deleted.
// This is enforced both at the application layer and via database triggers.
type AuditEvent struct {
	ID          uuid.UUID
	TenantID    uuid.UUID
	EntityType  string // "card", "cardholder", "batch"
	EntityID    uuid.UUID
	Action      string // "CREATED", "ISSUED", "SUSPENDED", "REACTIVATED", "CLOSED", "UPDATED", "BATCH_PROCESSED"
	OldValue    any    // serialized to JSONB — NEVER contains PAN data
	NewValue    any    // serialized to JSONB — NEVER contains PAN data
	PerformedBy string
	IPAddress   string
	RequestID   uuid.UUID
	CreatedAt   time.Time
}

// NewAuditEvent creates a new audit event with old and new values for change tracking.
// SECURITY: Callers MUST ensure that oldValue and newValue never contain PAN or SAD data.
func NewAuditEvent(tenantID uuid.UUID, entityType string, entityID uuid.UUID, action, oldValue, newValue, performedBy string) *AuditEvent {
	return &AuditEvent{
		ID:          uuid.New(),
		TenantID:    tenantID,
		EntityType:  entityType,
		EntityID:    entityID,
		Action:      action,
		OldValue:    oldValue,
		NewValue:    newValue,
		PerformedBy: performedBy,
		CreatedAt:   time.Now().UTC(),
	}
}
