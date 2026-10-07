# Backend Authentication & Authorization Documentation

This document provides a comprehensive technical overview and API reference for the authentication, authorization, and user management system in the `vpsmonitoring-backend` service.

---

## 1. What We Have Built (Implementation Overview)

We designed and implemented a production-grade, layered authentication and Role-Based Access Control (RBAC) architecture using **Go (Golang)**, the **GoFr microframework**, and **GORM** connected to **Neon Serverless PostgreSQL**.

### 1.1 Architecture & Layers

The authentication module is organized under `internal/auth/` adhering to Clean Architecture principles:

- **Entity / Model Layer (`internal/auth/models/user.go`)**:
  - Defines the core `User` model with fields `id`, `name`, `email`, `password_hash`, `role`, `status`, `created_at`, `updated_at`, and `deleted_at` (soft deletes).
  - Explicit roles: `ADMIN` and `USER`.
  - Account statuses: `ACTIVE` and `INACTIVE`.
  - Passwords are never serialized into JSON (`json:"-"`).

- **DTO Layer (`internal/auth/dto/auth_dto.go`)**:
  - Decouples API contracts from database schema.
  - Request DTOs: `LoginRequest`, `RegisterRequest`, `CreateAdminRequest`, `UpdateAdminRequest`, `UpdateStatusRequest`.
  - Response DTOs: `LoginResponse`, `UserResponse`.
  - Utility mapping function `ToUserResponse()` strips sensitive credentials before sending data to clients.

- **Repository Layer (`internal/auth/repository/`)**:
  - `InitGORM(dsn)`: Establishes a pooled PostgreSQL connection (`SetMaxIdleConns(5)`, `SetMaxOpenConns(30)`), applies Neon/PgBouncer compatibility (`PreferSimpleProtocol: true`), and safely handles unique constraints (`uni_users_email`) prior to executing `AutoMigrate`.
  - `UserRepository`: Provides abstracted database queries (`FindByEmail`, `FindByID`, `FindAllByRole`, `Create`, `Update`, `EmailExists`, `CountByRole`). All email queries are case-insensitive (`LOWER(email)`).

- **Service Layer (`internal/auth/service/auth_service.go`)**:
  - **Password Security**: Uses `golang.org/x/crypto/bcrypt` (default cost factor 10) for one-way password hashing and verification.
  - **JWT Token Management**: Generates and validates HS256 signed JSON Web Tokens using `github.com/golang-jwt/jwt/v5`.
    - Token Lifetime: **24 hours**.
    - Claims embedded: `user_id`, `email`, `role`, `name`, `iss`, `sub`, `exp`, `iat`, `nbf`.
  - **Business Rules Enforcement**:
    - Disallows inactive accounts (`INACTIVE`) from logging in.
    - Public registration is strictly restricted to the `USER` role.
    - Admin creation can only be performed by existing authenticated `ADMIN` users.
    - Admin status updates prevent an administrator from deactivating their own account (`ErrCannotSelfDeactivate`).
    - Enforces uniqueness of email across updates and creations.

- **Middleware Layer (`internal/auth/middleware/auth_middleware.go`)**:
  - **CORS Handling**: Injects headers (`Access-Control-Allow-Origin: *`, allowed methods, and allowed headers) and handles HTTP `OPTIONS` preflight requests.
  - **Route Protection & RBAC**:
    - Bypasses public endpoints: `/api/v1/auth/login`, `/api/v1/auth/register`, `/health`, `/check-health`, `/api/db-status`.
    - Enforces `Authorization: Bearer <token>` on all `/api/v1/*` routes.
    - Blocks unauthorized roles:
      - Routes starting with `/api/v1/admin/` strictly require `role == "ADMIN"`.
      - Routes starting with `/api/v1/user/` allow `role == "USER"` or `role == "ADMIN"`.
    - Injects parsed `UserClaims` into Go's request context for downstream handlers.

