-- Revert logistics carrier to NOT NULL
-- This will fail if there are any NULL values in the carrier column

ALTER TABLE logistics_deliveries 
ALTER COLUMN carrier SET NOT NULL;