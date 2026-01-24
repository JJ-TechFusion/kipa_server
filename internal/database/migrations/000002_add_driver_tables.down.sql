-- ============================================
-- ROLLBACK INTER-STATE DRIVER DELIVERY TABLES
-- ============================================

-- Remove driver_delivery_id from payment_requests
DROP INDEX IF EXISTS idx_payment_requests_driver_delivery;
ALTER TABLE payment_requests DROP COLUMN IF EXISTS driver_delivery_id;

-- Drop indexes
DROP INDEX IF EXISTS idx_driver_delivery_events_created;
DROP INDEX IF EXISTS idx_driver_delivery_events_delivery;
DROP INDEX IF EXISTS idx_driver_deliveries_unreachable;
DROP INDEX IF EXISTS idx_driver_deliveries_buyer;
DROP INDEX IF EXISTS idx_driver_deliveries_seller;
DROP INDEX IF EXISTS idx_driver_deliveries_awaiting;
DROP INDEX IF EXISTS idx_driver_deliveries_route;
DROP INDEX IF EXISTS idx_driver_deliveries_status;
DROP INDEX IF EXISTS idx_driver_deliveries_driver;
DROP INDEX IF EXISTS idx_driver_deliveries_trip;
DROP INDEX IF EXISTS idx_driver_deliveries_payment_request;
DROP INDEX IF EXISTS idx_driver_trips_active;
DROP INDEX IF EXISTS idx_driver_trips_route;
DROP INDEX IF EXISTS idx_driver_trips_status;
DROP INDEX IF EXISTS idx_driver_trips_driver;
DROP INDEX IF EXISTS idx_driver_profiles_route;
DROP INDEX IF EXISTS idx_driver_profiles_verified;
DROP INDEX IF EXISTS idx_driver_profiles_status;
DROP INDEX IF EXISTS idx_driver_profiles_user;

-- Drop tables (order matters for FK constraints)
DROP TABLE IF EXISTS driver_delivery_events;
DROP TABLE IF EXISTS driver_deliveries;

-- Remove FK constraint before dropping driver_trips
ALTER TABLE driver_profiles DROP CONSTRAINT IF EXISTS fk_driver_current_trip;

DROP TABLE IF EXISTS driver_trips;
DROP TABLE IF EXISTS driver_profiles;

-- Drop enums
DROP TYPE IF EXISTS driver_delivery_actor_type;
DROP TYPE IF EXISTS driver_delivery_status;
DROP TYPE IF EXISTS driver_trip_status;
DROP TYPE IF EXISTS driver_vehicle_type;
DROP TYPE IF EXISTS driver_status;