- **Controller Layer (`internal/auth/controller/auth_controller.go`)**:
  - Binds HTTP JSON payloads, extracts path variables (`id`), invokes business operations in `AuthService`, and maps responses for the GoFr framework.

- **Server Entrypoint (`cmd/vpsmonitoring/main.go`)**:
  - Initializes database pool, sets up dependency injection chain, mounts middleware, and registers routes.

---

## 2. API Endpoints, Purpose, and Responses

Base URL: `http://localhost:8080` (or production host)  
Standard API Prefix: `/api/v1`

### Summary Table

| Method | Endpoint | Access Level | Purpose |
| :--- | :--- | :--- | :--- |
| `POST` | `/api/v1/auth/login` | Public | Authenticate user or admin and return JWT access token |
| `POST` | `/api/v1/auth/register` | Public | Self-registration for standard client accounts (`USER` role) |
| `GET` | `/api/v1/user/profile` | Protected (`USER`, `ADMIN`) | Fetch authenticated user's profile |
| `GET` | `/api/v1/user/servers` | Protected (`USER`, `ADMIN`) | Fetch list of VPS servers belonging to the user |
| `GET` | `/api/v1/admin/admins` | Protected (`ADMIN` only) | List all administrator accounts |
| `POST` | `/api/v1/admin/admins` | Protected (`ADMIN` only) | Create a new administrator account |
| `PUT` | `/api/v1/admin/admins/{id}` | Protected (`ADMIN` only) | Update an administrator's profile name and email |
| `PATCH` | `/api/v1/admin/admins/{id}/status` | Protected (`ADMIN` only) | Update admin status (`ACTIVE` or `INACTIVE`) |

---

### Detailed API Specifications

#### 2.1 User & Admin Login
- **Endpoint**: `POST /api/v1/auth/login`
- **Access Level**: Public
- **Purpose**: Authenticates credentials for both admins and users. Checks account status (`ACTIVE`), verifies bcrypt password hash, and produces a 24-hour signed JWT access token.
- **Headers**:
  ```http
  Content-Type: application/json
  ```
- **Request Body**:
  ```json
  {
    "email": "admin@example.com",
    "password": "AdminPassword123"
  }
  ```
- **Success Response (`200 OK`)**:
  ```json
  {
    "data": {
      "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
      "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
      "token_type": "Bearer",
      "user": {
        "id": 1,
        "name": "Super Admin",
        "email": "admin@example.com",
        "role": "ADMIN",
        "status": "ACTIVE",
        "created_at": "2026-09-29T10:00:00Z",
        "updated_at": "2026-09-29T10:00:00Z"
      }
    }
  }
  ```
- **Error Responses**:
  - `400 Bad Request / 401 Unauthorized`:
    ```json
    {
      "error": {
        "message": "invalid email or password"
      }
    }
    ```
  - Account Inactive:
    ```json
    {
      "error": {
        "message": "account is inactive; contact an administrator"
      }
    }
    ```

---

#### 2.2 Public Registration
- **Endpoint**: `POST /api/v1/auth/register`
- **Access Level**: Public
- **Purpose**: Allows standard users to register. The backend strictly assigns `role = "USER"` and `status = "ACTIVE"`. Upon registration, an access token is immediately returned to log the user in automatically.
- **Headers**:
  ```http
  Content-Type: application/json
  ```
- **Request Body**:
  ```json
  {
    "name": "John Doe",
    "email": "john.doe@example.com",
    "password": "MyStrongPassword123"
  }
  ```
- **Success Response (`200 OK` / `201 Created`)**:
  ```json
  {
    "data": {
      "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
      "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
      "token_type": "Bearer",
      "user": {
        "id": 2,
        "name": "John Doe",
        "email": "john.doe@example.com",
        "role": "USER",
        "status": "ACTIVE",
        "created_at": "2026-09-29T10:15:00Z",
        "updated_at": "2026-09-29T10:15:00Z"
      }
    }
  }
  ```
