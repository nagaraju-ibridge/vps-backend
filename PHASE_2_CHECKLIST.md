# Phase 2 Implementation Checklist — Server & Telemetry Management

This document tracks progress and specifications for Phase 2 of the VPS Monitoring platform.

---

## 🏁 Phase Status Summary

| Phase | Description | Status |
| :--- | :--- | :---: |
| **Phase 1** | Authentication & Role-Based Access Control | ✅ DONE |
| **Phase 2A** | Server Management & Ownership Security | ✅ DONE |
| **Phase 2B** | Installation Token Management | ✅ DONE |
| **Phase 2C** | Agent Registration & Telemetry Ingestion | ✅ DONE |
| ↳ 2C.1 | Agent Registration | ✅ DONE |
| ↳ 2C.2 | Agent Authentication | ✅ DONE |
| ↳ 2C.3 | Heartbeat API | ✅ DONE |
| ↳ 2C.4 | Offline Detection | ✅ DONE |
| ↳ 2C.5 | Go Agent Core | ✅ DONE |
| ↳ 2C.6 | Basic Metric Collection | ✅ DONE |
| ↳ 2C.7 | Metric Ingestion | ✅ DONE |
| ↳ 2C.8 | Agent → Backend Integration | ✅ DONE |
| ↳ 2C.9 | Real VPS Testing | ✅ DONE |
| ↳ 2C.10 | Security Review | ✅ DONE |

---

## 📋 Phase 2A — Server Management (MVP Lifecycle & Ownership)

### Architecture & Module Setup
- [x] **2A.1 Server Module Directory Structure**
  - [x] `internal/server/models/` — Data model definition
  - [x] `internal/server/dto/` — Request and response contracts
  - [x] `internal/server/repository/` — Database access with GORM
  - [x] `internal/server/service/` — Business logic and ownership verification
  - [x] `internal/server/controller/` — HTTP routing & handler layer
  - [x] Preserved `internal/auth/` without breaking or altering existing auth logic

### Database & Schema
- [x] **2A.2 Database Migration**
  - [x] Migration file: `migrations/000002_create_servers_table.up.sql`
  - [x] Down migration: `migrations/000002_create_servers_table.down.sql`
  - [x] Columns created: `id`, `user_id`, `name`, `hostname`, `ip_address`, `os`, `architecture`, `agent_id`, `agent_status`, `last_seen`, `created_at`, `updated_at`, `deleted_at`
  - [x] Foreign key constraint linking `user_id` to `users(id)` with cascading delete
  - [x] Indexes on `user_id`, `agent_id`, and `deleted_at`
- [x] **2A.3 GORM Server Model**
  - [x] Defined in `internal/server/models/server.go`
  - [x] Table name mapped to `servers`
  - [x] Nullable fields for agent registration (`hostname`, `ip_address`, `os`, `architecture`, `agent_id`, `last_seen`)
  - [x] Soft delete enabled via `gorm.DeletedAt`

### Persistence & Business Logic
- [x] **2A.4 Server Repository**
  - [x] `Create(ctx, server)`
  - [x] `GetByID(ctx, id)`
  - [x] `GetByIDAndUserID(ctx, id, userID)`
  - [x] `GetByUserID(ctx, userID)`
  - [x] `Update(ctx, server)`
  - [x] `Delete(ctx, id, userID)` (GORM soft delete)
  - [x] Migration files own the database schema; GORM handles models/queries
- [x] **2A.5 Server Service**
  - [x] `CreateServer(ctx, userID, req)`
  - [x] `GetUserServers(ctx, userID)`
  - [x] `GetServer(ctx, id, userID)`
  - [x] `UpdateServer(ctx, id, userID, req)`
  - [x] `DeleteServer(ctx, id, userID)`
  - [x] Default status on creation = `PENDING`
  - [x] Strict user ownership rule: User A cannot view, update, or delete User B's servers

### API Contracts & Security
- [x] **2A.6 DTOs & Payloads**
  - [x] `CreateServerRequest` strictly accepts `{ "name": "..." }` (no `user_id` allowed in payload)
  - [x] `UpdateServerRequest` strictly accepts `{ "name": "..." }` (prevents altering telemetry or identity)
  - [x] `ServerResponse` cleanly formats all fields including null values
- [x] **2A.7 REST API Endpoints**
  - [x] `GET /api/v1/user/servers` — List user's servers
  - [x] `POST /api/v1/user/servers` — Create server
  - [x] `GET /api/v1/user/servers/{id}` — Get single server
  - [x] `PUT /api/v1/user/servers/{id}` — Rename server
  - [x] `DELETE /api/v1/user/servers/{id}` — Soft delete server
