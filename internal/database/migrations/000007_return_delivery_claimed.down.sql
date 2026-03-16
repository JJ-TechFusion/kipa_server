-- Remove return delivery claimed fields from logistics_deliveries
DROP INDEX IF EXISTS idx_logistics_deliveries_return_auto_escalation;

ALTER TABLE logistics_deliveries
DROP COLUMN IF EXISTS return_delivery_claimed_at,
DROP COLUMN IF EXISTS return_confirmation_deadline,
DROP COLUMN IF EXISTS return_auto_escalation_at;
