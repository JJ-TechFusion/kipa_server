Correct Mental Model

Seller defines DELIVERY INTENT, not a rider.

Seller provides:
	•	Pickup address
	•	Drop-off address
	•	Package size / type
	•	Delivery speed (optional)

System does:
	•	Calculates delivery fee
	•	Searches nearby riders
	•	Creates a delivery request

⸻

FINAL CORRECT FLOW (STEP-BY-STEP)

1️⃣ Seller creates payment request (NO LINK)

Status: DRAFT

Contains:
	•	Item details
	•	Item price
	•	Processing time

No delivery, no rider, no link.

⸻

2️⃣ Seller enters delivery details

Seller inputs:
	•	Pickup address
	•	Drop-off address
	•	Delivery constraints (bike/car)

System:
	•	Calculates delivery fee
	•	Validates coverage

Status: DELIVERY_CONFIGURED

⸻

3️⃣ System broadcasts delivery request to riders

Like Bolt:
	•	Find riders within X km
	•	Only AVAILABLE riders
	•	Send job request

Delivery job status:

3️⃣ System broadcasts delivery request to riders

Like Bolt:
	•	Find riders within X km
	•	Only AVAILABLE riders
	•	Send job request

Delivery job status: SEARCHING

4. Rider accepts delivery

First rider to accept:
	•	Is assigned
	•	Others are cancelled

Delivery job: ACCEPTED

At this moment:
✅ Delivery is viable
✅ Price is confirmed
✅ Rider availability is guaranteed

⸻

5️⃣ NOW generate payment link

Only now:
	•	Total amount is final
	•	Delivery is guaranteed

Status: LINK_ACTIVE

Seller shares link to buyer.

⸻

6️⃣ Buyer pays → escrow + delivery pool created

Everything you designed earlier now applies safely.

⸻

What if NO rider accepts?

Simple and safe:
	•	Delivery job times out
	•	Seller notified: “No riders available”
	•	Seller can:
	•	Retry
	•	Change delivery options
	•	Cancel

No link exists, no money locked.

⸻

Rider payment (still clean)
	•	Rider gets paid after delivery
	•	Independent of escrow
	•	Platform fee deducted

No changes needed.

State Machine Update (Important)

Delivery Job (updated)

CREATED
 ↓
SEARCHING
 ↓
ACCEPTED
 ↓
PICKED_UP
 ↓
IN_TRANSIT
 ↓
DELIVERED
 ↓
PAID_OUT

Failure:
SEARCHING → FAILED
ACCEPTED → CANCELLED


Running Migrations

  Option 1: Using the migrate binary directly
  # Build the migrate tool
  go build -o bin/migrate ./cmd/migrate

  # Run migrations (uses env vars or flags)
  ./bin/migrate -command up

  # Other commands
  ./bin/migrate -command down          # Rollback all
  ./bin/migrate -command down -steps 1 # Rollback one
  ./bin/migrate -command version       # Check current version

  Option 2: Using DATABASE_URL
  ./bin/migrate -database "postgres://user:pass@localhost:5432/kipa?sslmode=disable" -command up

  ---
  Running with Docker Compose

  1. Create your .env file:
  cp .env.example .env
  # Edit .env with your values

  2. Start everything:
  # Start PostgreSQL, Redis, run migrations, and start API
  docker compose up -d

  # View logs
  cd deployments/docker && docker compose logs -f api

  # Start services
cd deployments/docker && docker-compose up -d

# Or from root with -f flag
docker-compose -f deployments/docker/docker-compose.yml up -d

# View logs
cd deployments/docker && docker-compose logs api

# Stop services
cd deployments/docker && docker-compose down

  # Stop everything
  docker compose down

  # Stop and remove volumes (fresh start)
  docker compose down -v

  3. Run migrations only:
  docker compose run --rm migrate

  ---
  Build Docker Image Separately

  # Build
  docker build -t kipa-api:latest .

  # Run (requires external PostgreSQL and Redis)
  docker run -p 8080:8080 --env-file .env kipa-api:latest

  ---
  Quick Start (Development)

  # 1. Copy and edit env file
  cp .env.example .env

  # 2. Start infrastructure
  docker compose up -d postgres redis

  # 3. Run migrations locally
  go run ./cmd/migrate -command up

  # 4. Start the API
  go run ./cmd/api