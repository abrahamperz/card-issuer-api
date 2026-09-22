# Decision Log & Architectural Trade-offs

**Service:** Card Issuer API (Banking-as-a-Service Core Infrastructure)  
**Target Domain:** Multi-Tenant Card Lifecycle Management, PCI-DSS v4.0 Compliance, High-Consistency Batch Processing  
**Language:** Go (1.22+)  

---

## 1. Architectural Decisions Summary

| # | Architectural Decision | Chosen Pattern | Key Trade-off / Rationale |
|---|------------------------|----------------|---------------------------|
| **ADR-001** | Multi-Tenant Data Isolation | **Defense-in-Depth (3 Layers)**: Context Injection + Parameterized SQL + PostgreSQL RLS | Prioritizes complete data siloing over single-point developer ergonomics; eliminates tenant cross-talk even under connection-pool reuse or app-layer bugs. |
| **ADR-002** | PAN Storage & Queryability | **Field-Level Encryption (AES-256-GCM)** + **Blind Indexing (HMAC-SHA256)** | Rejects deterministic encryption (vulnerable to frequency analysis); accepts index storage overhead to achieve safe $O(1)$ exact-match queries. |
| **ADR-003** | PAN In-Memory Protection | **Custom Encapsulated `domain.PAN` Type** with custom `fmt.Stringer`, `json.Marshaler`, `slog.LogValuer` | Eliminates accidental PAN leakage to log streams, JSON payloads, and stack traces at compile/runtime. |
| **ADR-004** | Batch Status Engine Concurrency | **Dual-Mode Engine**: Atomic (*All-or-Nothing*) & Partial (*Best-Effort Chunked*) with **Deterministic Row Locking** (`ORDER BY id ASC FOR UPDATE`) | Eradicates deadlocks across concurrent multi-card updates while allowing clients to choose between all-or-nothing consistency and partial throughput. |
| **ADR-005** | State Mutation Consistency | **Optimistic Concurrency Control (OCC)** via `version` column increments | Avoids long-lived distributed locks; handles racing lifecycle transitions deterministically with HTTP 409 Conflict. |
| **ADR-006** | Framework Selection | **Standard Library `net/http` (Go 1.22+ routing)** | Eliminates external HTTP framework vulnerabilities, reduces binary footprint, and ensures direct control over HTTP connection lifecycles and memory allocations. |
| **ADR-007** | Storage Modularity | **Hexagonal Architecture (Ports & Adapters)**: Pluggable Storage Engine (`memory` vs `postgres`) | Decouples business & domain logic from database engines. Evaluators can run the service locally in 1 second with zero dependencies, while production operates over PostgreSQL 16 with RLS. |

---

## 2. In-Depth Architectural Rationale

### 2.1 Multi-Tenant Data Siloing (Defense-in-Depth)

In a BaaS environment where multiple regulated financial institutions share the same physical database cluster, relying on a single isolation mechanism represents an unacceptable single point of failure.

#### The Three Layers of Defense:
1. **Layer 1: Context & Edge Gateway Enforcement**
   - The edge authentication middleware inspects the API credential (hashed API key) and strictly injects an immutable `tenant_id` into the request's `context.Context`.
   - Any HTTP request attempting to access domain/repository logic without a valid `tenant_id` context triggers a panic (`domain.MustTenantID`), treating missing tenant scoping as a critical programming bug rather than a client-recoverable error.
   - If an optional `X-Tenant-ID` header is sent, it is strictly validated against the token's authenticated tenant. Mismatches yield `403 Forbidden`.

2. **Layer 2: Mandatory Query Parameterization (Application Invariant)**
   - Every database query in the repository layer includes `WHERE tenant_id = $1` as a bind parameter.
   - Dynamic query construction via string formatting (`fmt.Sprintf`, string concatenation) is strictly banned to eliminate SQL injection vectors.
   - Primary keys for tenant-scoped entities are composite: `(tenant_id, id)`. This ensures that even if an attacker guesses or intercepts a UUID from another institution, the database engine returns zero rows.

3. **Layer 3: PostgreSQL Row-Level Security (Database Invariant)**
   - Tables (`cards`, `cardholders`, `batch_operations`, `audit_events`) have PostgreSQL RLS enabled and forced (`FORCE ROW LEVEL SECURITY`).
   - Transactions execute `SELECT set_config('app.current_tenant', $1, true)` scoped strictly to the local transaction (`is_local = true`).
   - **Critical Trade-off Addressed**: We explicitly chose *not* to rely on RLS session variables alone. If a pooled database connection is reused by another goroutine without clearing session state, a pure RLS approach can silently leak tenant data. By coupling parameterized SQL with local-transaction RLS, each layer acts as a fail-safe for the other.

