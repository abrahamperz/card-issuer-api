package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/google/uuid"
	"github.com/novopayment/card-issuer-api/internal/domain"
	"github.com/novopayment/card-issuer-api/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

// --- In-Memory Card Repository ---

type MemoryCardRepository struct {
	mu    sync.RWMutex
	cards map[string]*domain.Card
}

func NewMemoryCardRepository() *MemoryCardRepository {
	return &MemoryCardRepository{cards: make(map[string]*domain.Card)}
}

func (r *MemoryCardRepository) key(tenantID, id uuid.UUID) string {
	return tenantID.String() + ":" + id.String()
}

func (r *MemoryCardRepository) clone(c *domain.Card) *domain.Card {
	if c == nil {
		return nil
	}
	cp := *c
	if c.PANEncrypted != nil {
		cp.PANEncrypted = make([]byte, len(c.PANEncrypted))
		copy(cp.PANEncrypted, c.PANEncrypted)
	}
	return &cp
}

func (r *MemoryCardRepository) Create(ctx context.Context, card *domain.Card) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := r.key(card.TenantID, card.ID)
	if _, exists := r.cards[k]; exists {
		return domain.ErrConflict
	}
	r.cards[k] = r.clone(card)
	return nil
}

func (r *MemoryCardRepository) GetByID(ctx context.Context, tenantID, cardID uuid.UUID) (*domain.Card, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	card, ok := r.cards[r.key(tenantID, cardID)]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return r.clone(card), nil
}

func (r *MemoryCardRepository) GetByBlindIndex(ctx context.Context, tenantID uuid.UUID, blindIndex string) (*domain.Card, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, c := range r.cards {
		if c.TenantID == tenantID && c.PANBlindIndex == blindIndex {
			return r.clone(c), nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *MemoryCardRepository) List(ctx context.Context, tenantID uuid.UUID, status *domain.CardStatus, limit, offset int) ([]*domain.Card, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var filtered []*domain.Card
	for _, c := range r.cards {
		if c.TenantID == tenantID {
			if status == nil || c.Status == *status {
				filtered = append(filtered, r.clone(c))
			}
		}
	}
	// Sort by CreatedAt desc
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})

	total := len(filtered)
	if offset > total {
		return nil, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return filtered[offset:end], total, nil
}

func (r *MemoryCardRepository) ListByCardholder(ctx context.Context, tenantID, cardholderID uuid.UUID) ([]*domain.Card, error) {
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

func (r *MemoryCardRepository) Update(ctx context.Context, card *domain.Card) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := r.key(card.TenantID, card.ID)
	existing, ok := r.cards[k]
	if !ok {
		return domain.ErrNotFound
	}
	// OCC Version Check
	if existing.Version != card.Version && existing.Version != card.Version-1 {
		return domain.ErrConflict
	}
	r.cards[k] = r.clone(card)
	return nil
}

func (r *MemoryCardRepository) GetForUpdateByIDs(ctx context.Context, tenantID uuid.UUID, cardIDs []uuid.UUID) ([]*domain.Card, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var res []*domain.Card
	for _, id := range cardIDs {
		card, ok := r.cards[r.key(tenantID, id)]
		if ok {
			res = append(res, r.clone(card))
		}
	}
	// Deterministic sorting by ID to mimic PostgreSQL ORDER BY id ASC FOR UPDATE
	sort.Slice(res, func(i, j int) bool {
		return res[i].ID.String() < res[j].ID.String()
	})
	return res, nil
}

func (r *MemoryCardRepository) BatchUpdateStatus(ctx context.Context, tenantID uuid.UUID, cards []*domain.Card) error {
	for _, c := range cards {
		if err := r.Update(ctx, c); err != nil {
			return err
		}
	}
	return nil
}

// --- In-Memory Cardholder Repository ---

type MemoryCardholderRepository struct {
	mu          sync.RWMutex
	cardholders map[string]*domain.Cardholder
}

func NewMemoryCardholderRepository() *MemoryCardholderRepository {
	return &MemoryCardholderRepository{cardholders: make(map[string]*domain.Cardholder)}
}

func (r *MemoryCardholderRepository) key(tenantID, id uuid.UUID) string {
	return tenantID.String() + ":" + id.String()
}

func (r *MemoryCardholderRepository) clone(ch *domain.Cardholder) *domain.Cardholder {
	if ch == nil {
		return nil
	}
	cp := *ch
	return &cp
}

func (r *MemoryCardholderRepository) Create(ctx context.Context, ch *domain.Cardholder) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := r.key(ch.TenantID, ch.ID)
	if _, exists := r.cardholders[k]; exists {
		return domain.ErrConflict
	}
	r.cardholders[k] = r.clone(ch)
	return nil
}

