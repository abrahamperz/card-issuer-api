package domain

import (
	"context"

	"github.com/google/uuid"
)

type contextKey string

const tenantContextKey = contextKey("tenant_id")

// WithTenantID stores the tenant ID in the context.
func WithTenantID(ctx context.Context, tenantID uuid.UUID) context.Context {
	return context.WithValue(ctx, tenantContextKey, tenantID)
}

// TenantIDFromContext extracts the tenant ID from the context.
// Returns the tenant ID and true if found, or uuid.Nil and false if not.
// For service/repository code that requires a tenant, prefer MustTenantID.
func TenantIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	val := ctx.Value(tenantContextKey)
	if tenantID, ok := val.(uuid.UUID); ok {
		return tenantID, true
	}
	return uuid.Nil, false
}

// MustTenantID extracts the tenant ID from the context.
// Panics if the tenant ID is not found — this indicates a programming bug,
// not a user error (the auth middleware should always set the tenant).
func MustTenantID(ctx context.Context) uuid.UUID {
	tenantID, ok := TenantIDFromContext(ctx)
	if !ok {
		panic("programming bug: tenant ID not found in context")
	}
	return tenantID
}
