-- Migration: Create audit_events table
-- Append-only audit trail for compliance. No updates or deletes allowed.

CREATE TABLE audit_events (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    entity_type VARCHAR(50) NOT NULL,
    entity_id   UUID NOT NULL,
    action      VARCHAR(50) NOT NULL,
    old_value   JSONB,
    new_value   JSONB,
    performed_by VARCHAR(255) NOT NULL,
    ip_address  INET,
    request_id  UUID,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE audit_events IS 'Append-only audit trail. Trigger protects against UPDATE/DELETE operations.';

CREATE INDEX idx_audit_events_tenant_entity ON audit_events (tenant_id, entity_type, entity_id);
CREATE INDEX idx_audit_events_tenant_created ON audit_events (tenant_id, created_at);

-- Trigger function to prevent updates and deletes
CREATE OR REPLACE FUNCTION prevent_audit_modifications()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'Updates and Deletes are not allowed on audit_events table';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_prevent_audit_modifications
BEFORE UPDATE OR DELETE ON audit_events
FOR EACH ROW EXECUTE FUNCTION prevent_audit_modifications();
