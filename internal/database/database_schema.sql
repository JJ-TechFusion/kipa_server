-- ============================================
-- KIPA DATABASE SCHEMA v3.0 (CONSOLIDATED)
-- Escrow + Logistics Platform
-- All migrations merged: 000001-000010
-- ============================================

-- ============================================
-- EXTENSIONS
-- ============================================

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "postgis";

-- ============================================
-- ENUMS
-- ============================================

-- Vehicle type for deliveries
CREATE TYPE vehicle_type AS ENUM ('motorcycle', 'car');

-- Delivery type determines the fulfillment flow
CREATE TYPE delivery_type AS ENUM (
    'intra_state',   -- Internal riders, Google Maps tracking
    'inter_state',   -- Third-party logistics (GIG, Fedex)
    'pickup'         -- Buyer picks up from seller
);

-- Payment request lifecycle
CREATE TYPE payment_request_status AS ENUM (
    'draft',                      -- Item details only, no link
    'link_active',                -- Fulfillment configured, link generated
    'paid_awaiting_fulfillment',  -- Buyer paid, seller preparing
    'ready_for_pickup',           -- Seller ready (inter-state/pickup)
    'searching_rider',            -- Intra-state: looking for rider
    'rider_assigned',             -- Intra-state: rider accepted
    'in_delivery',                -- Item in transit
    'delivered',                  -- Item delivered
    'confirmation_window',        -- 24h buyer confirmation period
    'completed',                  -- Escrow released
    'no_rider_found',             -- Intra-state: no riders available
    'expired',                    -- Link expired before payment
    'cancelled',                  -- Cancelled before payment
    'disputed',                   -- Buyer opened dispute
    'refunded'                    -- Full refund issued
);

-- Delivery job lifecycle (intra-state only)
CREATE TYPE delivery_job_status AS ENUM (
    'created',           -- Job created, not yet searching
    'searching',         -- Broadcasting to riders
    'accepted',          -- Rider accepted
    'en_route_pickup',   -- Rider heading to pickup
    'picked_up',         -- Item picked up
    'in_transit',        -- Item in transit to dropoff
    'delivered',         -- Item delivered
    'paid_out',          -- Rider paid
    'timeout',           -- No rider accepted in time
    'cancelled',         -- Job cancelled
    'failed'             -- Delivery failed
);

-- Rider verification status
CREATE TYPE rider_status AS ENUM (
    'pending_verification',
    'verified',
    'suspended',
    'deactivated'
);

-- Rider's response to job offer
CREATE TYPE rider_offer_response AS ENUM (
    'pending',
    'accepted',
    'declined',
    'expired'
);

-- Escrow lifecycle
CREATE TYPE escrow_status AS ENUM (
    'created',              -- Initial state
    'funded',               -- Payment received
    'delivery_in_progress', -- Being delivered
    'delivered',            -- Item delivered
    'confirmation_window',  -- Waiting for buyer confirmation
    'released',             -- Funds released to seller
    'disputed',             -- Under dispute
    'refunded',             -- Full refund
    'partial_refund'        -- Partial refund after dispute
);

-- Wallet transaction types
CREATE TYPE wallet_transaction_type AS ENUM (
    'escrow_fund',      -- Buyer pays into escrow
    'escrow_release',   -- Seller receives from escrow
    'delivery_payout',  -- Rider receives delivery fee
    'platform_fee',     -- Platform revenue
    'withdrawal',       -- User withdraws to bank
    'top_up',           -- User tops up wallet
    'refund',           -- Refund to buyer
    'adjustment'        -- Admin adjustment
);

-- Dispute status
CREATE TYPE dispute_status AS ENUM (
    'open',
    'under_review',
    'resolved',
    'closed'
);

-- Dispute outcome
CREATE TYPE dispute_outcome AS ENUM (
    'pending',
    'buyer_favor',
    'seller_favor',
    'rider_fault',
    'no_fault',
    'partial_refund'
);

