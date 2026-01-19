# Development Authentication Guide

This guide explains how to test authentication in development mode when your Termii sender ID is not yet verified.

## Overview

In development mode (`SERVER_ENVIRONMENT=development`), the Kipa API automatically logs OTP codes to the console, allowing you to test authentication without needing SMS delivery to work.

## How It Works

When running in development mode:

1. **OTP Generation**: OTPs are generated and logged to the console with a 🔐 emoji
2. **SMS Failure Handling**: If SMS delivery fails (e.g., unverified sender ID), the request continues successfully instead of failing
3. **Console Logging**: The OTP code is visible in the Docker logs for easy retrieval

## Quick Start

### Option 1: Using the Helper Script (Recommended)

We've created a helper script that automates the entire authentication flow:

```bash
# Navigate to the project directory
cd /Users/jessejosiah/Go/kipa

# Run the script (uses default test phone number)
./scripts/dev_auth.sh

# Or specify a custom phone number
./scripts/dev_auth.sh +2348098765432
```

The script will:
1. Send an OTP request
2. Automatically retrieve the OTP from Docker logs
3. Verify the OTP
4. Display your access and refresh tokens

### Option 2: Manual Testing

#### Step 1: Send OTP Request

```bash
curl -X POST http://localhost:8080/api/v1/auth/otp/send \
  -H "Content-Type: application/json" \
  -d '{
    "phone_number": "+2348012345678",
    "device_id": "test-device-12345678"
  }'
```

**Response:**
```json
{
  "success": true,
  "data": {
    "verification_id": "b33a3b35-0ed7-4902-99ba-90daa7a9b801",
    "expires_at": "2026-01-16T18:22:03.496381301Z",
    "retry_after_seconds": 60
  }
}
```

#### Step 2: Retrieve OTP from Logs

Check the Docker logs to find the OTP code:

```bash
docker-compose logs api --tail=50 | grep "DEVELOPMENT MODE"
```

**Example Output:**
```
kipa-api  | 2026-01-16T18:17:03.527Z	INFO	service/auth_service.go:111	🔐 DEVELOPMENT MODE - OTP Generated	{"phone": "+2348012345678", "otp_code": "948262", "verification_id": "b33a3b35-0ed7-4902-99ba-90daa7a9b801"}
```

In this example, the OTP code is **948262**.

#### Step 3: Verify OTP

```bash
curl -X POST http://localhost:8080/api/v1/auth/otp/verify \
  -H "Content-Type: application/json" \
  -d '{
    "phone_number": "+2348012345678",
    "verification_id": "b33a3b35-0ed7-4902-99ba-90daa7a9b801",
    "otp_code": "948262",
    "device_id": "test-device-12345678",
    "idempotency_key": "test-idempotency-key-12345678901234567890"
  }'
```

**Response:**
```json
{
  "success": true,
  "data": {
    "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "refresh_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "expires_in": 899,
    "token_type": "Bearer",
    "user": {
      "id": "79dfd4f7-a6bf-4431-b4ee-5083273bbfd3",
      "phone_number": "+2348012345678",
      "phone_verified": true,
      "is_seller": false,
      "is_buyer": false,
      "is_rider": false,
      "seller_verified": false,
      "created_at": "2026-01-16T18:17:40.670161Z"
    },
    "is_new_user": true
  }
}
```

#### Step 4: Test Authenticated Endpoints

Use the access token to call protected endpoints:

```bash
curl -X GET http://localhost:8080/api/v1/auth/me \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN"
```

## Important Notes

### Request Requirements

1. **device_id**: Must be 16-64 characters long
2. **phone_number**: Must be in E.164 format (e.g., +2348012345678)
3. **idempotency_key**: Must be 32-64 characters long (prevents duplicate requests)
4. **otp_code**: Must be exactly 6 numeric digits

### Security Considerations

⚠️ **Development Mode Only**: This feature is ONLY active when `SERVER_ENVIRONMENT=development`. In production, SMS delivery failures will cause the request to fail as expected.

### Token Expiration

- **Access Token**: Expires in 15 minutes (default)
- **Refresh Token**: Expires in 7 days (default)

Use the refresh token endpoint to get a new access token:

```bash
curl -X POST http://localhost:8080/api/v1/auth/token/refresh \
  -H "Content-Type: application/json" \
  -d '{
    "refresh_token": "YOUR_REFRESH_TOKEN",
    "device_id": "test-device-12345678"
  }'
```

## Troubleshooting

### OTP Not Showing in Logs

If you don't see the OTP in the logs:

1. Verify you're running in development mode:
   ```bash
   docker-compose exec api printenv | grep SERVER_ENVIRONMENT
   ```
   Should output: `SERVER_ENVIRONMENT=development`

2. Check if the API container is running:
   ```bash
   docker-compose ps api
   ```

3. View real-time logs:
   ```bash
   docker-compose logs -f api
   ```

### Invalid OTP Error

- OTPs expire after 5 minutes (default)
- Maximum 3 verification attempts per OTP
- If you exceed attempts, request a new OTP

### Rate Limiting

- Maximum 5 OTP requests per phone number per hour (default)
- If rate limited, wait for the cooldown period

## Production Deployment

When deploying to production:

1. Set `SERVER_ENVIRONMENT=production` in your environment variables
2. Ensure your Termii sender ID is verified
3. OTP codes will NOT be logged to console
4. SMS delivery failures will properly fail requests

## Testing Other APIs

Once you have an access token, you can test any authenticated endpoint:

```bash
# Set your token as an environment variable
export KIPA_TOKEN="your_access_token_here"

# Test wallet endpoint
curl -X GET http://localhost:8080/api/v1/wallet \
  -H "Authorization: Bearer $KIPA_TOKEN"

# Test payment request creation
curl -X POST http://localhost:8080/api/v1/payment-requests \
  -H "Authorization: Bearer $KIPA_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "amount": 5000,
    "description": "Test payment",
    "buyer_phone": "+2348098765432"
  }'
```

## Additional Resources

- [API Documentation](../API_DOCUMENTATION.md)
- [Postman Collection](../Kipa_API_Postman_Collection.json)
- [Platform Structure](./platform_structure.md)
