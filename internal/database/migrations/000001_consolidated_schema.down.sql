-- ============================================
-- KIPA CONSOLIDATED MIGRATION v2.0 DOWN
-- Reverses 000001_consolidated_schema.up.sql
-- ============================================

-- Drop triggers
DROP TRIGGER IF EXISTS update_driver_offers_updated_at ON driver_offers;
DROP TRIGGER IF EXISTS update_driver_deliveries_updated_at ON driver_deliveries;
DROP TRIGGER IF EXISTS update_driver_trips_updated_at ON driver_trips;
DROP TRIGGER IF EXISTS update_driver_profiles_updated_at ON driver_profiles;
DROP TRIGGER IF EXISTS update_logistics_deliveries_updated_at ON logistics_deliveries;
DROP TRIGGER IF EXISTS update_errands_updated_at ON errands;
DROP TRIGGER IF EXISTS update_orders_updated_at ON orders;
DROP TRIGGER IF EXISTS update_top_ups_updated_at ON top_ups;
DROP TRIGGER IF EXISTS update_withdrawals_updated_at ON withdrawals;
DROP TRIGGER IF EXISTS update_virtual_accounts_updated_at ON virtual_accounts;
DROP TRIGGER IF EXISTS update_bank_accounts_updated_at ON bank_accounts;
DROP TRIGGER IF EXISTS update_delivery_jobs_updated_at ON delivery_jobs;
DROP TRIGGER IF EXISTS update_payment_requests_updated_at ON payment_requests;
DROP TRIGGER IF EXISTS update_wallets_updated_at ON wallets;
DROP TRIGGER IF EXISTS update_rider_profiles_updated_at ON rider_profiles;
DROP TRIGGER IF EXISTS update_users_updated_at ON users;

DROP FUNCTION IF EXISTS update_updated_at_column();

-- Drop Circular Foreign Keys first for safety
ALTER TABLE IF EXISTS payment_requests DROP CONSTRAINT IF EXISTS fk_pr_escrow;
ALTER TABLE IF EXISTS payment_requests DROP CONSTRAINT IF EXISTS fk_pr_delivery_job;
ALTER TABLE IF EXISTS payment_requests DROP CONSTRAINT IF EXISTS fk_pr_logistics_delivery;
ALTER TABLE IF EXISTS payment_requests DROP CONSTRAINT IF EXISTS fk_pr_return_job;
ALTER TABLE IF EXISTS payment_requests DROP CONSTRAINT IF EXISTS fk_pr_forced_return;
ALTER TABLE IF EXISTS payment_requests DROP CONSTRAINT IF EXISTS fk_pr_driver_delivery;
ALTER TABLE IF EXISTS delivery_jobs DROP CONSTRAINT IF EXISTS fk_dj_errand;
ALTER TABLE IF EXISTS delivery_jobs DROP CONSTRAINT IF EXISTS fk_dj_dispute;
ALTER TABLE IF EXISTS orders DROP CONSTRAINT IF EXISTS fk_orders_escrow;
ALTER TABLE IF EXISTS driver_profiles DROP CONSTRAINT IF EXISTS fk_driver_current_trip;

-- Drop admin/settings tables
DROP TABLE IF EXISTS admin_audit_logs;
DROP TABLE IF EXISTS platform_settings;

-- Drop support tables
DROP TABLE IF EXISTS payment_request_events;
DROP TABLE IF EXISTS chat_messages;
DROP TABLE IF EXISTS otp_verifications;
DROP TABLE IF EXISTS idempotency_keys;
DROP TABLE IF EXISTS rider_location_history;

-- Drop financial tables
DROP TABLE IF EXISTS commission_payments;
DROP TABLE IF EXISTS rider_commissions;
DROP TABLE IF EXISTS top_ups;
DROP TABLE IF EXISTS withdrawals;
DROP TABLE IF EXISTS virtual_accounts;
DROP TABLE IF EXISTS bank_accounts;
DROP TABLE IF EXISTS wallet_transactions;
DROP TABLE IF EXISTS wallets;

-- Drop driver tables
DROP TABLE IF EXISTS driver_offers;
DROP TABLE IF EXISTS driver_delivery_events;
DROP TABLE IF EXISTS driver_deliveries;
DROP TABLE IF EXISTS driver_trips;
DROP TABLE IF EXISTS driver_profiles;

-- Drop core business tables
DROP TABLE IF EXISTS logistics_evidence;
DROP TABLE IF EXISTS logistics_delivery_events;
DROP TABLE IF EXISTS logistics_deliveries;
DROP TABLE IF EXISTS errands;
DROP TABLE IF EXISTS disputes;
DROP TABLE IF EXISTS escrows;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS rider_job_offers;
DROP TABLE IF EXISTS delivery_jobs;
DROP TABLE IF EXISTS payment_requests;

-- Drop base tables
DROP TABLE IF EXISTS delivery_zones;
DROP TABLE IF EXISTS rider_profiles;
DROP TABLE IF EXISTS notification_logs;
DROP TABLE IF EXISTS user_device_tokens;
DROP TABLE IF EXISTS users;

-- Drop driver enums
DROP TYPE IF EXISTS driver_offer_status;
DROP TYPE IF EXISTS driver_delivery_actor_type;
DROP TYPE IF EXISTS driver_delivery_status;
DROP TYPE IF EXISTS driver_trip_status;
DROP TYPE IF EXISTS driver_vehicle_type;
DROP TYPE IF EXISTS driver_status;

-- Drop base enums
DROP TYPE IF EXISTS logistics_carrier;
DROP TYPE IF EXISTS logistics_delivery_status;
DROP TYPE IF EXISTS errand_status;
DROP TYPE IF EXISTS rider_commission_status;
DROP TYPE IF EXISTS top_up_status;
DROP TYPE IF EXISTS withdrawal_status;
DROP TYPE IF EXISTS order_status;
DROP TYPE IF EXISTS order_payment_status;
DROP TYPE IF EXISTS otp_purpose;
DROP TYPE IF EXISTS dispute_outcome;
DROP TYPE IF EXISTS dispute_status;
DROP TYPE IF EXISTS wallet_transaction_type;
DROP TYPE IF EXISTS escrow_status;
DROP TYPE IF EXISTS rider_offer_response;
DROP TYPE IF EXISTS rider_status;
DROP TYPE IF EXISTS delivery_job_status;
DROP TYPE IF EXISTS payment_request_status;
DROP TYPE IF EXISTS delivery_type;
DROP TYPE IF EXISTS vehicle_type;

-- Drop Extensions
DROP EXTENSION IF EXISTS "postgis";
DROP EXTENSION IF EXISTS "uuid-ossp";
