# AI Interaction Log & Engineering Evaluation

**Candidate / Engineer:** Senior/Principal Backend Engineer Submission  
**Role:** Card Issuer API Architecture & Implementation (Banking-as-a-Service)  
**Tools Leveraged:** Google Antigravity Agentic AI (Gemini / Claude Opus multi-agent collaboration)  

---

## 1. Executive Summary: AI Delegation Strategy

Modern senior engineering involves orchestrating LLMs to accelerate development velocity while exercising rigorous technical governance. For this exercise, AI tools were treated as **junior/mid-level engineering agents executing defined sub-tasks**, while architectural oversight, cryptographic invariants, concurrency models, and security boundary reviews were strictly maintained by the senior engineer.

---

## 2. Structured Task Delegation Matrix

| Phase | Task Delegated to AI | AI Output Assessment | Engineer Intervention / Correction |
|---|---|---|---|
| **Phase 1: Domain & Crypto** | Generate AES-256-GCM cipher routines and HMAC blind index hashing. | AI correctly implemented `cipher.NewGCM`, but initially generated a standard random IV without tenant context. | **Overrode to enforce Authenticated Additional Data (AAD)** using `tenant_id`. Decryption now fails if ciphertext is copied across tenant records. |
| **Phase 1: Crypto** | Implement memory zeroing for sensitive PAN buffers. | AI proposed `panBytes = nil` or a naive `for i := range b { b[i] = 0 }`. | **Corrected with compiler-safe `Zeroize`**: Naive zero loops can be eliminated by the Go compiler's dead-code elimination pass. Replaced with volatile-safe memory overwriting. |
| **Phase 2: Database** | Generate PostgreSQL DDL migrations with multi-tenant constraints. | AI generated valid DDL with indexes, but initially used simple primary keys `id UUID PRIMARY KEY`. | **Overrode to enforce composite primary keys `(tenant_id, id)`** and PostgreSQL Row-Level Security policies with `FORCE ROW LEVEL SECURITY`. |
| **Phase 3: Batch Engine** | Implement bulk card status updates across database records. | AI suggested a single bulk `UPDATE cards SET status = $1 WHERE id IN (...)` without row locking or order. | **Overrode completely**: Identified severe **deadlock vulnerability** under concurrent requests and lack of OCC. Implemented deterministic row locking (`ORDER BY id ASC FOR UPDATE`) and dual-mode execution (Atomic vs. Partial with chunking). |
| **Phase 4: State Machine** | Implement domain lifecycle methods (`Issue`, `Suspend`, `Reactivate`, `Close`). | AI wrote a permissive `canTransitionTo(CardStatus)` check allowing `Reactivate()` on a `PENDING` card. | **Corrected state transitions**: Enforced explicit preconditions per method (e.g., `Reactivate()` strictly requires `SUSPENDED`; `Issue()` strictly requires `PENDING`). |
| **Phase 5: HTTP Layer** | Implement HTTP handlers and JSON serialization. | AI generated DTOs that initially accepted raw string parameters into domain services without validating UUID structure at the boundary. | **Corrected handler boundary**: Enforced strict UUID parsing and input validation in HTTP handlers, returning immediate 400 Bad Request before hitting service logic. |

---

## 3. Key Instances Where AI Suggestions Were Overridden or Rejected

### Case 1: Rejecting Deterministic Encryption for PAN Lookups
* **AI Initial Proposal**: To allow operators to query cards by PAN (`GET /v1/cards?pan=4111...`), the AI suggested using deterministic AES-256 (encrypting with a static Initialization Vector / Nonce) so that the resulting ciphertext could be queried directly with `WHERE pan_encrypted = $1`.
* **Technical Reason for Rejection**: In financial systems, deterministic encryption violates fundamental PCI-DSS compliance principles because identical credit card numbers always generate identical ciphertext. An adversary with read access to the database could perform **frequency analysis**, infer card types, or correlate transactions across merchants.
* **Engineered Solution**: Implemented **HMAC-SHA256 Blind Indexing** with an independent secret key (Key Separation Principle). The encrypted card uses non-deterministic AES-256-GCM with a CSPRNG 12-byte random nonce per record, while searches leverage the non-reversible blind index column.

