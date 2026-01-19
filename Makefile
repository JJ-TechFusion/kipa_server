.PHONY: help build run run-worker test migrate-up migrate-down docker-up docker-down

help:
	@echo "Available commands:"
	@echo "  make build       - Build the application"
	@echo "  make run         - Run the API server"
	@echo "  make run-worker  - Run the background worker"
	@echo "  make test        - Run tests"
	@echo "  make migrate-up  - Run database migrations"
	@echo "  make docker-up   - Start Docker services"

build:
	go build -o bin/api cmd/api/main.go
	go build -o bin/worker cmd/worker/main.go

run:
	go run cmd/api/main.go

run-worker:
	go run cmd/worker/main.go

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