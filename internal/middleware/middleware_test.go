package middleware_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/novopayment/card-issuer-api/internal/domain"
	"github.com/novopayment/card-issuer-api/internal/middleware"
)

type mockTenantLookup struct {
	validKey string
	tenantID uuid.UUID
}

func (m *mockTenantLookup) GetTenantByAPIKey(ctx context.Context, apiKey string) (uuid.UUID, error) {
	if apiKey == m.validKey {
		return m.tenantID, nil
	}
	return uuid.Nil, domain.ErrUnauthorized
}

type inMemoryIdempotencyStore struct {
	mu      sync.Mutex
	records map[string]*idempItem
}

type idempItem struct {
	completed    bool
	responseCode int
	responseBody []byte
}

func newInMemoryIdempotencyStore() *inMemoryIdempotencyStore {
	return &inMemoryIdempotencyStore{records: make(map[string]*idempItem)}
}

func (s *inMemoryIdempotencyStore) Check(ctx context.Context, tenantID uuid.UUID, key string) (bool, bool, int, []byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.records[tenantID.String()+":"+key]
	if !ok {
		return false, false, 0, nil, nil
	}
	return true, item.completed, item.responseCode, item.responseBody, nil
}

func (s *inMemoryIdempotencyStore) Start(ctx context.Context, tenantID uuid.UUID, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[tenantID.String()+":"+key] = &idempItem{completed: false}
	return nil
}

func (s *inMemoryIdempotencyStore) Complete(ctx context.Context, tenantID uuid.UUID, key string, responseCode int, responseBody []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[tenantID.String()+":"+key] = &idempItem{
		completed:    true,
		responseCode: responseCode,
		responseBody: responseBody,
	}
	return nil
}

func TestAuthMiddleware(t *testing.T) {
	tenantID := uuid.New()
	lookup := &mockTenantLookup{validKey: "valid-secret-key", tenantID: tenantID}
	authMw := middleware.Auth(lookup)

	handler := authMw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tid, ok := domain.TenantIDFromContext(r.Context())
		if !ok || tid != tenantID {
			t.Fatalf("expected tenant %s in context, got %s", tenantID, tid)
		}
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("Valid Token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/cards", nil)
		req.Header.Set("Authorization", "Bearer valid-secret-key")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("Invalid Token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/cards", nil)
		req.Header.Set("Authorization", "Bearer wrong-key")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("Missing Header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/cards", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})
}

func TestTenantEnforcementMiddleware(t *testing.T) {
	tenantID := uuid.New()
	enforcement := middleware.TenantEnforcement()

	t.Run("Missing Tenant Context Rejection", func(t *testing.T) {
		handler := enforcement(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/v1/cards", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for missing tenant context, got %d", rec.Code)
		}
	})

	t.Run("Tenant Header Mismatch Rejection", func(t *testing.T) {
		handler := enforcement(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/v1/cards", nil)
		req = req.WithContext(domain.WithTenantID(req.Context(), tenantID))
		req.Header.Set("X-Tenant-ID", uuid.New().String()) // Mismatched header

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for mismatched tenant header, got %d", rec.Code)
		}
	})
}

func TestIdempotencyMiddleware(t *testing.T) {
	tenantID := uuid.New()
	store := newInMemoryIdempotencyStore()
	idempMw := middleware.Idempotency(store)

	callCount := 0
	handler := idempMw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"status":"created"}`))
	}))

	t.Run("First Call Executes Handler", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/cards", nil)
		req = req.WithContext(domain.WithTenantID(req.Context(), tenantID))
		req.Header.Set("Idempotency-Key", "test-key-123")

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d", rec.Code)
		}
		if callCount != 1 {
			t.Fatalf("expected 1 call, got %d", callCount)
		}
	})

	t.Run("Second Call Returns Cached Response Without Executing Handler", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/cards", nil)
		req = req.WithContext(domain.WithTenantID(req.Context(), tenantID))
		req.Header.Set("Idempotency-Key", "test-key-123")

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 from cache, got %d", rec.Code)
		}
		if callCount != 1 {
			t.Fatalf("handler should not have been called again! count=%d", callCount)
		}
		if rec.Body.String() != `{"status":"created"}` {
			t.Fatalf("cached body mismatch: %s", rec.Body.String())
		}
	})

	t.Run("Mutating Request Missing Idempotency Key Fails", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/cards", nil)
		req = req.WithContext(domain.WithTenantID(req.Context(), tenantID))

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for missing idempotency key, got %d", rec.Code)
		}
	})
}

func TestRecoveryMiddleware(t *testing.T) {
	noopLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	recoveryMw := middleware.Recovery(noopLogger)

	handler := recoveryMw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("simulated unexpected runtime panic with PAN: 4111111111111234")
	}))

	req := httptest.NewRequest("GET", "/v1/cards", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on recovered panic, got %d", rec.Code)
	}
}
