package domain

import (
	"time"

	"github.com/google/uuid"
)

type MessageStatus string

const (
	MessageStatusSent      MessageStatus = "sent"
	MessageStatusDelivered MessageStatus = "delivered"
	MessageStatusRead      MessageStatus = "read"
)

type MediaType string

const (
	MediaTypeText  MediaType = "text"
	MediaTypeImage MediaType = "image"
)

type ChatMessage struct {
	ID          uuid.UUID  `db:"id" json:"id"`
	JobID       uuid.UUID  `db:"job_id" json:"job_id"`
	SenderID    uuid.UUID  `db:"sender_id" json:"sender_id"`
	ReceiverID  uuid.UUID  `db:"receiver_id" json:"receiver_id"`
	Content     string     `db:"content" json:"content"`
	MediaURL    *string    `db:"media_url" json:"media_url,omitempty"`
	MediaType   string     `db:"media_type" json:"media_type"`
	Status      string     `db:"status" json:"status"`
	CreatedAt   time.Time  `db:"created_at" json:"created_at"`
	DeliveredAt *time.Time `db:"delivered_at" json:"delivered_at,omitempty"`
	ReadAt      *time.Time `db:"read_at" json:"read_at,omitempty"`
}

func (m *ChatMessage) IsFromRider(riderUserID uuid.UUID) bool {
	return m.SenderID == riderUserID
}

func (m *ChatMessage) MarkDelivered() {
	now := time.Now()
	m.DeliveredAt = &now
	m.Status = string(MessageStatusDelivered)
}

func (m *ChatMessage) MarkRead() {
	now := time.Now()
	m.ReadAt = &now
	m.Status = string(MessageStatusRead)
}
