-- ============================================
-- KIPA CONSOLIDATED MIGRATION v2.0
-- Complete schema including all driver features
-- ============================================

-- Extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "postgis";

-- ============================================
-- ENUMS
-- ============================================

CREATE TYPE vehicle_type AS ENUM ('motorcycle', 'car');

CREATE TYPE delivery_type AS ENUM (
    'intra_state',
    'inter_state',
    'pickup'
);

CREATE TYPE payment_request_status AS ENUM (
    'draft',
    'link_active',
    'paid_awaiting_fulfillment',
    'ready_for_pickup',
    'searching_rider',
    'rider_assigned',
    'in_delivery',
    'delivered',
    'confirmation_window',
    'completed',
    'no_rider_found',
    'expired',
    'cancelled',
    'disputed',
    'refunded',
    'return_required',
    'return_searching_rider',
    'return_in_progress',
    'return_delivered',
    'return_confirmed',
    'return_damage_disputed',
    'pending_rebook'
);

CREATE TYPE delivery_job_status AS ENUM (
    'created',
    'searching',
    'accepted',
    'en_route_pickup',
    'picked_up',
    'in_transit',
    'delivered',
    'paid_out',
    'timeout',
    'cancelled',
    'failed',
    'return_damage_disputed',
    'buyer_unavailable',
    'seller_unavailable',
    'forced_return',
    'forced_return_done',
    'return_delivered',
    'return_confirmed',
    'return_refunded'
);

CREATE TYPE rider_status AS ENUM (
    'pending_verification',
    'verified',
    'suspended',
    'deactivated'
);

CREATE TYPE rider_offer_response AS ENUM (
    'pending',
    'accepted',
    'declined',
    'expired'
);

CREATE TYPE escrow_status AS ENUM (
    'created',
    'funded',
    'delivery_in_progress',
    'delivered',
    'confirmation_window',
    'released',
    'disputed',
    'refunded',
    'partial_refund'
);

CREATE TYPE wallet_transaction_type AS ENUM (
    'escrow_fund',
    'escrow_release',
    'delivery_payout',
    'platform_fee',
    'withdrawal',
    'top_up',
    'refund',
    'adjustment'
);

CREATE TYPE dispute_status AS ENUM (
    'open',
    'under_review',
    'resolved',
    'closed'
);

CREATE TYPE dispute_outcome AS ENUM (
    'pending',
    'buyer_favor',
    'seller_favor',
    'rider_fault',
    'no_fault',
    'partial_refund'
);

CREATE TYPE otp_purpose AS ENUM (
    'login',
    'signup',
    'verify'
);

CREATE TYPE order_payment_status AS ENUM (
    'pending',
    'initialized',
    'verified',
    'failed'
);

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

CREATE TYPE withdrawal_status AS ENUM (
    'pending',
    'processing',
    'success',
    'failed',
    'reversed'
);

CREATE TYPE top_up_status AS ENUM (
    'pending',
    'success',
    'failed',
    'abandoned'
);

CREATE TYPE rider_commission_status AS ENUM ('pending', 'paid', 'waived');

CREATE TYPE errand_status AS ENUM (
    'draft',
    'searching',
    'accepted',
    'picked_up',
    'in_transit',
    'delivered',
    'completed',
    'cancelled'
);

CREATE TYPE logistics_delivery_status AS ENUM (
    'awaiting_shipment',
    'shipped',
    'delivery_claimed',
    'confirmed',
    'pending_admin_approval',
    'funds_released',
    'disputed',
    'return_required',
    'return_shipped',
    'return_confirmed',
    'refunded',
    'released_by_admin',
    'cancelled',
    'return_damage_disputed',
    'pending_refund_approval'
);

CREATE TYPE logistics_carrier AS ENUM (
    'gig_logistics',
    'kwik_delivery',
    'abc_transport',
    'peace_mass',
    'god_is_good',
    'dhl',
    'fedex',
    'other'
);

