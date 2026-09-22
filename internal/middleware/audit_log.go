package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/novopayment/card-issuer-api/internal/domain"
)

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// AuditLog logs information about each incoming HTTP request.
func AuditLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(rw, r)

			duration := time.Since(start)

			tenantID := ""
			if tid, ok := domain.TenantIDFromContext(r.Context()); ok {
				tenantID = tid.String()
			}
			reqID := RequestIDFromContext(r.Context())

			logger.InfoContext(r.Context(), "http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("tenant_id", tenantID),
				slog.String("request_id", reqID),
				slog.Int("status_code", rw.statusCode),
				slog.Duration("duration", duration),
			)
		})
	}
}
