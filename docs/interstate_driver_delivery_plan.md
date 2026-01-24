# Inter-State Driver Delivery System Plan

## Overview

Inter-state drivers deliver packages between cities using our app. Unlike intra-city riders, drivers:
- Carry multiple packages per trip
- Travel between cities (Lagos → Abuja, etc.)
- Have monetary carrying limits
- Update status manually (no real-time GPS tracking)

This is NOT third-party logistics - these are app-registered drivers.

---

## 1. Delivery State Machine

```
┌─────────┐    ┌──────────────┐    ┌──────────────┐    ┌───────────────┐
│  DRAFT  │───▶│ AWAITING     │───▶│ DRIVER       │───▶│ AWAITING      │
│         │    │ DRIVER       │    │ ASSIGNED     │    │ PICKUP        │
└─────────┘    └──────────────┘    └──────────────┘    └───────────────┘
     │              │                    │                    │
     ▼              ▼                    ▼                    ▼
┌─────────┐    ┌──────────────┐    ┌──────────────┐    ┌───────────────┐
│CANCELLED│    │ EXPIRED      │    │ CANCELLED    │    │ PICKED_UP     │
└─────────┘    └──────────────┘    └──────────────┘    └───────────────┘
                                                             │
                                                             ▼
┌──────────────┐    ┌──────────────┐    ┌───────────────────────────────┐
│ DISPUTED     │◀───│ CONFIRMATION │◀───│ IN_TRANSIT                    │
│              │    │ WINDOW       │    │ (driver marks departure)      │
└──────────────┘    └──────────────┘    └───────────────────────────────┘
       │                   │                         │
       ▼                   ▼                         ▼
┌──────────────┐    ┌──────────────┐    ┌───────────────────────────────┐
│ REFUNDED     │    │ COMPLETED    │    │ ARRIVED                       │
└──────────────┘    └──────────────┘    │ (driver marks arrival at city)│
                           │            └───────────────────────────────┘
                           ▼
                    ┌──────────────┐
                    │ DRIVER_PAID  │
                    └──────────────┘
```

### Status Definitions

| Status | Description | Triggered By |
|--------|-------------|--------------|
| `draft` | Payment request created, not paid | Seller |
| `awaiting_driver` | Buyer paid, waiting for driver | System (payment webhook) |
| `driver_assigned` | Driver accepted delivery | Driver |
| `awaiting_pickup` | Driver at pickup, ready for code | Driver (manual) |
| `picked_up` | Pickup code verified, driver has item | Driver (code entry) |
| `in_transit` | Driver departed origin city | Driver (manual) |
| `arrived` | Driver arrived at destination city | Driver (manual) |
| `confirmation_window` | Dropoff code verified, buyer has 24h | Driver (code entry) |
| `completed` | Buyer confirmed or window expired | Buyer/System |
| `driver_paid` | Driver payment processed | System |
| `disputed` | Issue raised | Buyer/Seller |
| `refunded` | Buyer refunded | Admin |
| `cancelled` | Cancelled before pickup | Seller |
| `expired` | No driver found | System |

---

## 2. Driver State vs Delivery State

### Driver States

| State | Description |
|-------|-------------|
| `offline` | Not accepting deliveries |
| `idle` | Online, no current packages |
| `loading` | Accepting packages for a trip |
| `in_transit` | En route to destination city |
| `delivering` | At destination, completing deliveries |

### Separation of Concerns

- **Driver state**: Operational status (can accept new work?)
- **Delivery state**: Individual package status (where is this item?)

### One Driver Action → Multiple Deliveries

| Driver Action | Affected Deliveries |
|---------------|---------------------|
| Accept trip | All in trip → `driver_assigned` |
| Mark departed | All picked up → `in_transit` |
| Mark arrived | All in transit → `arrived` |
| Complete one | Only that delivery → `confirmation_window` |

### Partial Failure

Each delivery is **independent after arrival**:
- Driver arrives with 5 packages
- 4 buyers confirm → completed
- 1 buyer disputes → only that one disputed
- Driver paid for 4, holds payment for 1

---

## 3. Driver Capacity Management

### Limits

| Limit | Example Value | Purpose |
|-------|---------------|---------|
| `max_carrying_value` | ₦500,000 | Total item value limit |
| `max_package_count` | 20 | Physical space limit |

### Capacity Flow

```
1. Driver idle: capacity = 0
2. Accepts delivery A (₦100k): capacity = ₦100k
3. Accepts delivery B (₦150k): capacity = ₦250k
4. Cannot accept C (₦300k) if total > ₦500k
5. Completes A: capacity = ₦150k
6. Trip done: capacity = 0
```

