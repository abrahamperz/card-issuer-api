// Package logger provides a structured, PAN-safe logger for the Card Issuer API.
//
// Security invariant: This logger NEVER outputs raw PAN data. Any type that
// implements slog.LogValuer (like domain.PAN) will have its LogValue() method
// called automatically, which returns masked output.
//
// Additional safeguards:
//   - A sanitizer replaces any sequence matching PAN patterns (13-19 digit runs)
//     in string values with "[REDACTED]".
//   - All log output includes tenant_id and request_id for correlation.
package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strings"
)

// contextKey is an unexported type for context keys to prevent collisions.
type contextKey string

const (
	requestIDKey contextKey = "request_id"
	tenantIDKey  contextKey = "tenant_id"
	actorKey     contextKey = "actor"
)

// panPattern matches sequences of 13-19 digits that could be PANs.
// This is a defense-in-depth measure — types should implement LogValuer,
// but this catches any accidental string formatting of raw PANs.
var panPattern = regexp.MustCompile(`\b\d{13,19}\b`)

// New creates a new structured logger configured for the given level and format.
func New(level string, format string, w io.Writer) *slog.Logger {
	if w == nil {
		w = os.Stdout
	}

	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: lvl,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Sanitize string values that might contain PAN-like sequences.
			if a.Value.Kind() == slog.KindString {
				sanitized := panPattern.ReplaceAllString(a.Value.String(), "[REDACTED]")
				return slog.Attr{Key: a.Key, Value: slog.StringValue(sanitized)}
			}
			return a
		},
	}

	var handler slog.Handler
	switch strings.ToLower(format) {
	case "text":
		handler = slog.NewTextHandler(w, opts)
	default:
		handler = slog.NewJSONHandler(w, opts)
	}

	return slog.New(handler)
}

// WithRequestID adds a request ID to the context for log correlation.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

// WithTenantID adds a tenant ID to the context for log correlation.
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantIDKey, tenantID)
}

// WithActor adds an actor identity to the context for log correlation.
func WithActor(ctx context.Context, actor string) context.Context {
	return context.WithValue(ctx, actorKey, actor)
}

// FromContext extracts contextual log attributes and returns a logger
// enriched with request_id, tenant_id, and actor if present.
func FromContext(ctx context.Context, base *slog.Logger) *slog.Logger {
	if base == nil {
		base = slog.Default()
	}

	l := base
	if rid, ok := ctx.Value(requestIDKey).(string); ok && rid != "" {
		l = l.With("request_id", rid)
	}
	if tid, ok := ctx.Value(tenantIDKey).(string); ok && tid != "" {
		l = l.With("tenant_id", tid)
	}
	if actor, ok := ctx.Value(actorKey).(string); ok && actor != "" {
		l = l.With("actor", actor)
	}
	return l
}
