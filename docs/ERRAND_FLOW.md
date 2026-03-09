# Errand Flow Documentation

This document explains the errand system flow for frontend implementation.

## Overview

Errands are standalone delivery requests where a user needs something picked up from one location and delivered to another. Unlike marketplace transactions, errands do NOT involve payment escrow - they are pure delivery services.

## Errand vs Marketplace Transaction

| Feature | Errand | Marketplace Transaction |
|---------|--------|------------------------|
| Payment Escrow | No | Yes |
| Product Purchase | No | Yes |
| Delivery Only | Yes | Part of transaction |
| Use Case | "Pick up my package from X" | "Buy this item from seller" |

---

## Errand Statuses

```
draft → searching → accepted → picked_up → in_transit → delivered → completed
                                    ↘                              ↗
                                      → cancelled (at certain stages)
```

| Status | Description | User Action Available |
|--------|-------------|----------------------|
| `draft` | Errand created but not confirmed | Confirm, Cancel |
| `searching` | Looking for available rider | Cancel |
| `accepted` | Rider accepted the job | Cancel (with conditions) |
| `picked_up` | Rider picked up the package | - |
| `in_transit` | Package in transit | - |
| `delivered` | Rider arrived at destination | Complete (enter dropoff code) |
| `completed` | Errand finished successfully | - |
| `cancelled` | Errand was cancelled | - |

---

## API Endpoints

### 1. Create Errand (Draft)
```
POST /errands
```

**Request Body:**
```json
{
  "pickup_address": "123 Pickup Street, Lagos",
  "pickup_latitude": 6.5244,
  "pickup_longitude": 3.3792,
  "pickup_contact_name": "John Doe",
  "pickup_contact_phone": "+2348012345678",
  "dropoff_address": "456 Dropoff Avenue, Lagos",
  "dropoff_latitude": 6.4541,
  "dropoff_longitude": 3.3947,
  "dropoff_contact_name": "Jane Doe",
  "dropoff_contact_phone": "+2348087654321",
  "package_description": "Small box with documents",
  "package_size": "small",
  "notes": "Handle with care"
}
```

**Package Sizes:** `small`, `medium`, `large`

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "status": "draft",
    "pickup_address": "...",
    "dropoff_address": "...",
    "estimated_price": 1500.00,
    "estimated_distance_km": 5.2,
    "estimated_duration_mins": 25,
    "created_at": "2024-01-15T10:30:00Z"
  }
}
```

**Frontend Flow:**
1. User enters pickup & dropoff details
2. Show estimated price and duration
3. User reviews details before confirming

---

### 2. Get User's Errands
```
GET /errands
```

**Query Parameters:**
- `status` (optional): Filter by status
- `limit` (optional): Default 20
- `offset` (optional): For pagination

**Response:**
```json
{
  "success": true,
  "data": {
    "errands": [...],
    "count": 10
  }
}
```

---

### 3. Get Active Errand
```
GET /errands/active
```

Returns the user's currently active errand (if any). Use this to check if user has an ongoing errand.

**Response (if active errand exists):**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "status": "in_transit",
    "delivery_job": {
      "id": "uuid",
      "rider": {
        "id": "uuid",
        "name": "Rider Name",
        "phone": "+234...",
        "photo_url": "...",
        "vehicle_type": "motorcycle",
        "rating": 4.8
      },
      "pickup_code": "1234",
      "dropoff_code": "5678",
      "current_latitude": 6.5100,
      "current_longitude": 3.3800
    }
  }
}
```

**Frontend Flow:**
- Check on app launch if user has active errand
- If yes, navigate to tracking screen

---

### 4. Get Single Errand
```
GET /errands/:id
```

Returns full errand details including delivery job info if assigned.

---

### 5. Confirm Errand
```
POST /errands/:id/confirm
```

Confirms the draft errand and starts searching for a rider.

