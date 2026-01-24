-- Remove state fields from payment_requests
DROP INDEX IF EXISTS idx_payment_requests_interstate_route;

ALTER TABLE payment_requests
DROP COLUMN IF EXISTS pickup_state,
DROP COLUMN IF EXISTS dropoff_state;
