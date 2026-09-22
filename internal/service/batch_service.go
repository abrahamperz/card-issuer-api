package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/novopayment/card-issuer-api/internal/domain"
	"github.com/novopayment/card-issuer-api/internal/repository"
)

// BatchConfig holds configuration for batch processing.
type BatchConfig struct {
	MaxBatchSize int // Maximum number of cards in a single batch
	ChunkSize    int // Number of cards per chunk in partial mode
	WorkerCount  int // Number of concurrent workers for partial mode
}

// BatchService orchestrates batch card status update operations.
// It supports two processing modes:
//
//   - Atomic: All-or-nothing within a single transaction. If any card fails
//     validation, the entire batch is rolled back. Uses ORDER BY id ASC FOR UPDATE
//     for deadlock-free locking.
//
//   - Partial: Best-effort processing with per-card failure reporting.
//     Cards are split into chunks processed by a worker pool, each chunk in its
//     own transaction. Results are aggregated with mutex protection.
type BatchService struct {
	cards     repository.CardRepository
	batches   repository.BatchRepository
	audit     repository.AuditRepository
	txManager repository.TxManager
	config    BatchConfig
	logger    *slog.Logger
}

// NewBatchService creates a new BatchService with all required dependencies.
func NewBatchService(
	cards repository.CardRepository,
	batches repository.BatchRepository,
	audit repository.AuditRepository,
	txManager repository.TxManager,
	config BatchConfig,
	logger *slog.Logger,
) *BatchService {
	return &BatchService{
		cards:     cards,
		batches:   batches,
		audit:     audit,
		txManager: txManager,
		config:    config,
		logger:    logger,
	}
}

// ProcessBatch executes a batch card status update.
func (s *BatchService) ProcessBatch(
	ctx context.Context,
	action domain.BatchAction,
	mode domain.BatchMode,
	cardIDs []uuid.UUID,
	idempotencyKey string,
) (*domain.BatchOperation, error) {
	tenantID := domain.MustTenantID(ctx)

	if len(cardIDs) == 0 {
		return nil, fmt.Errorf("%w: at least 1 card required", domain.ErrInvalidInput)
	}
	if len(cardIDs) > s.config.MaxBatchSize {
		return nil, fmt.Errorf("%w: max batch size is %d", domain.ErrInvalidInput, s.config.MaxBatchSize)
	}

	batch := domain.NewBatchOperation(tenantID, action, mode, cardIDs, idempotencyKey)

	// Persist the batch in PENDING status
	if err := s.batches.Create(ctx, batch); err != nil {
		return nil, fmt.Errorf("creating batch: %w", err)
	}

	// Mark as PROCESSING
	batch.Status = domain.BatchStatusProcessing
	batch.UpdatedAt = time.Now().UTC()
	if err := s.batches.Update(ctx, batch); err != nil {
		return nil, fmt.Errorf("updating batch status: %w", err)
	}

	// Execute based on mode
	var processErr error
	if mode == domain.BatchModeAtomic {
		processErr = s.processAtomic(ctx, batch)
	} else if mode == domain.BatchModePartial {
		processErr = s.processPartial(ctx, batch)
	} else {
		return nil, fmt.Errorf("%w: invalid batch mode: %s", domain.ErrInvalidInput, mode)
	}

	// Finalize batch status
	now := time.Now().UTC()
	if processErr != nil {
		batch.Status = domain.BatchStatusFailed
		batch.Error = processErr.Error()
	} else {
		batch.Status = domain.BatchStatusCompleted
	}
	batch.CompletedAt = &now
	batch.UpdatedAt = now

	if updateErr := s.batches.Update(ctx, batch); updateErr != nil {
		s.logger.Error("failed to update final batch status",
			"error", updateErr,
			"batch_id", batch.ID.String(),
		)
	}

	return batch, processErr
}

// GetBatchOperation retrieves a batch operation by ID.
func (s *BatchService) GetBatchOperation(ctx context.Context, batchID uuid.UUID) (*domain.BatchOperation, error) {
	tenantID := domain.MustTenantID(ctx)
	return s.batches.GetByID(ctx, tenantID, batchID)
}

// applyAction maps a BatchAction to the corresponding Card domain method.
func applyAction(card *domain.Card, action domain.BatchAction) error {
	switch action {
	case domain.BatchActionSuspend:
		return card.Suspend()
	case domain.BatchActionActivate, domain.BatchActionReactivate:
		return card.Reactivate()
	case domain.BatchActionClose:
		return card.Close()
	default:
		return fmt.Errorf("unknown action: %s", action)
	}
}