-- Driver-related enums
CREATE TYPE driver_status AS ENUM (
    'offline',
    'idle',
    'loading',
    'in_transit',
    'delivering'
);

CREATE TYPE driver_vehicle_type AS ENUM (
    'car',
    'van',
    'bus'
);

CREATE TYPE driver_trip_status AS ENUM (
    'accepting',
    'loading',
    'in_transit',
    'arrived',
    'delivering',
    'completed',
    'cancelled'
);

-- IMPORTANT: awaiting_offers included from start
CREATE TYPE driver_delivery_status AS ENUM (
    'awaiting_driver',
    'awaiting_offers',
    'driver_assigned',
    'awaiting_pickup',
    'picked_up',
    'in_transit',
    'arrived',
    'confirmation_window',
    'completed',
    'driver_paid',
    'disputed',
    'refunded',
    'cancelled',
    'expired',
    'buyer_unreachable',
    'return_to_seller',
    'returned_to_seller',
    'pickup_code_lockout',
    'dropoff_code_lockout'
);

CREATE TYPE driver_delivery_actor_type AS ENUM (
    'driver',
    'buyer',
    'seller',
    'system',
    'admin'
);

CREATE TYPE driver_offer_status AS ENUM (
    'pending',
    'accepted',
    'rejected',
    'countered',
    'counter_accepted',
    'counter_rejected',
    'expired',
    'withdrawn'
);

-- ============================================
-- BASE TABLES
-- ============================================

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    phone_number VARCHAR(20) UNIQUE NOT NULL,
    phone_verified BOOLEAN DEFAULT FALSE,
    email VARCHAR(255),
    first_name VARCHAR(100),
    last_name VARCHAR(100),
    profile_photo_url TEXT,
    is_seller BOOLEAN DEFAULT FALSE,
    is_buyer BOOLEAN DEFAULT FALSE,
    is_rider BOOLEAN DEFAULT FALSE,
    seller_verified BOOLEAN DEFAULT FALSE,
    seller_verified_at TIMESTAMP WITH TIME ZONE,
    is_suspended BOOLEAN DEFAULT FALSE,
    suspension_reason TEXT,
    last_login_at TIMESTAMP WITH TIME ZONE,
    is_admin BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_users_phone ON users(phone_number);
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_admin ON users(is_admin) WHERE is_admin = TRUE;

CREATE TABLE user_device_tokens (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token TEXT NOT NULL,
    platform VARCHAR(20) NOT NULL CHECK (platform IN ('android', 'ios', 'web')),
    last_used_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(user_id, token)
);
CREATE INDEX idx_device_tokens_user ON user_device_tokens(user_id);

CREATE TABLE notification_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    title VARCHAR(255) NOT NULL,
    body TEXT,
    data JSONB,
    notification_type VARCHAR(50) NOT NULL,
    sent_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    delivered BOOLEAN DEFAULT FALSE
);
CREATE INDEX idx_notification_logs_user ON notification_logs(user_id);
CREATE INDEX idx_notification_logs_type ON notification_logs(notification_type);

