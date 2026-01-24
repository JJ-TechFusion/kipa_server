-- ============================================
-- INTER-STATE DRIVER DELIVERY TABLES
-- ============================================

-- Driver status enum
CREATE TYPE driver_status AS ENUM (
    'offline',
    'idle',
    'loading',
    'in_transit',
    'delivering'
);

-- Driver vehicle type enum
CREATE TYPE driver_vehicle_type AS ENUM (
    'car',
    'van',
    'bus'
);

-- Driver trip status enum
CREATE TYPE driver_trip_status AS ENUM (
    'accepting',
    'loading',
    'in_transit',
    'arrived',
    'delivering',
    'completed',
    'cancelled'
);

-- Driver delivery status enum
CREATE TYPE driver_delivery_status AS ENUM (
    'awaiting_driver',
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

-- Driver delivery actor type enum
CREATE TYPE driver_delivery_actor_type AS ENUM (
    'driver',
    'buyer',
    'seller',
    'system',
    'admin'
);

-- ============================================
-- DRIVER PROFILES TABLE
-- ============================================

CREATE TABLE driver_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Status
    status driver_status NOT NULL DEFAULT 'offline',

    -- Capacity limits
    max_carrying_value DECIMAL(12,2) NOT NULL DEFAULT 500000,
    current_carry_value DECIMAL(12,2) NOT NULL DEFAULT 0,
    max_package_count INT NOT NULL DEFAULT 20,
    current_package_count INT NOT NULL DEFAULT 0,

    -- Vehicle info
    vehicle_type driver_vehicle_type NOT NULL DEFAULT 'car',
    license_plate VARCHAR(20) NOT NULL,
    is_verified BOOLEAN NOT NULL DEFAULT FALSE,

    -- Stats
    total_deliveries INT NOT NULL DEFAULT 0,
    total_earnings DECIMAL(12,2) NOT NULL DEFAULT 0,
    rating DECIMAL(3,2) NOT NULL DEFAULT 5.0,

    -- Current trip
    current_trip_id UUID,  -- FK added after driver_trips table
    origin_city VARCHAR(100),
    destination_city VARCHAR(100),

    -- Verification documents
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

-- ============================================
-- DRIVER TRIPS TABLE
-- ============================================

CREATE TABLE driver_trips (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    driver_id UUID NOT NULL REFERENCES driver_profiles(id) ON DELETE CASCADE,

    -- Route
    origin_city VARCHAR(100) NOT NULL,
    destination_city VARCHAR(100) NOT NULL,

    -- Status
    status driver_trip_status NOT NULL DEFAULT 'accepting',

    -- Aggregates
    total_packages INT NOT NULL DEFAULT 0,
    total_value DECIMAL(12,2) NOT NULL DEFAULT 0,
    total_driver_fee DECIMAL(12,2) NOT NULL DEFAULT 0,

    -- Timeline
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

-- ============================================
-- DRIVER DELIVERIES TABLE
-- ============================================

CREATE TABLE driver_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_request_id UUID NOT NULL REFERENCES payment_requests(id) ON DELETE CASCADE,
    trip_id UUID REFERENCES driver_trips(id) ON DELETE SET NULL,
    driver_id UUID REFERENCES driver_profiles(id) ON DELETE SET NULL,

    -- Route
    origin_city VARCHAR(100) NOT NULL,
    destination_city VARCHAR(100) NOT NULL,
    pickup_address TEXT NOT NULL,
    dropoff_address TEXT NOT NULL,

    -- Value
    item_value DECIMAL(12,2) NOT NULL,
    driver_fee DECIMAL(12,2) NOT NULL,

    -- Codes
    pickup_code VARCHAR(6),
    pickup_code_generated_at TIMESTAMP WITH TIME ZONE,
    pickup_attempts INT NOT NULL DEFAULT 0,
    dropoff_code VARCHAR(6),
    dropoff_code_generated_at TIMESTAMP WITH TIME ZONE,
    dropoff_attempts INT NOT NULL DEFAULT 0,

    -- Status
    status driver_delivery_status NOT NULL DEFAULT 'awaiting_driver',

    -- Timeline (manual updates, no GPS)
    driver_assigned_at TIMESTAMP WITH TIME ZONE,
    at_pickup_at TIMESTAMP WITH TIME ZONE,
    picked_up_at TIMESTAMP WITH TIME ZONE,
    departed_at TIMESTAMP WITH TIME ZONE,
    arrived_at TIMESTAMP WITH TIME ZONE,
    delivered_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,

    -- Buyer unreachable
    buyer_unreachable_at TIMESTAMP WITH TIME ZONE,
    unreachable_deadline TIMESTAMP WITH TIME ZONE,

    -- Escrow
    escrow_id UUID REFERENCES escrows(id) ON DELETE SET NULL,

    -- Seller/Buyer references
    seller_id UUID NOT NULL REFERENCES users(id),
    buyer_id UUID REFERENCES users(id),

    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- ============================================
-- DRIVER DELIVERY EVENTS TABLE (Audit Trail)
-- ============================================

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

-- ============================================
-- INDEXES
-- ============================================

-- Driver profiles
CREATE INDEX idx_driver_profiles_user ON driver_profiles(user_id);
CREATE INDEX idx_driver_profiles_status ON driver_profiles(status);
CREATE INDEX idx_driver_profiles_verified ON driver_profiles(is_verified) WHERE is_verified = TRUE;
CREATE INDEX idx_driver_profiles_route ON driver_profiles(origin_city, destination_city) WHERE status IN ('idle', 'loading');

-- Driver trips
CREATE INDEX idx_driver_trips_driver ON driver_trips(driver_id);
CREATE INDEX idx_driver_trips_status ON driver_trips(status);
CREATE INDEX idx_driver_trips_route ON driver_trips(origin_city, destination_city, status);
CREATE INDEX idx_driver_trips_active ON driver_trips(driver_id, status) WHERE status NOT IN ('completed', 'cancelled');

-- Driver deliveries
CREATE INDEX idx_driver_deliveries_payment_request ON driver_deliveries(payment_request_id);
CREATE INDEX idx_driver_deliveries_trip ON driver_deliveries(trip_id);
CREATE INDEX idx_driver_deliveries_driver ON driver_deliveries(driver_id);
CREATE INDEX idx_driver_deliveries_status ON driver_deliveries(status);
CREATE INDEX idx_driver_deliveries_route ON driver_deliveries(origin_city, destination_city, status);
CREATE INDEX idx_driver_deliveries_awaiting ON driver_deliveries(origin_city, destination_city)
    WHERE status = 'awaiting_driver';
CREATE INDEX idx_driver_deliveries_seller ON driver_deliveries(seller_id);
CREATE INDEX idx_driver_deliveries_buyer ON driver_deliveries(buyer_id);
CREATE INDEX idx_driver_deliveries_unreachable ON driver_deliveries(unreachable_deadline)
    WHERE status = 'buyer_unreachable' AND unreachable_deadline IS NOT NULL;

-- Driver delivery events
CREATE INDEX idx_driver_delivery_events_delivery ON driver_delivery_events(delivery_id);
CREATE INDEX idx_driver_delivery_events_created ON driver_delivery_events(created_at);

-- ============================================
-- ADD driver_delivery_id TO payment_requests
-- ============================================

ALTER TABLE payment_requests
ADD COLUMN IF NOT EXISTS driver_delivery_id UUID REFERENCES driver_deliveries(id);

CREATE INDEX idx_payment_requests_driver_delivery ON payment_requests(driver_delivery_id)
    WHERE driver_delivery_id IS NOT NULL;
