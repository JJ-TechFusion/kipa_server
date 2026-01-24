# Unavailability Handling Plan

## 1. Seller Unavailable (Pickup)

**Flow:**
1. Rider at pickup → seller unreachable
2. Rider marks `seller_unreachable`
3. Job cancelled
4. Buyer refunded (full)

**Changes:**
- `DeliveryJob` status: `seller_unreachable`
- `PaymentRequest` status: `cancelled_seller_unavailable`
- Rider endpoint: `POST /rider/jobs/:id/mark-seller-unavailable`

---

## 2. Buyer Unavailable (Drop-off)

**Flow:**
1. Rider at dropoff → buyer unreachable
2. Rider marks `buyer_unreachable`
3. Auto-create forced return job (same coords swapped)
4. Platform pays rider for return (from platform wallet or deducted from escrow)
5. `PaymentRequest` → `pending_rebook` status
6. Buyer has 24h to rebook
7. If rebook → new delivery job, same escrow
8. If 24h expires → refund minus 5% penalty

**Changes:**

### Domain Models

**DeliveryJob:**
```go
DJStatusBuyerUnavailable    = "buyer_unavailable"
DJStatusForcedReturn        = "forced_return"
DJStatusForcedReturnDone    = "forced_return_done"

// New fields
IsForcedReturn      bool       `db:"is_forced_return"`
UnavailableMarkedAt *time.Time `db:"unavailable_marked_at"`
```

**PaymentRequest:**
```go
PRStatusPendingRebook       = "pending_rebook"
PRStatusRebookExpired       = "rebook_expired"

// New fields
RebookDeadline      *time.Time `db:"rebook_deadline"`
RebookAttempts      int        `db:"rebook_attempts"`
UnavailabilityFee   *float64   `db:"unavailability_fee"` // 5% penalty
ForcedReturnJobID   *uuid.UUID `db:"forced_return_job_id"`
```

### Database Migration (000007)

```sql
-- DeliveryJob fields
ALTER TABLE delivery_jobs
ADD COLUMN IF NOT EXISTS is_forced_return BOOLEAN DEFAULT FALSE,
ADD COLUMN IF NOT EXISTS unavailable_marked_at TIMESTAMP WITH TIME ZONE;

-- PaymentRequest fields
ALTER TABLE payment_requests
ADD COLUMN IF NOT EXISTS rebook_deadline TIMESTAMP WITH TIME ZONE,
ADD COLUMN IF NOT EXISTS rebook_attempts INT DEFAULT 0,
ADD COLUMN IF NOT EXISTS unavailability_fee DECIMAL(12,2),
ADD COLUMN IF NOT EXISTS forced_return_job_id UUID REFERENCES delivery_jobs(id);

-- Indexes
CREATE INDEX idx_pr_pending_rebook ON payment_requests(status, rebook_deadline)
WHERE status = 'pending_rebook';
```

### API Endpoints

**Rider:**
- `POST /rider/jobs/:id/mark-seller-unavailable` - Mark seller unreachable
- `POST /rider/jobs/:id/mark-buyer-unavailable` - Mark buyer unreachable
- `POST /rider/jobs/:id/complete-forced-return` - Complete forced return

**Buyer:**
- `POST /purchases/:id/rebook` - Rebook delivery within 24h window
- `GET /purchases/:id/rebook-status` - Check rebook deadline

### Service Methods

**RiderService:**
```go
MarkSellerUnavailable(ctx, userID, jobID uuid.UUID) error
MarkBuyerUnavailable(ctx, userID, jobID uuid.UUID) error
CompleteForcedReturn(ctx, userID, jobID uuid.UUID) error
```

**PaymentRequestService or new UnavailabilityService:**
```go
CreateForcedReturnJob(ctx, prID uuid.UUID) (*DeliveryJob, error)
RebookDelivery(ctx, prID, buyerID uuid.UUID) (*DeliveryJob, error)
ProcessRebookExpiry(ctx, prID uuid.UUID) error // Called by background job
```

### Background Job

**RebookExpiryJob:**
- Runs every 5 mins
- Finds `pending_rebook` with expired `rebook_deadline`
- Processes refund minus 5%
- Updates status to `rebook_expired`

### Fee Handling

**Seller Unavailable:**
- Full refund to buyer
- Rider gets partial payment (pickup attempt fee from platform)

**Buyer Unavailable:**
- Return delivery fee: paid by platform (deducted from escrow or platform absorbs)
- If rebook: buyer pays new delivery fee
- If expired: buyer gets refund minus 5% (covers return + admin)

---

## State Transitions

### DeliveryJob
```
en_route_pickup → seller_unreachable (terminal - cancelled)
picked_up/in_transit → buyer_unavailable → forced_return → forced_return_done
```

### PaymentRequest
```
in_delivery → cancelled_seller_unavailable (terminal)
in_delivery → pending_rebook → searching_rider (rebook) OR rebook_expired → refunded
```

---

## Unresolved Questions

1. Pickup attempt fee for rider when seller unavailable? (Fixed amount like ₦200?)
2. Who pays forced return fee - platform absorbs or deducted from escrow?
3. Max rebook attempts? (Suggest: 1 rebook only)
4. What if buyer unavailable on rebook too? (Suggest: immediate refund minus 10%)
