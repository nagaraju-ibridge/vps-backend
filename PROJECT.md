# VPSPulse Backend — Project Documentation

> **For AI Agents**: This document fully describes the backend codebase, architecture, all API routes, database schema, middleware, and security model. Read before making any changes.

---

## 1. Project Overview

The **VPSPulse Backend** (`vpsmonitoring-backend`) is a Go microservice built with the **GoFr framework** (v1.31). It powers the entire VPSPulse monitoring platform and serves:

- **User/Admin Portal**: JWT-authenticated REST API for user management, server registration, and token issuance
- **Agent API**: Credential-authenticated endpoints for agent registration, heartbeat, and metric ingestion
- **Health API**: Public database diagnostics endpoint

### Tech Stack
- **Language**: Go 1.23
- **Framework**: GoFr (gofr.dev) — provides routing, middleware, structured logging, metrics
- **ORM**: GORM v1.31.2 with PostgreSQL dialect
- **Database**: Neon PostgreSQL (serverless, connection pooling via pgxpool)
- **Auth**: JWT (HS256) for users/admins; bcrypt-hashed bearer tokens for agents
- **Config**: Environment variables via `.env` file in `configs/`

---

## 2. Directory Structure

```
vpsmonitoring-backend/
├── cmd/
│   └── vpsmonitoring/
│       └── main.go                    # Application entry point — wires all layers and registers routes
├── configs/
│   └── .env                           # Runtime configuration (DATABASE_URL, JWT_SECRET, PORT, etc.)
├── internal/
│   ├── config/
│   │   ├── config.go                  # LoadConfig: validates required secrets in production
│   │   └── config_test.go
│   ├── auth/
│   │   ├── models/user.go             # User model (id, name, email, password_hash, role, status, timestamps)
│   │   ├── repository/
│   │   │   ├── user_repository.go     # GORM user CRUD + AutoMigrate
│   │   │   └── gorm_init.go           # InitGORM: connects to PostgreSQL + AutoMigrate users table
│   │   ├── service/
│   │   │   └── auth_service.go        # AuthService: Login, Register, GetProfile, admin operations, JWT
│   │   ├── controller/
│   │   │   └── auth_controller.go     # HTTP handlers: Login, Register, GetProfile, admin CRUD
│   │   ├── middleware/
│   │   │   └── jwt_middleware.go      # JWTRoleMiddleware: validates JWT, injects Claims into context
│   │   └── dto/
│   │       └── auth_dto.go            # Request/Response DTOs for auth endpoints
│   ├── server/
│   │   ├── models/server.go           # Server model (id, user_id, name, hostname, ip, os, arch, agent_id, status, timestamps)
│   │   ├── repository/
│   │   │   └── server_repository.go   # GORM server CRUD + AutoMigrate
│   │   ├── service/
│   │   │   └── server_service.go      # Business logic: create, list, get, rename, delete (ownership enforced)
│   │   ├── controller/
│   │   │   └── server_controller.go   # HTTP handlers for server management routes
│   │   └── dto/
│   │       └── server_dto.go          # CreateServerRequest, UpdateServerRequest, ServerResponse
│   ├── installation_token/
│   │   ├── models/
│   │   │   └── installation_token.go  # Token model (id, server_id, token_hash, expires_at, used, used_at)
│   │   ├── repository/
│   │   │   └── installation_token_repository.go  # GORM token CRUD + AutoMigrate
│   │   ├── service/
│   │   │   └── installation_token_service.go     # Token gen (crypto/rand 32B), SHA-256 hash, install_command
│   │   └── controller/
│   │       └── installation_token_controller.go  # HTTP handlers: CreateToken, GetTokenStatus
│   ├── agent/
│   │   ├── models/agent.go            # Agent model (id UUID, server_id, credential_hash, last_seen, status)
│   │   ├── repository/
│   │   │   └── agent_repository.go    # GORM agent CRUD + AutoMigrate, offline detection query
│   │   ├── service/
│   │   │   └── agent_service.go       # RegisterAgent (validate token + create agent), Heartbeat, ValidateCredential
│   │   ├── controller/
│   │   │   └── agent_controller.go    # HTTP handlers: Register, Heartbeat
│   │   ├── middleware/
│   │   │   └── agent_auth_middleware.go  # AgentAuthMiddleware: validates Bearer agent credential
│   │   └── worker/
│   │       └── offline_worker.go      # Background goroutine: marks agents OFFLINE if no heartbeat within threshold
│   ├── metric/
│   │   ├── models/
│   │   │   ├── metric.go              # Metric model (UUID, server_id, agent_id, all telemetry fields)
│   │   │   └── metric_disk.go         # MetricDisk model (per mount point disk stats, FK→metrics)
│   │   ├── repository/
│   │   │   └── metric_repository.go   # GORM metric CRUD + AutoMigrate
│   │   ├── service/
│   │   │   └── metric_service.go      # MetricService: validates + stores incoming metric snapshot
│   │   └── controller/
│   │       └── metric_controller.go   # HTTP handler: Ingest (POST /api/v1/agent/metrics)
│   └── integration_test/
│       └── *_test.go                  # End-to-end integration tests
├── migrations/
│   ├── 000001_create_users_table.up.sql
│   ├── 000002_create_servers_table.up.sql
│   └── *.down.sql
├── PHASE_2_CHECKLIST.md               # Phase-by-phase implementation checklist
├── AUTH_DOCUMENTATION.md              # Detailed auth system documentation
├── go.mod                             # Module: vpsmonitoring-backend, Go 1.23
└── go.sum
```

