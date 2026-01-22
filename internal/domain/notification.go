package domain

import (
	"time"

	"github.com/google/uuid"
)

type NotificationType string

const (
	// Order lifecycle
	NotificationOrderPaid           NotificationType = "order_paid"
	NotificationOrderReadyForPickup NotificationType = "order_ready_pickup"
	NotificationRiderAssigned       NotificationType = "rider_assigned"
	NotificationOrderPickedUp       NotificationType = "order_picked_up"
	NotificationOrderDelivered      NotificationType = "order_delivered"
	NotificationEscrowReleased      NotificationType = "escrow_released"
	NotificationOrderCancelled      NotificationType = "order_cancelled"

	// Errand lifecycle
	NotificationErrandAccepted  NotificationType = "errand_accepted"
	NotificationErrandPickedUp  NotificationType = "errand_picked_up"
	NotificationErrandDelivered NotificationType = "errand_delivered"
	NotificationErrandCompleted NotificationType = "errand_completed"

	// Wallet events
	NotificationTopUpSuccess        NotificationType = "topup_success"
	NotificationDVATransfer         NotificationType = "dva_transfer"
	NotificationWithdrawalInitiated NotificationType = "withdrawal_initiated"
	NotificationWithdrawalSuccess   NotificationType = "withdrawal_success"
	NotificationWithdrawalFailed    NotificationType = "withdrawal_failed"

	// Dispute & Admin
	NotificationDisputeOpened   NotificationType = "dispute_opened"
	NotificationDisputeResolved NotificationType = "dispute_resolved"
	NotificationRefundProcessed NotificationType = "refund_processed"
	NotificationRiderVerified   NotificationType = "rider_verified"
	NotificationRiderSuspended  NotificationType = "rider_suspended"
	NotificationUserSuspended   NotificationType = "user_suspended"

	// Chat
	NotificationChatMessage NotificationType = "chat_message"

	// General
	NotificationBroadcast NotificationType = "broadcast"
)

type NotificationLog struct {
	ID               uuid.UUID         `db:"id" json:"id"`
	UserID           *uuid.UUID        `db:"user_id" json:"user_id,omitempty"`
	Title            string            `db:"title" json:"title"`
	Body             string            `db:"body" json:"body"`
	Data             map[string]string `db:"-" json:"data,omitempty"`
	NotificationType NotificationType  `db:"notification_type" json:"notification_type"`
	SentAt           time.Time         `db:"sent_at" json:"sent_at"`
	Delivered        bool              `db:"delivered" json:"delivered"`
}

type NotificationPayload struct {
	UserID uuid.UUID
	Title  string
	Body   string
	Data   map[string]string
	Type   NotificationType
}