### When Capacity Resets

| Event | Capacity Action |
|-------|-----------------|
| Delivery completed | Release that delivery's value |
| Delivery cancelled (pre-pickup) | Release immediately |
| Delivery disputed | Hold until resolution |
| Trip completed | Full reset to 0 |

### Partial Cancellation

- Driver has 5 packages
- Seller cancels 1 (before pickup)
- That package's capacity released
- Other 4 continue normally
- If all cancelled → trip cancelled, driver idle

---

## 4. Code-Based Verification

### Pickup Code Flow

```
1. Driver assigned → pickup code generated
2. Code sent to SELLER (not buyer)
3. Driver arrives at pickup (marks manually)
4. Driver enters code from seller
5. Max 3 attempts
6. Success → status = picked_up
7. Failure after 3 → lockout, admin notified
```

### Dropoff Code Flow

```
1. After pickup → dropoff code generated
2. Code sent to BUYER
3. Driver arrives at destination (marks manually)
4. Driver enters code from buyer
5. Max 3 attempts
6. Success → status = confirmation_window
7. Buyer has 24h to confirm or dispute
```

### Code Rules

| Rule | Value |
|------|-------|
| Code length | 6 digits |
| Max attempts | 3 |
| Code expiry | 24 hours |
| Lockout action | Admin alert, manual resolution |

### Fraud Prevention

- Code attempts tracked
- Suspicious patterns flagged (fast pickups, repeat driver-seller pairs)
- No location spoofing concern (no GPS used)
- Manual status updates audited

---

## 5. Role Capabilities

### Seller Can

| Action | When |
|--------|------|
| Create delivery | Any time |
| Cancel delivery | Before pickup only |
| Give pickup code | At pickup |
| View status | Always |
| Raise dispute | After delivery attempt |

### Seller Cannot

- Confirm delivery (buyer only)
- See dropoff code
- Cancel after pickup
- Change delivery after driver assigned

### Buyer Can

| Action | When |
|--------|------|
| Pay for delivery | Draft status |
| Track status | After payment |
| Receive dropoff code | After pickup |
| Confirm delivery | Confirmation window |
| Raise dispute | Confirmation window + 48h |

### Buyer Cannot

- Cancel delivery (seller can)
- See pickup code
- Change dropoff after driver assigned

### Driver Can

| Action | When |
|--------|------|
| View available deliveries | Idle/loading |
| Accept deliveries | Within capacity |
| Mark at pickup | After assignment |
| Enter pickup code | At pickup |
| Mark departed | All packages picked up |
| Mark arrived | In transit |
| Enter dropoff code | At dropoff |
| Mark buyer unreachable | At dropoff, after wait |

### Driver Cannot

- Confirm delivery (buyer only)
- See escrow amounts
- Cancel after pickup
- Accept new deliveries while in transit

---

## 6. Dispute Handling

### Who Can Dispute

| Party | When | Reasons |
|-------|------|---------|
| Buyer | Confirmation window + 48h | Not received, damaged, wrong item |
| Seller | After delivery attempt | Buyer claims false, fraud |

### Dispute Impact

| Outcome | Escrow | Driver Payment |
|---------|--------|----------------|
| Buyer wins | Full refund | No payment |
| Seller wins | Release to seller | Full payment |
| Partial | Split per admin | Proportional or penalty |

### Partial Disputes

- Trip has 5 deliveries
- 1 disputed, 4 completed
- 4 get paid immediately
- 1 payment held until resolution
- Driver can still complete other trips

---

## 7. Edge Cases

### Driver Delays

| Scenario | Detection | Action |
|----------|-----------|--------|
| No departure after 24h | Background job | Alert admin, notify buyers |
| No arrival after 72h | Background job | Mark abandoned, suspend driver, refund |

### Buyer Unreachable

```
1. Driver marks arrived
2. Driver waits 30 mins
3. Marks buyer unreachable
4. Buyer has 24h to respond
5. No response → return to seller
6. Buyer loses 10% penalty
```

### Seller Cancels After Assignment

- Before pickup: allowed, full refund
- Driver at pickup: 25% driver compensation
- After pickup: NOT allowed

### Driver Abandons

```
1. No status update for 48h → alert admin
2. No response for 72h → mark abandoned
3. All deliveries refunded
4. Driver permanently suspended
```

### City Change Mid-Trip

- During loading: allowed, cancel all current, start new
- After departure: NOT allowed

---

## 8. Database Schema

### New Tables

