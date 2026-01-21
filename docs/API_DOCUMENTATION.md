# Kipa Platform API Documentation

Kipa is an escrow + logistics platform with three user roles:
- **Buyer**: Pays for items via escrow
- **Seller**: Creates payment requests, receives funds after confirmation
- **Rider**: Delivers items (intra-state only), earns delivery fees

Base URL: `/api/v1`

---

## Table of Contents

1. [Authentication](#authentication)
2. [Payment Requests (Seller)](#payment-requests-seller)
3. [Buyer / Payment](#buyer--payment)
4. [Rider](#rider)
5. [Wallet](#wallet)
6. [Webhooks](#webhooks)
7. [Paystack Integration](#paystack-integration)
8. [Admin Endpoints](#admin-endpoints)
9. [Domain Models](#domain-models)
10. [State Machines](#state-machines)
11. [Fee Structure](#fee-structure)

---

## Authentication

### Send OTP
Sends a one-time password to the user's phone number.

```
POST /auth/otp/send
```

**Request Body:**
```json
{
  "phone_number": "+2348012345678",
  "delivery_method": "sms"  // "sms" or "voice"
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "message": "OTP sent successfully",
    "expires_at": "2024-01-15T10:05:00Z"
  }
}
```

---

### Verify OTP
Verifies the OTP and returns authentication tokens.

```
POST /auth/otp/verify
```

**Request Body:**
```json
{
  "phone_number": "+2348012345678",
  "otp": "123456"
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "access_token": "eyJhbGciOiJIUzI1NiIs...",
    "refresh_token": "eyJhbGciOiJIUzI1NiIs...",
    "expires_at": "2024-01-15T11:00:00Z",
    "user": {
      "id": "uuid",
      "phone_number": "+2348012345678",
      "phone_verified": true,
      "first_name": "John",
      "last_name": "Doe",
      "is_seller": false,
      "is_buyer": false,
      "is_rider": false,
      "seller_verified": false
    }
  }
}
```

---

### Refresh Token
Refreshes the access token using the refresh token.

```
POST /auth/token/refresh
```

**Request Body:**
```json
{
  "refresh_token": "eyJhbGciOiJIUzI1NiIs..."
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "access_token": "eyJhbGciOiJIUzI1NiIs...",
    "refresh_token": "eyJhbGciOiJIUzI1NiIs...",
    "expires_at": "2024-01-15T12:00:00Z"
  }
}
```

---

### Logout
Revokes the current session.

```
POST /auth/logout
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "message": "Logged out successfully"
  }
}
```

---

### Get Current User
Returns the authenticated user's profile.

```
GET /auth/me
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "phone_number": "+2348012345678",
    "phone_verified": true,
    "email": "john@example.com",
    "first_name": "John",
    "last_name": "Doe",
    "is_seller": true,
    "is_buyer": true,
    "is_rider": false,
    "seller_verified": true,
    "profile_photo_url": "https://..."
  }
}
```

---

### Update Profile
Updates the authenticated user's profile (first name, last name, email, photo).

```
PATCH /auth/profile
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Request Body:**
```json
{
  "first_name": "John",
  "last_name": "Doe",
  "email": "john@example.com",
  "profile_photo_url": "https://example.com/photo.jpg"
}
```

All fields are optional. Only provided fields will be updated.

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "phone_number": "+2348012345678",
    "phone_verified": true,
    "email": "john@example.com",
    "first_name": "John",
    "last_name": "Doe",
    "is_seller": false,
    "is_buyer": true,
    "is_rider": false,
    "seller_verified": false,
    "profile_photo_url": "https://example.com/photo.jpg"
  }
}
```

---

## Payment Requests (Seller)

All endpoints require authentication.

### Create Draft Payment Request
Creates a new draft payment request (Step 1).

```
POST /payment-requests
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Request Body:**
```json
{
  "item_name": "iPhone 15 Pro Max",
  "item_description": "Brand new, sealed in box",
  "item_price": 850000.00,
  "item_images": ["https://example.com/image1.jpg"],
  "processing_time_hours": 24
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "seller_id": "uuid",
    "item_name": "iPhone 15 Pro Max",
    "item_description": "Brand new, sealed in box",
    "item_price": 850000.00,
    "delivery_type": "pickup",
    "status": "draft",
    "created_at": "2024-01-15T10:00:00Z"
  }
}
```

---

### List My Payment Requests
Lists all payment requests for the authenticated seller.

```
GET /payment-requests
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "payment_requests": [...],
    "count": 10
  }
}
```

---

### Get Payment Request by ID
Gets a specific payment request.

```
GET /payment-requests/:id
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "seller_id": "uuid",
    "item_name": "iPhone 15 Pro Max",
    "item_price": 850000.00,
    "delivery_type": "intra_state",
    "pickup_address": "123 Lagos Street",
    "dropoff_address": "456 Ikeja Avenue",
    "estimated_delivery_fee": 2500.00,
    "buyer_service_fee": 17000.00,
    "estimated_total": 869500.00,
    "payment_code": "KIP-ABC12345",
    "status": "link_active",
    "created_at": "2024-01-15T10:00:00Z"
  }
}
```

---

### Configure Fulfillment
Configures fulfillment details and generates payment link (Step 2).

```
PATCH /payment-requests/:id/fulfillment
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Request Body:**
```json
{
  "delivery_type": "intra_state",
  "pickup_address": "123 Lagos Street, Victoria Island",
  "pickup_lat": 6.4281,
  "pickup_lng": 3.4219,
  "dropoff_address": "456 Ikeja Avenue, Ikeja",
  "dropoff_lat": 6.6018,
  "dropoff_lng": 3.3515,
  "is_reusable": false,
  "max_uses": null
}
```

**Delivery Types:**
- `intra_state` - Internal riders, Google Maps tracking
- `inter_state` - Third-party logistics (GIG, Fedex, etc.)
- `pickup` - Buyer picks up from seller

**Link Usage Options:**
- `is_reusable: false` - Single-use link (expires after one payment) - DEFAULT
- `is_reusable: true` - Reusable link (can be used by multiple buyers)
- `max_uses: null` - Unlimited uses (only applies if is_reusable is true)
- `max_uses: 10` - Maximum 10 uses (only applies if is_reusable is true)

**Response:**
```json
{
  "success": true,
  "data": {
    "payment_request": {
      "id": "uuid",
      "status": "link_active",
      "payment_code": "KIP-ABC12345",
      "estimated_delivery_fee": 2500.00,
      "buyer_service_fee": 17000.00,
      "estimated_total": 869500.00,
      "is_reusable": false,
      "max_uses": null,
      "current_uses": 0
    },
    "payment_url": "https://kipa.ng/pay/KIP-ABC12345"
  }
}
```

---

### Mark Ready for Pickup
Marks the payment request as ready (triggers rider search for intra-state).

```
POST /payment-requests/:id/ready-for-pickup
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "status": "searching_rider",
    "ready_for_pickup_at": "2024-01-15T12:00:00Z"
  }
}
```

---

### Cancel Payment Request
Cancels a payment request (only if not paid).

```
DELETE /payment-requests/:id
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "message": "Payment request cancelled"
  }
}
```

---

### Edit Payment Request
Edits a payment request (only draft or link_active status).

```
PATCH /payment-requests/:id
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Request Body:**
```json
{
  "item_name": "Updated Item Name",
  "item_description": "Updated description",
  "item_price": 95000.00,
  "item_images": ["https://example.com/new-image.jpg"],
  "processing_time_hours": 48
}
```

All fields are optional. Only provided fields will be updated.

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "seller_id": "uuid",
    "item_name": "Updated Item Name",
    "item_price": 95000.00,
    "status": "draft",
    "created_at": "2024-01-15T10:00:00Z"
  }
}
```

**Error Responses:**
- `NOT_EDITABLE` (400) - Payment request cannot be edited (status is not draft or link_active)
- `UNAUTHORIZED` (403) - Not the owner of this payment request

---

### List Payment Request History
Lists expired, cancelled, completed, and refunded payment requests.

```
GET /payment-requests/history
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "payment_requests": [
      {
        "id": "uuid",
        "seller_id": "uuid",
        "item_name": "Old Item",
        "item_price": 50000.00,
        "status": "expired",
        "created_at": "2024-01-01T10:00:00Z"
      }
    ],
    "count": 1
  }
}
```

---

## Uploads

All upload endpoints require authentication and accept `multipart/form-data` with a `file` field.

**Allowed file types:** JPEG, PNG, WebP, GIF  
**Max file size:** 5MB (configurable)

### Upload Profile Picture

```
POST /uploads/profile-picture
```

**Request:** `multipart/form-data` with `file` field

**Response:**
```json
{
  "success": true,
  "data": {
    "url": "https://kipa-uploads.s3.amazonaws.com/profiles/uuid.jpg"
  }
}
```

---

### Upload Item Image

```
POST /uploads/item-image
```

**Response:**
```json
{
  "success": true,
  "data": {
    "url": "https://kipa-uploads.s3.amazonaws.com/items/uuid.jpg"
  }
}
```

---

### Upload Rider Document

```
POST /uploads/rider/:type
```

**Path Parameters:**
- `type` - Document type: `license`, `nin`, `face`, or `vehicle`

**Response:**
```json
{
  "success": true,
  "data": {
    "url": "https://kipa-uploads.s3.amazonaws.com/riders/license/uuid.jpg",
    "type": "license"
  }
}
```

**Error Responses:**
- `INVALID_FILE_TYPE` (400) - File type not allowed
- `FILE_TOO_LARGE` (400) - File exceeds max size
- `INVALID_DOC_TYPE` (400) - Invalid rider document type
- `UPLOAD_UNAVAILABLE` (503) - S3 not configured

---

## Buyer / Payment

### Get Payment Page (Public)
Returns the payment page data for a payment code. No authentication required.

```
GET /pay/:code
```

**Response:**
```json
{
  "success": true,
  "data": {
    "item_name": "iPhone 15 Pro Max",
    "item_description": "Brand new, sealed in box",
    "item_price": 850000.00,
    "delivery_type": "intra_state",
    "estimated_delivery_fee": 2500.00,
    "buyer_service_fee": 17000.00,
    "estimated_total": 869500.00,
    "seller_id": "uuid"
  }
}
```

---

### Complete Payment
Completes the payment and creates escrow. Buyer can choose to pay via wallet balance or card (Paystack).

```
POST /pay/:code/complete
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Request Body (Card Payment):**
```json
{
  "payment_method": "card",
  "payment_reference": "PAYSTACK-REF-123456",
  "payment_provider": "paystack"
}
```

**Request Body (Wallet Payment):**
```json
{
  "payment_method": "wallet"
}
```

**Payment Methods:**
- `card` - Pay via Paystack (requires `payment_reference` and `payment_provider`)
- `wallet` - Pay from wallet balance (must have sufficient balance)

**Response:**
```json
{
  "success": true,
  "data": {
    "message": "Payment completed successfully",
    "escrow_id": "uuid",
    "status": "paid_awaiting_fulfillment",
    "payment_method": "wallet"
  }
}
```

**Error Responses:**
- `INSUFFICIENT_BALANCE` (400) - Wallet balance too low for wallet payment
- `INVALID_INPUT` (400) - Missing payment_reference/payment_provider for card payment
- `LINK_USED` (400) - Single-use link already used
- `MAX_USES_REACHED` (400) - Reusable link reached maximum uses

---

### List Purchases
Lists all purchases for the authenticated buyer.

```
GET /purchases
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "purchases": [
      {
        "id": "uuid",
        "item_name": "iPhone 15 Pro Max",
        "item_price": 850000.00,
        "delivery_type": "intra_state",
        "status": "in_delivery",
        "seller_id": "uuid",
        "escrow_id": "uuid",
        "paid_at": "2024-01-15T10:30:00Z"
      }
    ],
    "count": 1
  }
}
```

---

### Get Purchase Details
Gets a specific purchase.

```
GET /purchases/:id
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "item_name": "iPhone 15 Pro Max",
    "item_price": 850000.00,
    "delivery_type": "intra_state",
    "estimated_delivery_fee": 2500.00,
    "actual_delivery_fee": 2800.00,
    "status": "delivered",
    "seller_id": "uuid",
    "escrow_id": "uuid",
    "paid_at": "2024-01-15T10:30:00Z",
    "delivered_at": "2024-01-15T14:00:00Z"
  }
}
```

---

### Confirm Delivery
Confirms delivery early and releases escrow to seller.

```
POST /purchases/:id/confirm
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "message": "Delivery confirmed. Payment released to seller."
  }
}
```

---

### Dispute Purchase
Opens a dispute on a purchase.

```
POST /purchases/:id/dispute
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Request Body:**
```json
{
  "reason": "Item received was damaged",
  "evidence": ["https://example.com/photo1.jpg", "https://example.com/photo2.jpg"]
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "message": "Dispute opened. Our team will review your case."
  }
}
```

