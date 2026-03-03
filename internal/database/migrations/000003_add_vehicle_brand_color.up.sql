-- Add vehicle brand and color columns to rider_profiles
ALTER TABLE rider_profiles ADD COLUMN IF NOT EXISTS vehicle_brand VARCHAR(100);
ALTER TABLE rider_profiles ADD COLUMN IF NOT EXISTS vehicle_color VARCHAR(50);
