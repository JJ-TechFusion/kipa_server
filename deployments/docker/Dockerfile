# ============================================
# KIPA API - Multi-stage Dockerfile
# ============================================

# Stage 1: Build
FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata bash

# Enable automatic toolchain download for newer Go versions
ENV GOTOOLCHAIN=auto

WORKDIR /app

# Copy go mod files first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Copy Firebase service account file
COPY firebase-service-account.json /app/firebase-service-account.json

# Build the API binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -o /app/bin/api \
    ./cmd/api

# Build the migrate binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -o /app/bin/migrate \
    ./cmd/migrate

# Build the worker binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -o /app/bin/worker \
    ./cmd/worker

# Copy wait-for-it.sh (needed to wait for Postgres)
COPY deployments/docker/wait-for-it.sh /app/wait-for-it.sh
RUN chmod +x /app/wait-for-it.sh

# Stage 2: Runtime
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata bash wget

# Create non-root user
RUN addgroup -g 1000 kipa && \
    adduser -u 1000 -G kipa -s /bin/sh -D kipa

WORKDIR /app

# Copy binaries from builder
COPY --from=builder /app/bin/api /app/api
COPY --from=builder /app/bin/migrate /app/migrate
COPY --from=builder /app/bin/worker /app/worker
COPY --from=builder /app/internal/database/migrations /app/migrations
COPY --from=builder /app/firebase-service-account.json /app/firebase-service-account.json
COPY --from=builder /app/wait-for-it.sh /app/wait-for-it.sh
RUN chmod +x /app/wait-for-it.sh

# Set ownership
RUN chown -R kipa:kipa /app

# Switch to non-root user
USER kipa

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Default command (wait for Postgres, then start API)
CMD ["/app/wait-for-it.sh", "postgres:5432", "--", "/app/api"]