package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/novopayment/card-issuer-api/internal/domain"
	"golang.org/x/crypto/bcrypt"
)

// Tenant represents a financial institution using the BaaS platform.
type Tenant struct {
	ID         uuid.UUID
	Name       string
	APIKeyHash string
	IsActive   bool
}

type TenantRepository interface {
	GetTenantByAPIKey(ctx context.Context, apiKey string) (uuid.UUID, error)
	CreateTenant(ctx context.Context, name, rawAPIKey string) (*Tenant, error)
}

type tenantRepository struct {
	db DBTX
}

// NewTenantRepository creates a new TenantRepository.
func NewTenantRepository(db DBTX) TenantRepository {
	return &tenantRepository{db: db}
}

// GetTenantByAPIKey iterates through active tenants and verifies the API key against the bcrypt hash.
// For high-throughput production, an in-memory TTL cache or deterministic key-id prefix is used.
func (r *tenantRepository) GetTenantByAPIKey(ctx context.Context, apiKey string) (uuid.UUID, error) {
	query := `
		SELECT id, name, api_key_hash, is_active
		FROM tenants
		WHERE is_active = true
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return uuid.Nil, fmt.Errorf("querying tenants: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var t Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.APIKeyHash, &t.IsActive); err != nil {
			return uuid.Nil, fmt.Errorf("scanning tenant: %w", err)
		}

		if err := bcrypt.CompareHashAndPassword([]byte(t.APIKeyHash), []byte(apiKey)); err == nil {
			return t.ID, nil
		}
	}

	if err := rows.Err(); err != nil {
		return uuid.Nil, fmt.Errorf("iterating tenants: %w", err)
	}

	return uuid.Nil, domain.ErrUnauthorized
}

// CreateTenant inserts a new tenant with a hashed API key.
func (r *tenantRepository) CreateTenant(ctx context.Context, name, rawAPIKey string) (*Tenant, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(rawAPIKey), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hashing api key: %w", err)
	}

	tenant := &Tenant{
		ID:         uuid.New(),
		Name:       name,
		APIKeyHash: string(hash),
		IsActive:   true,
	}

	query := `
		INSERT INTO tenants (id, name, api_key_hash, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
	`
	if _, err := r.db.ExecContext(ctx, query, tenant.ID, tenant.Name, tenant.APIKeyHash, tenant.IsActive); err != nil {
		return nil, fmt.Errorf("inserting tenant: %w", err)
	}

	return tenant, nil
}

// IdempotencyStoreAdapter adapts IdempotencyRepository to the middleware.IdempotencyStore interface.
type IdempotencyStoreAdapter struct {
	repo IdempotencyRepository
}

// NewIdempotencyStoreAdapter creates a new IdempotencyStoreAdapter.
func NewIdempotencyStoreAdapter(repo IdempotencyRepository) *IdempotencyStoreAdapter {
	return &IdempotencyStoreAdapter{repo: repo}
}

func (a *IdempotencyStoreAdapter) Check(ctx context.Context, tenantID uuid.UUID, key string) (bool, bool, int, []byte, error) {
	record, err := a.repo.Get(ctx, tenantID, key)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return false, false, 0, nil, nil
		}
		return false, false, 0, nil, err
	}

	completed := record.Status == "COMPLETED"
	return true, completed, record.ResponseCode, record.ResponseBody, nil
}

func (a *IdempotencyStoreAdapter) Start(ctx context.Context, tenantID uuid.UUID, key string) error {
	return a.repo.Create(ctx, tenantID, key)
}

func (a *IdempotencyStoreAdapter) Complete(ctx context.Context, tenantID uuid.UUID, key string, responseCode int, responseBody []byte) error {
	return a.repo.Complete(ctx, tenantID, key, responseCode, responseBody)
}
