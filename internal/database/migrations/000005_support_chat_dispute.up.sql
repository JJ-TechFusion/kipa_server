-- Add dispute_id to support_conversations for dispute-linked chats
ALTER TABLE support_conversations ADD COLUMN dispute_id UUID REFERENCES disputes(id) ON DELETE SET NULL;

-- Unique index ensures only one conversation per dispute
CREATE UNIQUE INDEX idx_support_conversations_dispute ON support_conversations(dispute_id) WHERE dispute_id IS NOT NULL;

-- Index for querying by user_id where dispute_id is null (general support chats)
CREATE INDEX idx_support_conversations_user_general ON support_conversations(user_id) WHERE dispute_id IS NULL;