---

## Rider

All endpoints require authentication.

### Get Profile
Gets the rider's profile.

```
GET /rider/profile
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "user_id": "uuid",
    "status": "verified",
    "is_online": true,
    "vehicle_type": "motorcycle",
    "vehicle_plate": "ABC-123-XY",
    "rating": 4.8,
    "total_deliveries": 156,
    "total_earnings": 234500.00,
    "created_at": "2024-01-01T00:00:00Z"
  }
}
```

---

### Create Profile
Creates a new rider profile.

```
POST /rider/profile
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Request Body:**
```json
{
  "license_number": "LIC-12345678",
  "nin_number": "12345678901",
  "license_photo_url": "https://example.com/license.jpg",
  "nin_photo_url": "https://example.com/nin.jpg",
  "face_verification_url": "https://example.com/face.jpg",
  "vehicle_type": "motorcycle",
  "vehicle_plate": "ABC-123-XY",
  "vehicle_photo_url": "https://example.com/vehicle.jpg"
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "user_id": "uuid",
    "status": "pending_verification",
    "is_online": false,
    "vehicle_type": "motorcycle",
    "vehicle_plate": "ABC-123-XY",
    "rating": 5.0,
    "total_deliveries": 0,
    "total_earnings": 0
  }
}
```

---

### Update Profile
Updates the rider's profile.

```
PATCH /rider/profile
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Request Body:**
```json
{
  "vehicle_type": "car",
  "vehicle_plate": "NEW-456-AB",
  "vehicle_photo_url": "https://example.com/new-vehicle.jpg"
}
```

