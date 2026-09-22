package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/novopayment/card-issuer-api/internal/domain"
)

// TenantEnforcement validates that a tenant ID is present in the context, and optionally checks the X-Tenant-ID header.
func TenantEnforcement() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" || r.URL.Path == "/ready" {
				next.ServeHTTP(w, r)
				return
			}

			tenantID, ok := domain.TenantIDFromContext(r.Context())
			if !ok {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized: missing tenant"})
				return
			}

			headerTenant := r.Header.Get("X-Tenant-ID")
			if headerTenant != "" {
				parsedHeader, err := uuid.Parse(headerTenant)
				if err != nil || parsedHeader != tenantID {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusForbidden)
					json.NewEncoder(w).Encode(map[string]string{"error": "forbidden: tenant mismatch"})
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}