---

## 3. Architecture Pattern

Every domain follows a strict **3-layer architecture**:

```
HTTP Request
    ↓
Controller   — Validates path params, binds request body, extracts JWT claims
    ↓
Service      — Business logic, ownership checks, domain errors
    ↓
Repository   — GORM database operations (no business logic here)
    ↓
PostgreSQL (Neon)
```

**No cross-domain imports** — each domain (auth, server, token, agent, metric) is self-contained.

---

## 4. All API Routes

### 4.1 Public Health Endpoints
| Method | Route | Handler | Description |
|---|---|---|---|
| GET | `/check-health` | inline | DB ping + latency + version |
| GET | `/health` | inline | Same as above |
| GET | `/api/db-status` | inline | Same as above |

### 4.2 Authentication (Public)
| Method | Route | Handler | Description |
|---|---|---|---|
| POST | `/api/v1/auth/login` | `authController.Login` | Login with email + password, returns JWT |
| POST | `/api/v1/auth/register` | `authController.Register` | Self-register new user account |

### 4.3 Admin Routes (JWT required, ADMIN role)
| Method | Route | Handler | Description |
|---|---|---|---|
| GET | `/api/v1/admin/admins` | `authController.GetAdmins` | List all admin accounts |
| POST | `/api/v1/admin/admins` | `authController.CreateAdmin` | Create a new admin account |
| PUT | `/api/v1/admin/admins/{id}` | `authController.UpdateAdmin` | Update admin name/email/password |
| PATCH | `/api/v1/admin/admins/{id}/status` | `authController.UpdateStatus` | Toggle ACTIVE/INACTIVE status |

### 4.4 User Routes (JWT required, USER or ADMIN role)
| Method | Route | Handler | Description |
|---|---|---|---|
| GET | `/api/v1/user/profile` | `authController.GetProfile` | Get authenticated user's profile |
| GET | `/api/v1/user/servers` | `serverCtrl.ListServers` | List all servers owned by user |
| POST | `/api/v1/user/servers` | `serverCtrl.CreateServer` | Register new server (status: PENDING) |
| GET | `/api/v1/user/servers/{id}` | `serverCtrl.GetServer` | Get single server detail with agent metadata |
| PUT | `/api/v1/user/servers/{id}` | `serverCtrl.UpdateServer` | Rename server (name only) |
| DELETE | `/api/v1/user/servers/{id}` | `serverCtrl.DeleteServer` | Soft-delete server |
| POST | `/api/v1/user/servers/{id}/tokens` | `tokenCtrl.CreateToken` | Generate single-use installation token |
| GET | `/api/v1/user/servers/{id}/tokens` | `tokenCtrl.GetTokenStatus` | Check if active token exists |

