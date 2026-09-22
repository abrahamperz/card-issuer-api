# Card Issuer API — Evaluation & Run Guide

This runbook guides technical interviewers and evaluators through running, testing, and verifying the Card Issuer API service.

---

## ⚡ 60-Second Quickstart (Zero Dependencies)

You do **not** need Docker or PostgreSQL installed to evaluate this project. The service includes a thread-safe, in-memory storage engine adhering to Hexagonal Architecture (*Ports & Adapters*).

```bash
# 1. Clone & Enter project
cd card-issuer-api

# 2. Run standalone (starts on :8080 in < 1 second)
go run ./cmd/server
```

When the service starts, it prints pre-seeded test fixtures:
- **Tenant ID**: `00000000-0000-0000-0000-000000000001` (*NovoBank International*)
- **API Key**: `dev-api-key-tenant-1`
- **Cardholder ID**: `11111111-1111-1111-1111-111111111111` (*Elena Rostova*)
- **Pre-seeded Active Card**: `33333333-3333-3333-3333-333333333333`
- **Pre-seeded Pending Card**: `22222222-2222-2222-2222-222222222222`

---

## 🧪 Verifying Concurrency & Data Race Freedom

Run the full test suite with Go's race detector:

```bash
go test ./... -v -race -count=1
```

**What this verifies:**
1. **`internal/crypto`**: AES-256-GCM authenticated encryption roundtrip, random nonce uniqueness, tenant AAD binding, HMAC blind index determinism, and memory zeroization.
2. **`internal/domain`**: State machine validation (PENDING ➔ ACTIVE ➔ SUSPENDED ➔ CLOSED), rejection of illegal transitions, Luhn algorithm checks, and PAN masking formatters (`String()`, `MarshalJSON()`, `LogValue()`).
3. **`internal/middleware`**: Multi-tenant context extraction, 401 unauthorized on missing tokens, 403 forbidden on mismatched tenant headers, panic recovery, and idempotency deduplication.
4. **`internal/service`**: Card issuance with field-level encryption, Atomic batch mode all-or-nothing rollback on validation failure, Partial batch mode chunking, and concurrent multi-card updates without data races.

---

## 💻 Live API Verification with cURL

Open another terminal and execute these commands:

### 1. Healthcheck (Public)
```bash
curl -i http://localhost:8080/health
```

### 2. Query Active Card (Masked PAN)
```bash
curl -s -H "Authorization: Bearer dev-api-key-tenant-1" \
  http://localhost:8080/v1/cards/33333333-3333-3333-3333-333333333333
```
*Notice that the PAN is never leaked in plaintext: only `last_four` is returned.*

### 3. Issue a Pending Card (`PENDING` ➔ `ACTIVE`)
```bash
curl -s -X POST http://localhost:8080/v1/cards/22222222-2222-2222-2222-222222222222/issue \
  -H "Authorization: Bearer dev-api-key-tenant-1" \
  -H "Idempotency-Key: issue-key-01"
```
*Generates a Luhn-valid PAN, encrypts using AES-256-GCM with tenant AAD, stores the blind index, and transitions status to `ACTIVE`.*

### 4. Batch Status Update: Atomic Mode (All-or-Nothing)
```bash
curl -s -X POST http://localhost:8080/v1/batch/card-status \
  -H "Authorization: Bearer dev-api-key-tenant-1" \
  -H "Idempotency-Key: batch-key-01" \
  -H "Content-Type: application/json" \
  -d '{
    "card_ids": ["33333333-3333-3333-3333-333333333333"],
    "action": "SUSPEND",
    "mode": "atomic"
  }'
```
*Locks cards in lexicographical order (`ORDER BY id ASC FOR UPDATE`) to prevent deadlocks and transitions them atomically.*

### 5. Idempotency Test
Repeat the exact same curl command from Step 4.  
*Notice it returns immediately with the cached result without duplicate processing.*

---

## 🐳 Optional: Running with Docker Compose & PostgreSQL 16

If you wish to test with a full PostgreSQL 16 database and database-level Row-Level Security (RLS):

```bash
docker-compose up --build -d
```

Check logs:
```bash
docker-compose logs -f api
```

Tear down:
```bash
docker-compose down -v
```

---

## 📚 Technical Documentation & Evaluation Deliverables

- **Architectural Decisions & Trade-off Write-up**: [`docs/DECISION_LOG_AND_TRADEOFFS.md`](DECISION_LOG_AND_TRADEOFFS.md)
- **AI Tool Usage & Override Log**: [`docs/AI_INTERACTION_LOG.md`](AI_INTERACTION_LOG.md)
