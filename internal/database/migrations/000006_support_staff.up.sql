-- Support Staff Role
-- Allows admin to designate users as support staff with configurable permissions

CREATE TABLE support_staff (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    added_by UUID NOT NULL REFERENCES users(id),
    can_reply_chats BOOLEAN NOT NULL DEFAULT TRUE,
    can_view_disputes BOOLEAN NOT NULL DEFAULT FALSE,
    can_resolve_chats BOOLEAN NOT NULL DEFAULT FALSE,
    can_message_users BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE(user_id)
);

CREATE INDEX idx_support_staff_user_active ON support_staff(user_id) WHERE is_active = TRUE;
CREATE INDEX idx_support_staff_added_by ON support_staff(added_by);