CREATE TABLE rider_profiles (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    license_number VARCHAR(100) UNIQUE,
    nin_number VARCHAR(20) UNIQUE,
    license_photo_url TEXT,
    nin_photo_url TEXT,
    face_verification_url TEXT,
    vehicle_type VARCHAR(50),
    vehicle_plate VARCHAR(20),
    vehicle_photo_url TEXT,
    status rider_status DEFAULT 'pending_verification',
    is_online BOOLEAN DEFAULT FALSE,
    current_location GEOGRAPHY(POINT, 4326),
    last_location_update TIMESTAMP WITH TIME ZONE,
    rating DECIMAL(2,1) DEFAULT 5.0,
    total_deliveries INT DEFAULT 0,
    total_earnings DECIMAL(18,2) DEFAULT 0,
    unpaid_commission DECIMAL(18,2) DEFAULT 0,
    unpaid_delivery_count INT DEFAULT 0,
    last_commission_paid_at TIMESTAMP WITH TIME ZONE,
    is_commission_blocked BOOLEAN DEFAULT FALSE,
    location_sharing_enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_rider_profiles_location ON rider_profiles USING GIST(current_location);
CREATE INDEX idx_rider_profiles_status ON rider_profiles(status, is_online);
CREATE INDEX idx_rider_profiles_commission_blocked ON rider_profiles(is_commission_blocked) WHERE is_commission_blocked = TRUE;
CREATE INDEX idx_rider_location_sharing ON rider_profiles(id, location_sharing_enabled);

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
-- CORE BUSINESS TABLES
-- ============================================

-- Payment requests with state fields and driver_delivery_id included from start
CREATE TABLE payment_requests (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    seller_id UUID REFERENCES users(id),
    item_name VARCHAR(255) NOT NULL,
    item_description TEXT,
    item_price DECIMAL(18,2) NOT NULL,
    item_images JSONB DEFAULT '[]',
    processing_time_hours INT,
    delivery_type delivery_type DEFAULT 'pickup',
    vehicle_type VARCHAR(20),
    pickup_address TEXT,
    pickup_location GEOGRAPHY(POINT, 4326),
    pickup_state VARCHAR(100),
    dropoff_address TEXT,
    dropoff_location GEOGRAPHY(POINT, 4326),
    dropoff_state VARCHAR(100),
    estimated_delivery_fee DECIMAL(18,2),
    buyer_service_fee DECIMAL(18,2),
    seller_platform_fee DECIMAL(18,2),
    estimated_total DECIMAL(18,2),
    actual_delivery_fee DECIMAL(18,2),
    actual_total DECIMAL(18,2),
    payment_code VARCHAR(20) UNIQUE,
    link_expires_at TIMESTAMP WITH TIME ZONE,
    is_reusable BOOLEAN DEFAULT FALSE,
    max_uses INT,
    current_uses INT DEFAULT 0,
    logistics_carrier VARCHAR(100),
    tracking_number VARCHAR(255),
    tracking_url TEXT,
    status payment_request_status DEFAULT 'draft',
    escrow_id UUID,
    delivery_job_id UUID,
    buyer_id UUID REFERENCES users(id),
    logistics_delivery_id UUID,
    return_delivery_job_id UUID,
    forced_return_job_id UUID,
    driver_delivery_id UUID,
    dispute_outcome VARCHAR(20),
    rebook_deadline TIMESTAMP WITH TIME ZONE,
    rebook_attempts INT DEFAULT 0,
    unavailability_fee DECIMAL(12,2),
    last_reminder_sent_at TIMESTAMP,
    reminder_count INT DEFAULT 0,
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
CREATE INDEX idx_payment_requests_return_flow ON payment_requests(status) WHERE status IN ('return_required', 'return_searching_rider', 'return_in_progress', 'return_delivered', 'return_confirmed', 'return_damage_disputed');
CREATE INDEX idx_pr_pending_rebook ON payment_requests(status, rebook_deadline) WHERE status = 'pending_rebook';
CREATE INDEX idx_payment_requests_interstate_route ON payment_requests(pickup_state, dropoff_state) WHERE delivery_type = 'inter_state';

CREATE TABLE delivery_jobs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    payment_request_id UUID REFERENCES payment_requests(id),
    rider_id UUID REFERENCES rider_profiles(id),
    pickup_address TEXT NOT NULL,
    pickup_location GEOGRAPHY(POINT, 4326) NOT NULL,
    dropoff_address TEXT NOT NULL,
    dropoff_location GEOGRAPHY(POINT, 4326) NOT NULL,
    distance_km DECIMAL(5,2),
    delivery_fee DECIMAL(18,2) NOT NULL,
    rider_payout DECIMAL(18,2),
    platform_delivery_fee DECIMAL(18,2),
    required_vehicle_type vehicle_type DEFAULT 'motorcycle',
    pickup_code VARCHAR(6),
    dropoff_code VARCHAR(6),
    search_radius_km INT DEFAULT 5,
    search_started_at TIMESTAMP WITH TIME ZONE,
    search_timeout_at TIMESTAMP WITH TIME ZONE,
    last_radius_expand_at TIMESTAMP,
    current_eta_minutes INT,
    eta_updated_at TIMESTAMP WITH TIME ZONE,
    status delivery_job_status DEFAULT 'created',
    source_type VARCHAR(20) DEFAULT 'payment_request',
    errand_id UUID,
    is_return BOOLEAN DEFAULT FALSE,
    original_job_id UUID REFERENCES delivery_jobs(id),
    return_condition VARCHAR(20),
    damage_claim_reason TEXT,
    return_confirmed_at TIMESTAMP WITH TIME ZONE,
    dispute_id UUID,
    is_forced_return BOOLEAN DEFAULT FALSE,
    unavailable_marked_at TIMESTAMP WITH TIME ZONE,
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
CREATE INDEX idx_delivery_jobs_is_return ON delivery_jobs(is_return) WHERE is_return = true;
CREATE INDEX idx_dj_forced_return ON delivery_jobs(is_forced_return) WHERE is_forced_return = TRUE;

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

CREATE TABLE orders (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
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
CREATE INDEX idx_escrows_order ON escrows(order_id);
CREATE INDEX idx_escrows_buyer ON escrows(buyer_id);
CREATE INDEX idx_escrows_seller ON escrows(seller_id);
CREATE INDEX idx_escrows_status ON escrows(status);

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

-- Logistics deliveries with nullable carrier from start
CREATE TABLE logistics_deliveries (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    payment_request_id UUID NOT NULL REFERENCES payment_requests(id),
    escrow_id UUID NOT NULL REFERENCES escrows(id),
    seller_id UUID NOT NULL REFERENCES users(id),
    buyer_id UUID NOT NULL REFERENCES users(id),
    carrier logistics_carrier,
    carrier_other_name VARCHAR(255),
    tracking_number VARCHAR(255),
    tracking_url TEXT,
    shipment_receipt_url TEXT,
    estimated_delivery_at TIMESTAMP WITH TIME ZONE,
    pickup_address TEXT NOT NULL,
    pickup_state VARCHAR(100) NOT NULL,
    dropoff_address TEXT NOT NULL,
    dropoff_state VARCHAR(100) NOT NULL,
    status logistics_delivery_status NOT NULL DEFAULT 'awaiting_shipment',
    ship_deadline TIMESTAMP WITH TIME ZONE NOT NULL,
    confirmation_deadline TIMESTAMP WITH TIME ZONE,
    dispute_auto_escalation_at TIMESTAMP WITH TIME ZONE,
    requires_admin_approval BOOLEAN NOT NULL DEFAULT FALSE,
    return_tracking_number VARCHAR(255),
    return_carrier VARCHAR(255),
    return_cost_buyer_share DECIMAL(18,2),
    return_cost_seller_share DECIMAL(18,2),
    dispute_id UUID REFERENCES disputes(id),
    return_condition VARCHAR(20),
    damage_claim_reason TEXT,
    return_received_at TIMESTAMP WITH TIME ZONE,
    dispute_outcome VARCHAR(20),
    last_reminder_sent_at TIMESTAMP WITH TIME ZONE,
    reminder_count INT NOT NULL DEFAULT 0,
    shipped_at TIMESTAMP WITH TIME ZONE,
    delivery_claimed_at TIMESTAMP WITH TIME ZONE,
    confirmed_at TIMESTAMP WITH TIME ZONE,
    released_at TIMESTAMP WITH TIME ZONE,
    cancelled_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
CREATE INDEX idx_logistics_deliveries_payment_request ON logistics_deliveries(payment_request_id);
CREATE INDEX idx_logistics_deliveries_escrow ON logistics_deliveries(escrow_id);
CREATE INDEX idx_logistics_deliveries_damage_claim ON logistics_deliveries(status) WHERE status = 'return_damage_disputed';
CREATE INDEX idx_logistics_deliveries_pending_refund ON logistics_deliveries(status) WHERE status = 'pending_refund_approval';

CREATE TABLE logistics_delivery_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    logistics_delivery_id UUID NOT NULL REFERENCES logistics_deliveries(id) ON DELETE CASCADE,
    from_status logistics_delivery_status,
    to_status logistics_delivery_status NOT NULL,
    actor_id UUID REFERENCES users(id),
    actor_type VARCHAR(20) NOT NULL,
    reason TEXT,
    metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
CREATE INDEX idx_logistics_delivery_events_delivery ON logistics_delivery_events(logistics_delivery_id);

CREATE TABLE logistics_evidence (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    logistics_delivery_id UUID NOT NULL REFERENCES logistics_deliveries(id) ON DELETE CASCADE,
    uploaded_by UUID NOT NULL REFERENCES users(id),
    uploader_type VARCHAR(20) NOT NULL,
    evidence_type VARCHAR(50) NOT NULL,
    file_url TEXT NOT NULL,
    description TEXT,
    uploaded_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
CREATE INDEX idx_logistics_evidence_delivery ON logistics_evidence(logistics_delivery_id);

CREATE TABLE errands (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    pickup_address TEXT NOT NULL,
    pickup_lat DECIMAL(10,8) NOT NULL,
    pickup_lng DECIMAL(11,8) NOT NULL,
    pickup_contact_name VARCHAR(255),
    pickup_contact_phone VARCHAR(20),
    pickup_instructions TEXT,
    dropoff_address TEXT NOT NULL,
    dropoff_lat DECIMAL(10,8) NOT NULL,
    dropoff_lng DECIMAL(11,8) NOT NULL,
    dropoff_contact_name VARCHAR(255),
    dropoff_contact_phone VARCHAR(20),
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
    completed_at TIMESTAMP WITH TIME ZONE,
    cancelled_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
CREATE INDEX idx_errands_user ON errands(user_id);
CREATE INDEX idx_errands_status ON errands(status);

-- ============================================
-- DRIVER TABLES (Inter-state delivery)
-- ============================================

CREATE TABLE driver_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status driver_status NOT NULL DEFAULT 'offline',
    max_carrying_value DECIMAL(12,2) NOT NULL DEFAULT 500000,
    current_carry_value DECIMAL(12,2) NOT NULL DEFAULT 0,
    max_package_count INT NOT NULL DEFAULT 20,
    current_package_count INT NOT NULL DEFAULT 0,
    vehicle_type driver_vehicle_type NOT NULL DEFAULT 'car',
    license_plate VARCHAR(20) NOT NULL,
    is_verified BOOLEAN NOT NULL DEFAULT FALSE,
    total_deliveries INT NOT NULL DEFAULT 0,
    total_earnings DECIMAL(12,2) NOT NULL DEFAULT 0,
    rating DECIMAL(3,2) NOT NULL DEFAULT 5.0,
    current_trip_id UUID,
    origin_city VARCHAR(100),
    destination_city VARCHAR(100),
    nin_number VARCHAR(20),
    nin_photo_url TEXT,
    license_number VARCHAR(50),
    license_photo_url TEXT,
    vehicle_photo_url TEXT,
    face_photo_url TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT unique_driver_user UNIQUE (user_id),
    CONSTRAINT unique_driver_license_plate UNIQUE (license_plate)
);

CREATE TABLE driver_trips (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    driver_id UUID NOT NULL REFERENCES driver_profiles(id) ON DELETE CASCADE,
    origin_city VARCHAR(100) NOT NULL,
    destination_city VARCHAR(100) NOT NULL,
    status driver_trip_status NOT NULL DEFAULT 'accepting',
    total_packages INT NOT NULL DEFAULT 0,
    total_value DECIMAL(12,2) NOT NULL DEFAULT 0,
    total_driver_fee DECIMAL(12,2) NOT NULL DEFAULT 0,
    accepting_until TIMESTAMP WITH TIME ZONE,
    departed_at TIMESTAMP WITH TIME ZONE,
    arrived_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Add FK from driver_profiles to driver_trips
ALTER TABLE driver_profiles
ADD CONSTRAINT fk_driver_current_trip
FOREIGN KEY (current_trip_id) REFERENCES driver_trips(id) ON DELETE SET NULL;

CREATE TABLE driver_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_request_id UUID NOT NULL REFERENCES payment_requests(id) ON DELETE CASCADE,
    trip_id UUID REFERENCES driver_trips(id) ON DELETE SET NULL,
    driver_id UUID REFERENCES driver_profiles(id) ON DELETE SET NULL,
    origin_city VARCHAR(100) NOT NULL,
    destination_city VARCHAR(100) NOT NULL,
    pickup_address TEXT NOT NULL,
    dropoff_address TEXT NOT NULL,
    item_value DECIMAL(12,2) NOT NULL,
    driver_fee DECIMAL(12,2) NOT NULL,
    pickup_code VARCHAR(6),
    pickup_code_generated_at TIMESTAMP WITH TIME ZONE,
    pickup_attempts INT NOT NULL DEFAULT 0,
    dropoff_code VARCHAR(6),
    dropoff_code_generated_at TIMESTAMP WITH TIME ZONE,
    dropoff_attempts INT NOT NULL DEFAULT 0,
    status driver_delivery_status NOT NULL DEFAULT 'awaiting_driver',
    driver_assigned_at TIMESTAMP WITH TIME ZONE,
    at_pickup_at TIMESTAMP WITH TIME ZONE,
    picked_up_at TIMESTAMP WITH TIME ZONE,
    departed_at TIMESTAMP WITH TIME ZONE,
    arrived_at TIMESTAMP WITH TIME ZONE,
    delivered_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    buyer_unreachable_at TIMESTAMP WITH TIME ZONE,
    unreachable_deadline TIMESTAMP WITH TIME ZONE,
    escrow_id UUID REFERENCES escrows(id) ON DELETE SET NULL,
    seller_id UUID NOT NULL REFERENCES users(id),
    buyer_id UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE driver_delivery_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    delivery_id UUID NOT NULL REFERENCES driver_deliveries(id) ON DELETE CASCADE,
    from_status driver_delivery_status,
    to_status driver_delivery_status NOT NULL,
    actor_type driver_delivery_actor_type NOT NULL,
    actor_id UUID,
    reason VARCHAR(255),
    metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE driver_offers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    delivery_id UUID NOT NULL REFERENCES driver_deliveries(id),
    driver_id UUID NOT NULL REFERENCES driver_profiles(id),
    offered_amount DECIMAL(12,2) NOT NULL,
    counter_amount DECIMAL(12,2),
    agreed_amount DECIMAL(12,2),
    status driver_offer_status NOT NULL DEFAULT 'pending',
    expires_at TIMESTAMPTZ NOT NULL,
    counter_expires_at TIMESTAMPTZ,
    countered_at TIMESTAMPTZ,
    responded_at TIMESTAMPTZ,
    driver_note TEXT,
    seller_note TEXT,
    offer_count INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Driver indexes
CREATE INDEX idx_driver_profiles_user ON driver_profiles(user_id);
CREATE INDEX idx_driver_profiles_status ON driver_profiles(status);
CREATE INDEX idx_driver_profiles_verified ON driver_profiles(is_verified) WHERE is_verified = TRUE;
CREATE INDEX idx_driver_profiles_route ON driver_profiles(origin_city, destination_city) WHERE status IN ('idle', 'loading');

CREATE INDEX idx_driver_trips_driver ON driver_trips(driver_id);
CREATE INDEX idx_driver_trips_status ON driver_trips(status);
CREATE INDEX idx_driver_trips_route ON driver_trips(origin_city, destination_city, status);
CREATE INDEX idx_driver_trips_active ON driver_trips(driver_id, status) WHERE status NOT IN ('completed', 'cancelled');

CREATE INDEX idx_driver_deliveries_payment_request ON driver_deliveries(payment_request_id);
CREATE INDEX idx_driver_deliveries_trip ON driver_deliveries(trip_id);
CREATE INDEX idx_driver_deliveries_driver ON driver_deliveries(driver_id);
CREATE INDEX idx_driver_deliveries_status ON driver_deliveries(status);
CREATE INDEX idx_driver_deliveries_route ON driver_deliveries(origin_city, destination_city, status);
CREATE INDEX idx_driver_deliveries_awaiting ON driver_deliveries(origin_city, destination_city) WHERE status = 'awaiting_driver';
CREATE INDEX idx_driver_deliveries_seller ON driver_deliveries(seller_id);
CREATE INDEX idx_driver_deliveries_buyer ON driver_deliveries(buyer_id);
CREATE INDEX idx_driver_deliveries_unreachable ON driver_deliveries(unreachable_deadline) WHERE status = 'buyer_unreachable' AND unreachable_deadline IS NOT NULL;

CREATE INDEX idx_driver_delivery_events_delivery ON driver_delivery_events(delivery_id);
CREATE INDEX idx_driver_delivery_events_created ON driver_delivery_events(created_at);

CREATE INDEX idx_driver_offers_delivery ON driver_offers(delivery_id);
CREATE INDEX idx_driver_offers_driver ON driver_offers(driver_id);
CREATE INDEX idx_driver_offers_expiry ON driver_offers(expires_at) WHERE status IN ('pending', 'countered');
CREATE INDEX idx_driver_offers_status ON driver_offers(status);

-- ============================================
-- FINANCIAL TABLES
-- ============================================

CREATE TABLE wallets (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    available_balance DECIMAL(18,2) NOT NULL DEFAULT 0,
    locked_balance DECIMAL(18,2) NOT NULL DEFAULT 0,
    total_earned DECIMAL(18,2) NOT NULL DEFAULT 0,
    total_withdrawn DECIMAL(18,2) NOT NULL DEFAULT 0,
    pin_hash VARCHAR(255),
    pin_created_at TIMESTAMP WITH TIME ZONE,
    pin_attempts INT DEFAULT 0,
    pin_locked_until TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT positive_available CHECK (available_balance >= 0),
    CONSTRAINT positive_locked CHECK (locked_balance >= 0)
);

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

CREATE TABLE commission_payments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    rider_profile_id UUID REFERENCES rider_profiles(id),
    amount DECIMAL(18,2) NOT NULL,
    commissions_paid INT NOT NULL,
    payment_method VARCHAR(50) NOT NULL,
    wallet_transaction_id UUID REFERENCES wallet_transactions(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- ============================================
-- SUPPORT TABLES
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

CREATE TABLE idempotency_keys (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    key VARCHAR(255) UNIQUE NOT NULL,
    operation VARCHAR(50) NOT NULL,
    request_hash VARCHAR(255),
    response_data JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

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

CREATE TABLE chat_messages (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    job_id UUID NOT NULL REFERENCES delivery_jobs(id) ON DELETE CASCADE,
    sender_id UUID NOT NULL REFERENCES users(id),
    receiver_id UUID NOT NULL REFERENCES users(id),
    content TEXT NOT NULL,
    media_url TEXT,
    media_type VARCHAR(20) DEFAULT 'text',
    status VARCHAR(20) DEFAULT 'sent',
    created_at TIMESTAMP DEFAULT NOW(),
    delivered_at TIMESTAMP,
    read_at TIMESTAMP
);
CREATE INDEX idx_chat_messages_job ON chat_messages(job_id);

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

CREATE TABLE platform_settings (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    setting_key VARCHAR(50) UNIQUE NOT NULL,
    setting_value JSONB NOT NULL,
    description TEXT,
    updated_by UUID,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE admin_audit_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    admin_id UUID NOT NULL REFERENCES users(id),
    action VARCHAR(50) NOT NULL,
    target_type VARCHAR(50) NOT NULL,
    target_id UUID,
    details JSONB,
    ip_address VARCHAR(45),
    user_agent TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
CREATE INDEX idx_admin_audit_logs_admin ON admin_audit_logs(admin_id);
CREATE INDEX idx_admin_audit_logs_target ON admin_audit_logs(target_type, target_id);
CREATE INDEX idx_admin_audit_logs_created ON admin_audit_logs(created_at DESC);

-- Seed default settings
INSERT INTO platform_settings (setting_key, setting_value, description) VALUES
('delivery_pricing', '{"base_fee": 500, "per_km_fee": 100}', 'Default delivery pricing config'),
('platform_fees', '{"buyer_fee_rate": 0.02, "seller_fee_rate": 0.03, "rider_payout_rate": 0.85}', 'Default platform fees config'),
('commission_settings', '{"max_unpaid_rides": 5, "commission_rate": 0.15}', 'Rider commission settings');

-- ============================================
-- FOREIGN KEY CONSTRAINTS (Circular dependencies)
-- ============================================

ALTER TABLE payment_requests ADD CONSTRAINT fk_pr_escrow
    FOREIGN KEY (escrow_id) REFERENCES escrows(id);

ALTER TABLE payment_requests ADD CONSTRAINT fk_pr_delivery_job
    FOREIGN KEY (delivery_job_id) REFERENCES delivery_jobs(id);

ALTER TABLE payment_requests ADD CONSTRAINT fk_pr_logistics_delivery
    FOREIGN KEY (logistics_delivery_id) REFERENCES logistics_deliveries(id);

ALTER TABLE payment_requests ADD CONSTRAINT fk_pr_return_job
    FOREIGN KEY (return_delivery_job_id) REFERENCES delivery_jobs(id);

ALTER TABLE payment_requests ADD CONSTRAINT fk_pr_forced_return
    FOREIGN KEY (forced_return_job_id) REFERENCES delivery_jobs(id);

ALTER TABLE payment_requests ADD CONSTRAINT fk_pr_driver_delivery
    FOREIGN KEY (driver_delivery_id) REFERENCES driver_deliveries(id);

ALTER TABLE delivery_jobs ADD CONSTRAINT fk_dj_errand
    FOREIGN KEY (errand_id) REFERENCES errands(id);

ALTER TABLE delivery_jobs ADD CONSTRAINT fk_dj_dispute
    FOREIGN KEY (dispute_id) REFERENCES disputes(id);

ALTER TABLE orders ADD CONSTRAINT fk_orders_escrow
    FOREIGN KEY (escrow_id) REFERENCES escrows(id);

-- ============================================
-- TRIGGERS
-- ============================================

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_rider_profiles_updated_at BEFORE UPDATE ON rider_profiles FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_wallets_updated_at BEFORE UPDATE ON wallets FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_payment_requests_updated_at BEFORE UPDATE ON payment_requests FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_delivery_jobs_updated_at BEFORE UPDATE ON delivery_jobs FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_bank_accounts_updated_at BEFORE UPDATE ON bank_accounts FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_virtual_accounts_updated_at BEFORE UPDATE ON virtual_accounts FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_withdrawals_updated_at BEFORE UPDATE ON withdrawals FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_top_ups_updated_at BEFORE UPDATE ON top_ups FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_orders_updated_at BEFORE UPDATE ON orders FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_errands_updated_at BEFORE UPDATE ON errands FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_logistics_deliveries_updated_at BEFORE UPDATE ON logistics_deliveries FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_driver_profiles_updated_at BEFORE UPDATE ON driver_profiles FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_driver_trips_updated_at BEFORE UPDATE ON driver_trips FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_driver_deliveries_updated_at BEFORE UPDATE ON driver_deliveries FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_driver_offers_updated_at BEFORE UPDATE ON driver_offers FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