---

### Go Online
Sets the rider as online and available for deliveries.

```
POST /rider/go-online
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "message": "You are now online and available for deliveries"
  }
}
```

---

### Go Offline
Sets the rider as offline.

```
POST /rider/go-offline
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "message": "You are now offline"
  }
}
```

---

### Update Location
Updates the rider's current location.

```
PATCH /rider/location
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Request Body:**
```json
{
  "latitude": 6.5244,
  "longitude": 3.3792
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "message": "Location updated"
  }
}
```

---

### Get Available Jobs
Gets delivery jobs available to the rider.

```
GET /rider/available-jobs
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "jobs": [
      {
        "id": "uuid",
        "delivery_job_id": "uuid",
        "distance_km": 3.5,
        "offered_at": "2024-01-15T12:00:00Z"
      }
    ],
    "count": 1
  }
}
```

---

### Accept Job
Accepts a delivery job.

```
POST /rider/jobs/:id/accept
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "message": "Job accepted successfully",
    "job": {
      "id": "uuid",
      "status": "accepted",
      "pickup_address": "123 Lagos Street",
      "dropoff_address": "456 Ikeja Avenue",
      "distance_km": 8.5,
      "delivery_fee": 2500.00,
      "rider_payout": 2125.00
    },
    "pickup_code": "123456"
  }
}
```

---

### Get Active Job
Gets the rider's current active job.

```
GET /rider/jobs/active
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "status": "picked_up",
    "pickup_address": "123 Lagos Street",
    "dropoff_address": "456 Ikeja Avenue",
    "distance_km": 8.5,
    "delivery_fee": 2500.00,
    "rider_payout": 2125.00,
    "pickup_code": "123456",
    "dropoff_code": "654321",
    "accepted_at": "2024-01-15T12:05:00Z",
    "picked_up_at": "2024-01-15T12:30:00Z"
  }
}
```

---

### Confirm Pickup
Confirms item pickup with verification code.

```
POST /rider/jobs/:id/pickup
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Request Body:**
```json
{
  "code": "123456"
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "message": "Pickup confirmed. Item collected successfully."
  }
}
```

