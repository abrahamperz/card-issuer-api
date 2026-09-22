package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// BatchMode determines how a batch operation processes cards.
type BatchMode string

// BatchStatus tracks the lifecycle of a batch operation.
type BatchStatus string

// BatchAction is the card state transition to apply in a batch.
type BatchAction string

const (
	BatchModeAtomic  BatchMode = "ATOMIC"
	BatchModePartial BatchMode = "PARTIAL"

	BatchStatusPending    BatchStatus = "PENDING"
	BatchStatusProcessing BatchStatus = "PROCESSING"
	BatchStatusCompleted  BatchStatus = "COMPLETED"
	BatchStatusFailed     BatchStatus = "FAILED"

	BatchActionSuspend    BatchAction = "SUSPEND"
	BatchActionActivate   BatchAction = "ACTIVATE"
	BatchActionClose      BatchAction = "CLOSE"
	BatchActionReactivate BatchAction = "REACTIVATE"
)

// BatchItemResult holds the outcome of a single card within a batch operation.
type BatchItemResult struct {
	CardID  uuid.UUID `json:"card_id"`
	Success bool      `json:"success"`
	Error   string    `json:"error,omitempty"`
}

// BatchOperation represents a batch card status update operation.
type BatchOperation struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	Action         BatchAction
	Mode           BatchMode
	Status         BatchStatus
	TotalCount     int
	SuccessCount   int
	FailureCount   int
	CardIDs        []uuid.UUID       // input card IDs
	Results        []BatchItemResult // per-card outcomes
	IdempotencyKey string
	Error          string // top-level error message for failed batches
	CreatedAt      time.Time
	UpdatedAt      time.Time
	CompletedAt    *time.Time
}

// NewBatchOperation creates a new batch operation in PENDING status.
func NewBatchOperation(tenantID uuid.UUID, action BatchAction, mode BatchMode, cardIDs []uuid.UUID, idempotencyKey string) *BatchOperation {
	now := time.Now().UTC()
	return &BatchOperation{
		ID:             uuid.New(),
		TenantID:       tenantID,
		Action:         action,
		Mode:           mode,
		Status:         BatchStatusPending,
		TotalCount:     len(cardIDs),
		CardIDs:        cardIDs,
		Results:        make([]BatchItemResult, 0),
		IdempotencyKey: idempotencyKey,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// Validate checks that the batch operation parameters are within acceptable bounds.
func (b *BatchOperation) Validate() error {
	if b.TotalCount < 1 {
		return fmt.Errorf("%w: at least 1 card required", ErrInvalidInput)
	}
	if b.TotalCount > 1000 {
		return fmt.Errorf("%w: max batch size is 1000", ErrInvalidInput)
	}
	return nil
}

// ValidBatchAction checks if a string is a valid batch action.
func ValidBatchAction(s string) (BatchAction, bool) {
	switch BatchAction(s) {
	case BatchActionSuspend, BatchActionActivate, BatchActionClose, BatchActionReactivate:
		return BatchAction(s), true
	default:
		return "", false
	}
}

// ValidBatchMode checks if a string is a valid batch mode.
func ValidBatchMode(s string) (BatchMode, bool) {
	switch BatchMode(s) {
	case BatchModeAtomic, BatchModePartial:
		return BatchMode(s), true
	default:
		return "", false
	}
}
