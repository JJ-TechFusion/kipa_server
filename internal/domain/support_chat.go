package domain

import (
	"time"

	"github.com/google/uuid"
)

type SupportConversationStatus string

const (
	SupportConversationStatusOpen     SupportConversationStatus = "open"
	SupportConversationStatusResolved SupportConversationStatus = "resolved"
	SupportConversationStatusClosed   SupportConversationStatus = "closed"
)

type SupportConversation struct {
	ID            uuid.UUID                 `db:"id" json:"id"`
	UserID        uuid.UUID                 `db:"user_id" json:"user_id"`
	Status        SupportConversationStatus `db:"status" json:"status"`
	LastMessageAt *time.Time                `db:"last_message_at" json:"last_message_at,omitempty"`
	CreatedAt     time.Time                 `db:"created_at" json:"created_at"`
}

func NewSupportConversation(userID uuid.UUID) *SupportConversation {
	return &SupportConversation{
		ID:        uuid.New(),
		UserID:    userID,
		Status:    SupportConversationStatusOpen,
		CreatedAt: time.Now(),
	}
}

func (c *SupportConversation) Resolve() {
	c.Status = SupportConversationStatusResolved
}

func (c *SupportConversation) Close() {
	c.Status = SupportConversationStatusClosed
}

func (c *SupportConversation) IsOpen() bool {
	return c.Status == SupportConversationStatusOpen
}

type SupportMessage struct {
	ID             uuid.UUID     `db:"id" json:"id"`
	ConversationID uuid.UUID     `db:"conversation_id" json:"conversation_id"`
	SenderID       uuid.UUID     `db:"sender_id" json:"sender_id"`
	IsAdmin        bool          `db:"is_admin" json:"is_admin"`
	Content        string        `db:"content" json:"content"`
	MediaURL       *string       `db:"media_url" json:"media_url,omitempty"`
	MediaType      string        `db:"media_type" json:"media_type"`
	Status         MessageStatus `db:"status" json:"status"`
	CreatedAt      time.Time     `db:"created_at" json:"created_at"`
	ReadAt         *time.Time    `db:"read_at" json:"read_at,omitempty"`
}

func NewSupportMessage(conversationID, senderID uuid.UUID, isAdmin bool, content string, mediaURL *string, mediaType string) *SupportMessage {
	if mediaType == "" {
		mediaType = string(MediaTypeText)
	}
	return &SupportMessage{
		ID:             uuid.New(),
		ConversationID: conversationID,
		SenderID:       senderID,
		IsAdmin:        isAdmin,
		Content:        content,
		MediaURL:       mediaURL,
		MediaType:      mediaType,
		Status:         MessageStatusSent,
		CreatedAt:      time.Now(),
	}
}

func (m *SupportMessage) MarkRead() {
	now := time.Now()
	m.ReadAt = &now
	m.Status = MessageStatusRead
}
