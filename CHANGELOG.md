# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- GitHub Actions CI workflow with lint, format, typecheck, tests, coverage, and integration jobs
- CHANGELOG.md (this file)

## [1.0.0] - 2026-09-21

### Added
- **Multi-tenant Architecture**: Hexagonal (Ports & Adapters) with interchangeable storage drivers
  - In-memory driver (zero dependencies, instant demo)
  - PostgreSQL driver (production, RLS enforced)
- **Domain Layer**: Pure Go entities with zero external dependencies
  - `Card` with PAN type (AES-256-GCM encrypted, masked in logs/JSON)
  - `Cardholder` entity with validation
  - Card lifecycle state machine: `PENDING` → `ISSUED` → `ACTIVE` ↔ `SUSPENDED` → `CLOSED`
  - Strict transition validation (no invalid state jumps)
  - Luhn algorithm for PAN generation/validation
- **PCI-DSS Compliant Cryptography**:
  - AES-256-GCM with random 12-byte nonce per record
  - Additional Authenticated Data (AAD) binding tenant_id to ciphertext
  - HMAC-SHA256 Blind Index for O(1) exact-match search without decryption
  - Memory zeroization of plaintext buffers
- **Batch Processing Engine**:
  - Atomic mode: all-or-nothing in single transaction
  - Partial mode: chunked worker pool with per-item results
  - Deadlock prevention via `ORDER BY id ASC FOR UPDATE` (deterministic lock ordering)
  - Optimistic Concurrency Control via `version` column
- **HTTP API** (stdlib net/http, Go 1.22+ ServeMux):
  - `POST   /v1/cardholders` - Create cardholder
  - `GET    /v1/cardholders/:id` - Get cardholder
  - `GET    /v1/cardholders` - List cardholders (paginated)
  - `PATCH  /v1/cardholders/:id` - Update cardholder
  - `POST   /v1/cards` - Request card (PENDING)
  - `GET    /v1/cards/:id` - Get card (masked PAN)
  - `GET    /v1/cards` - List cards (paginated, filterable)
  - `POST   /v1/cards/:id/issue` - Issue card (generates PAN, encrypts, ACTIVE)
  - `POST   /v1/cards/:id/suspend` - Suspend card
  - `POST   /v1/cards/:id/reactivate` - Reactivate card
  - `POST   /v1/cards/:id/close` - Close card (terminal)
  - `POST   /v1/batch/card-status` - Batch status update (atomic/partial)
  - `GET    /v1/batch/:id` - Get batch operation result
  - `GET    /health` / `GET /ready` - Health & readiness probes
- **Security Middleware**:
  - Bearer token authentication with tenant extraction
  - Mandatory tenant context enforcement (panic on missing)
  - Idempotency key support (`Idempotency-Key` header)
  - Request ID propagation
  - Panic recovery with structured logging
  - Audit logging of all mutating operations
- **Repository Layer**:
  - Parameterized queries throughout (zero string interpolation)
  - Row-Level Security policies in PostgreSQL migrations
  - Transaction manager with `WithTransaction(ctx, fn)` pattern
- **Database Migrations** (7 files):
  - Tenants, Cardholders, Cards, Audit Events, Batch Operations, Idempotency Keys
  - RLS policies forced on all tables with `app.current_tenant` session variable
- **Documentation**:
  - `README.md` - Project overview, quickstart, API examples
  - `RUNBOOK.md` - Detailed evaluation guide for interviewers
  - `DECISION_LOG_AND_TRADEOFFS.md` - ADRs, PCI rationale, deadlock proofs, gap analysis
  - `AI_INTERACTION_LOG.md` - AI usage record, overridden suggestions with justifications
- **Testing**:
  - Unit tests: crypto (88%), domain (63%), middleware (59%), service (68%)
  - Race detector clean on all packages
  - Concurrency tests for batch processing
- **Operational**:
  - Dockerfile (multi-stage, distroless-compatible)
  - docker-compose.yml (API + PostgreSQL 16)
  - Makefile (build, test, test-race, docker-up, migrate, generate-keys)
  - Structured JSON logging with PAN sanitization

### Security
- No plaintext PAN in logs, JSON, or memory after encryption
- Blind index uses separate key from encryption key
- Tenant isolation at application, query, and database levels
- Idempotency prevents duplicate financial operations

### Changed
- Go module updated to 1.25.0 with compatible golang.org/x/crypto v0.50.0

[Unreleased]: https://github.com/novopayment/card-issuer-api/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/novopayment/card-issuer-api/releases/tag/v1.0.0