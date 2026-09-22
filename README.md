# Card Issuer API — Banking-as-a-Service (BaaS)

A production-grade Card Issuer core infrastructure service implemented in **Go (1.22+)** for multi-tenant financial institutions. Engineered with **Hexagonal Architecture (Ports & Adapters)**, strict data siloing, PCI-DSS v4.0 cardholder data protection, and high-consistency, deadlock-free batch status operations.

---

## 📑 Table of Contents

- [Executive Architecture Summary](#-executive-architecture-summary)
- [How to Run (Execution Modes)](#-how-to-run-execution-modes)
  - [Option A: Standalone Zero-Dependency Run (Instant Evaluation in 1s)](#option-a-standalone-zero-dependency-run-instant-evaluation-in-1s)
  - [Option B: Containerized Run (Docker Compose + PostgreSQL 16)](#option-b-containerized-run-docker-compose--postgresql-16)
- [Running the Test Suite with Race Detector](#-running-the-test-suite-with-race-detector)
- [Complete Step-by-Step API Walkthrough (cURL Examples)](#-complete-step-by-step-api-walkthrough-curl-examples)
- [Key Security & Architectural Invariants](#-key-security--architectural-invariants)
- [Interview & Evaluation Deliverables](#-interview--evaluation-deliverables)

---

## 🏛️ Executive Architecture Summary

```
                          ┌─────────────────────────────────────┐
                          │     Client Application / Ingress    │
                          └──────────────────┬──────────────────┘
                                             │  (mTLS / Bearer Token)
                                             ▼
┌───────────────────────────────────────────────────────────────────────────────────┐
│ Card Issuer API (Go Service)                                                      │
│                                                                                   │
│  ┌───────────────────────┐   ┌───────────────────────┐   ┌─────────────────────┐  │
│  │ Recovery & Request ID │──▶│ Auth (API Key Hash)   │──▶│ Tenant Enforcement │  │
│  └───────────────────────┘   └───────────────────────┘   └─────────────────────┘  │
│                                          │                                        │
│                                          ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────────────────┐  │
│  │ Idempotency Middleware (Mutating Request Deduplication via Database Cache)  │  │
│  └───────────────────────────────────────┬─────────────────────────────────────┘  │
│                                          │                                        │
│                                          ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────────────────┐  │
│  │ HTTP Handlers Layer (Go 1.22+ Enhanced Routing, Strict UUID & DTO Boundary) │  │
│  └───────────────────────────────────────┬─────────────────────────────────────┘  │
│                                          │                                        │
│                                          ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────────────────┐  │
│  │ Domain & Business Services Layer (Pure Go, Zero External Dependencies)      │  │
│  │  • Card Lifecycle State Machine (PENDING -> ACTIVE <-> SUSPENDED -> CLOSED) │  │
│  │  • Cryptographic Engine (AES-256-GCM AEAD with Tenant AAD + Blind Indexing) │  │
│  │  • Batch Status Engine (Deterministic Deadlock-Free Locking & OCC)          │  │
│  └───────────────────────────────────────┬─────────────────────────────────────┘  │
│                                          │                                        │
│                                          ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────────────────┐  │
│  │ Repository Layer (Parameterized SQL Queries Only + Immutable Audit Log)     │  │
│  └───────────────────────────────────────┬─────────────────────────────────────┘  │
└──────────────────────────────────────────┼────────────────────────────────────────┘
                                           │
                        ┌──────────────────┴──────────────────┐
                        ▼                                     ▼
        ┌───────────────────────────────┐     ┌────────────────────────────────┐
        │ Modular In-Memory Storage     │     │ PostgreSQL 16 Cluster          │
        │ • Zero dependencies           │     │ • Row-Level Security (RLS)     │
        │ • Instant local demo in 1s    │     │ • Composite PKs (tenant_id,id) │
        │ • Thread-safe OCC & rollback  │     │ • Append-only audit trigger    │
        └───────────────────────────────┘     └────────────────────────────────┘
```

---

## 🚀 How to Run (Execution Modes)

The service is fully decoupled from the storage engine via Go interfaces. You can run it either standalone with zero setup or containerized with PostgreSQL.

### Option A: Standalone Zero-Dependency Run (Instant Evaluation in 1s)

> **Recommended for Evaluators / Live Demos**: No Docker or PostgreSQL installation needed. The modular in-memory storage engine initializes in milliseconds and pre-seeds mock data for immediate testing.

1. Navigate to the project directory:
   ```bash
   cd card-issuer-api
   ```

2. Start the service directly:
   ```bash
   go run ./cmd/server
   ```
   *(or compile and run via `make run`)*

3. The service boots up immediately and prints the live evaluation banner:
   ```
   =========================================================================
   🚀 DEMO READY (Zero-dependency modular mode)
   -------------------------------------------------------------------------
   • Demo Tenant ID:   00000000-0000-0000-0000-000000000001
   • Demo API Key:     dev-api-key-tenant-1
   • Demo Cardholder:  11111111-1111-1111-1111-111111111111 (Elena Rostova)
   • Sample PENDING:   22222222-2222-2222-2222-222222222222
   • Sample ACTIVE:    33333333-3333-3333-3333-333333333333
   -------------------------------------------------------------------------
   ```

---

### Option B: Containerized Run (Docker Compose + PostgreSQL 16)

Runs with the full PostgreSQL database pool, DDL migrations, and enforced Row-Level Security:

1. Start both the API service and PostgreSQL 16:
   ```bash
   cd card-issuer-api
   docker-compose up --build -d
   ```

2. Check container health:
   ```bash
   docker-compose ps
   ```

3. View live logs:
   ```bash
   docker-compose logs -f api
   ```

4. Stop and remove containers:
   ```bash
   docker-compose down -v
   ```

---

## 🧪 Running the Test Suite with Race Detector

Run all unit, domain, cryptographic, middleware, and concurrent batch processing tests with Go's race condition detector:

```bash
cd card-issuer-api
go test ./... -v -race -count=1
```

Expected output:
```
ok  github.com/novopayment/card-issuer-api/internal/crypto      1.411s
ok  github.com/novopayment/card-issuer-api/internal/domain      1.554s
ok  github.com/novopayment/card-issuer-api/internal/middleware  1.689s
ok  github.com/novopayment/card-issuer-api/internal/service     1.883s
```
*(All tests pass with **0 data races**).*

---

## 📡 Complete Step-by-Step API Walkthrough (cURL Examples)

*All requests use the default pre-seeded API key: `dev-api-key-tenant-1`.*

### 1. Health & Liveness Probes (Public Endpoints)
```bash
curl -i http://localhost:8080/health
```
```json
HTTP/1.1 200 OK
Content-Type: application/json

{"status":"ok"}
```

---

### 2. Query Pre-seeded Active Card
Query the sample active card with masked PAN:
```bash
curl -s -H "Authorization: Bearer dev-api-key-tenant-1" \
  http://localhost:8080/v1/cards/33333333-3333-3333-3333-333333333333 | jq
```
```json
{
  "id": "33333333-3333-3333-3333-333333333333",
  "cardholder_id": "11111111-1111-1111-1111-111111111111",
  "last_four": "4802",
  "expiry_month": 12,
  "expiry_year": 2029,
  "status": "ACTIVE",
  "created_at": "2026-09-21T17:33:31Z",
  "updated_at": "2026-09-21T17:33:31Z"
}
```

---

### 3. Create a New Cardholder
```bash
curl -s -X POST http://localhost:8080/v1/cardholders \
  -H "Authorization: Bearer dev-api-key-tenant-1" \
  -H "Idempotency-Key: idemp-ch-001" \
  -H "Content-Type: application/json" \
  -d '{
    "first_name": "Carlos",
    "last_name": "Mendoza",
    "email": "carlos.mendoza@example.com",
    "phone": "+15559876543"
  }' | jq
```
```json
{
  "id": "9f38a5e8-1422-4217-b778-57d383b194d2",
  "first_name": "Carlos",
  "last_name": "Mendoza",
  "email": "carlos.mendoza@example.com",
  "phone": "+15559876543",
  "created_at": "2026-09-21T17:40:00Z",
  "updated_at": "2026-09-21T17:40:00Z"
}
```

---

### 4. Request a New Card (`PENDING`)
```bash
curl -s -X POST http://localhost:8080/v1/cards \
  -H "Authorization: Bearer dev-api-key-tenant-1" \
  -H "Idempotency-Key: idemp-card-001" \
  -H "Content-Type: application/json" \
  -d '{
    "cardholder_id": "11111111-1111-1111-1111-111111111111"
  }' | jq
```
```json
{
  "id": "a4d3f2c1-8899-4321-b001-123456789abc",
  "cardholder_id": "11111111-1111-1111-1111-111111111111",
  "status": "PENDING",
  "created_at": "2026-09-21T17:41:00Z",
  "updated_at": "2026-09-21T17:41:00Z"
}
```

---

### 5. Issue Card (`PENDING` ➔ `ACTIVE`)
Generates a Luhn-valid PAN, encrypts via **AES-256-GCM** with tenant AAD binding, computes an HMAC-SHA256 blind index, and zeroizes memory:
```bash
curl -s -X POST http://localhost:8080/v1/cards/22222222-2222-2222-2222-222222222222/issue \
  -H "Authorization: Bearer dev-api-key-tenant-1" \
  -H "Idempotency-Key: idemp-issue-001" | jq
```
```json
{
  "id": "22222222-2222-2222-2222-222222222222",
  "cardholder_id": "11111111-1111-1111-1111-111111111111",
  "last_four": "1049",
  "expiry_month": 12,
  "expiry_year": 2029,
  "status": "ACTIVE",
  "created_at": "2026-09-21T17:33:31Z",
  "updated_at": "2026-09-21T17:42:00Z"
}
```

---

### 6. Suspend Card (`ACTIVE` ➔ `SUSPENDED`)
Reversible freeze operation:
```bash
curl -s -X POST http://localhost:8080/v1/cards/22222222-2222-2222-2222-222222222222/suspend \
  -H "Authorization: Bearer dev-api-key-tenant-1" \
  -H "Idempotency-Key: idemp-suspend-001" | jq
```
```json
{
  "id": "22222222-2222-2222-2222-222222222222",
  "cardholder_id": "11111111-1111-1111-1111-111111111111",
  "last_four": "1049",
  "expiry_month": 12,
  "expiry_year": 2029,
  "status": "SUSPENDED",
  "created_at": "2026-09-21T17:33:31Z",
  "updated_at": "2026-09-21T17:43:00Z"
}
```

---

### 7. Reactivate Card (`SUSPENDED` ➔ `ACTIVE`)
Restores the card to active status:
```bash
curl -s -X POST http://localhost:8080/v1/cards/22222222-2222-2222-2222-222222222222/reactivate \
  -H "Authorization: Bearer dev-api-key-tenant-1" \
  -H "Idempotency-Key: idemp-reactivate-001" | jq
```
```json
{
  "id": "22222222-2222-2222-2222-222222222222",
  "cardholder_id": "11111111-1111-1111-1111-111111111111",
  "last_four": "1049",
  "expiry_month": 12,
  "expiry_year": 2029,
  "status": "ACTIVE",
  "created_at": "2026-09-21T17:33:31Z",
  "updated_at": "2026-09-21T17:44:00Z"
}
```

---

### 8. Close Card (`ACTIVE`/`SUSPENDED` ➔ `CLOSED`)
Terminal state transition. Cannot be reversed:
```bash
curl -s -X POST http://localhost:8080/v1/cards/22222222-2222-2222-2222-222222222222/close \
  -H "Authorization: Bearer dev-api-key-tenant-1" \
  -H "Idempotency-Key: idemp-close-001" | jq
```
```json
{
  "id": "22222222-2222-2222-2222-222222222222",
  "cardholder_id": "11111111-1111-1111-1111-111111111111",
  "last_four": "1049",
  "expiry_month": 12,
  "expiry_year": 2029,
  "status": "CLOSED",
  "created_at": "2026-09-21T17:33:31Z",
  "updated_at": "2026-09-21T17:45:00Z"
}
```

---

### 9. Batch Status Update: Atomic Mode (All-or-Nothing)
Executes across cards in a single atomic transaction with `ORDER BY id ASC FOR UPDATE` deadlock prevention:
```bash
curl -s -X POST http://localhost:8080/v1/batch/card-status \
  -H "Authorization: Bearer dev-api-key-tenant-1" \
  -H "Idempotency-Key: idemp-batch-atomic-01" \
  -H "Content-Type: application/json" \
  -d '{
    "card_ids": ["33333333-3333-3333-3333-333333333333"],
    "action": "SUSPEND",
    "mode": "atomic"
  }' | jq
```
```json
{
  "batch_id": "b7418827-de31-46f9-bf69-a4d63bfc77a2",
  "status": "COMPLETED",
  "mode": "ATOMIC",
  "action": "SUSPEND",
  "total": 1,
  "succeeded": 1,
  "failed": 0,
  "results": [
    {
      "card_id": "33333333-3333-3333-3333-333333333333",
      "success": true
    }
  ]
}
```

---

### 10. Batch Status Update: Partial Mode (Best-Effort Chunking)
High-throughput worker pool execution. Valid cards succeed, and invalid transitions return detailed error breakdowns:
```bash
curl -s -X POST http://localhost:8080/v1/batch/card-status \
  -H "Authorization: Bearer dev-api-key-tenant-1" \
  -H "Idempotency-Key: idemp-batch-partial-01" \
  -H "Content-Type: application/json" \
  -d '{
    "card_ids": [
      "33333333-3333-3333-3333-333333333333",
      "22222222-2222-2222-2222-222222222222"
    ],
    "action": "ACTIVATE",
    "mode": "partial"
  }' | jq
```
```json
{
  "batch_id": "c1920381-fe44-4890-a112-987654321def",
  "status": "COMPLETED",
  "mode": "PARTIAL",
  "action": "ACTIVATE",
  "total": 2,
  "succeeded": 1,
  "failed": 1,
  "results": [
    {
      "card_id": "33333333-3333-3333-3333-333333333333",
      "success": true
    },
    {
      "card_id": "22222222-2222-2222-2222-222222222222",
      "success": false,
      "error": "cannot reactivate card from status CLOSED (must be SUSPENDED)"
    }
  ]
}
```

---

### 11. Idempotency Verification
Re-submitting any mutating request with the same `Idempotency-Key` immediately returns the cached response with zero duplicate database operations:
```bash
curl -s -X POST http://localhost:8080/v1/batch/card-status \
  -H "Authorization: Bearer dev-api-key-tenant-1" \
  -H "Idempotency-Key: idemp-batch-atomic-01" \
  -H "Content-Type: application/json" \
  -d '{
    "card_ids": ["33333333-3333-3333-3333-333333333333"],
    "action": "SUSPEND",
    "mode": "atomic"
  }' | jq
```
*(Returns the cached response from Step 9 with matching `batch_id`).*

---

## 🔒 Key Security & Architectural Invariants

| Invariant | Technical Implementation | Purpose |
|---|---|---|
| **Multi-Tenant Isolation** | Context Injection + Parameterized SQL (`WHERE tenant_id = $1`) + PostgreSQL RLS | Prevents cross-tenant data leaks even under connection pool reuse. |
| **No Plaintext PAN on Disk** | AES-256-GCM authenticated encryption with 12-byte CSPRNG random nonces | Satisfies PCI-DSS v4.0 Requirement 3.4. |
| **Tamper-Proof Ciphertext** | `tenant_id` injected as Additional Authenticated Data (AAD) into AES-GCM | Prevents moving encrypted card blobs between tenant partitions. |
| **Frequency Analysis Defense** | HMAC-SHA256 Blind Indexing with an independent secret key | Enables exact-match indexed queries without deterministic encryption risks. |
| **Zero Memory Leakage** | `crypto.Zeroize` wipes raw byte buffers; custom `domain.PAN` implements `slog.LogValuer` | Guarantees raw PAN is never written to logs, metrics, or JSON dumps. |
| **Deadlock Elimination** | `SELECT ... FOR UPDATE ORDER BY id ASC` | Imposes a globally consistent lock acquisition order across concurrent batches. |
| **Lost Update Prevention** | Optimistic Concurrency Control (OCC) via `version` column increments | Detects racing status transitions and returns `409 Conflict`. |
| **Immutable Audit Log** | Append-only `audit_events` table with database triggers preventing UPDATE/DELETE | Ensures compliance-ready audit trails for financial regulators. |

---

## 📑 Interview & Evaluation Deliverables

1. **[docs/DECISION_LOG_AND_TRADEOFFS.md](docs/DECISION_LOG_AND_TRADEOFFS.md)**:
   - Detailed write-up of Architectural Decision Records (ADR-001 through ADR-007).
   - Rationale for batch processing strategies (Atomic vs. Partial modes).
   - Cryptographic design analysis (AES-GCM + Blind Indexing vs. Deterministic AES).
   - Observed compliance gaps in the exercise and corresponding production mitigations (AWS KMS, mTLS, WORM audit storage).

2. **[docs/AI_INTERACTION_LOG.md](docs/AI_INTERACTION_LOG.md)**:
   - Structured audit of AI tool usage during development.
   - Documentation of tasks delegated to LLMs.
   - Specific instances where AI suggestions were **overridden or rejected** (e.g., rejecting naive batch updates causing deadlocks, rejecting deterministic encryption, enforcing strict state machine transitions).
   - Raw prompt history and engineering decision trail.