---

### Confirm Delivery
Confirms item delivery with verification code.

```
POST /rider/jobs/:id/deliver
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Request Body:**
```json
{
  "code": "654321"
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "message": "Delivery confirmed. Payment will be credited to your wallet."
  }
}
```

---

### Get Job History
Gets the rider's completed jobs.

```
GET /rider/jobs/history
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "jobs": [...],
    "count": 50
  }
}
```

---

### Get Earnings
Gets the rider's earnings summary.

```
GET /rider/earnings
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "total_earnings": 234500.00,
    "total_deliveries": 156,
    "available_balance": 45000.00,
    "rating": 4.8
  }
}
```

---

## Wallet

All endpoints require authentication.

### Get Wallet Balance
Gets the user's wallet balance.

```
GET /wallet
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "available_balance": 125000.00,
    "locked_balance": 0.00,
    "total_earned": 500000.00,
    "total_withdrawn": 375000.00
  }
}
```

---

### Get Transactions
Gets the user's transaction history.

```
GET /wallet/transactions
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "transactions": [
      {
        "id": "uuid",
        "type": "escrow_release",
        "amount": 825000.00,
        "balance_after": 125000.00,
        "reference_type": "escrow",
        "reference_id": "uuid",
        "description": "Payment for order abc123",
        "created_at": "2024-01-15T14:00:00Z"
      },
      {
        "id": "uuid",
        "type": "withdrawal",
        "amount": -50000.00,
        "balance_after": 75000.00,
        "description": "Withdrawal to bank account",
        "created_at": "2024-01-14T10:00:00Z"
      }
    ],
    "count": 2
  }
}
```

**Transaction Types:**
- `escrow_fund` - Funds locked in escrow
- `escrow_release` - Funds released from escrow to seller
- `delivery_payout` - Rider delivery payment
- `platform_fee` - Platform fee deduction
- `withdrawal` - Withdrawal to bank account
- `top_up` - Wallet top-up
- `refund` - Refund to buyer
- `adjustment` - Manual adjustment

---

### Request Withdrawal
Initiates a withdrawal request.

```
POST /wallet/withdraw
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Request Body:**
```json
{
  "amount": 50000.00,
  "bank_code": "058",
  "account_number": "0123456789",
  "bank_account_name": "John Doe"
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "message": "Withdrawal request submitted successfully",
    "amount": 50000.00,
    "status": "processing",
    "idempotency_key": "uuid"
  }
}
```