### 4.5 Agent Routes (Agent credential required)
| Method | Route | Handler | Description |
|---|---|---|---|
| POST | `/api/v1/agent/register` | `agentCtrl.Register` | First-time agent registration with installation token |
| POST | `/api/v1/agent/heartbeat` | `agentCtrl.Heartbeat` | Periodic liveness ping (updates last_seen, status→ONLINE) |
| POST | `/api/v1/agent/metrics` | `metricCtrl.Ingest` | Submit system metric snapshot |

---

## 5. Database Schema

### users
| Column | Type | Notes |
|---|---|---|
| id | BIGSERIAL PK | |
| name | VARCHAR(255) | |
| email | VARCHAR(255) UNIQUE | |
| password_hash | TEXT | bcrypt hashed |
| role | VARCHAR(50) | `USER` or `ADMIN` |
| status | VARCHAR(50) | `ACTIVE` or `INACTIVE` |
| created_at, updated_at | TIMESTAMP | GORM managed |
| deleted_at | TIMESTAMP | Soft delete |

### servers
| Column | Type | Notes |
|---|---|---|
| id | BIGSERIAL PK | |
| user_id | BIGINT FK→users | Indexed |
| name | VARCHAR(255) | User-defined label |
| hostname | VARCHAR(255) | Set by agent on registration |
| ip_address | VARCHAR(100) | Set by agent on registration |
| os | VARCHAR(100) | Set by agent on registration |
| architecture | VARCHAR(50) | Set by agent on registration |
| agent_id | VARCHAR(100) | Set after agent registers |
| agent_status | VARCHAR(50) | `PENDING` → `ONLINE` → `OFFLINE` |
| last_seen | TIMESTAMP | Updated on every heartbeat |
| created_at, updated_at, deleted_at | TIMESTAMP | GORM managed |

### installation_tokens
| Column | Type | Notes |
|---|---|---|
| id | BIGSERIAL PK | |
| server_id | BIGINT FK→servers | Indexed |
| token_hash | VARCHAR(64) | SHA-256 hex of raw token |
| expires_at | TIMESTAMP | 1 hour from creation |
| used | BOOLEAN | false until consumed |
| used_at | TIMESTAMP | Set when consumed |
| created_at, updated_at | TIMESTAMP | GORM managed |

### agents
| Column | Type | Notes |
|---|---|---|
| id | UUID PK | Generated on registration |
| server_id | BIGINT FK→servers | Unique — one agent per server |
| credential_hash | TEXT | bcrypt hash of bearer credential |
| last_seen | TIMESTAMP | Updated on heartbeat |
| status | VARCHAR(50) | `ONLINE` or `OFFLINE` |
| created_at, updated_at | TIMESTAMP | GORM managed |

### metrics
| Column | Type | Notes |
|---|---|---|
| id | UUID PK | |
| server_id | BIGINT FK→servers | Indexed |
| agent_id | VARCHAR(100) | Indexed |
| collected_at | TIMESTAMP | When agent collected the snapshot |
| received_at | TIMESTAMP | When backend received it |
| cpu_usage_percent | DOUBLE | |
| cpu_cores | INTEGER | |
| load_1, load_5, load_15 | DOUBLE | 1/5/15-min load averages |
| memory_total, memory_used, memory_available | BIGINT | Bytes |
| memory_usage_percent | DOUBLE | |
| swap_total, swap_used, swap_free | BIGINT | Bytes |
| swap_usage_percent | DOUBLE | |
| network_rx_bytes, network_tx_bytes | BIGINT | |
| network_rx_packets, network_tx_packets | BIGINT | |
| network_errors, network_drops | BIGINT | |
| created_at | TIMESTAMP | |

### metric_disks
| Column | Type | Notes |
|---|---|---|
| id | UUID PK | |
| metric_id | UUID FK→metrics CASCADE DELETE | |
| mount_point | VARCHAR(255) | |
| filesystem | VARCHAR(100) | |
| total, used, free | BIGINT | Bytes |
| usage_percent | DOUBLE | |
| created_at | TIMESTAMP | |