### Case 2: Rejecting RLS as the Sole Multi-Tenant Guardrail
* **AI Initial Proposal**: The AI suggested relying exclusively on PostgreSQL Row Level Security (RLS) via `SET app.current_tenant = '...'` to avoid having to specify `tenant_id` in every SQL query.
* **Technical Reason for Rejection**: Go's `database/sql` connection pool reuses physical TCP sockets across different goroutines. If a connection is returned to the pool without resetting the session variable, or if a connection error occurs midway through a pipeline, the next query executing on that connection could run with a stale tenant context, causing catastrophic cross-tenant data leaks.
* **Engineered Solution**: Enforced **three layers of defense**:
  1. Mandated parameterized `WHERE tenant_id = $N` bind parameters in every SQL statement.
  2. Used composite primary keys `(tenant_id, id)`.
  3. Applied PostgreSQL RLS with `set_config('app.current_tenant', $1, true)` (transaction-local) purely as defense-in-depth.

### Case 3: Fixing Batch Processing Deadlocks Under Concurrency
* **AI Initial Proposal**: The AI suggested processing batch updates by executing concurrent goroutines each updating individual cards directly, or locking rows in whatever order they appeared in the request slice.
* **Technical Reason for Rejection**: If Batch 1 updates cards `[A, B]` and Batch 2 concurrently updates cards `[B, A]`, Transaction 1 acquires lock on `A` and waits for `B`, while Transaction 2 acquires lock on `B` and waits for `A`. This triggers a PostgreSQL deadlock (`40P01`) under moderate production load.
* **Engineered Solution**: Enforced **deterministic lexicographical sorting** (`ORDER BY id ASC FOR UPDATE`) in the query before acquiring row locks. Because all concurrent transactions request locks in the identical order, cycle formation in the wait-for graph is mathematically prevented.

---

## 4. Prompt Log & Interaction History

Below is a structured log of the primary prompts and instructions provided to the AI during the planning and build phases:

### Interaction 1: Architectural Scaffolding & Security Invariants
* **Prompt**:
  > "Design a Card Issuer API in Go for a BaaS platform with strict multi-tenant isolation, PCI-DSS v4.0 aligned PAN handling, and batch operations. The domain model must be dependency-free. The cryptographic layer must support field-level encryption and searchable lookups without deterministic encryption. Outline the project layout and state machine."
* **AI Deliverable**: Generated architectural plan (`ARCHITECTURE_PLAN.md`) outlining package breakdown, state machine matrix, and schema design.
* **Engineer Critique & Adjustment**: Added explicit requirement for AAD tenant binding in AES-GCM and required optimistic locking (`version` column) on state updates.

### Interaction 2: Cryptographic & Domain Layer Implementation
* **Prompt**:
  > "Implement the cryptographic package with AES-256-GCM AEAD and HMAC-SHA256 blind indexing in `internal/crypto`. Implement the `domain` package with Card, Cardholder, Batch, and Audit entities. Ensure custom types prevent PAN leakage to logs or JSON serialization."
* **AI Deliverable**: Created `internal/crypto/aes.go`, `blind_index.go`, `domain/card.go`, and test fixtures.
* **Engineer Critique & Adjustment**: Corrected `domain.PAN` methods to implement `slog.LogValuer` in addition to `fmt.Stringer`. Adjusted state transition validation to reject `Reactivate` from `PENDING`.

### Interaction 3: Repository & Transaction Pattern Alignment
* **Prompt**:
  > "Implement repository interfaces and PostgreSQL implementations. Provide transactional safety for operations that touch multiple tables."
* **AI Deliverable**: Repositories created, but initially attempted passing `*sql.Tx` as a parameter to every repository method, creating tight coupling.
* **Engineer Critique & Adjustment**: Refactored to the `TxRepositories` factory pattern (`txManager.WithTransaction(ctx, func(ctx, repos TxRepositories) error)`). This kept repository interfaces clean and decoupled from transaction management while guaranteeing atomicity.

### Interaction 4: Concurrency & Race Condition Verification
* **Prompt**:
  > "Write unit and concurrency tests for CardService and BatchService, specifically testing atomic rollback when one item in a batch fails, partial mode error reporting, and concurrent batch updates under `go test -race`."
* **AI Deliverable**: Generated `internal/service/service_test.go` with mock repositories.
* **Engineer Critique & Adjustment**: Identified a bug where mock card setup used `cardRepo.Create` instead of `cardRepo.Update` for closed cards, causing test assertions to fail. Fixed mock data fixture and verified 100% test pass under race detector.

---

## 5. Conclusion & Engineering Judgment

AI tools dramatically accelerated repetitive tasks (generating DDL tables, writing mock structs, setting up boilerplate tests). However, **security and concurrency cannot be outsourced to generative models**. 

The core differentiators of this submission—AAD tenant binding, blind indexing to defeat frequency analysis, deterministic row-locking to eradicate deadlocks, and multi-layered tenant isolation—stem from deliberate, senior engineering judgment overruling standard, naive AI defaults.