---

### Wallet Top-up

#### Initialize Top-up
Initializes a wallet top-up via card payment.

```
POST /wallet/top-up
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Request Body:**
```json
{
  "amount": 5000.00,
  "callback_url": "https://yourapp.com/payment/callback"
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "top_up_id": "uuid",
    "payment_reference": "topup_abc123_1234567890",
    "authorization_url": "https://checkout.paystack.com/xxxxx",
    "access_code": "xxxxxxx",
    "public_key": "pk_live_xxxxx"
  }
}
```

**Minimum Amount:** ₦100

---

#### Verify Top-up
Verifies a top-up payment after completion.

```
POST /wallet/top-up/verify
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Request Body:**
```json
{
  "reference": "topup_abc123_1234567890"
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "message": "Top-up processed successfully"
  }
}
```

---

#### Get Top-up History
Returns the user's top-up history.

```
GET /wallet/top-up/history?limit=20&offset=0
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "top_ups": [
      {
        "id": "uuid",
        "amount": 5000.00,
        "payment_method": "card",
        "payment_ref": "topup_abc123_1234567890",
        "status": "success",
        "paid_at": "2024-01-15T10:30:00Z",
        "created_at": "2024-01-15T10:25:00Z"
      }
    ],
    "limit": 20,
    "offset": 0
  }
}
```

**Top-up Statuses:**
- `pending` - Waiting for payment
- `success` - Payment successful, wallet credited
- `failed` - Payment failed
- `abandoned` - User abandoned payment

---

### Virtual Account (DVA)

#### Get Virtual Account
Returns the user's dedicated virtual account for bank transfer funding.

```
GET /wallet/virtual-account
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Response (if exists):**
```json
{
  "success": true,
  "data": {
    "virtual_account": {
      "id": "uuid",
      "bank_name": "Wema Bank",
      "account_number": "7812345678",
      "account_name": "KIPA/JOHN DOE",
      "is_active": true
    }
  }
}
```

**Response (if not exists):**
```json
{
  "success": true,
  "data": {
    "virtual_account": null,
    "message": "No virtual account found. Create one to fund wallet via bank transfer."
  }
}
```

---

#### Create Virtual Account
Creates a dedicated virtual account for the user.

```
POST /wallet/virtual-account
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "virtual_account": {
      "id": "uuid",
      "bank_name": "Wema Bank",
      "account_number": "7812345678",
      "account_name": "KIPA/JOHN DOE",
      "is_active": true
    },
    "message": "Virtual account created. Transfer funds to this account to top up your wallet."
  }
}
```

**Note:** When users transfer money to this account, the funds are automatically credited to their wallet via Paystack webhooks.

---

### Bank Accounts (for Withdrawals)

#### List Banks
Returns the list of supported Nigerian banks.

```
GET /banks
```

**Response:**
```json
{
  "success": true,
  "data": {
    "banks": [
      {
        "code": "058",
        "name": "Guaranty Trust Bank",
        "slug": "gtb"
      },
      {
        "code": "044",
        "name": "Access Bank",
        "slug": "access-bank"
      }
    ]
  }
}
```

---

#### Add Bank Account
Adds and verifies a new bank account for withdrawals.

```
POST /wallet/bank-accounts
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Request Body:**
```json
{
  "bank_code": "058",
  "account_number": "0123456789"
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "bank_account": {
      "id": "uuid",
      "bank_code": "058",
      "bank_name": "Guaranty Trust Bank",
      "account_number": "0123456789",
      "account_name": "JOHN DOE",
      "is_default": true,
      "is_verified": true
    }
  }
}
```

