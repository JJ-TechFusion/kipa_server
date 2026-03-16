-- Add return delivery claimed fields to logistics_deliveries
ALTER TABLE logistics_deliveries
ADD COLUMN IF NOT EXISTS return_delivery_claimed_at TIMESTAMPTZ,
ADD COLUMN IF NOT EXISTS return_confirmation_deadline TIMESTAMPTZ,
ADD COLUMN IF NOT EXISTS return_auto_escalation_at TIMESTAMPTZ;

-- Add index for return auto-escalation queries
CREATE INDEX IF NOT EXISTS idx_logistics_deliveries_return_auto_escalation
ON logistics_deliveries (return_auto_escalation_at)
WHERE status = 'return_delivery_claimed' AND return_auto_escalation_at IS NOT NULL;