```sql
-- Driver profiles
CREATE TABLE driver_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    status VARCHAR(20) NOT NULL DEFAULT 'offline',

    -- Capacity
    max_carrying_value DECIMAL(12,2) NOT NULL DEFAULT 500000,
    current_carry_value DECIMAL(12,2) NOT NULL DEFAULT 0,
    max_package_count INT NOT NULL DEFAULT 20,
    current_package_count INT NOT NULL DEFAULT 0,

    -- Vehicle
    vehicle_type VARCHAR(20) NOT NULL, -- car, van, bus
    license_plate VARCHAR(20) NOT NULL,
    is_verified BOOLEAN NOT NULL DEFAULT FALSE,

    -- Stats
    total_deliveries INT NOT NULL DEFAULT 0,
    total_earnings DECIMAL(12,2) NOT NULL DEFAULT 0,
    rating DECIMAL(3,2) NOT NULL DEFAULT 5.0,

    -- Current trip
    current_trip_id UUID REFERENCES driver_trips(id),
    origin_city VARCHAR(100),
    destination_city VARCHAR(100),

    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Driver trips (groups multiple deliveries)
CREATE TABLE driver_trips (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    driver_id UUID NOT NULL REFERENCES driver_profiles(id),
    origin_city VARCHAR(100) NOT NULL,
    destination_city VARCHAR(100) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'accepting',

    -- Aggregates
    total_packages INT NOT NULL DEFAULT 0,
    total_value DECIMAL(12,2) NOT NULL DEFAULT 0,
    total_driver_fee DECIMAL(12,2) NOT NULL DEFAULT 0,

    -- Timeline
    accepting_until TIMESTAMP WITH TIME ZONE,
    departed_at TIMESTAMP WITH TIME ZONE,
    arrived_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,

    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Driver deliveries
CREATE TABLE driver_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_request_id UUID NOT NULL REFERENCES payment_requests(id),
    trip_id UUID REFERENCES driver_trips(id),
    driver_id UUID REFERENCES driver_profiles(id),

    -- Route
    origin_city VARCHAR(100) NOT NULL,
    destination_city VARCHAR(100) NOT NULL,
    pickup_address TEXT NOT NULL,
    dropoff_address TEXT NOT NULL,

    -- Value
    item_value DECIMAL(12,2) NOT NULL,
    driver_fee DECIMAL(12,2) NOT NULL,

    -- Codes
    pickup_code VARCHAR(6),
    pickup_code_generated_at TIMESTAMP WITH TIME ZONE,
    pickup_attempts INT NOT NULL DEFAULT 0,
    dropoff_code VARCHAR(6),
    dropoff_code_generated_at TIMESTAMP WITH TIME ZONE,
    dropoff_attempts INT NOT NULL DEFAULT 0,

    -- Status
    status VARCHAR(30) NOT NULL DEFAULT 'awaiting_driver',

    -- Timeline (manual updates, no GPS)
    driver_assigned_at TIMESTAMP WITH TIME ZONE,
    at_pickup_at TIMESTAMP WITH TIME ZONE,
    picked_up_at TIMESTAMP WITH TIME ZONE,
    departed_at TIMESTAMP WITH TIME ZONE,
    arrived_at TIMESTAMP WITH TIME ZONE,
    delivered_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,

    -- Buyer unreachable
    buyer_unreachable_at TIMESTAMP WITH TIME ZONE,
    unreachable_deadline TIMESTAMP WITH TIME ZONE,

    -- Escrow
    escrow_id UUID REFERENCES escrows(id),

    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Driver delivery events (audit trail)
CREATE TABLE driver_delivery_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    delivery_id UUID NOT NULL REFERENCES driver_deliveries(id),
    from_status VARCHAR(30),
    to_status VARCHAR(30) NOT NULL,
    actor_type VARCHAR(20) NOT NULL, -- driver, buyer, seller, system, admin
    actor_id UUID,
    reason VARCHAR(100),
    metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_driver_deliveries_status ON driver_deliveries(status);
CREATE INDEX idx_driver_deliveries_trip ON driver_deliveries(trip_id);
CREATE INDEX idx_driver_deliveries_route ON driver_deliveries(origin_city, destination_city, status);
CREATE INDEX idx_driver_trips_status ON driver_trips(status);
CREATE INDEX idx_driver_profiles_status ON driver_profiles(status);
```

---

## 9. API Endpoints

### Driver Endpoints