-- OTP purpose
CREATE TYPE otp_purpose AS ENUM (
    'login',
    'signup',
    'verify'
);

-- Order payment status
CREATE TYPE order_payment_status AS ENUM (
    'pending',
    'initialized',
    'verified',
    'failed'
);

-- Order status
CREATE TYPE order_status AS ENUM (
    'pending',
    'paid',
    'processing',
    'ready',
    'in_delivery',
    'delivered',
    'completed',
    'cancelled',
    'disputed'
);

-- Withdrawal status
CREATE TYPE withdrawal_status AS ENUM (
    'pending',
    'processing',
    'success',
    'failed',
    'reversed'
);

-- Top-up status
CREATE TYPE top_up_status AS ENUM (
    'pending',
    'success',
    'failed',
    'abandoned'
);

-- Rider commission status
CREATE TYPE rider_commission_status AS ENUM ('pending', 'paid', 'waived');

-- Errand status
CREATE TYPE errand_status AS ENUM (
    'draft',
    'searching',
    'accepted',
    'picked_up',
    'in_transit',
    'delivered',
    'cancelled'
);

-- ============================================
-- USERS (Multi-role via flags)
-- ============================================

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    phone_number VARCHAR(20) UNIQUE NOT NULL,
    phone_verified BOOLEAN DEFAULT FALSE,
    email VARCHAR(255),
    first_name VARCHAR(100),
    last_name VARCHAR(100),
    profile_photo_url TEXT,

    -- Role flags (user can have multiple roles)
    is_seller BOOLEAN DEFAULT FALSE,
    is_buyer BOOLEAN DEFAULT FALSE,
    is_rider BOOLEAN DEFAULT FALSE,

    -- Seller verification
    seller_verified BOOLEAN DEFAULT FALSE,
    seller_verified_at TIMESTAMP WITH TIME ZONE,

    -- Status
    is_suspended BOOLEAN DEFAULT FALSE,
    suspension_reason TEXT,
    last_login_at TIMESTAMP WITH TIME ZONE,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_users_phone ON users(phone_number);
CREATE INDEX idx_users_email ON users(email);

-- ============================================
-- RIDER PROFILES
-- ============================================