- **Error Responses**:
  - Duplicate Email:
    ```json
    {
      "error": {
        "message": "email is already registered"
      }
    }
    ```
  - Password Too Short (< 6 characters):
    ```json
    {
      "error": {
        "message": "valid name, email, and password (minimum 6 characters) are required"
      }
    }
    ```

---

#### 2.3 Get Current User Profile
- **Endpoint**: `GET /api/v1/user/profile`
- **Access Level**: Protected (`USER` or `ADMIN`)
- **Purpose**: Returns the account profile of the currently authenticated token owner.
- **Headers**:
  ```http
  Authorization: Bearer <jwt_access_token>
  ```
- **Success Response (`200 OK`)**:
  ```json
  {
    "data": {
      "id": 2,
      "name": "John Doe",
      "email": "john.doe@example.com",
      "role": "USER",
      "status": "ACTIVE",
      "created_at": "2026-09-29T10:15:00Z",
      "updated_at": "2026-09-29T10:15:00Z"
    }
  }
  ```
- **Error Responses**:
  - Missing or Invalid Token:
    ```json
    {
      "error": {
        "message": "Authorization header missing"
      }
    }
    ```

---

#### 2.4 Get User's Servers
- **Endpoint**: `GET /api/v1/user/servers`
- **Access Level**: Protected (`USER` or `ADMIN`)
- **Purpose**: Returns server telemetry records belonging to the authenticated client.
- **Headers**:
  ```http
  Authorization: Bearer <jwt_access_token>
  ```
- **Success Response (`200 OK`)**:
  ```json
  {
    "data": {
      "owner_id": 2,
      "email": "john.doe@example.com",
      "servers": [],
      "message": "VPS fleet telemetry module ready for server onboarding."
    }
  }
  ```

---

#### 2.5 List All Admins
- **Endpoint**: `GET /api/v1/admin/admins`
- **Access Level**: Protected (`ADMIN` only)
- **Purpose**: Fetches the list of all admin accounts in the database for management purposes.
- **Headers**:
  ```http
  Authorization: Bearer <jwt_admin_token>
  ```
- **Success Response (`200 OK`)**:
  ```json
  {
    "data": [
      {
        "id": 1,
        "name": "Super Admin",
        "email": "admin@example.com",
        "role": "ADMIN",
        "status": "ACTIVE",
        "created_at": "2026-09-29T10:00:00Z",
        "updated_at": "2026-09-29T10:00:00Z"
      }
    ]
  }
  ```
- **Error Responses**:
  - Non-admin token (`403 Forbidden`):
    ```json
    {
      "error": {
        "message": "Forbidden: ADMIN role required to access this endpoint"
      }
    }
    ```

---

#### 2.6 Create Admin Account
- **Endpoint**: `POST /api/v1/admin/admins`
- **Access Level**: Protected (`ADMIN` only)
- **Purpose**: Allows an existing administrator to create an additional administrator account with `role = "ADMIN"` and `status = "ACTIVE"`.
- **Headers**:
  ```http
  Authorization: Bearer <jwt_admin_token>
  Content-Type: application/json
  ```
- **Request Body**:
  ```json
  {
    "name": "DevOps Admin",
    "email": "ops@example.com",
    "password": "AdminPassword123"
  }
  ```
- **Success Response (`200 OK`)**:
  ```json
  {
    "data": {
      "id": 3,
      "name": "DevOps Admin",
      "email": "ops@example.com",
      "role": "ADMIN",
      "status": "ACTIVE",
      "created_at": "2026-09-29T10:30:00Z",
      "updated_at": "2026-09-29T10:30:00Z"
    }
  }
  ```
- **Error Responses**:
  - Duplicate Email:
    ```json
    {
      "error": {
        "message": "email is already registered"
      }
    }
    ```

---

