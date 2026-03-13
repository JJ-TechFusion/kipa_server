-- Remove dispute integration from support_conversations
DROP INDEX IF EXISTS idx_support_conversations_user_general;
DROP INDEX IF EXISTS idx_support_conversations_dispute;
ALTER TABLE support_conversations DROP COLUMN IF EXISTS dispute_id;
