# Kipa - Delivery Platform

A delivery platform built with Go.

## Project Structure

```
kipa/
├── cmd/                    # Application entry points
├── internal/              # Private application code
├── pkg/                   # Public packages
├── api/                   # API documentation
├── scripts/              # Build and deployment scripts
├── tests/                # Test files
└── deployments/          # Deployment configurations
```

## Getting Started

1. Copy `.env.example` to `.env` and configure your environment variables
2. Run `make dev` to start the development environment
3. Run `make migrate-up` to apply database migrations

## Run the Server

1. Set up environment variables or create a `config.yaml`:
   - JWT secrets (`jwt.access_secret`, `jwt.refresh_secret`)
   - AWS credentials (`aws.access_key_id`, `aws.secret_access_key`, `aws.region`)
   - Database connection
   - Redis connection
2. Start dependencies: `make docker-up`
3. Run migrations: `make migrate-up`
4. Run the API: `make run`

## Commands

- `make dev` - Start development server
- `make test` - Run tests
- `make migrate-up` - Apply database migrations
- `make migrate-down` - Rollback database migrations