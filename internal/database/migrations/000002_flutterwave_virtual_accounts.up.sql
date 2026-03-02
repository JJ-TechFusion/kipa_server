-- Flutterwave Virtual Accounts for wallet funding via bank transfer
CREATE TABLE IF NOT EXISTS flutterwave_virtual_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_number VARCHAR(20) NOT NULL,
    bank_name VARCHAR(100) NOT NULL,
    tx_ref VARCHAR(100) NOT NULL UNIQUE,
    flw_ref VARCHAR(100),
    order_ref VARCHAR(100),
    email VARCHAR(255),
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for fast lookups
CREATE INDEX IF NOT EXISTS idx_flw_va_user_id ON flutterwave_virtual_accounts(user_id);
CREATE INDEX IF NOT EXISTS idx_flw_va_account_number ON flutterwave_virtual_accounts(account_number) WHERE is_active = true;
CREATE INDEX IF NOT EXISTS idx_flw_va_tx_ref ON flutterwave_virtual_accounts(tx_ref);

-- Track user's decision about virtual account creation
ALTER TABLE users ADD COLUMN IF NOT EXISTS flw_virtual_account_declined_at TIMESTAMPTZ;