#### 2.7 Update Admin Profile
- **Endpoint**: `PUT /api/v1/admin/admins/{id}`
- **Access Level**: Protected (`ADMIN` only)
- **Purpose**: Modifies an administrator's name and/or email address. Verifies target user is an admin and email is not already in use by another user.
- **Path Parameter**: `id` (int64) - The unique ID of the admin.
- **Headers**:
  ```http
  Authorization: Bearer <jwt_admin_token>
  Content-Type: application/json
  ```
- **Request Body**:
  ```json
  {
    "name": "Lead DevOps Admin",
    "email": "lead.ops@example.com"
  }
  ```
- **Success Response (`200 OK`)**:
  ```json
  {
    "data": {
      "id": 3,
      "name": "Lead DevOps Admin",
      "email": "lead.ops@example.com",
      "role": "ADMIN",
      "status": "ACTIVE",
      "created_at": "2026-09-29T10:30:00Z",
      "updated_at": "2026-09-29T10:35:00Z"
    }
  }
  ```
- **Error Responses**:
  - Admin Not Found:
    ```json
    {
      "error": {
        "message": "user not found"
      }
    }
    ```

---

#### 2.8 Update Admin Status (Active / Inactive)
- **Endpoint**: `PATCH /api/v1/admin/admins/{id}/status`
- **Access Level**: Protected (`ADMIN` only)
- **Purpose**: Activates or deactivates an administrator. Includes safety guard preventing self-deactivation.
- **Path Parameter**: `id` (int64) - Target admin ID.
- **Headers**:
  ```http
  Authorization: Bearer <jwt_admin_token>
  Content-Type: application/json
  ```
- **Request Body**:
  ```json
  {
    "status": "INACTIVE"
  }
  ```
- **Success Response (`200 OK`)**:
  ```json
  {
    "data": {
      "id": 3,
      "name": "Lead DevOps Admin",
      "email": "lead.ops@example.com",
      "role": "ADMIN",
      "status": "INACTIVE",
      "created_at": "2026-09-29T10:30:00Z",
      "updated_at": "2026-09-29T10:40:00Z"
    }
  }
  ```
- **Error Responses**:
  - Self-Deactivation Guard:
    ```json
    {
      "error": {
        "message": "cannot deactivate your own admin account"
      }
    }
    ```
  - Invalid Status Value:
    ```json
    {
      "error": {
        "message": "status must be either ACTIVE or INACTIVE"
      }
    }
    ```

---

## 3. Database Schema Reference

Table: `users` (Managed by GORM AutoMigrate)

| Column | Data Type | Constraints / Attributes | Description |
| :--- | :--- | :--- | :--- |
| `id` | `BIGSERIAL` / `INT8` | `PRIMARY KEY`, `AUTO_INCREMENT` | Unique identifier |
| `name` | `VARCHAR(255)` | `NOT NULL` | Display full name |
| `email` | `VARCHAR(255)` | `NOT NULL`, `UNIQUE` (`uni_users_email`) | Unique email address |
| `password_hash` | `VARCHAR(255)` | `NOT NULL` | Bcrypt hash with salt |
| `role` | `VARCHAR(50)` | `NOT NULL`, `INDEX` | `ADMIN` or `USER` |
| `status` | `VARCHAR(50)` | `NOT NULL`, `DEFAULT 'ACTIVE'` | `ACTIVE` or `INACTIVE` |
| `created_at` | `TIMESTAMPTZ` | `NOT NULL` | Account creation timestamp |
| `updated_at` | `TIMESTAMPTZ` | `NOT NULL` | Last update timestamp |
| `deleted_at` | `TIMESTAMPTZ` | `INDEX` | Soft delete timestamp |

---

## 4. Configuration & Environment Variables

| Variable | Description | Example / Default |
| :--- | :--- | :--- |
| `HTTP_PORT` | Port on which the GoFr server listens | `8080` |
| `DATABASE_URL` | Neon PostgreSQL pooled connection string | `postgresql://user:pass@host/db?sslmode=require` |
| `JWT_SECRET` | Secret key used for signing HS256 tokens | `vpspulse-super-secret-jwt-key-2026...` |
