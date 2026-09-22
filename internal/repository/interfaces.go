package repository

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/novopayment/card-issuer-api/internal/domain"
)

// DBTX is the common interface between *sql.DB and *sql.Tx.
// All repository implementations accept this interface, allowing them to work
// transparently inside or outside of a transaction.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// CardRepository defines data access operations for cards.
// Every method that touches data MUST scope queries by tenant_id.
type CardRepository interface {
	Create(ctx context.Context, card *domain.Card) error
	GetByID(ctx context.Context, tenantID, cardID uuid.UUID) (*domain.Card, error)
	GetByBlindIndex(ctx context.Context, tenantID uuid.UUID, blindIndex string) (*domain.Card, error)
	List(ctx context.Context, tenantID uuid.UUID, status *domain.CardStatus, limit, offset int) ([]*domain.Card, int, error)
	ListByCardholder(ctx context.Context, tenantID, cardholderID uuid.UUID) ([]*domain.Card, error)
	Update(ctx context.Context, card *domain.Card) error // checks version for OCC
	GetForUpdateByIDs(ctx context.Context, tenantID uuid.UUID, cardIDs []uuid.UUID) ([]*domain.Card, error)
	BatchUpdateStatus(ctx context.Context, tenantID uuid.UUID, cards []*domain.Card) error
}

// CardholderRepository defines data access operations for cardholders.
type CardholderRepository interface {
	Create(ctx context.Context, ch *domain.Cardholder) error
	GetByID(ctx context.Context, tenantID, id uuid.UUID) (*domain.Cardholder, error)
	List(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]*domain.Cardholder, int, error)
	Update(ctx context.Context, ch *domain.Cardholder) error
}

// AuditRepository defines data access for append-only audit events.
type AuditRepository interface {
	Create(ctx context.Context, event *domain.AuditEvent) error
	ListByEntity(ctx context.Context, tenantID uuid.UUID, entityType string, entityID uuid.UUID) ([]*domain.AuditEvent, error)
}

// BatchRepository defines data access for batch operations.
type BatchRepository interface {
	Create(ctx context.Context, op *domain.BatchOperation) error
	GetByID(ctx context.Context, tenantID, id uuid.UUID) (*domain.BatchOperation, error)
	Update(ctx context.Context, op *domain.BatchOperation) error
}

// IdempotencyRepository defines data access for idempotency key storage.
type IdempotencyRepository interface {
	Get(ctx context.Context, tenantID uuid.UUID, key string) (*IdempotencyRecord, error)
	Create(ctx context.Context, tenantID uuid.UUID, key string) error
	Complete(ctx context.Context, tenantID uuid.UUID, key string, responseCode int, responseBody []byte) error
	Cleanup(ctx context.Context) error
}

// IdempotencyRecord represents a stored idempotency key.
type IdempotencyRecord struct {
	TenantID     uuid.UUID
	Key          string
	Status       string // PROCESSING, COMPLETED
	ResponseCode int
	ResponseBody []byte
}

// TxRepositories provides access to repositories within a transaction.
// This pattern ensures all repos in a transaction share the same *sql.Tx,
// while keeping the interface clean (no tx param on every method).
type TxRepositories interface {
	Cards() CardRepository
	Cardholders() CardholderRepository
	Audit() AuditRepository
	Batches() BatchRepository
}

// TxManager manages database transactions.
// It creates a transaction, wraps repos in TxRepositories, and passes them
// to the callback function. On success it commits; on error/panic it rolls back.
type TxManager interface {
	WithTransaction(ctx context.Context, fn func(ctx context.Context, repos TxRepositories) error) error
}
