DROP TABLE IF EXISTS flutterwave_virtual_accounts;
ALTER TABLE users DROP COLUMN IF EXISTS flw_virtual_account_declined_at;