**Notes:**
- Account number must be 10 digits
- Account is automatically verified with Paystack
- First added account becomes the default

---

#### Get Bank Accounts
Returns all bank accounts for the user.

```
GET /wallet/bank-accounts
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "bank_accounts": [
      {
        "id": "uuid",
        "bank_code": "058",
        "bank_name": "Guaranty Trust Bank",
        "account_number": "0123456789",
        "account_name": "JOHN DOE",
        "is_default": true,
        "is_verified": true,
        "created_at": "2024-01-15T10:00:00Z"
      }
    ]
  }
}
```

---

#### Set Default Bank Account
Sets a bank account as the default for withdrawals.

```
PUT /wallet/bank-accounts/{id}/default
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "message": "Default bank account updated"
  }
}
```

---

#### Delete Bank Account
Removes a bank account.

```
DELETE /wallet/bank-accounts/{id}
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "message": "Bank account deleted"
  }
}
```

---

### Withdrawals (v2)

#### Request Withdrawal
Initiates a withdrawal to a verified bank account.

```
POST /wallet/withdraw/v2
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Request Body:**
```json
{
  "bank_account_id": "uuid",
  "amount": 50000.00
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "withdrawal": {
      "id": "uuid",
      "amount": 50000.00,
      "fee": 50.00,
      "net_amount": 49950.00,
      "status": "processing",
      "transfer_ref": "wd_abc123_1234567890",
      "created_at": "2024-01-15T10:30:00Z"
    },
    "message": "Withdrawal initiated. You will receive the funds shortly."
  }
}
```

**Minimum Amount:** ₦1,000
**Withdrawal Fee:** ₦50 (fixed)

---

#### Get Withdrawal History
Returns the user's withdrawal history.

```
GET /wallet/withdrawals?limit=20&offset=0
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "withdrawals": [
      {
        "id": "uuid",
        "amount": 50000.00,
        "fee": 50.00,
        "net_amount": 49950.00,
        "status": "success",
        "transfer_ref": "wd_abc123_1234567890",
        "processed_at": "2024-01-15T10:35:00Z",
        "created_at": "2024-01-15T10:30:00Z"
      }
    ],
    "limit": 20,
    "offset": 0
  }
}
```

**Withdrawal Statuses:**
- `pending` - Withdrawal initiated
- `processing` - Transfer in progress
- `success` - Transfer completed
- `failed` - Transfer failed (wallet refunded)
- `reversed` - Transfer reversed (wallet refunded)

---

#### Get Withdrawal by ID
Returns a specific withdrawal.

```
GET /wallet/withdrawals/{id}
```

**Headers:**
```
Authorization: Bearer <access_token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "withdrawal": {
      "id": "uuid",
      "amount": 50000.00,
      "fee": 50.00,
      "net_amount": 49950.00,
      "status": "success",
      "transfer_ref": "wd_abc123_1234567890",
      "transfer_code": "TRF_xxxxx",
      "processed_at": "2024-01-15T10:35:00Z",
      "created_at": "2024-01-15T10:30:00Z"
    }
  }
}
```

---

## Webhooks

### Paystack Webhook
Receives webhook events from Paystack for payment notifications.

```
POST /webhooks/paystack
```

**Headers:**
```
X-Paystack-Signature: <signature>
Content-Type: application/json
```

**Supported Events:**
- `charge.success` - Payment completed successfully
- `transfer.success` - Bank transfer completed
- `transfer.failed` - Bank transfer failed
- `transfer.reversed` - Bank transfer reversed
- `refund.processed` - Refund completed
- `refund.failed` - Refund failed

**Example Webhook Payload (charge.success):**
```json
{
  "event": "charge.success",
  "data": {
    "id": 123456789,
    "domain": "live",
    "status": "success",
    "reference": "PAYSTACK-REF-123456",
    "amount": 51250000,
    "gateway_response": "Successful",
    "paid_at": "2024-01-15T10:30:00.000Z",
    "channel": "card",
    "currency": "NGN",
    "customer": {
      "email": "buyer@example.com",
      "customer_code": "CUS_xxxxx"
    }
  }
}
```

**Response:**
```json
{
  "message": "Charge success acknowledged",
  "reference": "PAYSTACK-REF-123456"
}
```

**Webhook Setup:**
1. Configure webhook URL in Paystack Dashboard: `https://yourdomain.com/api/v1/webhooks/paystack`
2. Set the webhook secret in your configuration
3. Enable the events you want to receive