CREATE TABLE rider_profiles (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID UNIQUE REFERENCES users(id) ON DELETE CASCADE,

    -- Verification documents
    license_number VARCHAR(100) UNIQUE,
    nin_number VARCHAR(20) UNIQUE,
    license_photo_url TEXT,
    nin_photo_url TEXT,
    face_verification_url TEXT,

    -- Vehicle info
    vehicle_type VARCHAR(50),
    vehicle_plate VARCHAR(20),
    vehicle_photo_url TEXT,

    -- Status and availability
    status rider_status DEFAULT 'pending_verification',
    is_online BOOLEAN DEFAULT FALSE,
    current_location GEOGRAPHY(POINT, 4326),
    last_location_update TIMESTAMP WITH TIME ZONE,

    -- Performance
    rating DECIMAL(2,1) DEFAULT 5.0,
    total_deliveries INT DEFAULT 0,
    total_earnings DECIMAL(18,2) DEFAULT 0,

    -- Commission tracking
    unpaid_commission DECIMAL(18,2) DEFAULT 0,
    unpaid_delivery_count INT DEFAULT 0,
    last_commission_paid_at TIMESTAMP WITH TIME ZONE,
    is_commission_blocked BOOLEAN DEFAULT FALSE,

    -- Location sharing
    location_sharing_enabled BOOLEAN NOT NULL DEFAULT true,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_rider_profiles_location ON rider_profiles USING GIST(current_location);
CREATE INDEX idx_rider_profiles_status ON rider_profiles(status, is_online);
CREATE INDEX idx_rider_profiles_commission_blocked ON rider_profiles(is_commission_blocked) WHERE is_commission_blocked = TRUE;
CREATE INDEX idx_rider_location_sharing ON rider_profiles(id, location_sharing_enabled);

-- ============================================
-- DELIVERY ZONES
-- ============================================

CREATE TABLE delivery_zones (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(100) NOT NULL,
    city VARCHAR(100) NOT NULL,
    state VARCHAR(100) NOT NULL,
    polygon GEOGRAPHY(POLYGON, 4326),

    initial_search_radius_km INT DEFAULT 3,
    max_search_radius_km INT DEFAULT 10,
    radius_expand_interval_seconds INT DEFAULT 30,

    base_delivery_fee DECIMAL(18,2) NOT NULL,
    per_km_fee DECIMAL(18,2) NOT NULL,

    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_delivery_zones_polygon ON delivery_zones USING GIST(polygon);

-- ============================================
-- PAYMENT REQUESTS
-- ============================================

CREATE TABLE payment_requests (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    seller_id UUID REFERENCES users(id),

    -- Item details
    item_name VARCHAR(255) NOT NULL,
    item_description TEXT,
    item_price DECIMAL(18,2) NOT NULL,
    item_images JSONB DEFAULT '[]',
    processing_time_hours INT,

    -- Fulfillment details
    delivery_type delivery_type DEFAULT 'pickup',
    vehicle_type VARCHAR(20),
    pickup_address TEXT,
    pickup_location GEOGRAPHY(POINT, 4326),
    dropoff_address TEXT,
    dropoff_location GEOGRAPHY(POINT, 4326),

    -- Estimated fees
    estimated_delivery_fee DECIMAL(18,2),
    buyer_service_fee DECIMAL(18,2),
    seller_platform_fee DECIMAL(18,2),
    estimated_total DECIMAL(18,2),

    -- Actual fees
    actual_delivery_fee DECIMAL(18,2),
    actual_total DECIMAL(18,2),

    -- Payment link
    payment_code VARCHAR(20) UNIQUE,
    link_expires_at TIMESTAMP WITH TIME ZONE,

    -- Reusable links
    is_reusable BOOLEAN DEFAULT FALSE,
    max_uses INT,
    current_uses INT DEFAULT 0,

    -- Inter-state tracking
    logistics_carrier VARCHAR(100),
    tracking_number VARCHAR(255),
    tracking_url TEXT,

    -- Status
    status payment_request_status DEFAULT 'draft',

    -- References
    escrow_id UUID,
    delivery_job_id UUID,
    buyer_id UUID REFERENCES users(id),

    -- Reminder tracking
    last_reminder_sent_at TIMESTAMP,
    reminder_count INT DEFAULT 0,

    -- Timestamps
    link_generated_at TIMESTAMP WITH TIME ZONE,
    paid_at TIMESTAMP WITH TIME ZONE,
    ready_for_pickup_at TIMESTAMP WITH TIME ZONE,
    delivered_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_payment_requests_seller ON payment_requests(seller_id);
CREATE INDEX idx_payment_requests_buyer ON payment_requests(buyer_id);
CREATE INDEX idx_payment_requests_code ON payment_requests(payment_code);
CREATE INDEX idx_payment_requests_status ON payment_requests(status);
CREATE INDEX idx_payment_requests_pickup_location ON payment_requests USING GIST(pickup_location);
CREATE INDEX idx_payment_requests_link_expiry ON payment_requests(link_expires_at) WHERE status = 'link_active';
CREATE INDEX idx_payment_requests_reminder ON payment_requests(last_reminder_sent_at) WHERE status = 'paid_awaiting_fulfillment';

-- ============================================
-- DELIVERY JOBS
-- ============================================

CREATE TABLE delivery_jobs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    payment_request_id UUID REFERENCES payment_requests(id),
    rider_id UUID REFERENCES rider_profiles(id),

    -- Locations
    pickup_address TEXT NOT NULL,
    pickup_location GEOGRAPHY(POINT, 4326) NOT NULL,
    dropoff_address TEXT NOT NULL,
    dropoff_location GEOGRAPHY(POINT, 4326) NOT NULL,
    distance_km DECIMAL(5,2),

    -- Fees
    delivery_fee DECIMAL(18,2) NOT NULL,
    rider_payout DECIMAL(18,2),
    platform_delivery_fee DECIMAL(18,2),

    -- Vehicle type
    required_vehicle_type vehicle_type DEFAULT 'motorcycle',

    -- Verification codes
    pickup_code VARCHAR(6),
    dropoff_code VARCHAR(6),

    -- Search tracking
    search_radius_km INT DEFAULT 5,
    search_started_at TIMESTAMP WITH TIME ZONE,
    search_timeout_at TIMESTAMP WITH TIME ZONE,
    last_radius_expand_at TIMESTAMP,

    -- ETA tracking
    current_eta_minutes INT,
    eta_updated_at TIMESTAMP WITH TIME ZONE,

    -- Status
    status delivery_job_status DEFAULT 'created',

    -- Source tracking
    source_type VARCHAR(20) DEFAULT 'payment_request',
    errand_id UUID,

    -- Timestamps
    accepted_at TIMESTAMP WITH TIME ZONE,
    picked_up_at TIMESTAMP WITH TIME ZONE,
    delivered_at TIMESTAMP WITH TIME ZONE,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_delivery_jobs_payment_request ON delivery_jobs(payment_request_id);
CREATE INDEX idx_delivery_jobs_rider ON delivery_jobs(rider_id);
CREATE INDEX idx_delivery_jobs_status ON delivery_jobs(status);
CREATE INDEX idx_delivery_jobs_pickup ON delivery_jobs USING GIST(pickup_location);
CREATE INDEX idx_delivery_jobs_searching_timeout ON delivery_jobs(search_timeout_at) WHERE status = 'searching';
CREATE INDEX idx_delivery_jobs_radius_expand ON delivery_jobs(last_radius_expand_at) WHERE status = 'searching';
CREATE INDEX idx_delivery_jobs_vehicle_type ON delivery_jobs(required_vehicle_type) WHERE status = 'searching';

-- ============================================
-- RIDER JOB OFFERS
-- ============================================

CREATE TABLE rider_job_offers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    delivery_job_id UUID REFERENCES delivery_jobs(id),
    rider_id UUID REFERENCES rider_profiles(id),
    distance_km DECIMAL(5,2),
    offered_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    response rider_offer_response DEFAULT 'pending',
    responded_at TIMESTAMP WITH TIME ZONE,

    UNIQUE(delivery_job_id, rider_id)
);

CREATE INDEX idx_rider_job_offers_job ON rider_job_offers(delivery_job_id);
CREATE INDEX idx_rider_job_offers_rider ON rider_job_offers(rider_id);

-- ============================================
-- ORDERS
-- ============================================

CREATE TABLE orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_request_id UUID NOT NULL REFERENCES payment_requests(id),
    buyer_id UUID NOT NULL REFERENCES users(id),
    escrow_id UUID,

    payment_method VARCHAR(20) NOT NULL DEFAULT 'card',
    paystack_reference VARCHAR(255) UNIQUE,
    paystack_access_code VARCHAR(255),
    payment_status order_payment_status DEFAULT 'pending',

    item_amount DECIMAL(18,2) NOT NULL,
    delivery_fee_estimate DECIMAL(18,2) NOT NULL DEFAULT 0,
    buyer_service_fee DECIMAL(18,2) NOT NULL,
    total_amount DECIMAL(18,2) NOT NULL,

    status order_status DEFAULT 'pending',

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    paid_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_orders_payment_request ON orders(payment_request_id);
CREATE INDEX idx_orders_buyer ON orders(buyer_id);
CREATE INDEX idx_orders_paystack_ref ON orders(paystack_reference) WHERE paystack_reference IS NOT NULL;
CREATE INDEX idx_orders_status ON orders(status);
CREATE INDEX idx_orders_payment_status ON orders(payment_status);

-- ============================================
-- ESCROWS
-- ============================================

CREATE TABLE escrows (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    payment_request_id UUID UNIQUE REFERENCES payment_requests(id),
    order_id UUID REFERENCES orders(id),
    buyer_id UUID REFERENCES users(id),
    seller_id UUID REFERENCES users(id),

    item_amount DECIMAL(18,2) NOT NULL,
    delivery_fee_estimate DECIMAL(18,2) NOT NULL,
    buyer_fee DECIMAL(18,2) NOT NULL,
    total_locked DECIMAL(18,2) NOT NULL,

    actual_delivery_fee DECIMAL(18,2),
    delivery_fee_adjustment DECIMAL(18,2) DEFAULT 0,

    seller_payout DECIMAL(18,2),
    rider_payout DECIMAL(18,2),
    platform_revenue DECIMAL(18,2),

    payment_reference VARCHAR(255),
    payment_provider VARCHAR(50),

    status escrow_status DEFAULT 'created',

    funded_at TIMESTAMP WITH TIME ZONE,
    delivery_confirmed_at TIMESTAMP WITH TIME ZONE,
    confirmation_window_ends_at TIMESTAMP WITH TIME ZONE,
    released_at TIMESTAMP WITH TIME ZONE,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_escrows_payment_request ON escrows(payment_request_id);
CREATE INDEX idx_escrows_order ON escrows(order_id) WHERE order_id IS NOT NULL;
CREATE INDEX idx_escrows_buyer ON escrows(buyer_id);
CREATE INDEX idx_escrows_seller ON escrows(seller_id);
CREATE INDEX idx_escrows_status ON escrows(status);
CREATE INDEX idx_escrows_confirmation_window ON escrows(confirmation_window_ends_at) WHERE status = 'confirmation_window';

-- ============================================
-- WALLETS
-- ============================================

CREATE TABLE wallets (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    available_balance DECIMAL(18,2) NOT NULL DEFAULT 0,
    locked_balance DECIMAL(18,2) NOT NULL DEFAULT 0,
    total_earned DECIMAL(18,2) NOT NULL DEFAULT 0,
    total_withdrawn DECIMAL(18,2) NOT NULL DEFAULT 0,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    CONSTRAINT positive_available CHECK (available_balance >= 0),
    CONSTRAINT positive_locked CHECK (locked_balance >= 0)
);

-- ============================================
-- WALLET TRANSACTIONS
-- ============================================

CREATE TABLE wallet_transactions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    wallet_id UUID REFERENCES wallets(id),
    type wallet_transaction_type NOT NULL,
    amount DECIMAL(18,2) NOT NULL,
    balance_after DECIMAL(18,2) NOT NULL,

    reference_type VARCHAR(50),
    reference_id UUID,

    description TEXT,
    idempotency_key VARCHAR(255) UNIQUE,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_wallet_transactions_wallet ON wallet_transactions(wallet_id);
CREATE INDEX idx_wallet_transactions_type ON wallet_transactions(type);
CREATE INDEX idx_wallet_transactions_idempotency ON wallet_transactions(idempotency_key);

-- ============================================
-- BANK ACCOUNTS
-- ============================================

CREATE TABLE bank_accounts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    bank_code VARCHAR(10) NOT NULL,
    bank_name VARCHAR(100) NOT NULL,
    account_number VARCHAR(20) NOT NULL,
    account_name VARCHAR(255) NOT NULL,
    recipient_code VARCHAR(100),
    is_default BOOLEAN DEFAULT FALSE,
    is_verified BOOLEAN DEFAULT FALSE,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    UNIQUE(user_id, account_number, bank_code)
);

CREATE INDEX idx_bank_accounts_user ON bank_accounts(user_id);
CREATE INDEX idx_bank_accounts_default ON bank_accounts(user_id, is_default) WHERE is_default = TRUE;

-- ============================================
-- VIRTUAL ACCOUNTS
-- ============================================

CREATE TABLE virtual_accounts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    bank_name VARCHAR(100) NOT NULL,
    bank_slug VARCHAR(50),
    account_number VARCHAR(20) NOT NULL,
    account_name VARCHAR(255) NOT NULL,
    customer_code VARCHAR(100) NOT NULL,
    dedicated_acct_id BIGINT,
    is_active BOOLEAN DEFAULT TRUE,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_virtual_accounts_user ON virtual_accounts(user_id);
CREATE INDEX idx_virtual_accounts_account_number ON virtual_accounts(account_number);
CREATE INDEX idx_virtual_accounts_customer_code ON virtual_accounts(customer_code);

-- ============================================
-- WITHDRAWALS
-- ============================================

CREATE TABLE withdrawals (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id),
    wallet_id UUID REFERENCES wallets(id),
    bank_account_id UUID REFERENCES bank_accounts(id),

    amount DECIMAL(18,2) NOT NULL,
    fee DECIMAL(18,2) NOT NULL DEFAULT 0,
    net_amount DECIMAL(18,2) NOT NULL,

    status withdrawal_status DEFAULT 'pending',
    transfer_code VARCHAR(100),
    transfer_ref VARCHAR(100) NOT NULL,
    failure_reason TEXT,
    processed_at TIMESTAMP WITH TIME ZONE,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    CONSTRAINT positive_withdrawal_amount CHECK (amount > 0),
    CONSTRAINT positive_net_amount CHECK (net_amount >= 0)
);

CREATE INDEX idx_withdrawals_user ON withdrawals(user_id);
CREATE INDEX idx_withdrawals_wallet ON withdrawals(wallet_id);
CREATE INDEX idx_withdrawals_status ON withdrawals(status);
CREATE INDEX idx_withdrawals_transfer_ref ON withdrawals(transfer_ref);

-- ============================================
-- TOP-UPS
-- ============================================

CREATE TABLE top_ups (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id),
    wallet_id UUID REFERENCES wallets(id),

    amount DECIMAL(18,2) NOT NULL,
    payment_method VARCHAR(50) NOT NULL,
    payment_ref VARCHAR(100) NOT NULL,
    payment_provider VARCHAR(50) DEFAULT 'paystack',

    status top_up_status DEFAULT 'pending',
    paid_at TIMESTAMP WITH TIME ZONE,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    CONSTRAINT positive_topup_amount CHECK (amount > 0)
);

