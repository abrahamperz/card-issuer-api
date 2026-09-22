-- Migration: Enable Row Level Security (RLS) policies
-- This adds defense-in-depth isolation. Note: this is NOT the primary isolation mechanism.
-- The application still must explicitly query with tenant_id filters.
-- This ensures that even if an application query forgets the tenant filter, RLS blocks cross-tenant access.
-- Requires `app.current_tenant` configuration variable to be set per transaction.

-- Enable RLS
ALTER TABLE cardholders ENABLE ROW LEVEL SECURITY;
ALTER TABLE cards ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE batch_operations ENABLE ROW LEVEL SECURITY;

-- Force RLS so it applies even to the table owner
ALTER TABLE cardholders FORCE ROW LEVEL SECURITY;
ALTER TABLE cards FORCE ROW LEVEL SECURITY;
ALTER TABLE audit_events FORCE ROW LEVEL SECURITY;
ALTER TABLE batch_operations FORCE ROW LEVEL SECURITY;

-- Create policies based on current_setting
CREATE POLICY tenant_isolation_cardholders ON cardholders
    FOR ALL
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid);

CREATE POLICY tenant_isolation_cards ON cards
    FOR ALL
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid);

CREATE POLICY tenant_isolation_audit_events ON audit_events
    FOR ALL
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid);

CREATE POLICY tenant_isolation_batch_operations ON batch_operations
    FOR ALL
    USING (tenant_id = current_setting('app.current_tenant', true)::uuid);
