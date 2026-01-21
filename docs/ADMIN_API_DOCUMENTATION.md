
---

## Admin Endpoints

All admin endpoints require authentication with an admin account. Admin access is controlled by the `is_admin` flag in the users table.

**Base Path:** `/api/v1/admin`

**Authentication:** All endpoints require `Authorization: Bearer <admin_access_token>` header

**Authorization:** Returns `403 Forbidden` if the authenticated user is not an admin

---

### Rider Management

#### Verify Rider
Approves a rider's verification request.

```
POST /admin/riders/:id/verify
```

**Headers:**
```
Authorization: Bearer <admin_access_token>
```

**Response:**
```json
{
  "message": "rider verified successfully"
}
```

**Error Responses:**
- `400 Bad Request` - Rider already verified
- `404 Not Found` - Rider not found
- `403 Forbidden` - Not an admin

---

#### Suspend Rider
Suspends a rider and their user account.

```
POST /admin/riders/:id/suspend
```

**Request Body:**
```json
{
  "reason": "Multiple customer complaints"
}
```

**Response:**
```json
{
  "message": "rider suspended successfully"
}
```

---

#### Unsuspend Rider
Restores a suspended rider to verified status.

```
POST /admin/riders/:id/unsuspend
```

**Response:**
```json
{
  "message": "rider unsuspended successfully"
}
```

---

#### Get Pending Riders
Lists all riders awaiting verification.

```
GET /admin/riders/pending
```

**Response:**
```json
{
  "riders": [
    {
      "id": "uuid",
      "user_id": "uuid",
      "license_number": "ABC123456",
      "nin_number": "12345678901",
      "vehicle_type": "motorcycle",
      "vehicle_plate": "LAG-123-AB",
      "status": "pending_verification",
      "created_at": "2024-01-15T10:00:00Z"
    }
  ],
  "count": 5
}
```

---

#### Get Rider Statistics
Returns platform-wide rider statistics.

```
GET /admin/riders/statistics
```

**Response:**
```json
{
  "total_riders": 150,
  "verified_riders": 120,
  "pending_riders": 15,
  "suspended_riders": 5,
  "online_riders": 45,
  "average_rating": 4.6,
  "total_deliveries": 3250
}
```

---

### User Management

#### List Users
Lists all users with optional filtering.

```
GET /admin/users
```

**Query Parameters:**
- `limit` - Number of results (default: 50)
- `offset` - Pagination offset (default: 0)

**Response:**
```json
{
  "users": [
    {
      "id": "uuid",
      "phone_number": "+2348012345678",
      "first_name": "John",
      "last_name": "Doe",
      "is_seller": true,
      "is_buyer": true,
      "is_rider": false,
      "is_suspended": false,
      "seller_verified": true,
      "created_at": "2024-01-01T10:00:00Z"
    }
  ],
  "count": 50
}
```

---

#### Suspend User
Suspends a user account.

```
POST /admin/users/:id/suspend
```

**Request Body:**
```json
{
  "reason": "Fraudulent activity detected"
}
```

**Response:**
```json
{
  "message": "user suspended successfully"
}
```

---

#### Unsuspend User
Restores a suspended user account.

```
POST /admin/users/:id/unsuspend
```

**Response:**
```json
{
  "message": "user unsuspended successfully"
}
```

---

#### Verify Seller
Verifies a seller account.

```
POST /admin/users/:id/verify-seller
```

**Response:**
```json
{
  "message": "seller verified successfully"
}
```

---

#### Get User Statistics
Returns platform-wide user statistics.

```
GET /admin/users/statistics
```

**Response:**
```json
{
  "total_users": 5000,
  "total_sellers": 1200,
  "total_buyers": 3500,
  "total_riders": 150,
  "suspended_users": 25
}
```

---

### Platform Settings

#### Get Delivery Pricing
Retrieves current delivery pricing configuration.

```
GET /admin/settings/delivery-pricing
```

**Response:**
```json
{
  "base_fee": 500.0,
  "per_km_fee": 100.0,
  "vehicle_multipliers": {
    "motorcycle": 1.0,
    "car": 1.5,
    "van": 2.0
  }
}
```

---

#### Update Delivery Pricing
Updates delivery pricing configuration.

```
PUT /admin/settings/delivery-pricing
```

**Request Body:**
```json
{
  "base_fee": 600.0,
  "per_km_fee": 120.0,
  "vehicle_multipliers": {
    "motorcycle": 1.0,
    "car": 1.5,
    "van": 2.0
  }
}
```