CREATE INDEX idx_top_ups_user ON top_ups(user_id);
CREATE INDEX idx_top_ups_wallet ON top_ups(wallet_id);
CREATE INDEX idx_top_ups_status ON top_ups(status);
CREATE INDEX idx_top_ups_payment_ref ON top_ups(payment_ref);

-- ============================================
-- RIDER COMMISSIONS
-- ============================================

CREATE TABLE rider_commissions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    rider_profile_id UUID REFERENCES rider_profiles(id) ON DELETE CASCADE,
    delivery_job_id UUID UNIQUE REFERENCES delivery_jobs(id),
    
    delivery_fee DECIMAL(18,2) NOT NULL,
    commission_rate DECIMAL(5,4) NOT NULL DEFAULT 0.1500,
    commission_amount DECIMAL(18,2) NOT NULL,
    
    status rider_commission_status DEFAULT 'pending',
    paid_at TIMESTAMP WITH TIME ZONE,
    paid_via VARCHAR(20),
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_rider_commissions_rider ON rider_commissions(rider_profile_id);
CREATE INDEX idx_rider_commissions_status ON rider_commissions(status);
CREATE INDEX idx_rider_commissions_pending ON rider_commissions(rider_profile_id, status) WHERE status = 'pending';

-- ============================================
-- COMMISSION PAYMENTS
-- ============================================