---

## 6. Authentication System

### User JWT Auth
- **Algorithm**: HS256
- **Secret**: `JWT_SECRET` env var (required in production; dev falls back to `dev-secret-change-in-production`)
- **Claims**: `user_id`, `email`, `role`, `exp` (24h expiry)
- **Middleware**: `JWTRoleMiddleware` — runs on ALL routes, injects claims into context via `middleware.GetClaims(ctx)`
- **Role enforcement**: Controllers check `claims.Role` and reject unauthorized roles

### Agent Bearer Auth
- **Format**: `Authorization: Bearer <raw_credential>`
- **Storage**: bcrypt hash stored in `agents.credential_hash`
- **Middleware**: `AgentAuthMiddleware` — validates credential against stored hash, injects agent context
- **Isolation**: Agent routes reject JWT tokens; user routes reject agent credentials

---

## 7. Installation Token Flow

```
1. User calls POST /api/v1/user/servers/{id}/tokens (JWT auth)
2. Backend:
   a. Invalidates any existing active token for this server
   b. Generates 32 random bytes via crypto/rand
   c. Computes SHA-256 hex hash → stores as token_hash
   d. Sets expires_at = now + 1 hour
   e. Returns raw token + install_command to user (ONLY TIME raw token is visible)

3. install_command format:
   VPSMONITOR_BACKEND_URL=https://... VPSMONITOR_INSTALLATION_TOKEN=<raw> \
   bash <(curl -fsSL https://get.vpspulse.dev/agent.sh)

4. Agent reads installation_token from env, sends to POST /api/v1/agent/register
5. Backend:
   a. SHA-256 hashes the received token
   b. Looks up by hash, checks not expired, not used
   c. Creates agent record with bcrypt credential
   d. Marks token as used
   e. Returns credential (raw, one-time)

6. Agent saves credential to /var/lib/vpsmonitoring-agent/agent.json
```

---

## 8. Offline Detection Worker

`internal/agent/worker/offline_worker.go`:
- Runs as background goroutine started in `main.go`
- Interval: `AGENT_WORKER_INTERVAL_SECONDS` (default: 30s)
- Threshold: `AGENT_OFFLINE_THRESHOLD_SECONDS` (default: 90s)
- Query: finds all ONLINE agents where `last_seen < now - threshold`
- Action: bulk-updates their status to `OFFLINE`

---

## 9. Configuration

### Environment Variables (`configs/.env`)

| Variable | Required | Default (dev) | Description |
|---|---|---|---|
| `DATABASE_URL` | Yes (prod) | dev fallback removed | Neon PostgreSQL connection string |
| `JWT_SECRET` | Yes (prod) | dev fallback exists | HS256 signing secret |
| `ENV` | No | `development` | Set to `production` to enforce strict validation |
| `PORT` | No | `8080` | HTTP listen port |
| `AGENT_OFFLINE_THRESHOLD_SECONDS` | No | `90` | Seconds before agent marked OFFLINE |
| `AGENT_WORKER_INTERVAL_SECONDS` | No | `30` | Offline detection worker tick interval |
| `GOFR_TELEMETRY` | No | `true` | Set to `false` to disable GoFr telemetry |

### Production Validation (`internal/config/config.go`)
When `ENV=production`: both `DATABASE_URL` and `JWT_SECRET` must be set; startup fails with fatal error if missing.

---

## 10. Build & Run

```bash
# Build
go build -o vpsmonitoring.exe ./cmd/vpsmonitoring

# Run
./vpsmonitoring.exe

# Format & Test
go fmt ./...
go test -count=1 ./...
```

---

## 11. Security Hardening Applied

1. Installation token: single-use + expiry enforced at service layer
2. Raw token never stored (SHA-256 hash only)
3. Agent credential: bcrypt hashed (never stored raw)
4. Agent credential never returned in API responses after initial registration
5. Agent credential never logged
6. Agent endpoints reject user JWT (different middleware path)
7. User endpoints reject agent credentials
8. All server/token operations enforce user_id ownership (JWT user_id must match server.user_id)
9. No hardcoded secrets in production (config.LoadConfig enforces this)