- [x] **2A.8 Ownership Security Enforcement**
  - [x] Enforces `WHERE id = ? AND user_id = ?` across all single-server operations
  - [x] Returns `404 / server not found` or `403` if cross-tenant access is attempted
- [x] **2A.9 Initial Status**
  - [x] New servers always start with status `PENDING`
- [x] **2A.10 Soft Delete**
  - [x] Sets `deleted_at` timestamp rather than hard deletion
  - [x] Standard queries automatically exclude soft-deleted records
- [x] **2A.11 Input Validation**
  - [x] Server name is required, trimmed, and length checked (2 to 100 chars)
  - [x] Server ID must be a positive integer
  - [x] JWT claims extracted and verified via `middleware.GetClaims`
- [x] **2A.12 Automated Testing**
  - [x] Comprehensive test suite for server CRUD, ownership validation, soft delete, and auth

### Frontend Integration
- [x] **2A.13 Next.js Frontend Integration**
  - [x] TypeScript interfaces defined in `src/types/server.ts`
  - [x] Real-time fleet retrieval (`GET /api/v1/user/servers`) on User Dashboard (`src/app/dashboard/page.tsx`)
  - [x] Modal for new server creation (`POST /api/v1/user/servers`) with `CreateServerModal.tsx`
  - [x] Modal for renaming servers (`PUT /api/v1/user/servers/{id}`) with `EditServerModal.tsx`
  - [x] Modal for deleting servers (`DELETE /api/v1/user/servers/{id}`) with `DeleteServerModal.tsx`
  - [x] Server card status indicators (`PENDING` with amber badge, `ONLINE` with emerald badge) in `ServerNodeCard.tsx`
  - [x] Interactive node registration directly on Connect page (`src/app/dashboard/connect/page.tsx`)
  - [x] Empty state illustration with quick action for onboarding first server
  - [x] Full production build verified (`npm run build` passed)

---

## 📋 Phase 2B — Installation Token Management (Completed)

- [x] **2B.1 Installation Token Model & Migration**
  - [x] Migration file: `migrations/000003_create_installation_tokens_table.up.sql`
  - [x] Down migration: `migrations/000003_create_installation_tokens_table.down.sql`
  - [x] Model defined in `internal/installation_token/models/installation_token.go`
  - [x] Columns: `id`, `server_id`, `token_hash`, `expires_at`, `is_used`, `used_at`, `created_at`
  - [x] Relational integrity: Foreign key constraint to `servers(id)` with cascading delete
  - [x] Performance indexes: on `token_hash`, `server_id`, and `expires_at`
  - [x] Security design: Raw token is never stored in DB; only cryptographic hash (`token_hash`)
  - [x] Helper validation methods: `IsExpired()` and `IsValid()` with unit tests passing
- [x] **2B.2 Token Repository & Service (Generation & Hashing)**
  - [x] Cryptographically secure random token generation (32-byte / 64-char hex string)
  - [x] SHA-256 token hashing prior to persistence
  - [x] Default expiration policy (1 hour lifetime)
  - [x] Ownership guard: user can only issue token for their own server (`WHERE id = ? AND user_id = ?`)
  - [x] Database sees only `token_hash`, never raw secret
- [x] **2B.3 Token Verification & Script Delivery (Controller & API)**
  - [x] `POST /api/v1/user/servers/{id}/tokens` — Generate installation token & install script
  - [x] `GET /api/v1/user/servers/{id}/tokens` — Retrieve active installation token status (metadata only)
  - [x] Dynamic curl command generation (`curl -sSL https://get.vpspulse.dev/agent.sh | bash -s -- --token <RAW_TOKEN>`)
  - [x] Token consumption and verification logic prepared for Phase 2C (`ValidateToken` checks hash, expiry, usage)
  - [x] Comprehensive unit tests for token generation, hashing, ownership, status, and validation
- [x] **2B.4 Frontend Integration for Installation Tokens & Admin APIs**
  - [x] On-demand token generation and status inspection modal (`InstallTokenModal.tsx`) calling `POST /api/v1/user/servers/{id}/tokens` and `GET /api/v1/user/servers/{id}/tokens`
  - [x] Automatic token generation upon node creation in Connect page (`src/app/dashboard/connect/page.tsx`)
  - [x] Quick token access button on every server card (`ServerNodeCard.tsx`)
  - [x] Admin profile modification modal (`EditAdminModal.tsx`) integrated with `PUT /api/v1/admin/admins/{id}` in Admin panel and Admin Management page
  - [x] Verified full Next.js production build (`npm run build` compiled 16/16 routes with 0 errors)

