-- Add new enum value for return_delivery_claimed status
ALTER TYPE logistics_delivery_status ADD VALUE IF NOT EXISTS 'return_delivery_claimed';
