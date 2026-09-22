package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/novopayment/card-issuer-api/internal/domain"
)

// IdempotencyStore defines the storage interface for idempotent requests.
type IdempotencyStore interface {
	Check(ctx context.Context, tenantID uuid.UUID, key string) (found bool, completed bool, responseCode int, responseBody []byte, err error)
	Start(ctx context.Context, tenantID uuid.UUID, key string) error
	Complete(ctx context.Context, tenantID uuid.UUID, key string, responseCode int, responseBody []byte) error
}

type idempotencyResponseWriter struct {
	http.ResponseWriter
	statusCode int
	body       *bytes.Buffer
}

func (rw *idempotencyResponseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *idempotencyResponseWriter) Write(b []byte) (int, error) {
	rw.body.Write(b)
	return rw.ResponseWriter.Write(b)
}

// Idempotency handles idempotent requests for POST, PUT, and PATCH methods.
func Idempotency(store IdempotencyStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch {
				next.ServeHTTP(w, r)
				return
			}

			key := r.Header.Get("Idempotency-Key")
			if key == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": "Idempotency-Key header required for mutating requests"})
				return
			}

			tenantID, ok := domain.TenantIDFromContext(r.Context())
			if !ok {
				// Safety net
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
				return
			}

			found, completed, respCode, respBody, err := store.Check(r.Context(), tenantID, key)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "internal server error"})
				return
			}

			if found {
				if !completed {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusConflict)
					json.NewEncoder(w).Encode(map[string]string{"error": "idempotency conflict: same key in-progress"})
					return
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(respCode)
				w.Write(respBody)
				return
			}

			if err := store.Start(r.Context(), tenantID, key); err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "internal server error"})
				return
			}

			rw := &idempotencyResponseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
				body:           bytes.NewBuffer(nil),
			}

			next.ServeHTTP(rw, r)

			// Record completion asynchronously or synchronously based on the setup
			_ = store.Complete(r.Context(), tenantID, key, rw.statusCode, rw.body.Bytes())
		})
	}
}
