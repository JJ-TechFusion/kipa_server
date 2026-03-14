package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/integration/telegram"
)

// LogisticsDisputeDetails contains tracking and evidence info for logistics disputes
type LogisticsDisputeDetails struct {
	DeliveryID         uuid.UUID
	DisputeID          uuid.UUID
	OpenerName         string
	OpenerType         string // "buyer" or "seller"
	ItemName           string
	Amount             float64
	Reason             string
	Carrier            string
	TrackingNumber     string
	TrackingURL        string
	ShipmentReceiptURL string // Receipt image uploaded when seller shipped
	EvidenceURLs       []string
}

// AdminAlertService sends critical alerts to admins via Telegram
type AdminAlertService interface {
	// Disputes
	AlertDisputeOpened(ctx context.Context, disputeID uuid.UUID, userName, itemName string, amount float64, reason string)
	AlertLogisticsDisputeOpened(ctx context.Context, details *LogisticsDisputeDetails)
	AlertReturnDamageClaimed(ctx context.Context, jobID uuid.UUID, userName, itemName string, amount float64, reason string)

	// High-value orders
	AlertHighValuePendingApproval(ctx context.Context, deliveryID uuid.UUID, buyerName, sellerName, itemName string, amount float64)

	// Withdrawals
	AlertWithdrawalFailed(ctx context.Context, withdrawalID uuid.UUID, userName string, amount float64, reason string)

	// Deadlines
	AlertShipDeadlineExceeded(ctx context.Context, deliveryID uuid.UUID, sellerName, itemName string, daysOverdue int)
	AlertAutoEscalatedToDispute(ctx context.Context, deliveryID uuid.UUID, buyerName, sellerName, itemName string, reason string)

	// Support
	AlertNewSupportMessage(ctx context.Context, conversationID uuid.UUID, userName, message string)
}

type AdminAlertServiceImpl struct {
	telegramClient  *telegram.Client
	notificationSvc NotificationService
	logger          *zap.Logger
}

func NewAdminAlertService(
	telegramClient *telegram.Client,
	notificationSvc NotificationService,
	logger *zap.Logger,
) AdminAlertService {
	return &AdminAlertServiceImpl{
		telegramClient:  telegramClient,
		notificationSvc: notificationSvc,
		logger:          logger,
	}
}

func (s *AdminAlertServiceImpl) sendTelegram(ctx context.Context, message string) {
	if s.telegramClient == nil || !s.telegramClient.IsEnabled() {
		return
	}

	if err := s.telegramClient.SendMessage(ctx, message); err != nil {
		s.logger.Warn("failed to send telegram alert", zap.Error(err))
	}
}

func (s *AdminAlertServiceImpl) sendPushToAdmins(ctx context.Context, title, body string, data map[string]string) {
	if s.notificationSvc == nil {
		return
	}

	if err := s.notificationSvc.SendToAdmins(ctx, title, body, data); err != nil {
		s.logger.Warn("failed to send push notification to admins", zap.Error(err))
	}
}

func (s *AdminAlertServiceImpl) AlertDisputeOpened(ctx context.Context, disputeID uuid.UUID, userName, itemName string, amount float64, reason string) {
	title := "Dispute Opened"
	body := fmt.Sprintf("%s opened a dispute for \"%s\" (₦%.2f)", userName, itemName, amount)

	telegramMsg := fmt.Sprintf(
		"<b>🚨 Dispute Opened</b>\n\n"+
			"<b>User:</b> %s\n"+
			"<b>Item:</b> %s\n"+
			"<b>Amount:</b> ₦%.2f\n"+
			"<b>Reason:</b> %s\n\n"+
			"<i>ID: %s</i>",
		userName, itemName, amount, reason, disputeID.String()[:8],
	)

	s.sendTelegram(ctx, telegramMsg)
	s.sendPushToAdmins(ctx, title, body, map[string]string{
		"type":       "dispute_opened",
		"dispute_id": disputeID.String(),
	})

	s.logger.Info("admin alert: dispute opened",
		zap.String("dispute_id", disputeID.String()),
		zap.String("user", userName),
	)
}

