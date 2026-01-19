#!/bin/bash

# Development Authentication Helper Script
# This script helps you generate auth tokens for testing when Termii sender ID is not verified

set -e

API_URL="${API_URL:-http://localhost:8080}"
PHONE_NUMBER="${1:-+2348012345678}"
DEVICE_ID="dev-test-device-$(date +%s)"

echo "🔐 Kipa Development Authentication Helper"
echo "=========================================="
echo ""
echo "📱 Phone Number: $PHONE_NUMBER"
echo "📲 Device ID: $DEVICE_ID"
echo ""

# Step 1: Send OTP
echo "📤 Step 1: Sending OTP..."
OTP_RESPONSE=$(curl -s -X POST "$API_URL/api/v1/auth/otp/send" \
  -H "Content-Type: application/json" \
  -d "{\"phone_number\": \"$PHONE_NUMBER\", \"device_id\": \"$DEVICE_ID\"}")

if ! echo "$OTP_RESPONSE" | jq -e '.success' > /dev/null 2>&1; then
  echo "❌ Failed to send OTP:"
  echo "$OTP_RESPONSE" | jq .
  exit 1
fi

VERIFICATION_ID=$(echo "$OTP_RESPONSE" | jq -r '.data.verification_id')
echo "✅ OTP sent successfully"
echo "🔑 Verification ID: $VERIFICATION_ID"
echo ""

# Step 2: Get OTP from logs
echo "🔍 Step 2: Check Docker logs for your OTP..."
echo ""
echo "Run this command in another terminal to see the OTP:"
echo "  docker-compose logs api | grep '$VERIFICATION_ID' | grep 'otp_code'"
echo ""
echo "Or view the last few OTPs:"
echo "  docker-compose logs api --tail=20 | grep 'otp_code'"
echo ""
read -p "Enter the 6-digit OTP code: " OTP_CODE

echo "🔢 OTP Code: $OTP_CODE"
echo ""

# Step 3: Verify OTP
echo "✔️  Step 3: Verifying OTP..."
IDEMPOTENCY_KEY="dev-test-$(uuidgen | tr '[:upper:]' '[:lower:]')"

VERIFY_RESPONSE=$(curl -s -X POST "$API_URL/api/v1/auth/otp/verify" \
  -H "Content-Type: application/json" \
  -d "{
    \"phone_number\": \"$PHONE_NUMBER\",
    \"verification_id\": \"$VERIFICATION_ID\",
    \"otp_code\": \"$OTP_CODE\",
    \"device_id\": \"$DEVICE_ID\",
    \"idempotency_key\": \"$IDEMPOTENCY_KEY\"
  }")

if ! echo "$VERIFY_RESPONSE" | jq -e '.success' > /dev/null 2>&1; then
  echo "❌ Failed to verify OTP:"
  echo "$VERIFY_RESPONSE" | jq .
  exit 1
fi

ACCESS_TOKEN=$(echo "$VERIFY_RESPONSE" | jq -r '.data.access_token')
REFRESH_TOKEN=$(echo "$VERIFY_RESPONSE" | jq -r '.data.refresh_token')
USER_ID=$(echo "$VERIFY_RESPONSE" | jq -r '.data.user.id')

echo "✅ Authentication successful!"
echo ""
echo "=========================================="
echo "🎉 Your Authentication Tokens"
echo "=========================================="
echo ""
echo "👤 User ID: $USER_ID"
echo ""
echo "🔑 Access Token:"
echo "$ACCESS_TOKEN"
echo ""
echo "🔄 Refresh Token:"
echo "$REFRESH_TOKEN"
echo ""
echo "=========================================="
echo "📋 Usage Examples"
echo "=========================================="
echo ""
echo "# Test authenticated endpoint:"
echo "curl -X GET $API_URL/api/v1/auth/me \\"
echo "  -H 'Authorization: Bearer $ACCESS_TOKEN'"
echo ""
echo "# Export for use in other scripts:"
echo "export KIPA_ACCESS_TOKEN='$ACCESS_TOKEN'"
echo "export KIPA_USER_ID='$USER_ID'"
echo ""
