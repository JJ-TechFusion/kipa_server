-- ============================================
-- KIPA DATABASE SCHEMA v3.0 (DROP ALL)
-- ============================================

-- Drop views first
DROP VIEW IF EXISTS v_rider_commission_summary;
DROP VIEW IF EXISTS v_rider_earnings;
DROP VIEW IF EXISTS v_active_payment_requests;

-- Drop triggers
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

-- Drop tables in reverse dependency order
DROP TABLE IF EXISTS rider_location_history;
DROP TABLE IF EXISTS payment_request_events;
DROP TABLE IF EXISTS disputes;
DROP TABLE IF EXISTS otp_verifications;
DROP TABLE IF EXISTS idempotency_keys;
DROP TABLE IF EXISTS commission_payments;
DROP TABLE IF EXISTS rider_commissions;
DROP TABLE IF EXISTS top_ups;
DROP TABLE IF EXISTS withdrawals;
DROP TABLE IF EXISTS virtual_accounts;
DROP TABLE IF EXISTS bank_accounts;
DROP TABLE IF EXISTS wallet_transactions;
DROP TABLE IF EXISTS wallets;
DROP TABLE IF EXISTS rider_job_offers;
DROP TABLE IF EXISTS errands;
DROP TABLE IF EXISTS escrows;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS delivery_jobs;
DROP TABLE IF EXISTS payment_requests;
DROP TABLE IF EXISTS delivery_zones;
DROP TABLE IF EXISTS rider_profiles;
DROP TABLE IF EXISTS users;

-- Drop enums
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

-- Drop extensions (optional, usually keep these)
-- DROP EXTENSION IF EXISTS postgis;
-- DROP EXTENSION IF EXISTS "uuid-ossp";