---

## Paystack Integration

### Configuration
Add the following environment variables or config settings:

```yaml
paystack:
  secret_key: "sk_live_xxxxx"      # Your Paystack secret key
  public_key: "pk_live_xxxxx"      # Your Paystack public key (for frontend)
  base_url: "https://api.paystack.co"
  webhook_secret: "your_webhook_secret"
```

### Payment Flow

1. **Frontend initializes Paystack:**
   - Use Paystack Popup or Inline JS
   - Set callback URL to receive payment reference
   - Amount should match the estimated total from payment page

2. **Frontend calls Complete Payment endpoint:**
   ```json
   {
     "payment_method": "card",
     "payment_reference": "PAYSTACK-REF-123456"
   }
   ```

3. **Backend verifies with Paystack:**
   - Calls `GET /transaction/verify/{reference}` on Paystack API
   - Validates payment status is "success"
   - Validates amount matches expected total

4. **If verification passes:**
   - Creates escrow with funds
   - Updates payment request status
   - Returns success to buyer

### Frontend Integration Example (JavaScript)

```javascript
const paymentData = await fetch('/api/v1/pay/KIP-ABC123').then(r => r.json());

const handler = PaystackPop.setup({
  key: 'pk_live_xxxxx',
  email: 'buyer@example.com',
  amount: paymentData.data.estimated_total * 100, // Convert to kobo
  currency: 'NGN',
  ref: 'KIP-' + Date.now(), // Unique reference
  callback: function(response) {
    fetch('/api/v1/pay/KIP-ABC123/complete', {
      method: 'POST',
      headers: {
        'Authorization': 'Bearer ' + accessToken,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        payment_method: 'card',
        payment_reference: response.reference
      })
    });
  },
  onClose: function() {
    console.log('Payment cancelled');
  }
});

handler.openIframe();
```

---

## Domain Models

### Payment Request Status
```
draft                    → Initial creation
link_active              → Payment link generated
paid_awaiting_fulfillment → Buyer paid, waiting for seller
ready_for_pickup         → Seller marked ready
searching_rider          → Looking for rider (intra-state)
rider_assigned           → Rider accepted job
in_delivery              → Item being delivered
delivered                → Item delivered
confirmation_window      → 24h buyer confirmation period
completed                → Transaction complete
no_rider_found           → No rider available
expired                  → Link expired
cancelled                → Cancelled by seller
disputed                 → Under dispute
refunded                 → Refunded to buyer
```

### Delivery Job Status
```
created       → Job created
searching     → Searching for riders
accepted      → Rider accepted
en_route_pickup → Rider heading to pickup
picked_up     → Item picked up
in_transit    → In transit to buyer
delivered     → Delivered to buyer
paid_out      → Rider paid
timeout       → No rider found
cancelled     → Cancelled
failed        → Delivery failed
```

### Escrow Status
```
created              → Escrow created
funded               → Payment received
delivery_in_progress → Item being delivered
delivered            → Item delivered
confirmation_window  → 24h confirmation period
released             → Funds released to seller
disputed             → Under dispute
refunded             → Refunded to buyer
partial_refund       → Partial refund issued
```

### Rider Status
```
pending_verification → Awaiting verification
verified             → Verified and can work
suspended            → Temporarily suspended
deactivated          → Permanently deactivated
```

---

## State Machines

### Payment Request Flow
```
DRAFT
  → LINK_ACTIVE (fulfillment configured)
    → PAID_AWAITING_FULFILLMENT (buyer paid)
      → READY_FOR_PICKUP (seller ready)
        → SEARCHING_RIDER (intra-state only)
          → RIDER_ASSIGNED (rider accepted)
            → IN_DELIVERY (picked up)
              → DELIVERED
                → CONFIRMATION_WINDOW (24h)
                  → COMPLETED (confirmed or auto-release)
                  → DISPUTED (buyer disputed)
```

### Escrow Flow
```
CREATED
  → FUNDED (payment received)
    → DELIVERY_IN_PROGRESS (rider picked up)
      → CONFIRMATION_WINDOW (delivered)
        → RELEASED (confirmed or 24h auto-release)
        → DISPUTED (buyer opened dispute)
          → REFUNDED / RELEASED (dispute resolution)
```

---

## Fee Structure

### Buyer Sees (Payment Page)
```
Item Price:              ₦500,000
Estimated Delivery Fee:  ₦2,500*
Service Fee (2%):        ₦10,000
-----------------------------------
Estimated Total:         ₦512,500

* Delivery fee is estimated. Final fee depends on rider availability.
```

