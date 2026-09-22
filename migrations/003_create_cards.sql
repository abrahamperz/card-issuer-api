-- Migration: Create cards table
-- Cards table stores encrypted PAN and uses a blind index for exact matches.
-- Scoped to a tenant and linked to a cardholder.

CREATE TABLE cards (
    id              UUID NOT NULL DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    cardholder_id   UUID NOT NULL,
    pan_encrypted   BYTEA,
    pan_blind_index VARCHAR(64),
    last_four       VARCHAR(4),
    expiry_month    SMALLINT,
    expiry_year     SMALLINT,
    status          VARCHAR(20) NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'ACTIVE', 'SUSPENDED', 'CLOSED', 'CANCELLED')),
    version         BIGINT NOT NULL DEFAULT 1,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, cardholder_id) REFERENCES cardholders(tenant_id, id)
);

CREATE INDEX idx_cards_tenant_blind_index ON cards (tenant_id, pan_blind_index);
CREATE INDEX idx_cards_tenant_status ON cards (tenant_id, status);
CREATE INDEX idx_cards_tenant_cardholder ON cards (tenant_id, cardholder_id);

CREATE UNIQUE INDEX uidx_cards_tenant_blind_index ON cards (tenant_id, pan_blind_index) WHERE pan_blind_index IS NOT NULL;