**Validation:**
- `base_fee` - Must be >= 0
- `per_km_fee` - Must be >= 0
- `vehicle_multipliers` - Optional, defaults to 1.0 for each vehicle type

**Response:**
```json
{
  "message": "delivery pricing updated successfully"
}
```

---

#### Get Platform Fees
Retrieves current platform fee rates.

```
GET /admin/settings/platform-fees
```

**Response:**
```json
{
  "buyer_fee_rate": 0.02,
  "seller_fee_rate": 0.03,
  "rider_payout_rate": 0.85
}
```

---

#### Update Platform Fees
Updates platform commission rates.

```
PUT /admin/settings/platform-fees
```

**Request Body:**
```json
{
  "buyer_fee_rate": 0.025,
  "seller_fee_rate": 0.035,
  "rider_payout_rate": 0.80
}
```

**Validation:**
- All rates must be between 0 and 1 (0% to 100%)

**Response:**
```json
{
  "message": "platform fees updated successfully"
}
```

---

### Analytics

#### Get Platform Revenue
Returns revenue analytics with optional date filtering.

```
GET /admin/analytics/revenue
```

**Query Parameters:**
- `start_date` - Start date (ISO 8601 format, optional)
- `end_date` - End date (ISO 8601 format, optional)

**Response:**
```json
{
  "total_revenue": 5000000.0,
  "buyer_fees": 1500000.0,
  "seller_fees": 2000000.0,
  "delivery_fees": 1500000.0,
  "total_transactions": 2500
}
```

**Note:** Currently returns placeholder data. Full implementation requires wallet transaction aggregation.

---

#### Get Active Deliveries
Returns real-time active delivery statistics.

```
GET /admin/analytics/active-deliveries
```

**Response:**
```json
{
  "total_active": 45,
  "searching": 12,
  "assigned": 18,
  "in_transit": 15,
  "average_distance_km": 0
}
```

---

#### Get Rider Performance
Returns rider performance metrics.

```
GET /admin/analytics/rider-performance
```

**Query Parameters:**
- `rider_id` - Optional. If provided, returns metrics for specific rider only

**Response:**
```json
[
  {
    "rider_id": "uuid",
    "rider_name": "John Doe",
    "total_deliveries": 250,
    "completed_today": 0,
    "average_rating": 4.8,
    "total_earnings": 125000.0,
    "online_status": true
  }
]
```

---

### Payment Request Management

#### List Payment Requests
Lists all payment requests with optional filtering.

```
GET /admin/payment-requests
```

**Query Parameters:**
- `seller_id` - Filter by seller (optional)
- `buyer_id` - Filter by buyer (optional)
- `limit` - Number of results (default: 50)
- `offset` - Pagination offset (default: 0)

**Response:**
```json
[
  {
    "id": "uuid",
    "seller_id": "uuid",
    "buyer_id": "uuid",
    "item_name": "iPhone 15 Pro Max",
    "item_price": 850000.0,
    "status": "link_active",
    "payment_code": "KIP-ABC12345",
    "created_at": "2024-01-15T10:00:00Z"
  }
]
```

---

#### Cancel Payment Request
Cancels a payment request (admin override).

```
POST /admin/payment-requests/:id/cancel
```

**Request Body:**
```json
{
  "reason": "Fraudulent listing detected"
}
```

**Response:**
```json
{
  "message": "payment request cancelled successfully"
}
```

**Validation:**
- Can only cancel requests in `draft` or `link_active` status

---

#### Get Expired Payment Requests
Lists all expired payment requests.

```
GET /admin/payment-requests/expired
```

**Response:**
```json
[
  {
    "id": "uuid",
    "seller_id": "uuid",
    "item_name": "Old Item",
    "status": "expired",
    "created_at": "2024-01-01T10:00:00Z",
    "expires_at": "2024-01-08T10:00:00Z"
  }
]
```

---

### System Health

#### Database Health Check
Checks database connection status.

```
GET /admin/health/database
```

**Response:**
```json
{
  "status": "healthy",
  "message": "database connection OK",
  "latency_ms": 0
}
```

**Status Values:**
- `healthy` - Database is operational
- `degraded` - Database is slow but functional
- `down` - Database connection failed

---

#### Redis Health Check
Checks Redis connection status.

```
GET /admin/health/redis
```

**Response:**
```json
{
  "status": "healthy",
  "message": "redis check not implemented",
  "latency_ms": 0
}
```

**Note:** Currently returns placeholder. Full implementation requires Redis client injection.

---