func (s *AdminAlertServiceImpl) AlertLogisticsDisputeOpened(ctx context.Context, details *LogisticsDisputeDetails) {
	title := "Interstate Dispute Opened"
	body := fmt.Sprintf("%s (%s) opened a dispute for \"%s\" (₦%.2f)", details.OpenerName, details.OpenerType, details.ItemName, details.Amount)

	// Build tracking info section
	trackingInfo := ""
	if details.Carrier != "" {
		trackingInfo += fmt.Sprintf("<b>Carrier:</b> %s\n", details.Carrier)
	}
	if details.TrackingNumber != "" {
		trackingInfo += fmt.Sprintf("<b>Tracking #:</b> %s\n", details.TrackingNumber)
	}
	if details.TrackingURL != "" {
		trackingInfo += fmt.Sprintf("<b>Track:</b> %s\n", details.TrackingURL)
	}
	if details.ShipmentReceiptURL != "" {
		trackingInfo += fmt.Sprintf("<b>Shipment Receipt:</b> %s\n", details.ShipmentReceiptURL)
	}

	// Build evidence section
	evidenceInfo := ""
	if len(details.EvidenceURLs) > 0 {
		evidenceInfo = "\n<b>Evidence:</b>\n"
		for i, url := range details.EvidenceURLs {
			if i >= 5 { // Limit to 5 images in telegram
				evidenceInfo += fmt.Sprintf("... and %d more\n", len(details.EvidenceURLs)-5)
				break
			}
			evidenceInfo += fmt.Sprintf("• %s\n", url)
		}
	}

	telegramMsg := fmt.Sprintf(
		"<b>🚨 Interstate Dispute Opened</b>\n\n"+
			"<b>Opened By:</b> %s (%s)\n"+
			"<b>Item:</b> %s\n"+
			"<b>Amount:</b> ₦%.2f\n"+
			"<b>Reason:</b> %s\n\n"+
			"%s"+
			"%s"+
			"<i>Delivery ID: %s</i>\n"+
			"<i>Dispute ID: %s</i>",
		details.OpenerName, details.OpenerType, details.ItemName, details.Amount, details.Reason,
		trackingInfo, evidenceInfo,
		details.DeliveryID.String()[:8], details.DisputeID.String()[:8],
	)

	s.sendTelegram(ctx, telegramMsg)
	s.sendPushToAdmins(ctx, title, body, map[string]string{
		"type":        "logistics_dispute_opened",
		"delivery_id": details.DeliveryID.String(),
		"dispute_id":  details.DisputeID.String(),
	})

	s.logger.Info("admin alert: logistics dispute opened",
		zap.String("delivery_id", details.DeliveryID.String()),
		zap.String("dispute_id", details.DisputeID.String()),
		zap.String("opener", details.OpenerName),
	)
}

func (s *AdminAlertServiceImpl) AlertReturnDamageClaimed(ctx context.Context, jobID uuid.UUID, userName, itemName string, amount float64, reason string) {
	title := "Return Damage Claimed"
	body := fmt.Sprintf("%s claims damage on returned \"%s\" (₦%.2f)", userName, itemName, amount)

	telegramMsg := fmt.Sprintf(
		"<b>⚠️ Return Damage Claimed</b>\n\n"+
			"<b>Buyer:</b> %s\n"+
			"<b>Item:</b> %s\n"+
			"<b>Amount:</b> ₦%.2f\n"+
			"<b>Reason:</b> %s\n\n"+
			"<i>Requires admin review</i>\n"+
			"<i>Job ID: %s</i>",
		userName, itemName, amount, reason, jobID.String()[:8],
	)

	s.sendTelegram(ctx, telegramMsg)
	s.sendPushToAdmins(ctx, title, body, map[string]string{
		"type":   "return_damage_claimed",
		"job_id": jobID.String(),
	})

	s.logger.Info("admin alert: return damage claimed",
		zap.String("job_id", jobID.String()),
		zap.String("user", userName),
	)
}

func (s *AdminAlertServiceImpl) AlertHighValuePendingApproval(ctx context.Context, deliveryID uuid.UUID, buyerName, sellerName, itemName string, amount float64) {
	title := "High-Value Order Pending Approval"
	body := fmt.Sprintf("\"%s\" (₦%.2f) requires admin approval", itemName, amount)

	telegramMsg := fmt.Sprintf(
		"<b>💰 High-Value Order Pending</b>\n\n"+
			"<b>Item:</b> %s\n"+
			"<b>Amount:</b> ₦%.2f\n"+
			"<b>Buyer:</b> %s\n"+
			"<b>Seller:</b> %s\n\n"+
			"<i>Requires admin approval to release funds</i>\n"+
			"<i>Delivery ID: %s</i>",
		itemName, amount, buyerName, sellerName, deliveryID.String()[:8],
	)

	s.sendTelegram(ctx, telegramMsg)
	s.sendPushToAdmins(ctx, title, body, map[string]string{
		"type":        "high_value_pending",
		"delivery_id": deliveryID.String(),
	})

	s.logger.Info("admin alert: high value pending approval",
		zap.String("delivery_id", deliveryID.String()),
		zap.Float64("amount", amount),
	)
}

