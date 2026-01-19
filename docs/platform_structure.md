# ESCROW PLATFORM - GO BACKEND STRUCTURE

```
escrow-platform/
├── cmd/
│   ├── api/
│   │   └── main.go                      # API server entry point
│   ├── worker/
│   │   └── main.go                      # Background job processor
│   └── migrate/
│       └── main.go                      # Database migration runner
│
├── internal/
│   ├── domain/                          # Core business entities
│   │   ├── user.go
│   │   ├── rider.go
│   │   ├── payment_link.go
│   │   ├── escrow.go
│   │   ├── delivery.go
│   │   ├── wallet.go
│   │   ├── transaction.go
│   │   ├── dispute.go
│   │   ├── notification.go
│   │   └── errors.go                    # Domain-specific errors
│   │
│   ├── dto/                             # Data Transfer Objects
│   │   ├── auth.go
│   │   ├── user.go
│   │   ├── payment.go
│   │   ├── escrow.go
│   │   ├── delivery.go
│   │   └── response.go                  # Standard API responses
│   │
│   ├── repository/                      # Data access layer
│   │   ├── interfaces.go                # Repository contracts
│   │   ├── postgres/
│   │   │   ├── user_repo.go
│   │   │   ├── rider_repo.go
│   │   │   ├── payment_link_repo.go
│   │   │   ├── escrow_repo.go
│   │   │   ├── delivery_repo.go
│   │   │   ├── wallet_repo.go
│   │   │   ├── transaction_repo.go
│   │   │   ├── dispute_repo.go
│   │   │   └── notification_repo.go
│   │   └── redis/
│   │       ├── session_repo.go
│   │       └── cache_repo.go
│   │
│   ├── service/                         # Business logic layer
│   │   ├── interfaces.go
│   │   ├── auth_service.go              # Registration, login, OTP
│   │   ├── user_service.go              # Profile, mode switching
│   │   ├── rider_service.go             # Onboarding, availability
│   │   ├── payment_service.go           # Payment gateway integration
│   │   ├── payment_link_service.go      # Link creation, validation
│   │   ├── escrow_service.go            # Escrow state machine
│   │   ├── delivery_service.go          # Rider matching, codes, GPS
│   │   ├── wallet_service.go            # Balance, withdrawals
│   │   ├── dispute_service.go           # Dispute handling
│   │   ├── notification_service.go      # Push, SMS
│   │   ├── logistics_service.go         # External logistics tracking
│   │   └── admin_service.go             # Admin operations
│   │
│   ├── handler/                         # HTTP handlers (controllers)
│   │   ├── middleware/
│   │   │   ├── auth.go                  # JWT validation
│   │   │   ├── rate_limit.go
│   │   │   ├── cors.go
│   │   │   ├── logger.go
│   │   │   └── recovery.go
│   │   ├── auth_handler.go
│   │   ├── user_handler.go
│   │   ├── rider_handler.go
│   │   ├── payment_link_handler.go
│   │   ├── escrow_handler.go
│   │   ├── delivery_handler.go
│   │   ├── wallet_handler.go
│   │   ├── dispute_handler.go
│   │   ├── webhook_handler.go           # Payment & logistics webhooks
│   │   └── admin_handler.go
│   │
│   ├── worker/                          # Background jobs
│   │   ├── job_processor.go
│   │   ├── escrow_timeout_job.go        # 48hr seller booking timeout
│   │   ├── dispute_window_job.go        # Auto-release after window
│   │   ├── timer_expiry_job.go          # 20min arrival timers
│   │   ├── logistics_tracker_job.go     # Poll external tracking
│   │   ├── rider_payout_job.go          # 3-day payout cycle
│   │   └── bonus_calculator_job.go      # Daily 50km bonus
│   │
│   ├── integration/                     # External service integrations
│   │   ├── interfaces.go
│   │   ├── paystack/
│   │   │   ├── client.go
│   │   │   ├── payment.go
│   │   │   └── webhook.go
│   │   ├── sms/
│   │   │   ├── termii.go
│   │   │   └── twilio.go
│   │   ├── push/
│   │   │   └── firebase.go
│   │   ├── storage/
│   │   │   └── s3.go                    # Document uploads
│   │   ├── maps/
│   │   │   └── google_maps.go           # Distance calculation
│   │   └── logistics/
│   │       ├── gig_logistics.go
│   │       └── kwik_delivery.go
│   │
│   ├── util/                            # Helper utilities
│   │   ├── validator.go                 # Input validation
│   │   ├── jwt.go                       # Token generation/parsing
│   │   ├── otp.go                       # OTP generation
│   │   ├── code_generator.go            # Pickup/dropoff codes
│   │   ├── distance.go                  # GPS distance calculation
│   │   ├── fee_calculator.go            # Delivery fee logic
│   │   ├── hasher.go                    # Password hashing
│   │   └── response.go                  # HTTP response helpers
│   │
│   ├── config/
│   │   └── config.go                    # Configuration management
│   │
│   └── database/
│       ├── postgres.go                  # DB connection
│       ├── redis.go                     # Redis connection
│       └── migrations/                  # SQL migration files
│           ├── 000001_create_users.up.sql
│           ├── 000001_create_users.down.sql
│           ├── 000002_create_riders.up.sql
│           ├── 000002_create_riders.down.sql
│           └── ...
│
├── pkg/                                 # Public packages (if needed)
│   └── logger/
│       └── logger.go                    # Structured logging
│
├── api/                                 # API documentation
│   └── openapi.yaml                     # OpenAPI/Swagger spec
│
├── scripts/
│   ├── dev.sh                           # Run dev environment
│   ├── test.sh                          # Run tests
│   └── seed.sh                          # Seed test data
│
├── tests/
│   ├── integration/
│   │   ├── auth_test.go
│   │   ├── escrow_test.go
│   │   └── delivery_test.go
│   └── mocks/                           # Mock implementations
│       ├── repository_mocks.go
│       └── service_mocks.go
│
├── deployments/
│   ├── docker/
│   │   ├── Dockerfile
│   │   ├── docker-compose.yml
│   │   └── docker-compose.dev.yml
│   └── k8s/                             # Kubernetes configs (future)
│       ├── api-deployment.yaml
│       └── worker-deployment.yaml
│
├── .env.example
├── .gitignore
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

---

## KEY ARCHITECTURAL DECISIONS

### 1. **Clean Architecture Layers**

```
Handler → Service → Repository → Database
         ↓
    Integration (External APIs)
