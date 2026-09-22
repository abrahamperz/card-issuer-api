-- Migration: Create batch_operations table
-- Tracks the status and results of batch operations per tenant.

CREATE TABLE batch_operations (
    id              UUID NOT NULL DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    action          VARCHAR(50) NOT NULL,
    mode            VARCHAR(20) NOT NULL CHECK (mode IN ('atomic','partial')),
    status          VARCHAR(20) NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','PROCESSING','COMPLETED','FAILED')),
    total_count     INT NOT NULL,
    success_count   INT NOT NULL DEFAULT 0,
    failure_count   INT NOT NULL DEFAULT 0,
    results         JSONB,
    idempotency_key VARCHAR(255),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at    TIMESTAMPTZ,
    PRIMARY KEY (tenant_id, id)
);

CREATE UNIQUE INDEX uidx_batch_ops_tenant_idemp ON batch_operations (tenant_id, idempotency_key) WHERE idempotency_key IS NOT NULL;