func (s *AdminAlertServiceImpl) AlertWithdrawalFailed(ctx context.Context, withdrawalID uuid.UUID, userName string, amount float64, reason string) {
	title := "Withdrawal Failed"
	body := fmt.Sprintf("%s's withdrawal of ₦%.2f failed", userName, amount)

	telegramMsg := fmt.Sprintf(
		"<b>❌ Withdrawal Failed</b>\n\n"+
			"<b>User:</b> %s\n"+
			"<b>Amount:</b> ₦%.2f\n"+
			"<b>Reason:</b> %s\n\n"+
			"<i>User may contact support</i>\n"+
			"<i>ID: %s</i>",
		userName, amount, reason, withdrawalID.String()[:8],
	)

	s.sendTelegram(ctx, telegramMsg)
	s.sendPushToAdmins(ctx, title, body, map[string]string{
		"type":          "withdrawal_failed",
		"withdrawal_id": withdrawalID.String(),
	})

	s.logger.Info("admin alert: withdrawal failed",
		zap.String("withdrawal_id", withdrawalID.String()),
		zap.String("user", userName),
	)
}

func (s *AdminAlertServiceImpl) AlertShipDeadlineExceeded(ctx context.Context, deliveryID uuid.UUID, sellerName, itemName string, daysOverdue int) {
	title := "Ship Deadline Exceeded"
	body := fmt.Sprintf("%s hasn't shipped \"%s\" (%d days overdue)", sellerName, itemName, daysOverdue)

	telegramMsg := fmt.Sprintf(
		"<b>📦 Ship Deadline Exceeded</b>\n\n"+
			"<b>Seller:</b> %s\n"+
			"<b>Item:</b> %s\n"+
			"<b>Days Overdue:</b> %d\n\n"+
			"<i>Delivery ID: %s</i>",
		sellerName, itemName, daysOverdue, deliveryID.String()[:8],
	)

	s.sendTelegram(ctx, telegramMsg)
	s.sendPushToAdmins(ctx, title, body, map[string]string{
		"type":        "ship_deadline_exceeded",
		"delivery_id": deliveryID.String(),
	})

	s.logger.Info("admin alert: ship deadline exceeded",
		zap.String("delivery_id", deliveryID.String()),
		zap.String("seller", sellerName),
	)
}

func (s *AdminAlertServiceImpl) AlertAutoEscalatedToDispute(ctx context.Context, deliveryID uuid.UUID, buyerName, sellerName, itemName string, reason string) {
	title := "Auto-Escalated to Dispute"
	body := fmt.Sprintf("\"%s\" auto-escalated due to %s", itemName, reason)

	telegramMsg := fmt.Sprintf(
		"<b>⚡ Auto-Escalated to Dispute</b>\n\n"+
			"<b>Item:</b> %s\n"+
			"<b>Buyer:</b> %s\n"+
			"<b>Seller:</b> %s\n"+
			"<b>Reason:</b> %s\n\n"+
			"<i>Delivery ID: %s</i>",
		itemName, buyerName, sellerName, reason, deliveryID.String()[:8],
	)

	s.sendTelegram(ctx, telegramMsg)
	s.sendPushToAdmins(ctx, title, body, map[string]string{
		"type":        "auto_escalated",
		"delivery_id": deliveryID.String(),
	})

	s.logger.Info("admin alert: auto-escalated to dispute",
		zap.String("delivery_id", deliveryID.String()),
	)
}

func (s *AdminAlertServiceImpl) AlertNewSupportMessage(ctx context.Context, conversationID uuid.UUID, userName, message string) {
	title := fmt.Sprintf("Support: %s", userName)
	if len(message) > 100 {
		message = message[:100] + "..."
	}

	telegramMsg := fmt.Sprintf("<b>%s</b>\n%s", title, message)

	s.sendTelegram(ctx, telegramMsg)
	s.sendPushToAdmins(ctx, title, message, map[string]string{
		"type":            "support_message",
		"conversation_id": conversationID.String(),
	})
}
