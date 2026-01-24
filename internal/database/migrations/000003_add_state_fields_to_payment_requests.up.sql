-- Add pickup_state and dropoff_state to payment_requests for inter-state deliveries
ALTER TABLE payment_requests
ADD COLUMN IF NOT EXISTS pickup_state VARCHAR(100),
ADD COLUMN IF NOT EXISTS dropoff_state VARCHAR(100);

-- Add index for inter-state route queries
CREATE INDEX IF NOT EXISTS idx_payment_requests_interstate_route
ON payment_requests(pickup_state, dropoff_state)
WHERE delivery_type = 'inter_state';