#### External Services Health Check
Checks status of external service integrations.

```
GET /admin/health/external-services
```

**Response:**
```json
{
  "paystack": {
    "status": "healthy",
    "message": "check not implemented"
  },
  "google_maps": {
    "status": "healthy",
    "message": "check not implemented"
  },
  "termii": {
    "status": "healthy",
    "message": "check not implemented"
  }
}
```

**Note:** Currently returns placeholders. Full implementation requires actual service health checks.

---

### Audit Logging

#### List Audit Logs
Returns all admin action audit logs.

```
GET /admin/audit-logs
```

**Response:**
```json
{
  "logs": [
    {
      "id": "uuid",
      "admin_id": "uuid",
      "action": "verify_rider",
      "target_type": "rider",
      "target_id": "uuid",
      "details": {"notes": "Approved after document review"},
      "created_at": "2024-01-15T10:00:00Z"
    }
  ],
  "count": 50
}
```

---

#### Get User Audit Trail
Returns audit logs related to a specific user.

```
GET /admin/audit-logs/user/:id
```

**Response:**
```json
{
  "logs": [...],
  "count": 10
}
```

---

### Dispute Management

#### List Disputes
Returns all disputes with optional filtering.

```
GET /admin/disputes
```

**Response:**
```json
{
  "disputes": [
    {
      "id": "uuid",
      "escrow_id": "uuid",
      "opened_by": "uuid",
      "reason": "Item not as described",
      "status": "open",
      "outcome": "pending",
      "created_at": "2024-01-15T10:00:00Z"
    }
  ],
  "count": 5
}
```

---

#### Get Dispute Details
Returns detailed dispute information including escrow, buyer, and seller.

```
GET /admin/disputes/:id
```

**Response:**
```json
{
  "dispute": {...},
  "escrow": {...},
  "payment_request": {...},
  "buyer": {...},
  "seller": {...}
}
```

---

#### Resolve Dispute
Resolves a dispute with specified outcome.

```
POST /admin/disputes/:id/resolve
```

**Request Body:**
```json
{
  "outcome": "buyer_wins",
  "notes": "Seller failed to provide proof of shipment"
}
```

**Outcome Values:**
- `buyer_wins` - Full refund to buyer
- `seller_wins` - Release funds to seller
- `partial_refund` - Partial refund (requires ProcessRefund call)
- `mutual` - Mutually agreed resolution

**Response:**
```json
{
  "message": "dispute resolved successfully"
}
```

---

#### Process Refund
Processes a refund for a dispute.

```
POST /admin/disputes/:id/refund
```

**Request Body:**
```json
{
  "refund_type": "full",
  "amount": 50000.0,
  "reason": "Item not received"
}
```

**Refund Types:**
- `full` - Complete refund
- `partial` - Partial refund (specify amount)

**Response:**
```json
{
  "message": "refund processed successfully"
}
```

---

#### Get Dispute Statistics
Returns dispute statistics.

```
GET /admin/disputes/statistics
```

**Response:**
```json
{
  "total_disputes": 50,
  "open_disputes": 10,
  "in_review_disputes": 5,
  "resolved_disputes": 35,
  "buyer_wins": 20,
  "seller_wins": 10,
  "partial_refunds": 5
}
```

---

### Commission Management

#### Get Commission Settings
Retrieves current commission settings for riders.

```
GET /admin/settings/commission
```

**Response:**
```json
{
  "max_unpaid_rides": 5,
  "commission_rate": 0.15
}
```

---

#### Update Commission Settings
Updates rider commission configuration.

```
PUT /admin/settings/commission
```

**Request Body:**
```json
{
  "max_unpaid_rides": 5,
  "commission_rate": 0.15
}
```

**Validation:**
- `max_unpaid_rides` - Must be >= 1
- `commission_rate` - Must be > 0 and <= 1 (0% to 100%)

**Response:**
```json
{
  "message": "commission settings updated successfully"
}
```

---

### Wallet Management

#### Get User Wallet
Retrieves wallet information for a specific user.

```
GET /admin/wallets/user/:id
```

**Response:**
```json
{
  "id": "uuid",
  "user_id": "uuid",
  "available_balance": 50000.0,
  "locked_balance": 10000.0,
  "total_earned": 120000.0,
  "total_withdrawn": 60000.0,
  "created_at": "2024-01-01T10:00:00Z",
  "updated_at": "2024-01-15T15:30:00Z"
}
```

---

#### Adjust User Wallet
Manually adjusts a user's wallet balance (credit or debit).

