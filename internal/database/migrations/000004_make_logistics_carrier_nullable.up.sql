-- Make logistics carrier nullable to allow creation during payment verification
-- Seller will set carrier later when marking as shipped

ALTER TABLE logistics_deliveries 
ALTER COLUMN carrier DROP NOT NULL;