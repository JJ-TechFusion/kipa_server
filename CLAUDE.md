# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Development Commands

```bash
make build          # Build API and worker binaries to bin/
make run            # Run the API server (go run cmd/api/main.go)
make test           # Run all tests (go test -v ./...)
make docker-up      # Start Docker services (Postgres, Redis)
make docker-down    # Stop Docker services
make migrate-up     # Apply database migrations
make migrate-down   # Rollback database migrations
make seed           # Seed database with test data
```

Run a single test:
```bash
go test -v ./tests/integration -run TestName
go test -v ./internal/service -run TestFunctionName
```

## Architecture Overview

Kipa is a delivery platform with escrow payment functionality built in Go using the Gin framework.

### Layer Structure

```
cmd/
├── api/           # HTTP API server entry point
├── worker/        # Background job processor entry point
└── migrate/       # Database migration runner

internal/
├── handler/       # HTTP handlers (controllers) - receive requests, call services
├── service/       # Business logic layer - orchestrates domain operations
├── repository/    # Data access layer - Postgres and Redis implementations
├── domain/        # Domain models and business entities
├── dto/           # Request/response data transfer objects
├── worker/        # Background job definitions (asynq jobs)
├── integration/   # External service clients
├── config/        # Configuration loading (viper)
├── database/      # Database connection setup
└── util/          # Shared utilities (JWT, OTP, validation, etc.)
```

### Key Domain Concepts

- **Escrow**: Payment held in escrow until delivery confirmation
- **Delivery**: Tracks shipment from pickup to delivery with rider assignment
- **Wallet**: User and rider balance management
- **Dispute**: Handles conflicts between buyers and sellers
- **Payment Links**: Shareable payment URLs for transactions

### External Integrations

- **AWS SNS**: SMS verification (primary) with voice call fallback
- **Paystack**: Payment processing and webhooks (`internal/integration/paystack/`)
- **Push**: Firebase Cloud Messaging (`internal/integration/push/`)
- **Maps**: Google Maps for distance/routing (`internal/integration/maps/`)
- **Logistics**: GIG Logistics and Kwik Delivery (`internal/integration/logistics/`)
- **Storage**: S3 for file uploads (`internal/integration/storage/`)

### Delivery Tracking

- **Intra-state**: Google Maps API for real-time rider tracking
- **Inter-state**: Third-party logistics providers (GIG, Kwik) - tracking via their APIs (integration pending)

### Background Jobs (Asynq/Redis)

Jobs in `internal/worker/`:
- `escrow_timeout_job`: Auto-release escrow after timeout
- `dispute_window_job`: Close dispute windows
- `timer_expiry_job`: Handle time-based expirations
- `logistics_tracker_job`: Poll logistics providers for status
- `rider_payout_job`: Process rider earnings
- `bonus_calculator_job`: Calculate rider bonuses

### Request Flow

1. Request → `handler/` (validation, auth via middleware)
2. Handler → `service/` (business logic)
3. Service → `repository/` (data access) + `integration/` (external APIs)
4. Response flows back through DTO transformation

### Middleware Stack

Located in `internal/handler/middleware/`:
- `auth.go`: JWT authentication
- `rate_limit.go`: Request throttling
- `cors.go`: CORS configuration
- `logger.go`: Request logging
- `recovery.go`: Panic recovery

## Critical Implementation Rules

### Idempotency (MANDATORY)

Before any financial operation (escrow release, refund, wallet credit), you MUST:
1. Insert idempotency key into database
2. If conflict (key exists) → abort operation
3. Else proceed with operation

This is critical with Go goroutines and job retries to prevent duplicate transactions.

### Authentication Flow

OTP-based authentication (not account creation on every OTP):
1. User inputs phone number
2. OTP sent via AWS SNS (SMS, voice fallback if SMS fails)
3. User enters OTP
4. Backend verifies OTP
5. If first time → create user account
6. Generate JWT access token + server-side refresh token (Redis/DB)
7. Token used for all authenticated requests

**Edge cases to handle:**
- OTP requested but never verified → expires cleanly
- Login on new device → old tokens revoked
- SIM change → re-verification required
- SMS fails → automatic voice call fallback
- App reinstall → fresh OTP required

**Do NOT:**
- Create new user record on every OTP verification
- Issue long-lived tokens without expiry
- Allow unlimited OTP retries (rate limit required)
- Skip rate limiting on OTP endpoints

### Authorization

- Role-based: user mode / rider mode
- Rider mode requires completed rider profile
- Mode switch blocked if user has active delivery job

## Tech Stack

- **Framework**: Gin
- **Database**: PostgreSQL (sqlx)
- **Cache/Queue**: Redis (go-redis)
- **Background Jobs**: Asynq
- **Migrations**: golang-migrate
- **Config**: Viper
- **Logging**: Zap

- Make the plan extremely concise. Sacrifice grammar for the sake of concision.
- At the end of each plan, give me a list of unresolved questions to answer, if any.