```
POST   /driver/profile                    # Create driver profile
GET    /driver/profile                    # Get profile
PATCH  /driver/profile                    # Update profile

POST   /driver/go-online                  # Start accepting
POST   /driver/go-offline                 # Stop accepting

GET    /driver/available-deliveries       # List matching deliveries
POST   /driver/deliveries/:id/accept      # Accept delivery
GET    /driver/trip                       # Get current trip
GET    /driver/trip/deliveries            # List deliveries in trip

# Manual status updates (no GPS)
POST   /driver/trip/at-pickup             # Mark arrived at pickup city
POST   /driver/deliveries/:id/pickup      # Enter pickup code
POST   /driver/trip/depart                # Mark departed (all must be picked up)
POST   /driver/trip/arrive                # Mark arrived at destination
POST   /driver/deliveries/:id/dropoff     # Enter dropoff code
POST   /driver/deliveries/:id/buyer-unreachable # Mark buyer no-show

GET    /driver/history                    # Past trips
GET    /driver/earnings                   # Earnings summary
```

### Seller Endpoints

```
POST   /seller/driver-delivery            # Create driver delivery request
GET    /seller/driver-deliveries          # List deliveries
GET    /seller/driver-deliveries/:id      # Get delivery details
DELETE /seller/driver-deliveries/:id      # Cancel (before pickup)
```

### Buyer Endpoints

```
GET    /purchases/:id/driver-delivery     # Track driver delivery
POST   /purchases/:id/confirm             # Confirm receipt
POST   /purchases/:id/dispute             # Raise dispute
```

---

## 10. Fee Structure

### Driver Fees

| Route Type | Base Fee | Notes |
|------------|----------|-------|
| Intra-region | ₦2,000 | Lagos → Ibadan |
| Inter-region | ₦5,000 | Lagos → Abuja |
| Long distance | ₦10,000 | Lagos → Kano |

### Platform Take

- 15% of driver fee (same as riders)
- Driver gets 85%

### Buyer Unavailable Penalty

- 10% of item value deducted from refund
- Covers return delivery and admin costs

---

## 11. Background Jobs

### Delivery Monitor Job

```go
// Runs every 5 minutes
func (j *DeliveryMonitorJob) Run(ctx context.Context) {
    j.checkStaleAwaitingDriver()  // No driver after 48h → expire
    j.checkStaleInTransit()       // No arrival after 72h → alert
    j.checkExpiredConfirmation()  // Auto-confirm after 24h
    j.checkUnreachableBuyers()    // Process 24h deadline
}
```

### Trip Monitor Job

```go
// Runs every 15 minutes
func (j *TripMonitorJob) Run(ctx context.Context) {
    j.checkStaleDepartures()      // Picked up but not departed after 24h
    j.checkAbandonedTrips()       // No update for 72h
}
```

---

## 12. Notifications

| Event | Recipient | Message |
|-------|-----------|---------|
| Driver assigned | Buyer, Seller | "Driver assigned for your delivery" |
| Pickup complete | Buyer | "Your item has been picked up" |
| Departed | Buyer | "Driver has departed for [city]" |
| Arrived | Buyer | "Driver has arrived in [city]" |
| Dropoff complete | Buyer | "Please confirm receipt within 24h" |
| Confirmed | Seller, Driver | "Delivery confirmed" |
| Buyer unreachable | Buyer | "URGENT: Driver waiting, respond in 24h" |
| Disputed | Other party | "A dispute has been raised" |

---

## 13. Implementation Order

### Phase 1: Core Models
1. Create domain models (driver, trip, delivery)
2. Create database migration
3. Implement repositories

### Phase 2: Driver Flow
4. Driver profile service
5. Trip management service
6. Delivery acceptance flow
7. Manual status update endpoints

### Phase 3: Verification
8. Pickup code flow
9. Dropoff code flow
10. Attempt tracking and lockout

### Phase 4: Completion
11. Confirmation window
12. Auto-confirm job
13. Driver payment

### Phase 5: Edge Cases
14. Buyer unreachable flow
15. Cancellation handling
16. Dispute integration
17. Abandoned trip handling

---

## 14. Questions to Resolve

1. **Fixed routes only?** Should drivers only accept deliveries on pre-defined routes (Lagos↔Abuja) or any city pair?

2. **Accepting window?** How long can driver accept packages before must depart? (Suggest: 4 hours)

3. **Multiple pickup cities?** Can one trip have pickups from different areas in origin city?

4. **Insurance requirement?** Should high-value items (>₦200k) require insurance?

5. **Driver deposit?** Should drivers put down a deposit against item loss?

6. **Return delivery fee?** Who pays when item returned due to buyer no-show? (Suggest: deduct from buyer's refund)