func (r *MemoryCardholderRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*domain.Cardholder, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ch, ok := r.cardholders[r.key(tenantID, id)]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return r.clone(ch), nil
}

func (r *MemoryCardholderRepository) List(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]*domain.Cardholder, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var res []*domain.Cardholder
	for _, ch := range r.cardholders {
		if ch.TenantID == tenantID {
			res = append(res, r.clone(ch))
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

func (r *MemoryCardholderRepository) Update(ctx context.Context, ch *domain.Cardholder) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := r.key(ch.TenantID, ch.ID)
	if _, exists := r.cardholders[k]; !exists {
		return domain.ErrNotFound
	}
	r.cardholders[k] = r.clone(ch)
	return nil
}

// --- In-Memory Batch Repository ---

type MemoryBatchRepository struct {
	mu      sync.RWMutex
	batches map[string]*domain.BatchOperation
}

func NewMemoryBatchRepository() *MemoryBatchRepository {
	return &MemoryBatchRepository{batches: make(map[string]*domain.BatchOperation)}
}

func (r *MemoryBatchRepository) key(tenantID, id uuid.UUID) string {
	return tenantID.String() + ":" + id.String()
}

func (r *MemoryBatchRepository) Create(ctx context.Context, op *domain.BatchOperation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *op
	r.batches[r.key(op.TenantID, op.ID)] = &cp
	return nil
}

func (r *MemoryBatchRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*domain.BatchOperation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	op, ok := r.batches[r.key(tenantID, id)]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *op
	return &cp, nil
}

func (r *MemoryBatchRepository) Update(ctx context.Context, op *domain.BatchOperation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *op
	r.batches[r.key(op.TenantID, op.ID)] = &cp
	return nil
}

// --- In-Memory Audit Repository ---

type MemoryAuditRepository struct {
	mu     sync.Mutex
	events []*domain.AuditEvent
}

func NewMemoryAuditRepository() *MemoryAuditRepository {
	return &MemoryAuditRepository{}
}

func (r *MemoryAuditRepository) Create(ctx context.Context, event *domain.AuditEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *event
	r.events = append(r.events, &cp)
	return nil
}

func (r *MemoryAuditRepository) ListByEntity(ctx context.Context, tenantID uuid.UUID, entityType string, entityID uuid.UUID) ([]*domain.AuditEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var res []*domain.AuditEvent
	for _, e := range r.events {
		if e.TenantID == tenantID && e.EntityType == entityType && e.EntityID == entityID {
			cp := *e
			res = append(res, &cp)
		}
	}
	return res, nil
}

// --- In-Memory Idempotency Repository ---

type MemoryIdempotencyRepository struct {
	mu      sync.RWMutex
	records map[string]*repository.IdempotencyRecord
}

func NewMemoryIdempotencyRepository() *MemoryIdempotencyRepository {
	return &MemoryIdempotencyRepository{records: make(map[string]*repository.IdempotencyRecord)}
}

func (r *MemoryIdempotencyRepository) key(tenantID uuid.UUID, k string) string {
	return tenantID.String() + ":" + k
}

func (r *MemoryIdempotencyRepository) Get(ctx context.Context, tenantID uuid.UUID, key string) (*repository.IdempotencyRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec, ok := r.records[r.key(tenantID, key)]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *rec
	return &cp, nil
}

func (r *MemoryIdempotencyRepository) Create(ctx context.Context, tenantID uuid.UUID, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records[r.key(tenantID, key)] = &repository.IdempotencyRecord{
		TenantID: tenantID,
		Key:      key,
		Status:   "PROCESSING",
	}
	return nil
}

func (r *MemoryIdempotencyRepository) Complete(ctx context.Context, tenantID uuid.UUID, key string, responseCode int, responseBody []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records[r.key(tenantID, key)] = &repository.IdempotencyRecord{
		TenantID:     tenantID,
		Key:          key,
		Status:       "COMPLETED",
		ResponseCode: responseCode,
		ResponseBody: responseBody,
	}
	return nil
}

func (r *MemoryIdempotencyRepository) Cleanup(ctx context.Context) error {
	return nil
}

// --- In-Memory Tenant Repository ---

type MemoryTenantRepository struct {
	mu      sync.RWMutex
	tenants map[string]*repository.Tenant
}

func NewMemoryTenantRepository() *MemoryTenantRepository {
	return &MemoryTenantRepository{tenants: make(map[string]*repository.Tenant)}
}

func (r *MemoryTenantRepository) CreateTenant(ctx context.Context, name, rawAPIKey string) (*repository.Tenant, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	hash, err := bcrypt.GenerateFromPassword([]byte(rawAPIKey), bcrypt.MinCost)
	if err != nil {
		return nil, err
	}

	t := &repository.Tenant{
		ID:         uuid.New(),
		Name:       name,
		APIKeyHash: string(hash),
		IsActive:   true,
	}
	r.tenants[t.ID.String()] = t
	return t, nil
}

func (r *MemoryTenantRepository) CreateWithID(id uuid.UUID, name, rawAPIKey string) (*repository.Tenant, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	hash, err := bcrypt.GenerateFromPassword([]byte(rawAPIKey), bcrypt.MinCost)
	if err != nil {
		return nil, err
	}

	t := &repository.Tenant{
		ID:         id,
		Name:       name,
		APIKeyHash: string(hash),
		IsActive:   true,
	}
	r.tenants[id.String()] = t
	return t, nil
}

func (r *MemoryTenantRepository) GetTenantByAPIKey(ctx context.Context, apiKey string) (uuid.UUID, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, t := range r.tenants {
		if t.IsActive && bcrypt.CompareHashAndPassword([]byte(t.APIKeyHash), []byte(apiKey)) == nil {
			return t.ID, nil
		}
	}
	return uuid.Nil, domain.ErrUnauthorized
}

// --- In-Memory Transaction Manager ---

type MemoryTxManager struct {
	cards       *MemoryCardRepository
	cardholders *MemoryCardholderRepository
	audit       *MemoryAuditRepository
	batches     *MemoryBatchRepository
}

func NewMemoryTxManager(
	cards *MemoryCardRepository,
	cardholders *MemoryCardholderRepository,
	audit *MemoryAuditRepository,
	batches *MemoryBatchRepository,
) *MemoryTxManager {
	return &MemoryTxManager{
		cards:       cards,
		cardholders: cardholders,
		audit:       audit,
		batches:     batches,
	}
}

type memoryTxRepos struct {
	cards       repository.CardRepository
	cardholders repository.CardholderRepository
	audit       repository.AuditRepository
	batches     repository.BatchRepository
}

func (m *memoryTxRepos) Cards() repository.CardRepository             { return m.cards }
func (m *memoryTxRepos) Cardholders() repository.CardholderRepository { return m.cardholders }
func (m *memoryTxRepos) Audit() repository.AuditRepository            { return m.audit }
func (m *memoryTxRepos) Batches() repository.BatchRepository          { return m.batches }

func (m *MemoryTxManager) WithTransaction(ctx context.Context, fn func(ctx context.Context, repos repository.TxRepositories) error) error {
	// Snapshot cards state to support atomic rollbacks
	m.cards.mu.Lock()
	snapshot := make(map[string]*domain.Card, len(m.cards.cards))
	for k, v := range m.cards.cards {
		snapshot[k] = m.cards.clone(v)
	}
	m.cards.mu.Unlock()

	repos := &memoryTxRepos{
		cards:       m.cards,
		cardholders: m.cardholders,
		audit:       m.audit,
		batches:     m.batches,
	}

	err := fn(ctx, repos)
	if err != nil {
		// Rollback on error
		m.cards.mu.Lock()
		m.cards.cards = snapshot
		m.cards.mu.Unlock()
		return err
	}
	return nil
}

// --- Memory Health Checker ---

type MemoryHealthChecker struct{}

func (m *MemoryHealthChecker) Check(ctx context.Context) error {
	return nil
}