---

## 📋 Phase 2C — Agent Registration & Telemetry Ingestion

### 2C.1 — Agent Registration (Completed)
- [x] Create Agent module (`internal/agent/`)
- [x] Create Agent model (`internal/agent/models/agent.go`)
- [x] Create agents migration (`migrations/000004_create_agents_table.up.sql` & `.down.sql`)
- [x] Create Agent repository with database transaction (`internal/agent/repository/agent_repository.go`)
- [x] Create Agent service (`internal/agent/service/agent_service.go`)
- [x] Create Agent DTOs (`internal/agent/dto/agent_dto.go`)
- [x] Create registration controller (`internal/agent/controller/agent_controller.go`)
- [x] Create `POST /api/v1/agent/register`
- [x] Validate installation token (exists, unexpired, unused)
- [x] Check token expiration
- [x] Check token `is_used`
- [x] Find associated server (verifying server exists & not deleted)
- [x] Create agent record
- [x] Generate permanent `agent_id` (UUID v4)
- [x] Generate agent credential (32-byte cryptographically secure random token)
- [x] Securely store agent credential (SHA-256 hash; raw credential never stored)
- [x] Mark installation token as used (`is_used = true`)
- [x] Set `used_at = NOW()`
- [x] Update server `hostname`
- [x] Update server `ip_address`
- [x] Update server `os`
- [x] Update server `architecture`
- [x] Transition server `PENDING` → `ONLINE`
- [x] Return registration response once with raw credential
- [x] Single database transaction wrapping agent creation, server update, and token consumption (atomic rollback on error)
- [x] Comprehensive unit tests covering registration, token checks, rollback, and duplicate handling

### 2C.2 — Agent Authentication (Completed)
- [x] Define agent credential format (`Authorization: Bearer <AGENT_CREDENTIAL>`)
- [x] Create Agent authentication middleware (`internal/agent/middleware/agent_auth.go`)
- [x] Validate agent credential via SHA-256 hash lookup in database
- [x] Identify agent and associate server
- [x] Store typed agent identity (`AgentIdentity`) in request context
- [x] Protect agent-only endpoints
- [x] Reject invalid agent credentials without disclosing enumeration hints (HTTP 401)
- [x] Reject revoked/invalid agents (`status != ACTIVE`)
- [x] Keep Agent auth separate from User JWT auth
- [x] Add authentication tests (unit, middleware, credential isolation)

### 2C.3 — Heartbeat API (Completed)
- [x] Create heartbeat DTO (`HeartbeatRequest`, `HeartbeatResponse` in `dto/agent_dto.go`)
- [x] Create heartbeat service (`Heartbeat` in `service/agent_service.go`)
- [x] Create heartbeat controller (`Heartbeat` in `controller/agent_controller.go`)
- [x] Create `POST /api/v1/agent/heartbeat` (registered in `cmd/vpsmonitoring/main.go`)
- [x] Authenticate agent via `AgentAuthMiddleware` (`Authorization: Bearer <AGENT_CREDENTIAL>`)
- [x] Update `agent.last_seen` using authoritative backend UTC timestamp
- [x] Update `server.last_seen` using authoritative backend UTC timestamp
- [x] Keep/set server `agent_status = ONLINE`
- [x] Validate heartbeat payload (optional RFC3339 timestamp format validation, agent version update)
- [x] Add heartbeat tests (service unit tests, controller handler pipeline tests, isolation tests)

### 2C.4 — Offline Detection (Completed)
- [x] Define heartbeat interval (`DefaultHeartbeatInterval = 30s`)
- [x] Define offline threshold (`DefaultOfflineThreshold = 90s`, configurable via `AGENT_OFFLINE_THRESHOLD_SECONDS`)
- [x] Create backend background worker (`internal/agent/worker/offline_worker.go`)
- [x] Periodically check `last_seen` (check interval 30s, configurable via `AGENT_WORKER_INTERVAL_SECONDS`)
- [x] Detect stale agents (`last_seen < UTC_NOW - threshold`)
- [x] Change `ONLINE` → `OFFLINE` directly at database level (`MarkStaleServersOffline`)
- [x] Avoid unnecessary repeated updates (`WHERE agent_status = 'ONLINE'`)
- [x] Handle application startup/shutdown cleanly with `context.Context` & `sync.WaitGroup`
- [x] Test offline transition (boundary, stale, deleted servers, multiple agents)
- [x] Test recovery `OFFLINE` → `ONLINE` via existing 2C.3 Heartbeat API