---

### 2.2 PCI-DSS v4.0 Compliant PAN Handling

Cardholder Data Environment (CDE) standards demand that the Primary Account Number (PAN) is rendered unreadable wherever it is stored or transmitted.

```
       Plaintext PAN (in memory during issuance)
                   │
         ┌─────────┴─────────┐
         ▼                   ▼
┌──────────────────┐ ┌───────────────────────────┐
│ HMAC-SHA256      │ │ AES-256-GCM (AEAD)        │
│ • Blind Index Key│ │ • Data Encryption Key     │
│ • Hex Digest     │ │ • Nonce: 12-byte CSPRNG   │
│                  │ │ • AAD: tenant_id (UUID)   │
└────────┬─────────┘ └─────────────┬─────────────┘
         │                         │
         ▼                         ▼
  pan_blind_index            pan_encrypted
  (Indexed column)          (Bytea ciphertext)
         │                         │
         └─────────┬───────────────┘
                   ▼
       Zeroize(plaintext PAN)
```

#### Why Blind Indexing Over Deterministic Encryption?
- **The Problem**: Operators need to search for cards by PAN (e.g., customer service lookups or duplicate detection during card creation). Using deterministic encryption (AES with fixed IV) means that identical PANs produce identical ciphertexts, exposing the database to **frequency analysis and inference attacks**.
- **The Solution**: 
  1. We store the PAN encrypted using **AES-256-GCM** with a **unique, cryptographically secure 12-byte random nonce** per record. Identical PANs produce completely different ciphertexts.
  2. We bind the ciphertext to the institution using **Additional Authenticated Data (AAD)** set to the `tenant_id`. If an attacker copies an encrypted blob from Tenant A's database partition into Tenant B's partition, decryption fails with an authentication tag mismatch.
  3. We compute a **Blind Index** using `HMAC-SHA256(BlindIndexKey, PAN)` with an isolated secret key (Key Separation Principle). This hash is irreversible, resistant to rainbow tables, and enables indexed $O(1)$ exact-match queries (`WHERE tenant_id = $1 AND pan_blind_index = $2`) without decrypting the dataset.
- **Memory Hygiene (`Zeroize`)**:
  - Sensitive plaintext PAN byte buffers are immediately wiped using a non-optimizable memory zeroization routine (`crypto.Zeroize`) after encryption.
  - Custom Go types (`domain.PAN`) implement `slog.LogValuer`, `fmt.Stringer`, and `json.Marshaler` returning masked values (`****-****-****-1234`), preventing accidental logging in ELK/Datadog or serializing into HTTP responses.

---

### 2.3 Batch Processing Engine & Concurrency Strategy

Bank operators regularly trigger bulk status updates (e.g., fraud blockades, mass card expirations, or regulatory suspensions). This presents severe concurrency challenges: **database deadlocks**, **lost updates**, and **inconsistent partial states**.

#### 1. Deadlock Elimination via Deterministic Row Locking
- **The Threat**: If Operator A triggers a batch updating cards `[UUID-1, UUID-2]` and Operator B simultaneously triggers a batch updating cards `[UUID-2, UUID-1]`, two PostgreSQL transactions can acquire locks in opposite order and trigger a database deadlock (`40P01`).
- **The Mitigation**: All batch queries execute:
  ```sql
  SELECT id, tenant_id, status, version 
  FROM cards 
  WHERE tenant_id = $1 AND id = ANY($2) 
  ORDER BY id ASC 
  FOR UPDATE;
  ```
  Sorting candidate IDs in ascending lexicographical order prior to locking guarantees a single, global lock-acquisition hierarchy. Deadlocks become mathematically impossible across concurrent batch jobs.

#### 2. Dual-Mode Architecture: Atomic vs. Partial

```
                      POST /v1/batch/card-status
                                  │
                    Mode = "atomic" or "partial"?
                                  │
         ┌────────────────────────┴────────────────────────┐
         ▼                                                 ▼
┌─────────────────────────────────┐       ┌──────────────────────────────────┐
│ Atomic Mode (All-or-Nothing)    │       │ Partial Mode (Partitioned Bulk)  │
├─────────────────────────────────┤       ├──────────────────────────────────┤
│ • Single DB Transaction         │       │ • Worker Pool (4 goroutines)     │
│ • Pessimistic row locking       │       │ • Chunk Size: 50 items/chunk     │
│ • Pre-validates ALL transitions │       │ • Sub-transactions per chunk     │
│ • ANY failure -> Full Rollback  │       │ • Aggregates per-card outcomes   │
│ • Use Case: Regulatory freeze   │       │ • Use Case: Mass issuance/expire │
└─────────────────────────────────┘       └──────────────────────────────────┘
```

