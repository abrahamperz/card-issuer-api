package service_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sort"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/novopayment/card-issuer-api/internal/crypto"
	"github.com/novopayment/card-issuer-api/internal/domain"
	"github.com/novopayment/card-issuer-api/internal/repository"
	"github.com/novopayment/card-issuer-api/internal/service"
)

// --- In-Memory Mock Repositories for Unit and Concurrency Testing ---

type mockCardRepo struct {
	mu    sync.RWMutex
	cards map[string]*domain.Card
}

func newMockCardRepo() *mockCardRepo {
	return &mockCardRepo{cards: make(map[string]*domain.Card)}
}

func (r *mockCardRepo) key(tenantID, id uuid.UUID) string {
	return tenantID.String() + ":" + id.String()
}

func (r *mockCardRepo) clone(c *domain.Card) *domain.Card {
	if c == nil {
		return nil
	}
	copyCard := *c
	if c.PANEncrypted != nil {
		copyCard.PANEncrypted = make([]byte, len(c.PANEncrypted))
		copy(copyCard.PANEncrypted, c.PANEncrypted)
	}
	return &copyCard
}

func (r *mockCardRepo) Create(ctx context.Context, card *domain.Card) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := r.key(card.TenantID, card.ID)
	if _, exists := r.cards[k]; exists {
		return domain.ErrConflict
	}
	r.cards[k] = r.clone(card)
	return nil
}

func (r *mockCardRepo) GetByID(ctx context.Context, tenantID, cardID uuid.UUID) (*domain.Card, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	card, ok := r.cards[r.key(tenantID, cardID)]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return r.clone(card), nil
}