### 🖥️ 2C.5 — Go Agent Core (Completed)
- [x] Create/complete Go agent project under `agent/`:
  - [x] `cmd/agent/main.go`
  - [x] `internal/config/`
  - [x] `internal/client/`
  - [x] `internal/registration/`
  - [x] `internal/heartbeat/`
  - [x] `go.mod`
  - [x] `README.md`
- [x] Initialize Go Agent project (`vpsmonitoring-agent`)
- [x] Agent configuration (`internal/config/config.go`)
- [x] Backend URL configuration (`VPSMONITOR_BACKEND_URL`)
- [x] Installation token configuration (`VPSMONITOR_INSTALLATION_TOKEN`)
- [x] Agent credential configuration (`VPSMONITOR_AGENT_CREDENTIAL`)
- [x] HTTP client (`internal/client/client.go`)
- [x] Registration client (`internal/registration/registration.go`)
- [x] Credential persistence (`0600` permissions on state file `agent.json`)
- [x] Heartbeat client (`internal/heartbeat/heartbeat.go`)
- [x] Config loading (from environment & persistent state file)
- [x] Graceful shutdown (`signal.NotifyContext` with `SIGINT`, `SIGTERM` and `runner.Stop()`)
- [x] Logging (clear, structured, non-sensitive)
- [x] Agent unit tests (`config_test.go`, `registration_test.go`, `heartbeat_test.go`)

### 📊 2C.6 — Basic Metric Collection (Completed)
- [x] **CPU**
  - [x] CPU usage
  - [x] CPU cores
  - [x] Load 1/5/15
- [x] **Memory**
  - [x] Total
  - [x] Used
  - [x] Available
  - [x] Usage %
- [x] **Swap**
  - [x] Total
  - [x] Used
  - [x] Free
  - [x] Usage %
- [x] **Disk**
  - [x] Total
  - [x] Used
  - [x] Free
  - [x] Usage %
  - [x] Mount points
- [x] **Network**
  - [x] RX bytes
  - [x] TX bytes
  - [x] RX packets
  - [x] TX packets
  - [x] Errors
  - [x] Drops

### 📡 2C.7 — Metric Ingestion (Completed)
- [x] Create metrics DTO
- [x] Create metric model
- [x] Create metric migration
- [x] Create metric repository
- [x] Create metric service
- [x] Create `POST /api/v1/agent/metrics`
- [x] Agent authentication
- [x] Validate metric payload
- [x] Associate metrics with server
- [x] Store timestamp
- [x] Store CPU metrics
- [x] Store memory metrics
- [x] Store swap metrics
- [x] Store disk metrics
- [x] Store network metrics
- [x] Add ingestion tests

### 🔄 2C.8 — Agent → Backend Integration (Completed)
- [x] Real Go Agent registration
- [x] Real agent credential exchange
- [x] Real heartbeat
- [x] Server becomes `ONLINE`
- [x] Metrics reach backend
- [x] Metrics stored in PostgreSQL
- [x] Agent restart recovery
- [x] Backend restart recovery
- [x] Invalid credential handling
- [x] Expired installation token handling

### 🧪 2C.9 — Real VPS Testing (Completed)
- [x] Build agent binary
- [x] Run on real Ubuntu VPS
- [x] Register using installation token
- [x] Verify agent credential
- [x] Verify heartbeat
- [x] Verify `ONLINE` status
- [x] Stop agent
- [x] Verify `OFFLINE` status
- [x] Restart agent
- [x] Verify `ONLINE` recovery
- [x] Verify CPU metrics collection/submission
- [x] Verify RAM metrics collection/submission
- [x] Verify disk metrics collection/submission
- [x] Verify network metrics collection/submission

### 🔐 2C.10 — Security Review (Completed)
- [x] Installation token is single-use
- [x] Installation token expires
- [x] Raw installation token isn't stored
- [x] Agent credential isn't exposed to frontend
- [x] Agent credential isn't logged
- [x] Agent endpoints don't accept user JWT
- [x] User endpoints don't accept agent credential
- [x] User ownership remains enforced
- [x] Agent can only access its own server
- [x] HTTPS used for production
- [x] No arbitrary command execution
- [x] No SSH key collection
- [x] No `.env` collection
- [x] No database-password collection
- [x] No source-code collection

