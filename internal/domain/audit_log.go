package domain

import (
	"time"

	"github.com/google/uuid"
)

type AuditAction string

const (
	AuditActionVerifyRider      AuditAction = "verify_rider"
	AuditActionSuspendRider     AuditAction = "suspend_rider"
	AuditActionUnsuspendRider   AuditAction = "unsuspend_rider"
	AuditActionSuspendUser      AuditAction = "suspend_user"
	AuditActionUnsuspendUser    AuditAction = "unsuspend_user"
	AuditActionVerifySeller     AuditAction = "verify_seller"
	AuditActionUpdatePricing    AuditAction = "update_delivery_pricing"
	AuditActionUpdateFees       AuditAction = "update_platform_fees"
	AuditActionCancelPayment    AuditAction = "cancel_payment_request"
	AuditActionResolveDispute   AuditAction = "resolve_dispute"
	AuditActionProcessRefund    AuditAction = "process_refund"
	AuditActionAdjustWallet     AuditAction = "adjust_wallet"
	AuditActionBroadcastNotification AuditAction = "broadcast_notification"
)

type AuditTargetType string

const (
	AuditTargetRider          AuditTargetType = "rider"
	AuditTargetUser           AuditTargetType = "user"
	AuditTargetPaymentRequest AuditTargetType = "payment_request"
	AuditTargetDispute        AuditTargetType = "dispute"
	AuditTargetWallet         AuditTargetType = "wallet"
	AuditTargetSettings       AuditTargetType = "settings"
	AuditTargetPlatform       AuditTargetType = "platform"
)

type AuditLog struct {
	ID         uuid.UUID       `json:"id"`
	AdminID    uuid.UUID       `json:"admin_id"`
	Action     AuditAction     `json:"action"`
	TargetType AuditTargetType `json:"target_type"`
	TargetID   *uuid.UUID      `json:"target_id,omitempty"`
	Details    map[string]any  `json:"details,omitempty"`
	IPAddress  string          `json:"ip_address,omitempty"`
	UserAgent  string          `json:"user_agent,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}
func NewAuditLog(adminID uuid.UUID, action AuditAction, targetType AuditTargetType, targetID *uuid.UUID) *AuditLog {
	return &AuditLog{
		ID:         uuid.New(),
		AdminID:    adminID,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		CreatedAt:  time.Now(),
	}
}
func (a *AuditLog) WithDetails(details map[string]any) *AuditLog {
	a.Details = details
	return a
}
func (a *AuditLog) WithRequest(ipAddress, userAgent string) *AuditLog {
	a.IPAddress = ipAddress
	a.UserAgent = userAgent
	return a
}