CREATE TABLE commission_payments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    rider_profile_id UUID REFERENCES rider_profiles(id),
    
    amount DECIMAL(18,2) NOT NULL,
    commissions_paid INT NOT NULL,
    
    payment_method VARCHAR(50) NOT NULL,
    wallet_transaction_id UUID REFERENCES wallet_transactions(id),
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_commission_payments_rider ON commission_payments(rider_profile_id);

-- ============================================
-- ERRANDS
-- ============================================

CREATE TABLE errands (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    
    pickup_address TEXT NOT NULL,
    pickup_lat DECIMAL(10,8) NOT NULL,
    pickup_lng DECIMAL(11,8) NOT NULL,
    pickup_instructions TEXT,
    
    dropoff_address TEXT NOT NULL,
    dropoff_lat DECIMAL(10,8) NOT NULL,
    dropoff_lng DECIMAL(11,8) NOT NULL,
    
    package_description TEXT NOT NULL,
    package_weight_kg DECIMAL(6,2),
    package_photo_url TEXT,
    
    vehicle_type vehicle_type NOT NULL,
    distance_km DECIMAL(8,2),
    delivery_fee DECIMAL(18,2) NOT NULL,
    
    status errand_status DEFAULT 'draft',
    dropoff_code VARCHAR(6),
    
    rider_id UUID REFERENCES rider_profiles(id),
    delivery_job_id UUID REFERENCES delivery_jobs(id),
    
    confirmed_at TIMESTAMP WITH TIME ZONE,
    accepted_at TIMESTAMP WITH TIME ZONE,
    picked_up_at TIMESTAMP WITH TIME ZONE,
    delivered_at TIMESTAMP WITH TIME ZONE,
    cancelled_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_errands_user ON errands(user_id);
CREATE INDEX idx_errands_status ON errands(status);
CREATE INDEX idx_errands_rider ON errands(rider_id);
CREATE INDEX idx_errands_searching ON errands(status) WHERE status = 'searching';

-- Add errand FK to delivery_jobs
ALTER TABLE delivery_jobs ADD CONSTRAINT fk_delivery_jobs_errand 
    FOREIGN KEY (errand_id) REFERENCES errands(id);
CREATE INDEX idx_delivery_jobs_errand ON delivery_jobs(errand_id) WHERE errand_id IS NOT NULL;

-- ============================================
-- RIDER LOCATION HISTORY
-- ============================================

CREATE TABLE rider_location_history (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    rider_id UUID NOT NULL REFERENCES rider_profiles(id),
    delivery_job_id UUID REFERENCES delivery_jobs(id),
    
    lat DECIMAL(10,8) NOT NULL,
    lng DECIMAL(11,8) NOT NULL,
    heading DECIMAL(5,2),
    speed DECIMAL(6,2),
    
    eta_minutes INT,
    distance_remaining_km DECIMAL(8,2),
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_location_history_job ON rider_location_history(delivery_job_id, created_at DESC);
CREATE INDEX idx_location_history_rider ON rider_location_history(rider_id, created_at DESC);
CREATE INDEX idx_location_history_cleanup ON rider_location_history(created_at);

-- ============================================
-- IDEMPOTENCY KEYS
-- ============================================

CREATE TABLE idempotency_keys (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    key VARCHAR(255) UNIQUE NOT NULL,
    operation VARCHAR(50) NOT NULL,
    request_hash VARCHAR(255),
    response_data JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_idempotency_keys_key ON idempotency_keys(key);

-- ============================================
-- OTP VERIFICATIONS
-- ============================================

CREATE TABLE otp_verifications (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    phone_number VARCHAR(20) NOT NULL,
    verification_id UUID NOT NULL,
    code_hash VARCHAR(255) NOT NULL,
    purpose otp_purpose NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    attempts INT DEFAULT 0,
    verified_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    UNIQUE(phone_number, verification_id)
);

CREATE INDEX idx_otp_phone ON otp_verifications(phone_number);
CREATE INDEX idx_otp_expires ON otp_verifications(expires_at);

-- ============================================
-- DISPUTES
-- ============================================

CREATE TABLE disputes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    escrow_id UUID NOT NULL REFERENCES escrows(id),
    opened_by UUID REFERENCES users(id),
    reason TEXT NOT NULL,
    evidence JSONB DEFAULT '[]',

    status dispute_status DEFAULT 'open',
    outcome dispute_outcome DEFAULT 'pending',

    resolution_notes TEXT,
    resolved_by UUID REFERENCES users(id),
    resolved_at TIMESTAMP WITH TIME ZONE,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_disputes_escrow ON disputes(escrow_id);
CREATE INDEX idx_disputes_status ON disputes(status);

-- ============================================
-- PAYMENT REQUEST EVENTS
-- ============================================

CREATE TABLE payment_request_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    payment_request_id UUID REFERENCES payment_requests(id),
    from_status payment_request_status,
    to_status payment_request_status NOT NULL,
    actor_id UUID REFERENCES users(id),
    actor_type VARCHAR(20),
    reason TEXT,
    metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_pr_events_request ON payment_request_events(payment_request_id);

-- ============================================
-- TRIGGERS FOR UPDATED_AT
-- ============================================

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_rider_profiles_updated_at BEFORE UPDATE ON rider_profiles
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_wallets_updated_at BEFORE UPDATE ON wallets
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_payment_requests_updated_at BEFORE UPDATE ON payment_requests
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_delivery_jobs_updated_at BEFORE UPDATE ON delivery_jobs
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_bank_accounts_updated_at BEFORE UPDATE ON bank_accounts
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_virtual_accounts_updated_at BEFORE UPDATE ON virtual_accounts
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_withdrawals_updated_at BEFORE UPDATE ON withdrawals
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_top_ups_updated_at BEFORE UPDATE ON top_ups
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_orders_updated_at BEFORE UPDATE ON orders
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_errands_updated_at BEFORE UPDATE ON errands
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ============================================
-- VIEWS
-- ============================================

CREATE OR REPLACE VIEW v_active_payment_requests AS
SELECT
    pr.*,
    e.status AS escrow_status,
    e.total_locked,
    u_seller.phone_number AS seller_phone,
    u_seller.first_name AS seller_first_name,
    u_buyer.phone_number AS buyer_phone,
    u_buyer.first_name AS buyer_first_name,
    dj.status AS delivery_status,
    dj.rider_id,
    rp.user_id AS rider_user_id
FROM payment_requests pr
LEFT JOIN escrows e ON pr.escrow_id = e.id
LEFT JOIN users u_seller ON pr.seller_id = u_seller.id
LEFT JOIN users u_buyer ON pr.buyer_id = u_buyer.id
LEFT JOIN delivery_jobs dj ON pr.delivery_job_id = dj.id
LEFT JOIN rider_profiles rp ON dj.rider_id = rp.id
WHERE pr.status NOT IN ('completed', 'refunded', 'cancelled', 'expired');

CREATE OR REPLACE VIEW v_rider_earnings AS
SELECT
    rp.id AS rider_profile_id,
    rp.user_id,
    u.first_name,
    u.last_name,
    rp.total_deliveries,
    rp.total_earnings,
    rp.rating,
    w.available_balance,
    COUNT(dj.id) FILTER (WHERE dj.status = 'delivered') AS completed_jobs,
    SUM(dj.rider_payout) FILTER (WHERE dj.status = 'paid_out') AS paid_out_total
FROM rider_profiles rp
JOIN users u ON rp.user_id = u.id
LEFT JOIN wallets w ON u.id = w.user_id
LEFT JOIN delivery_jobs dj ON rp.id = dj.rider_id
GROUP BY rp.id, rp.user_id, u.first_name, u.last_name, rp.total_deliveries,
         rp.total_earnings, rp.rating, w.available_balance;

CREATE OR REPLACE VIEW v_rider_commission_summary AS
SELECT 
    rp.id AS rider_profile_id,
    rp.user_id,
    u.first_name,
    u.phone_number,
    rp.unpaid_commission,
    rp.unpaid_delivery_count,
    rp.is_commission_blocked,
    GREATEST(0, 5 - rp.unpaid_delivery_count) AS rides_until_blocked,
    rp.last_commission_paid_at
FROM rider_profiles rp
JOIN users u ON rp.user_id = u.id;