**What happens:**
1. Status changes: `draft` → `searching`
2. System creates a DeliveryJob
3. Nearby riders are notified
4. Pickup and dropoff codes are generated

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "status": "searching",
    "delivery_job_id": "uuid",
    "pickup_code": "1234",
    "dropoff_code": "5678"
  }
}
```

**Frontend Flow:**
1. Show "Searching for rider..." screen
2. Poll or use WebSocket for status updates
3. Once rider accepts → show rider info

---

### 6. Complete Errand
```
POST /errands/:id/complete
```

User confirms delivery by entering the dropoff code.

**Request Body:**
```json
{
  "dropoff_code": "5678"
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "message": "Errand completed successfully"
  }
}
```

**Error Cases:**
- `INVALID_DROPOFF_CODE`: Wrong code entered
- `ERRAND_NOT_DELIVERED`: Errand not in delivered status yet

**Frontend Flow:**
1. When status is `delivered`, show code entry screen
2. User enters 4-digit dropoff code
3. On success, show completion screen with option to rate rider

---

### 7. Cancel Errand
```
DELETE /errands/:id
```

Cancels the errand. Only allowed in certain statuses.

**Cancellation Rules:**
| Status | Can Cancel? | Notes |
|--------|-------------|-------|
| `draft` | Yes | No fee |
| `searching` | Yes | No fee |
| `accepted` | Yes | May incur fee |
| `picked_up` | No | Package already collected |
| `in_transit` | No | Delivery in progress |
| `delivered` | No | Must complete |

**Response:**
```json
{
  "success": true,
  "data": {
    "message": "Errand cancelled successfully",
    "cancellation_fee": 0
  }
}
```

---

## Frontend Screen Flow

### Screen 1: Create Errand
```
┌─────────────────────────┐
│   Create New Errand     │
├─────────────────────────┤
│ Pickup Location         │
│ [________________________]
│ Contact: [______________]
│ Phone:   [______________]
│                         │
│ Dropoff Location        │
│ [________________________]
│ Contact: [______________]
│ Phone:   [______________]
│                         │
│ Package Size: [Small ▼] │
│ Description: [__________]
│                         │
│ Est. Price: ₦1,500      │
│ Est. Time: 25 mins      │
│                         │
│    [Confirm Errand]     │
└─────────────────────────┘
```

### Screen 2: Searching for Rider
```
┌─────────────────────────┐
│   Finding Rider...      │
├─────────────────────────┤
│                         │
│     🔍 (animated)       │
│                         │
│  Searching for nearby   │
│  riders...              │
│                         │
│  [Cancel]               │
└─────────────────────────┘
```

### Screen 3: Rider Assigned / Tracking
```
┌─────────────────────────┐
│   Errand In Progress    │
├─────────────────────────┤
│ ┌─────────────────────┐ │
│ │     [MAP VIEW]      │ │
│ │   rider location    │ │
│ └─────────────────────┘ │
│                         │
│ Rider: John Doe ⭐ 4.8  │
│ Vehicle: Motorcycle     │
│ [Call] [Message]        │
│                         │
│ Status: In Transit      │
│ Pickup Code: 1234       │
│ Dropoff Code: 5678      │
│                         │
│ Pickup:  123 Street     │
│ Dropoff: 456 Avenue     │
└─────────────────────────┘
```

### Screen 4: Confirm Delivery
```
┌─────────────────────────┐
│   Confirm Delivery      │
├─────────────────────────┤
│                         │
│  Rider has arrived!     │
│                         │
│  Enter dropoff code     │
│  to confirm delivery:   │
│                         │
│  ┌───┬───┬───┬───┐     │
│  │ 5 │ 6 │ 7 │ 8 │     │
│  └───┴───┴───┴───┘     │
│                         │
│    [Confirm]            │
└─────────────────────────┘
```

---

## Status Polling / Real-time Updates

### Option 1: Polling (Simple)
```javascript
const pollErrandStatus = async (errandId) => {
  const response = await fetch(`/errands/${errandId}`);
  const data = await response.json();

  if (data.data.status !== currentStatus) {
    handleStatusChange(data.data.status);
  }
};

setInterval(() => pollErrandStatus(errandId), 10000);
```

### Option 2: WebSocket (Recommended)
```javascript
const ws = new WebSocket('wss://api.kipa.com/ws/errands');

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  if (data.type === 'errand_status_update') {
    handleStatusChange(data.status);
  }
  if (data.type === 'rider_location') {
    updateMapLocation(data.latitude, data.longitude);
  }
};
```

---

## Codes Explanation

### Pickup Code
- 4-digit code given to user
- User shares with rider at pickup point
- Rider enters code to confirm pickup
- Ensures right person/package is picked up

### Dropoff Code
- 4-digit code given to user
- User enters code when rider delivers
- Confirms user received the package
- Completes the errand

**Important:** Store these codes securely. Don't share dropoff code until you physically receive the package.

---

## Error Handling

| Error Code | Description | User Message |
|------------|-------------|--------------|
| `ERRAND_NOT_FOUND` | Errand ID doesn't exist | "Errand not found" |
| `ERRAND_ALREADY_CONFIRMED` | Can't confirm twice | "Errand already confirmed" |
| `ERRAND_NOT_CONFIRMABLE` | Wrong status for confirm | "Cannot confirm errand" |
| `ERRAND_NOT_CANCELLABLE` | Wrong status for cancel | "Cannot cancel at this stage" |
| `INVALID_DROPOFF_CODE` | Wrong completion code | "Invalid code. Try again" |
| `NO_RIDERS_AVAILABLE` | No riders in area | "No riders available. Try later" |

---

## Edge Cases

### 1. User closes app during search
- Backend continues searching
- On app reopen, check `/errands/active`
- Resume from current status

### 2. Rider cancels after accepting
- Status reverts to `searching`
- Show "Finding new rider..." message
- Auto-retry finding rider

### 3. Network failure during completion
- Store code entry locally
- Retry on reconnection
- Show "Confirming..." with retry logic

### 4. Multiple active errands
- System allows only ONE active errand per user
- Check `/errands/active` before creating new

---

## Pricing Calculation

Price is calculated based on:
1. **Distance** (pickup to dropoff)
2. **Package size** (small/medium/large)
3. **Time of day** (surge pricing during peak)
4. **Vehicle type** (motorcycle vs car)

Frontend should display estimated price from create response and note that final price may vary slightly.