```

### 2. **Dependency Injection**

Use constructor injection for all services:

```go
type EscrowService struct {
    escrowRepo     repository.EscrowRepository
    walletRepo     repository.WalletRepository
    paymentService service.PaymentService
    notifService   service.NotificationService
}

func NewEscrowService(
    escrowRepo repository.EscrowRepository,
    walletRepo repository.WalletRepository,
    paymentService service.PaymentService,
    notifService service.NotificationService,
) *EscrowService {
    return &EscrowService{...}
}
```

### 3. **State Machine for Escrow**

```go
func (s *EscrowService) TransitionState(
    escrowID string,
    from domain.EscrowState,
    to domain.EscrowState,
) error {
}
```

### 4. **Background Job Queue**

Use Redis-backed queue (asynq or machinery):

```go
s.jobQueue.Enqueue(CheckEscrowTimeout{
    EscrowID: escrow.ID,
    RunAt: time.Now().Add(48 * time.Hour),
})
```

### 5. **Transaction Management**

```go
func (s *EscrowService) ProcessPayment(ctx context.Context, dto PaymentDTO) error {
    return s.db.WithTransaction(ctx, func(tx *sql.Tx) error {
        return nil
    })
}
```

---

## EXAMPLE SERVICE STRUCTURE

```go
package service

import (
    "context"
    "github.com/yourusername/escrow-platform/internal/domain"
    "github.com/yourusername/escrow-platform/internal/dto"
    "github.com/yourusername/escrow-platform/internal/repository"
)