```
POST /admin/wallets/user/:id/adjust
```

**Request Body:**
```json
{
  "adjustment_type": "credit",
  "amount": 5000.0,
  "reason": "Customer service compensation"
}
```

**Adjustment Types:**
- `credit` - Add funds to wallet
- `debit` - Remove funds from wallet

**Validation:**
- `amount` - Must be > 0
- For debit: User must have sufficient available balance

**Response:**
```json
{
  "message": "wallet adjusted successfully"
}
```

**Note:** Creates a wallet transaction record with type `adjustment` and logs the action in audit logs.

---

#### Get Wallet Transactions
Retrieves transaction history for a user's wallet.

```
GET /admin/wallets/user/:id/transactions
```

**Response:**
```json
{
  "transactions": [
    {
      "id": "uuid",
      "wallet_id": "uuid",
      "type": "adjustment",
      "amount": 5000.0,
      "balance_after": 55000.0,
      "description": "Customer service compensation",
      "created_at": "2024-01-15T15:30:00Z"
    }
  ],
  "count": 50
}
```

**Transaction Types:**
- `escrow_fund` - Funds locked in escrow
- `escrow_release` - Funds released from escrow
- `delivery_payout` - Rider delivery payment
- `platform_fee` - Platform commission deduction
- `withdrawal` - User withdrawal
- `top_up` - Manual top-up
- `refund` - Refund issued
- `adjustment` - Admin adjustment

---

### Admin Access Control

**Setting Up Admin Users:**

Admins must be manually promoted via database:

```sql
UPDATE users SET is_admin = true WHERE phone_number = '+234...';
```

**Security Considerations:**
- All admin actions are logged with admin ID
- Admin middleware checks both authentication AND admin status
- Returns `403 Forbidden` for non-admin users
- Recommended: Implement IP whitelisting for admin access
- Recommended: Add multi-factor authentication for admin accounts

---

### Admin Endpoints Summary

| Category | Endpoint | Method | Description |
|----------|----------|--------|-------------|
| **Riders** | `/admin/riders/:id/verify` | POST | Verify rider |
| | `/admin/riders/:id/suspend` | POST | Suspend rider |
| | `/admin/riders/:id/unsuspend` | POST | Unsuspend rider |
| | `/admin/riders/pending` | GET | List pending riders |
| | `/admin/riders/statistics` | GET | Rider statistics |
| **Users** | `/admin/users` | GET | List all users |
| | `/admin/users/:id/suspend` | POST | Suspend user |
| | `/admin/users/:id/unsuspend` | POST | Unsuspend user |
| | `/admin/users/:id/verify-seller` | POST | Verify seller |
| | `/admin/users/statistics` | GET | User statistics |
| **Settings** | `/admin/settings/delivery-pricing` | GET | Get delivery pricing |
| | `/admin/settings/delivery-pricing` | PUT | Update delivery pricing |
| | `/admin/settings/platform-fees` | GET | Get platform fees |
| | `/admin/settings/platform-fees` | PUT | Update platform fees |
| **Analytics** | `/admin/analytics/revenue` | GET | Platform revenue |
| | `/admin/analytics/active-deliveries` | GET | Active deliveries |
| | `/admin/analytics/rider-performance` | GET | Rider performance |
| **Payment Requests** | `/admin/payment-requests` | GET | List payment requests |
| | `/admin/payment-requests/:id/cancel` | POST | Cancel payment request |
| | `/admin/payment-requests/expired` | GET | Expired requests |
| **Health** | `/admin/health/database` | GET | Database status |
| | `/admin/health/redis` | GET | Redis status |
| | `/admin/health/external-services` | GET | External services |
| **Audit Logs** | `/admin/audit-logs` | GET | List audit logs |
| | `/admin/audit-logs/user/:id` | GET | User audit trail |
| **Disputes** | `/admin/disputes` | GET | List disputes |
| | `/admin/disputes/:id` | GET | Dispute details |
| | `/admin/disputes/:id/resolve` | POST | Resolve dispute |
| | `/admin/disputes/:id/refund` | POST | Process refund |
| | `/admin/disputes/statistics` | GET | Dispute stats |
| **Commission** | `/admin/settings/commission` | GET | Get commission settings |
| | `/admin/settings/commission` | PUT | Update commission settings |
| **Wallets** | `/admin/wallets/user/:id` | GET | Get user wallet |
| | `/admin/wallets/user/:id/adjust` | POST | Adjust wallet balance |
| | `/admin/wallets/user/:id/transactions` | GET | Wallet transactions |

---
