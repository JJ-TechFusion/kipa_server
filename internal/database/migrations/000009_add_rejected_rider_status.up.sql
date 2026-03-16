-- Add rejected status to rider_status enum
ALTER TYPE rider_status ADD VALUE IF NOT EXISTS 'rejected';