func (r *mockCardRepo) GetByBlindIndex(ctx context.Context, tenantID uuid.UUID, blindIndex string) (*domain.Card, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, c := range r.cards {
		if c.TenantID == tenantID && c.PANBlindIndex == blindIndex {
			return r.clone(c), nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *mockCardRepo) List(ctx context.Context, tenantID uuid.UUID, status *domain.CardStatus, limit, offset int) ([]*domain.Card, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var res []*domain.Card
	for _, c := range r.cards {
		if c.TenantID == tenantID {
			if status == nil || c.Status == *status {
				res = append(res, r.clone(c))
			}
		}
	}
	total := len(res)
	if offset > total {
		return nil, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return res[offset:end], total, nil
}

func (r *mockCardRepo) ListByCardholder(ctx context.Context, tenantID, cardholderID uuid.UUID) ([]*domain.Card, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var res []*domain.Card
	for _, c := range r.cards {
		if c.TenantID == tenantID && c.CardholderID == cardholderID {
			res = append(res, r.clone(c))
		}
	}
	return res, nil
}

func (r *mockCardRepo) Update(ctx context.Context, card *domain.Card) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := r.key(card.TenantID, card.ID)
	existing, ok := r.cards[k]
	if !ok {
		return domain.ErrNotFound
	}
	// Optimistic Concurrency Control Check:
	// If the in-memory version does not match card.Version (before update increment), conflict
	if existing.Version != card.Version && existing.Version != card.Version-1 {
		return domain.ErrConflict
	}
	r.cards[k] = r.clone(card)
	return nil
}

func (r *mockCardRepo) GetForUpdateByIDs(ctx context.Context, tenantID uuid.UUID, cardIDs []uuid.UUID) ([]*domain.Card, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var res []*domain.Card
	for _, id := range cardIDs {
		card, ok := r.cards[r.key(tenantID, id)]
		if ok {
			res = append(res, r.clone(card))
		}
	}
	// Sort deterministically by ID to simulate ORDER BY id ASC (deadlock prevention)
	sort.Slice(res, func(i, j int) bool {
		return res[i].ID.String() < res[j].ID.String()
	})
	return res, nil
}

func (r *mockCardRepo) BatchUpdateStatus(ctx context.Context, tenantID uuid.UUID, cards []*domain.Card) error {
	for _, c := range cards {
		if err := r.Update(ctx, c); err != nil {
			return err
		}
	}
	return nil
}

type mockBatchRepo struct {
	mu      sync.RWMutex
	batches map[string]*domain.BatchOperation
}

func newMockBatchRepo() *mockBatchRepo {
	return &mockBatchRepo{batches: make(map[string]*domain.BatchOperation)}
}

func (r *mockBatchRepo) key(tenantID, id uuid.UUID) string {
	return tenantID.String() + ":" + id.String()
}

func (r *mockBatchRepo) Create(ctx context.Context, op *domain.BatchOperation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	copyOp := *op
	r.batches[r.key(op.TenantID, op.ID)] = &copyOp
	return nil
}

func (r *mockBatchRepo) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*domain.BatchOperation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	op, ok := r.batches[r.key(tenantID, id)]
	if !ok {
		return nil, domain.ErrNotFound
	}
	copyOp := *op
	return &copyOp, nil
}

func (r *mockBatchRepo) Update(ctx context.Context, op *domain.BatchOperation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	copyOp := *op
	r.batches[r.key(op.TenantID, op.ID)] = &copyOp
	return nil
}

type mockAuditRepo struct {
	mu     sync.Mutex
	events []*domain.AuditEvent
}

func newMockAuditRepo() *mockAuditRepo {
	return &mockAuditRepo{}
}

func (r *mockAuditRepo) Create(ctx context.Context, event *domain.AuditEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
	return nil
}

func (r *mockAuditRepo) ListByEntity(ctx context.Context, tenantID uuid.UUID, entityType string, entityID uuid.UUID) ([]*domain.AuditEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var res []*domain.AuditEvent
	for _, e := range r.events {
		if e.TenantID == tenantID && e.EntityType == entityType && e.EntityID == entityID {
			res = append(res, e)
		}
	}
	return res, nil
}

type mockTxRepos struct {
	cards   *mockCardRepo
	batches *mockBatchRepo
	audit   *mockAuditRepo
}

func (m *mockTxRepos) Cards() repository.CardRepository { return m.cards }
func (m *mockTxRepos) Cardholders() repository.CardholderRepository {
	return nil
}
func (m *mockTxRepos) Audit() repository.AuditRepository   { return m.audit }
func (m *mockTxRepos) Batches() repository.BatchRepository { return m.batches }

type mockTxManager struct {
	repos *mockTxRepos
}

func (m *mockTxManager) WithTransaction(ctx context.Context, fn func(ctx context.Context, repos repository.TxRepositories) error) error {
	// Snapshot state before transaction to simulate rollback on error
	m.repos.cards.mu.Lock()
	snapshot := make(map[string]*domain.Card, len(m.repos.cards.cards))
	for k, v := range m.repos.cards.cards {
		snapshot[k] = m.repos.cards.clone(v)
	}
	m.repos.cards.mu.Unlock()

	err := fn(ctx, m.repos)
	if err != nil {
		// Rollback cards state
		m.repos.cards.mu.Lock()
		m.repos.cards.cards = snapshot
		m.repos.cards.mu.Unlock()
		return err
	}
	return nil
}

// --- Setup Test Fixtures ---

func setupTestServices(t *testing.T) (
	*service.CardService,
	*service.BatchService,
	*mockCardRepo,
	*mockBatchRepo,
	*mockAuditRepo,
) {
	encKey := make([]byte, 32)
	biKey := make([]byte, 32)
	for i := range encKey {
		encKey[i] = byte(i + 1)
	}
	for i := range biKey {
		biKey[i] = byte(i + 33)
	}

	encryptor, err := crypto.NewAESGCMEncryptor(encKey)
	if err != nil {
		t.Fatalf("failed to init encryptor: %v", err)
	}
	indexer, err := crypto.NewHMACBlindIndexer(biKey)
	if err != nil {
		t.Fatalf("failed to init indexer: %v", err)
	}

	cardRepo := newMockCardRepo()
	batchRepo := newMockBatchRepo()
	auditRepo := newMockAuditRepo()

	txRepos := &mockTxRepos{cards: cardRepo, batches: batchRepo, audit: auditRepo}
	txManager := &mockTxManager{repos: txRepos}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	cardService := service.NewCardService(cardRepo, auditRepo, encryptor, indexer, txManager, logger)
	batchService := service.NewBatchService(
		cardRepo,
		batchRepo,
		auditRepo,
		txManager,
		service.BatchConfig{
			MaxBatchSize: 1000,
			ChunkSize:    2,
			WorkerCount:  2,
		},
		logger,
	)

	return cardService, batchService, cardRepo, batchRepo, auditRepo
}

// --- Tests ---

func TestCardService_LifecycleAndCrypto(t *testing.T) {
	cardService, _, cardRepo, _, auditRepo := setupTestServices(t)
	tenantID := uuid.New()
	ctx := domain.WithTenantID(context.Background(), tenantID)
	cardholderID := uuid.New()

	// 1. Create Card in PENDING
	card, err := cardService.CreateCard(ctx, cardholderID)
	if err != nil {
		t.Fatalf("CreateCard failed: %v", err)
	}
	if card.Status != domain.CardStatusPending {
		t.Fatalf("expected PENDING, got %s", card.Status)
	}

	// 2. Issue Card (PENDING -> ACTIVE) with AES-GCM & HMAC blind index
	issued, err := cardService.IssueCard(ctx, card.ID)
	if err != nil {
		t.Fatalf("IssueCard failed: %v", err)
	}
	if issued.Status != domain.CardStatusActive {
		t.Fatalf("expected ACTIVE, got %s", issued.Status)
	}
	if len(issued.PANEncrypted) == 0 {
		t.Fatal("expected PAN to be encrypted")
	}
	if issued.PANBlindIndex == "" {
		t.Fatal("expected blind index to be generated")
	}
	if len(issued.LastFourDigits) != 4 {
		t.Fatalf("expected 4 last digits, got %s", issued.LastFourDigits)
	}

	// Verify blind index lookup works
	stored, err := cardRepo.GetByBlindIndex(ctx, tenantID, issued.PANBlindIndex)
	if err != nil || stored.ID != card.ID {
		t.Fatalf("blind index lookup failed: %v", err)
	}

	// 3. Suspend Card (ACTIVE -> SUSPENDED)
	suspended, err := cardService.SuspendCard(ctx, card.ID)
	if err != nil {
		t.Fatalf("SuspendCard failed: %v", err)
	}
	if suspended.Status != domain.CardStatusSuspended {
		t.Fatalf("expected SUSPENDED, got %s", suspended.Status)
	}

	// 4. Reactivate Card (SUSPENDED -> ACTIVE)
	activeAgain, err := cardService.ReactivateCard(ctx, card.ID)
	if err != nil {
		t.Fatalf("ReactivateCard failed: %v", err)
	}
	if activeAgain.Status != domain.CardStatusActive {
		t.Fatalf("expected ACTIVE, got %s", activeAgain.Status)
	}

	// 5. Close Card (ACTIVE -> CLOSED)
	closed, err := cardService.CloseCard(ctx, card.ID)
	if err != nil {
		t.Fatalf("CloseCard failed: %v", err)
	}
	if closed.Status != domain.CardStatusClosed {
		t.Fatalf("expected CLOSED, got %s", closed.Status)
	}

	// Verify Audit Events recorded
	auditEvents, err := auditRepo.ListByEntity(ctx, tenantID, "card", card.ID)
	if err != nil || len(auditEvents) < 5 {
		t.Fatalf("expected >= 5 audit events, got %d", len(auditEvents))
	}
}

func TestBatchService_AtomicMode(t *testing.T) {
	cardService, batchService, cardRepo, _, _ := setupTestServices(t)
	tenantID := uuid.New()
	ctx := domain.WithTenantID(context.Background(), tenantID)

	// Setup 3 ACTIVE cards
	var cardIDs []uuid.UUID
	for i := 0; i < 3; i++ {
		c, err := cardService.CreateCard(ctx, uuid.New())
		if err != nil {
			t.Fatalf("failed creating card: %v", err)
		}
		issued, err := cardService.IssueCard(ctx, c.ID)
		if err != nil {
			t.Fatalf("failed issuing card: %v", err)
		}
		cardIDs = append(cardIDs, issued.ID)
	}

	t.Run("Atomic All-or-Nothing Success", func(t *testing.T) {
		op, err := batchService.ProcessBatch(ctx, domain.BatchActionSuspend, domain.BatchModeAtomic, cardIDs, "idemp-atomic-1")
		if err != nil {
			t.Fatalf("batch atomic suspend failed: %v", err)
		}
		if op.Status != domain.BatchStatusCompleted {
			t.Fatalf("expected COMPLETED, got %s", op.Status)
		}
		if op.SuccessCount != 3 {
			t.Fatalf("expected 3 successes, got %d", op.SuccessCount)
		}

		// Verify in repository all cards are now SUSPENDED
		for _, id := range cardIDs {
			c, err := cardRepo.GetByID(ctx, tenantID, id)
			if err != nil || c.Status != domain.CardStatusSuspended {
				t.Fatalf("card %s should be SUSPENDED, got %s", id, c.Status)
			}
		}
	})

	t.Run("Atomic Rollback When One Card Fails", func(t *testing.T) {
		// cardIDs[0] is SUSPENDED.
		// Let's create a CLOSED card and add it to the batch.
		// Suspending a CLOSED card is invalid!
		closedCard, _ := cardService.CreateCard(ctx, uuid.New())
		_ = closedCard.Close()
		_ = cardRepo.Update(ctx, closedCard)

		mixedIDs := []uuid.UUID{cardIDs[0], closedCard.ID}

		// Reactivate action:
		// cardIDs[0] is SUSPENDED -> valid to reactivate
		// closedCard is CLOSED -> invalid to reactivate!
		op, err := batchService.ProcessBatch(ctx, domain.BatchActionActivate, domain.BatchModeAtomic, mixedIDs, "idemp-atomic-fail")
		if err == nil {
			t.Fatal("expected batch error, got nil")
		}
		if op.Status != domain.BatchStatusFailed {
			t.Fatalf("expected batch status FAILED, got %s", op.Status)
		}

		// Verify Rollback: cardIDs[0] MUST still be SUSPENDED! Not modified!
		c0, err := cardRepo.GetByID(ctx, tenantID, cardIDs[0])
		if err != nil || c0.Status != domain.CardStatusSuspended {
			t.Fatalf("atomic rollback failed: card 0 should still be SUSPENDED, got %s", c0.Status)
		}
	})
}

func TestBatchService_PartialMode(t *testing.T) {
	cardService, batchService, cardRepo, _, _ := setupTestServices(t)
	tenantID := uuid.New()
	ctx := domain.WithTenantID(context.Background(), tenantID)

	// Create 2 ACTIVE cards
	c1, _ := cardService.CreateCard(ctx, uuid.New())
	c1, _ = cardService.IssueCard(ctx, c1.ID)

	c2, _ := cardService.CreateCard(ctx, uuid.New())
	c2, _ = cardService.IssueCard(ctx, c2.ID)

	// Create 1 CLOSED card (cannot be suspended)
	c3, _ := cardService.CreateCard(ctx, uuid.New())
	_ = c3.Close()
	_ = cardRepo.Update(ctx, c3)

	cardIDs := []uuid.UUID{c1.ID, c2.ID, c3.ID}

	op, err := batchService.ProcessBatch(ctx, domain.BatchActionSuspend, domain.BatchModePartial, cardIDs, "idemp-partial-1")
	if err != nil {
		t.Fatalf("partial batch unexpected top-level error: %v", err)
	}

	if op.SuccessCount != 2 {
		t.Fatalf("expected 2 successes, got %d", op.SuccessCount)
	}
	if op.FailureCount != 1 {
		t.Fatalf("expected 1 failure, got %d", op.FailureCount)
	}

	// c1 and c2 should be SUSPENDED
	res1, _ := cardRepo.GetByID(ctx, tenantID, c1.ID)
	if res1.Status != domain.CardStatusSuspended {
		t.Fatalf("expected c1 SUSPENDED, got %s", res1.Status)
	}

	res2, _ := cardRepo.GetByID(ctx, tenantID, c2.ID)
	if res2.Status != domain.CardStatusSuspended {
		t.Fatalf("expected c2 SUSPENDED, got %s", res2.Status)
	}

	// c3 should remain CLOSED
	res3, _ := cardRepo.GetByID(ctx, tenantID, c3.ID)
	if res3.Status != domain.CardStatusClosed {
		t.Fatalf("expected c3 CLOSED, got %s", res3.Status)
	}
}

func TestBatchService_ConcurrencyAndRaceConditions(t *testing.T) {
	cardService, batchService, _, _, _ := setupTestServices(t)
	tenantID := uuid.New()
	ctx := domain.WithTenantID(context.Background(), tenantID)

	// Create 20 cards
	const totalCards = 20
	var allIDs []uuid.UUID
	for i := 0; i < totalCards; i++ {
		c, err := cardService.CreateCard(ctx, uuid.New())
		if err != nil {
			t.Fatalf("failed creating card: %v", err)
		}
		issued, err := cardService.IssueCard(ctx, c.ID)
		if err != nil {
			t.Fatalf("failed issuing card: %v", err)
		}
		allIDs = append(allIDs, issued.ID)
	}

	// Launch multiple concurrent batches on distinct partitions to stress worker pools & race detector
	var wg sync.WaitGroup
	const concurrentBatches = 4
	cardsPerBatch := totalCards / concurrentBatches

	for b := 0; b < concurrentBatches; b++ {
		wg.Add(1)
		batchSlice := allIDs[b*cardsPerBatch : (b+1)*cardsPerBatch]
		batchKey := uuid.New().String()

		go func(ids []uuid.UUID, key string) {
			defer wg.Done()
			op, err := batchService.ProcessBatch(ctx, domain.BatchActionSuspend, domain.BatchModePartial, ids, key)
			if err != nil && !errors.Is(err, domain.ErrConflict) {
				t.Errorf("concurrent batch failed: %v", err)
			}
			if op != nil && op.TotalCount != len(ids) {
				t.Errorf("batch total mismatch: expected %d, got %d", len(ids), op.TotalCount)
			}
		}(batchSlice, batchKey)
	}

	wg.Wait()
}