// processAtomic executes the batch in a single transaction.
// If ANY card fails validation, the ENTIRE batch is rolled back.
//
// Deadlock prevention: SELECT ... FOR UPDATE ORDER BY id ASC
// ensures deterministic lock acquisition order.
func (s *BatchService) processAtomic(ctx context.Context, batch *domain.BatchOperation) error {
	return s.txManager.WithTransaction(ctx, func(ctx context.Context, repos repository.TxRepositories) error {
		// Lock all cards in deterministic order to prevent deadlocks
		cards, err := repos.Cards().GetForUpdateByIDs(ctx, batch.TenantID, batch.CardIDs)
		if err != nil {
			return fmt.Errorf("locking cards: %w", err)
		}

		if len(cards) != len(batch.CardIDs) {
			return fmt.Errorf("%w: expected %d cards, found %d",
				domain.ErrNotFound, len(batch.CardIDs), len(cards))
		}

		// Phase 1: Validate ALL transitions before applying any
		for _, card := range cards {
			oldStatus := card.Status
			if err := applyAction(card, batch.Action); err != nil {
				return fmt.Errorf("card %s: %w (from %s)", card.ID, err, oldStatus)
			}
		}

		// Phase 2: Persist all updates (all validated, safe to commit)
		for _, card := range cards {
			if err := repos.Cards().Update(ctx, card); err != nil {
				return fmt.Errorf("updating card %s: %w", card.ID, err)
			}
		}

		// Phase 3: Audit all changes
		for _, card := range cards {
			auditEvent := domain.NewAuditEvent(
				batch.TenantID, "card", card.ID,
				string(batch.Action), "", string(card.Status), "batch",
			)
			if err := repos.Audit().Create(ctx, auditEvent); err != nil {
				return fmt.Errorf("audit for card %s: %w", card.ID, err)
			}
		}

		// Record success for all cards
		for _, cardID := range batch.CardIDs {
			batch.Results = append(batch.Results, domain.BatchItemResult{
				CardID:  cardID,
				Success: true,
			})
			batch.SuccessCount++
		}

		return nil
	})
}

// processPartial processes cards in chunks using a worker pool.
// Each chunk runs in its own transaction. Failed cards don't affect others.
func (s *BatchService) processPartial(ctx context.Context, batch *domain.BatchOperation) error {
	chunkSize := s.config.ChunkSize
	if chunkSize <= 0 {
		chunkSize = 50
	}

	workers := s.config.WorkerCount
	if workers <= 0 {
		workers = 4
	}

	// Split into chunks
	var chunks [][]uuid.UUID
	for i := 0; i < len(batch.CardIDs); i += chunkSize {
		end := i + chunkSize
		if end > len(batch.CardIDs) {
			end = len(batch.CardIDs)
		}
		chunks = append(chunks, batch.CardIDs[i:end])
	}

	// Process chunks concurrently with a bounded worker pool
	type chunkTask struct {
		index int
		ids   []uuid.UUID
	}

	tasks := make(chan chunkTask, len(chunks))
	for i, chunk := range chunks {
		tasks <- chunkTask{index: i, ids: chunk}
	}
	close(tasks)

	var wg sync.WaitGroup
	var mu sync.Mutex

	for i := 0; i < workers && i < len(chunks); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range tasks {
				s.processPartialChunk(ctx, batch, task.ids, &mu)
			}
		}()
	}

	wg.Wait()
	return nil
}

// processPartialChunk processes a single chunk of cards within its own transaction.
// Results are aggregated into the batch with mutex protection.
func (s *BatchService) processPartialChunk(
	ctx context.Context,
	batch *domain.BatchOperation,
	cardIDs []uuid.UUID,
	mu *sync.Mutex,
) {
	var chunkResults []domain.BatchItemResult
	var successCount, failureCount int

	err := s.txManager.WithTransaction(ctx, func(ctx context.Context, repos repository.TxRepositories) error {
		// Lock cards in this chunk (deterministic order)
		cards, err := repos.Cards().GetForUpdateByIDs(ctx, batch.TenantID, cardIDs)
		if err != nil {
			return err
		}

		cardMap := make(map[uuid.UUID]*domain.Card)
		for _, c := range cards {
			cardMap[c.ID] = c
		}

		for _, cardID := range cardIDs {
			card, exists := cardMap[cardID]
			if !exists {
				chunkResults = append(chunkResults, domain.BatchItemResult{
					CardID:  cardID,
					Success: false,
					Error:   "card not found",
				})
				failureCount++
				continue
			}

			oldStatus := card.Status
			if err := applyAction(card, batch.Action); err != nil {
				chunkResults = append(chunkResults, domain.BatchItemResult{
					CardID:  cardID,
					Success: false,
					Error:   err.Error(),
				})
				failureCount++
				continue
			}

			if err := repos.Cards().Update(ctx, card); err != nil {
				return fmt.Errorf("updating card %s: %w", cardID, err)
			}

			auditEvent := domain.NewAuditEvent(
				batch.TenantID, "card", card.ID,
				string(batch.Action), string(oldStatus), string(card.Status), "batch",
			)
			if err := repos.Audit().Create(ctx, auditEvent); err != nil {
				return fmt.Errorf("audit for card %s: %w", cardID, err)
			}

			chunkResults = append(chunkResults, domain.BatchItemResult{
				CardID:  cardID,
				Success: true,
			})
			successCount++
		}

		return nil
	})

	// If the entire transaction failed, mark all cards in this chunk as failed
	if err != nil {
		s.logger.Error("chunk transaction failed", "error", err)
		chunkResults = nil
		successCount = 0
		failureCount = len(cardIDs)
		for _, id := range cardIDs {
			chunkResults = append(chunkResults, domain.BatchItemResult{
				CardID:  id,
				Success: false,
				Error:   err.Error(),
			})
		}
	}

	// Thread-safe aggregation of results
	mu.Lock()
	batch.Results = append(batch.Results, chunkResults...)
	batch.SuccessCount += successCount
	batch.FailureCount += failureCount
	mu.Unlock()
}