- **Atomic Mode**:
  - Ideal for regulatory or sanctions compliance where a tenant's portfolio or account subset must transition as a single atomic unit.
  - Phase 1 pre-validates state transitions for all records. If a single card cannot transition (e.g., attempting to reactivate an already `CLOSED` card), the transaction aborts and rolls back completely.
- **Partial Mode**:
  - High-throughput mode designed for thousands of records.
  - Batches are sliced into chunks (default: 50 items) distributed across a bounded worker pool.
  - Each chunk executes in an isolated database transaction. If Card #34 fails validation, it does not prevent Cards #35-50 from succeeding. The API returns a detailed per-item execution ledger (`results` array with individual `status` and `error` codes).

#### 3. Optimistic Concurrency Control (OCC)
- Every status update checks and increments an integer `version` column:
  ```sql
  UPDATE cards 
  SET status = $1, version = version + 1, updated_at = NOW() 
  WHERE tenant_id = $2 AND id = $3 AND version = $4;
  ```
  If `RowsAffected == 0`, a concurrent worker or manual operator updated the card concurrently. The system aborts the stale operation with `ErrConflict` (`409 Conflict`), preventing silent overwrites (lost update anomaly).

---

## 3. Observed Security & Compliance Gaps & Production Mitigations

In an interview or audit context, recognizing the gaps between an exercise implementation and a multi-region PCI-DSS certified production system is vital:

| Observed Gap in Codebase | Threat / Compliance Impact | Production Mitigation Strategy |
|--------------------------|----------------------------|--------------------------------|
| **Local Hex Encryption Keys** | Environment variables on the host could leak via core dumps or process inspection. | Integrate with **AWS KMS / HashiCorp Vault Transit Engine / Cloud KMS** using Envelope Encryption (KEK wraps DEK, keys rotated every 90 days). |
| **Volatile Go GC Memory** | Go's garbage collector moves byte slices; `Zeroize()` clears the allocated slice, but GC copies may linger in unmanaged process memory. | Use Go `mlock` syscalls or CGO wrappers (`libsodium` / `memguard`) to lock cardholder data pages and guarantee zeroization in physical RAM. |
| **In-Cluster Database Audit Log** | A compromised database superuser could theoretically modify or truncate `audit_events`. | Replicate audit trails asynchronously to a **Write-Once-Read-Many (WORM)** tamper-proof store (e.g., AWS S3 Object Lock in Compliance Mode) and stream to a SIEM (Datadog/Splunk) with cryptographic checksum hashing. |
| **API Key Authentication via Table Scans** | Iterating through tenant bcrypt hashes creates $O(N)$ CPU latency under high tenant volume. | Prefix API keys with a high-entropy public routing ID (e.g., `np_live_<tenant_prefix>_<secret>`) to enable $O(1)$ key lookups before verifying the cryptographic hash. |
| **Missing Network Isolation / mTLS** | Network snooping between API ingress and the application container. | Enforce mutual TLS (mTLS) with client certificate verification at the ingress layer; cardholder data never travels over unencrypted internal networks. |
| **CVV/CVC Ingestion Risk** | Even temporary ingestion of CVV post-authorization violates PCI-DSS Requirement 3.2. | Zeroize CVV in memory immediately upon receiving issuance response from card schemes (Visa/Mastercard); never define CVV fields in database DDL or persistable entity structs. |

---

## 4. State Transition Invariants

```
               ┌───────────┐
               │  PENDING  │
               └─────┬─────┘
                     │ Issue()
                     ▼
  ┌────────────▶  ACTIVE   ────────────┐
  │              │     ▲               │
  │ Reactivate() │     │ Suspend()     │ Close()
  │              ▼     │               │
  └───────────  SUSPENDED              │
                 │                     │
                 │ Close()             │
                 ▼                     ▼
               ┌───────────────────────┐
               │        CLOSED         │  <-- TERMINAL STATE
               └───────────────────────┘
```

- **Invariant 1**: Transitions into `CLOSED` are irreversible. No card in `CLOSED` status can be suspended, reactivated, or re-issued.
- **Invariant 2**: A card in `PENDING` status cannot be suspended or reactivated without first being issued.
- **Invariant 3**: State changes cannot bypass audit logging; domain events are committed within the same database transaction as the status mutation.