### Fee Calculation
```
buyer_service_fee    = item_price × 0.02 (2%)
seller_platform_fee  = item_price × 0.03 (3%)
delivery_fee         = zone.base_fee + (distance_km × zone.per_km_fee)
rider_payout         = delivery_fee × 0.85 (85%)
platform_delivery_cut = delivery_fee × 0.15 (15%)
```

### Settlement Distribution
```
seller_receives   = item_price - seller_platform_fee
rider_receives    = actual_delivery_fee × 0.85
platform_receives = buyer_service_fee + seller_platform_fee + (delivery_fee × 0.15)
```

---

## Error Codes

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `INVALID_INPUT` | 400 | Invalid request body |
| `INVALID_OTP` | 400 | Invalid OTP code |
| `OTP_EXPIRED` | 400 | OTP has expired |
| `OTP_MAX_ATTEMPTS` | 400 | Max OTP attempts exceeded |
| `OTP_RATE_LIMITED` | 429 | Too many OTP requests |
| `INVALID_TOKEN` | 401 | Invalid access token |
| `TOKEN_EXPIRED` | 401 | Access token expired |
| `UNAUTHORIZED` | 401 | Not authenticated |
| `USER_NOT_FOUND` | 404 | User not found |
| `USER_SUSPENDED` | 403 | User account suspended |
| `PAYMENT_REQUEST_NOT_FOUND` | 404 | Payment request not found |
| `INVALID_STATUS` | 400 | Invalid status transition |
| `LINK_EXPIRED` | 410 | Payment link expired |
| `ALREADY_PAID` | 400 | Already paid |
| `LINK_USED` | 400 | Single-use link already used |
| `MAX_USES_REACHED` | 400 | Reusable link reached max uses |
| `PAYMENT_VERIFICATION_FAILED` | 400 | Paystack verification failed |
| `PAYMENT_NOT_SUCCESSFUL` | 400 | Payment was not successful |
| `AMOUNT_MISMATCH` | 400 | Payment amount doesn't match |
| `CANCELLED` | 410 | Payment request cancelled |
| `CANNOT_CANCEL` | 400 | Cannot cancel after payment |
| `PROFILE_NOT_FOUND` | 404 | Rider profile not found |
| `NOT_VERIFIED` | 403 | Rider not verified |
| `ALREADY_ONLINE` | 400 | Rider already online |
| `NOT_ONLINE` | 400 | Rider not online |
| `JOB_ALREADY_ACCEPTED` | 409 | Job taken by another rider |
| `JOB_NOT_FOUND` | 404 | Delivery job not found |
| `INVALID_CODE` | 400 | Invalid verification code |
| `WALLET_NOT_FOUND` | 404 | Wallet not found |
| `INSUFFICIENT_BALANCE` | 400 | Insufficient balance |
| `DUPLICATE_REQUEST` | 409 | Duplicate idempotency key |
| `INTERNAL_ERROR` | 500 | Internal server error |

---

## Implementation Files

### Handlers
- `internal/handler/auth_handler.go` - Authentication endpoints
- `internal/handler/payment_request_handler.go` - Seller payment request endpoints
- `internal/handler/buyer_handler.go` - Buyer payment and purchase endpoints
- `internal/handler/rider_handler.go` - Rider profile and job endpoints
- `internal/handler/wallet_handler.go` - Wallet endpoints

### Services
- `internal/service/auth_service.go` - Authentication logic
- `internal/service/payment_request_service.go` - Payment request business logic
- `internal/service/escrow_service.go` - Escrow management
- `internal/service/rider_service.go` - Rider operations
- `internal/service/wallet_service.go` - Wallet operations with idempotency
- `internal/service/fee_calculator_service.go` - Fee calculation

### Repositories
- `internal/repository/postgres/user_repo.go`
- `internal/repository/postgres/payment_request_repo.go`
- `internal/repository/postgres/escrow_repo.go`
- `internal/repository/postgres/delivery_job_repo.go`
- `internal/repository/postgres/rider_profile_repo.go`
- `internal/repository/postgres/wallet_repo.go`
- `internal/repository/postgres/delivery_zone_repo.go`

### Domain Models
- `internal/domain/user.go`
- `internal/domain/payment_request.go`
- `internal/domain/escrow.go`
- `internal/domain/delivery_job.go`
- `internal/domain/rider_profile.go`
- `internal/domain/wallet.go`
- `internal/domain/delivery_zone.go`
- `internal/domain/errors.go`

### Database Migration
- `internal/database/migrations/002_platform_tables.sql`