type EscrowService interface {
    CreateFromPaymentLink(ctx context.Context, req dto.PayLinkRequest) (*domain.Escrow, error)
    InitiateDelivery(ctx context.Context, escrowID, userID string, req dto.DeliveryRequest) error
    TransitionState(ctx context.Context, escrowID string, newState domain.EscrowState) error
    ConfirmDelivery(ctx context.Context, escrowID, code string) error
    CreateDispute(ctx context.Context, escrowID, userID string, req dto.DisputeRequest) error
}

type escrowService struct {
    escrowRepo    repository.EscrowRepository
    walletRepo    repository.WalletRepository
    txRepo        repository.TransactionRepository
    deliveryRepo  repository.DeliveryRepository
    paymentSvc    PaymentService
    notifSvc      NotificationService
    jobQueue      JobQueue
}

func NewEscrowService(...) EscrowService {
    return &escrowService{...}
}
```

---

## EXAMPLE HANDLER STRUCTURE

```go
package handler

import (
    "net/http"
    "github.com/gin-gonic/gin"
    "github.com/yourusername/escrow-platform/internal/service"
    "github.com/yourusername/escrow-platform/internal/dto"
)

type EscrowHandler struct {
    escrowService service.EscrowService
}

func NewEscrowHandler(escrowService service.EscrowService) *EscrowHandler {
    return &EscrowHandler{escrowService: escrowService}
}

func (h *EscrowHandler) CreateEscrow(c *gin.Context) {
    var req dto.PayLinkRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, dto.ErrorResponse(err))
        return
    }

    userID := c.GetString("user_id") // From JWT middleware
    escrow, err := h.escrowService.CreateFromPaymentLink(c.Request.Context(), req)
    if err != nil {
        c.JSON(http.StatusInternalServerError, dto.ErrorResponse(err))
        return
    }

    c.JSON(http.StatusCreated, dto.SuccessResponse(escrow))
}
```

---

## RECOMMENDED GO PACKAGES

```go
module github.com/yourusername/escrow-platform

go 1.21

require (
    github.com/gin-gonic/gin v1.10.0              // HTTP framework
    github.com/lib/pq v1.10.9                     // PostgreSQL driver
    github.com/jmoiron/sqlx v1.3.5                // SQL extensions
    github.com/golang-jwt/jwt/v5 v5.2.0           // JWT
    github.com/go-redis/redis/v8 v8.11.5          // Redis client
    github.com/hibiken/asynq v0.24.1              // Background jobs
    github.com/golang-migrate/migrate/v4 v4.17.0  // Migrations
    github.com/spf13/viper v1.18.2                // Config management
    github.com/go-playground/validator/v10 v10.19.0 // Validation
    go.uber.org/zap v1.27.0                       // Logging
    github.com/google/uuid v1.6.0                 // UUID generation
    golang.org/x/crypto v0.19.0                   // Bcrypt
    github.com/stretchr/testify v1.9.0            // Testing
)
```

---

## MAKEFILE EXAMPLE

```makefile
.PHONY: help build run test migrate-up migrate-down docker-up docker-down

help:
	@echo "Available commands:"
	@echo "  make build       - Build the application"
	@echo "  make run         - Run the API server"
	@echo "  make test        - Run tests"
	@echo "  make migrate-up  - Run database migrations"
	@echo "  make docker-up   - Start Docker services"

build:
	go build -o bin/api cmd/api/main.go
	go build -o bin/worker cmd/worker/main.go

run:
	go run cmd/api/main.go

test:
	go test -v ./...

migrate-up:
	migrate -path internal/database/migrations -database "postgres://user:pass@localhost:5432/escrow?sslmode=disable" up

migrate-down:
	migrate -path internal/database/migrations -database "postgres://user:pass@localhost:5432/escrow?sslmode=disable" down

docker-up:
	docker-compose -f deployments/docker/docker-compose.dev.yml up -d

docker-down:
	docker-compose -f deployments/docker/docker-compose.dev.yml down

seed:
	go run scripts/seed/main.go
```

This structure gives you:

- ✅ Clear separation of concerns
- ✅ Easy testing (mock repositories/services)
- ✅ Scalable (add features without refactoring)
- ✅ Production-ready
- ✅ Easy onboarding for frontend work later
