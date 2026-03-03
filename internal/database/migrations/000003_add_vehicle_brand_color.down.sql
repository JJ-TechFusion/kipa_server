-- Remove vehicle brand and color columns from rider_profiles
ALTER TABLE rider_profiles DROP COLUMN IF EXISTS vehicle_brand;
ALTER TABLE rider_profiles DROP COLUMN IF EXISTS vehicle_color;
