package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"kipa/internal/domain"
	"kipa/internal/integration/flutterwave"
	"kipa/internal/integration/paystack"
	"kipa/internal/service"
	"kipa/internal/util"
)

type WebhookHandler struct {
	paystackClient    *paystack.Client
	flutterwaveClient *flutterwave.Client
	fundingService    service.WalletFundingService
	orderService      service.OrderService
	logger            *zap.Logger
}

func NewWebhookHandler(
	paystackClient *paystack.Client,
	flutterwaveClient *flutterwave.Client,
	fundingService service.WalletFundingService,
	orderService service.OrderService,
	logger *zap.Logger,
) *WebhookHandler {
	return &WebhookHandler{
		paystackClient:    paystackClient,
		flutterwaveClient: flutterwaveClient,
		fundingService:    fundingService,
		orderService:      orderService,
		logger:            logger,
	}
}

func (h *WebhookHandler) RegisterRoutes(router *gin.RouterGroup) {
	webhooks := router.Group("/webhooks")
	{
		webhooks.POST("/paystack", h.HandlePaystackWebhook)
		webhooks.POST("/flutterwave", h.HandleFlutterwaveWebhook)
	}
}
func (h *WebhookHandler) HandlePaystackWebhook(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.logger.Error("failed to read webhook body", zap.Error(err))
		util.Error(c, http.StatusBadRequest, "INVALID_BODY", "Failed to read request body")
		return
	}
	signature := c.GetHeader("X-Paystack-Signature")
	if !h.paystackClient.VerifyWebhookSignature(body, signature) {
		h.logger.Warn("invalid webhook signature",
			zap.String("signature", signature),
		)
		util.Error(c, http.StatusUnauthorized, "INVALID_SIGNATURE", "Invalid webhook signature")
		return
	}
	event, err := h.paystackClient.ParseWebhookEvent(body)
	if err != nil {
		h.logger.Error("failed to parse webhook event", zap.Error(err))
		util.Error(c, http.StatusBadRequest, "INVALID_EVENT", "Failed to parse webhook event")
		return
	}

	h.logger.Info("received paystack webhook",
		zap.String("event", event.Event),
	)
	switch event.Event {
	case paystack.EventChargeSuccess:
		h.handleChargeSuccess(c, event)
	case paystack.EventTransferSuccess:
		h.handleTransferSuccess(c, event)
	case paystack.EventTransferFailed:
		h.handleTransferFailed(c, event)
	case paystack.EventTransferReversed:
		h.handleTransferReversed(c, event)
	case paystack.EventRefundProcessed:
		h.handleRefundProcessed(c, event)
	case "dedicatedaccount.assign.success":
		h.handleDVAAssigned(c, event)
	default:
		h.logger.Info("unhandled webhook event type",
			zap.String("event", event.Event),
		)
		c.JSON(http.StatusOK, gin.H{"message": "Event received"})
	}
}
func (h *WebhookHandler) handleChargeSuccess(c *gin.Context, event *paystack.WebhookEvent) {
	txData, err := h.paystackClient.GetTransactionDataFromEvent(event)
	if err != nil {
		h.logger.Error("failed to parse transaction data", zap.Error(err))
		util.Error(c, http.StatusBadRequest, "INVALID_DATA", "Failed to parse transaction data")
		return
	}

	h.logger.Info("charge success webhook received",
		zap.String("reference", txData.Reference),
		zap.Int64("amount", txData.Amount),
		zap.String("status", string(txData.Status)),
		zap.String("channel", txData.Channel),
	)
	if len(txData.Reference) >= 9 && txData.Reference[:9] == "kipa_ord_" {
		h.handleOrderPayment(c, txData.Reference)
		return
	}
	if len(txData.Reference) >= 6 && txData.Reference[:6] == "topup_" {
		if err := h.fundingService.ProcessTopUp(c.Request.Context(), txData.Reference); err != nil {
			h.logger.Error("failed to process top-up from webhook",
				zap.String("reference", txData.Reference),
				zap.Error(err),
			)
		} else {
			h.logger.Info("top-up processed from webhook",
				zap.String("reference", txData.Reference),
			)
		}
	}
	if txData.Channel == "dedicated_nuban" {
		h.handleDVATransfer(c, txData)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Charge success acknowledged",
		"reference": txData.Reference,
	})
}
func (h *WebhookHandler) handleOrderPayment(c *gin.Context, reference string) {
	h.logger.Info("order payment webhook received",
		zap.String("reference", reference),
	)
	resp, err := h.orderService.VerifyPayment(c.Request.Context(), reference)
	if err != nil {
		h.logger.Error("failed to verify order payment from webhook",
			zap.String("reference", reference),
			zap.Error(err),
		)
		c.JSON(http.StatusOK, gin.H{
			"message":   "Order payment webhook received but verification failed",
			"reference": reference,
			"error":     err.Error(),
		})
		return
	}

	h.logger.Info("order payment verified from webhook",
		zap.String("reference", reference),
		zap.String("order_id", resp.OrderID.String()),
		zap.String("status", resp.Status),
	)

	c.JSON(http.StatusOK, gin.H{
		"message":   "Order payment verified",
		"reference": reference,
		"order_id":  resp.OrderID,
		"status":    resp.Status,
	})
}
type DVATransferData struct {
	Reference       string `json:"reference"`
	Amount          int64  `json:"amount"`
	CustomerCode    string `json:"customer_code"`
	VirtualAccount  string `json:"virtual_account"`
	SenderBank      string `json:"sender_bank"`
	SenderName      string `json:"sender_name"`
	SenderBankAccNo string `json:"sender_bank_account_number"`
}
func (h *WebhookHandler) handleDVATransfer(c *gin.Context, txData *paystack.TransactionData) {
	h.logger.Info("DVA transfer received",
		zap.String("customer_code", txData.Customer.CustomerCode),
		zap.Int64("amount", txData.Amount),
		zap.String("reference", txData.Reference),
	)
	var metadata map[string]interface{}
	if err := json.Unmarshal(txData.Metadata, &metadata); err != nil {
		h.logger.Warn("failed to parse metadata", zap.Error(err))
	}

	h.logger.Info("DVA funding processed",
		zap.String("customer_code", txData.Customer.CustomerCode),
		zap.Float64("amount_naira", paystack.KoboToNaira(txData.Amount)),
	)

	c.JSON(http.StatusOK, gin.H{
		"message":       "DVA transfer acknowledged",
		"reference":     txData.Reference,
		"customer_code": txData.Customer.CustomerCode,
	})
}
type TransferWebhookData struct {
	Amount        int64  `json:"amount"`
	Currency      string `json:"currency"`
	Domain        string `json:"domain"`
	ID            int64  `json:"id"`
	Integration   int64  `json:"integration"`
	Reason        string `json:"reason"`
	Reference     string `json:"reference"`
	Source        string `json:"source"`
	SourceDetails struct {
		Narration string `json:"narration"`
	} `json:"source_details"`
	Status       string `json:"status"`
	TransferCode string `json:"transfer_code"`
	Recipient    struct {
		Active        bool   `json:"active"`
		Currency      string `json:"currency"`
		Domain        string `json:"domain"`
		Email         string `json:"email"`
		ID            int64  `json:"id"`
		Integration   int64  `json:"integration"`
		Name          string `json:"name"`
		RecipientCode string `json:"recipient_code"`
		Type          string `json:"type"`
		Details       struct {
			AccountNumber string `json:"account_number"`
			AccountName   string `json:"account_name"`
			BankCode      string `json:"bank_code"`
			BankName      string `json:"bank_name"`
		} `json:"details"`
	} `json:"recipient"`
	Session struct {
		Provider string `json:"provider"`
		ID       string `json:"id"`
	} `json:"session"`
	FailureReason string `json:"failure_reason"`
}
func (h *WebhookHandler) handleTransferSuccess(c *gin.Context, event *paystack.WebhookEvent) {
	var transferData TransferWebhookData
	if err := json.Unmarshal(event.Data, &transferData); err != nil {
		h.logger.Error("failed to parse transfer data", zap.Error(err))
		util.Error(c, http.StatusBadRequest, "INVALID_DATA", "Failed to parse transfer data")
		return
	}

	h.logger.Info("transfer success webhook received",
		zap.String("reference", transferData.Reference),
		zap.Int64("amount", transferData.Amount),
		zap.String("transfer_code", transferData.TransferCode),
	)
	if err := h.fundingService.ProcessWithdrawalWebhook(
		c.Request.Context(),
		transferData.Reference,
		domain.WithdrawalStatusSuccess,
		"",
	); err != nil {
		h.logger.Error("failed to process transfer success webhook",
			zap.String("reference", transferData.Reference),
			zap.Error(err),
		)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Transfer success acknowledged",
		"reference": transferData.Reference,
	})
}
func (h *WebhookHandler) handleTransferFailed(c *gin.Context, event *paystack.WebhookEvent) {
	var transferData TransferWebhookData
	if err := json.Unmarshal(event.Data, &transferData); err != nil {
		h.logger.Error("failed to parse transfer data", zap.Error(err))
		util.Error(c, http.StatusBadRequest, "INVALID_DATA", "Failed to parse transfer data")
		return
	}

	h.logger.Warn("transfer failed webhook received",
		zap.String("reference", transferData.Reference),
		zap.Int64("amount", transferData.Amount),
		zap.String("failure_reason", transferData.FailureReason),
	)
	if err := h.fundingService.ProcessWithdrawalWebhook(
		c.Request.Context(),
		transferData.Reference,
		domain.WithdrawalStatusFailed,
		transferData.FailureReason,
	); err != nil {
		h.logger.Error("failed to process transfer failed webhook",
			zap.String("reference", transferData.Reference),
			zap.Error(err),
		)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Transfer failed acknowledged",
		"reference": transferData.Reference,
	})
}
func (h *WebhookHandler) handleTransferReversed(c *gin.Context, event *paystack.WebhookEvent) {
	var transferData TransferWebhookData
	if err := json.Unmarshal(event.Data, &transferData); err != nil {
		h.logger.Error("failed to parse transfer data", zap.Error(err))
		util.Error(c, http.StatusBadRequest, "INVALID_DATA", "Failed to parse transfer data")
		return
	}

	h.logger.Warn("transfer reversed webhook received",
		zap.String("reference", transferData.Reference),
		zap.Int64("amount", transferData.Amount),
	)
	if err := h.fundingService.ProcessWithdrawalWebhook(
		c.Request.Context(),
		transferData.Reference,
		domain.WithdrawalStatusReversed,
		"Transfer was reversed",
	); err != nil {
		h.logger.Error("failed to process transfer reversed webhook",
			zap.String("reference", transferData.Reference),
			zap.Error(err),
		)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Transfer reversed acknowledged",
		"reference": transferData.Reference,
	})
}
func (h *WebhookHandler) handleRefundProcessed(c *gin.Context, event *paystack.WebhookEvent) {
	txData, err := h.paystackClient.GetTransactionDataFromEvent(event)
	if err != nil {
		h.logger.Error("failed to parse refund data", zap.Error(err))
		util.Error(c, http.StatusBadRequest, "INVALID_DATA", "Failed to parse refund data")
		return
	}

	h.logger.Info("refund processed webhook received",
		zap.String("reference", txData.Reference),
		zap.Int64("amount", txData.Amount),
	)

	c.JSON(http.StatusOK, gin.H{
		"message":   "Refund processed acknowledged",
		"reference": txData.Reference,
	})
}
func (h *WebhookHandler) handleDVAAssigned(c *gin.Context, _ *paystack.WebhookEvent) {
	h.logger.Info("DVA assigned webhook received")

	c.JSON(http.StatusOK, gin.H{
		"message": "DVA assignment acknowledged",
	})
}

func (h *WebhookHandler) HandleFlutterwaveWebhook(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.logger.Error("failed to read Flutterwave webhook body", zap.Error(err))
		util.Error(c, http.StatusBadRequest, "INVALID_BODY", "Failed to read request body")
		return
	}
	rawBody := string(body)
	if len(rawBody) > 2000 {
		rawBody = rawBody[:2000] + "...(truncated)"
	}
	h.logger.Debug("Flutterwave raw webhook body",
		zap.String("body", rawBody),
	)
	verifHash := c.GetHeader("verif-hash")
	if !h.flutterwaveClient.VerifyWebhookSignature(verifHash) {
		h.logger.Warn("invalid Flutterwave webhook signature",
			zap.String("verif_hash", verifHash),
		)
		util.Error(c, http.StatusUnauthorized, "INVALID_SIGNATURE", "Invalid webhook signature")
		return
	}

	event, err := h.flutterwaveClient.ParseWebhookEvent(body)
	if err != nil {
		h.logger.Error("failed to parse Flutterwave webhook event", zap.Error(err))
		util.Error(c, http.StatusBadRequest, "INVALID_EVENT", "Failed to parse webhook event")
		return
	}

	h.logger.Info("received Flutterwave webhook",
		zap.String("event", event.Event),
		zap.String("raw_data_preview", string(event.Data)[:min(500, len(event.Data))]),
	)

	switch event.Event {
	case flutterwave.EventChargeCompleted:
		h.handleFlutterwaveChargeCompleted(c, event)
	case flutterwave.EventTransferCompleted:
		h.handleFlutterwaveTransferCompleted(c, event)
	case flutterwave.EventTransferFailed:
		h.handleFlutterwaveTransferFailed(c, event)
	default:
		h.logger.Info("unhandled Flutterwave webhook event type",
			zap.String("event", event.Event),
		)
		c.JSON(http.StatusOK, gin.H{"message": "Event received"})
	}
}

func (h *WebhookHandler) handleFlutterwaveChargeCompleted(c *gin.Context, event *flutterwave.WebhookEvent) {
	chargeData, err := h.flutterwaveClient.GetChargeDataFromEvent(event)
	if err != nil {
		h.logger.Error("failed to parse Flutterwave charge data", zap.Error(err))
		util.Error(c, http.StatusBadRequest, "INVALID_DATA", "Failed to parse charge data")
		return
	}

	h.logger.Info("Flutterwave charge completed webhook received",
		zap.String("tx_ref", chargeData.TxRef),
		zap.Float64("amount", chargeData.Amount),
		zap.String("status", chargeData.Status),
		zap.String("payment_type", chargeData.PaymentType),
	)
	if len(chargeData.TxRef) >= 3 && chargeData.TxRef[:3] == "va-" {
		reference := chargeData.FlwRef
		if reference == "" {
			reference = fmt.Sprintf("%d", chargeData.ID)
		}

		if err := h.fundingService.ProcessVirtualAccountPayment(c.Request.Context(), chargeData.TxRef, chargeData.Amount, reference); err != nil {
			h.logger.Error("failed to process virtual account payment from webhook",
				zap.String("tx_ref", chargeData.TxRef),
				zap.Error(err),
			)
		} else {
			h.logger.Info("Virtual account payment processed from webhook",
				zap.String("tx_ref", chargeData.TxRef),
				zap.Float64("amount", chargeData.Amount),
			)
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Virtual account payment acknowledged",
			"tx_ref":  chargeData.TxRef,
		})
		return
	}
	if len(chargeData.TxRef) >= 9 && chargeData.TxRef[:9] == "fw_topup_" {
		if err := h.fundingService.ProcessFlutterwaveTopUp(c.Request.Context(), chargeData.TxRef); err != nil {
			h.logger.Error("failed to process Flutterwave top-up from webhook",
				zap.String("tx_ref", chargeData.TxRef),
				zap.Error(err),
			)
		} else {
			h.logger.Info("Flutterwave top-up processed from webhook",
				zap.String("tx_ref", chargeData.TxRef),
			)
		}
	}
	if len(chargeData.TxRef) >= 9 && chargeData.TxRef[:9] == "kipa_ord_" {
		if _, err := h.orderService.VerifyPayment(c.Request.Context(), chargeData.TxRef); err != nil {
			h.logger.Error("failed to process Flutterwave order payment from webhook",
				zap.String("tx_ref", chargeData.TxRef),
				zap.Error(err),
			)
		} else {
			h.logger.Info("Flutterwave order payment processed from webhook",
				zap.String("tx_ref", chargeData.TxRef),
			)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Charge completed acknowledged",
		"tx_ref":  chargeData.TxRef,
	})
}

func (h *WebhookHandler) handleFlutterwaveTransferCompleted(c *gin.Context, event *flutterwave.WebhookEvent) {
	transferData, err := h.flutterwaveClient.GetTransferDataFromEvent(event)
	if err != nil {
		h.logger.Error("failed to parse Flutterwave transfer data", zap.Error(err))
		util.Error(c, http.StatusBadRequest, "INVALID_DATA", "Failed to parse transfer data")
		return
	}

	h.logger.Info("Flutterwave transfer completed webhook received",
		zap.String("reference", transferData.Reference),
		zap.Float64("amount", transferData.Amount),
		zap.String("status", transferData.Status),
		zap.String("debit_currency", transferData.DebitCurrency),
		zap.String("currency", transferData.Currency),
		zap.String("narration", transferData.Narration),
		zap.String("account_number", transferData.AccountNumber),
		zap.String("bank_name", transferData.BankName),
	)
	if err := h.fundingService.ProcessFlutterwaveWithdrawalWebhook(
		c.Request.Context(),
		transferData.Reference,
		domain.WithdrawalStatusSuccess,
		"",
	); err != nil {
		h.logger.Error("failed to process Flutterwave transfer success webhook",
			zap.String("reference", transferData.Reference),
			zap.Error(err),
		)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Transfer success acknowledged",
		"reference": transferData.Reference,
	})
}

func (h *WebhookHandler) handleFlutterwaveTransferFailed(c *gin.Context, event *flutterwave.WebhookEvent) {
	transferData, err := h.flutterwaveClient.GetTransferDataFromEvent(event)
	if err != nil {
		h.logger.Error("failed to parse Flutterwave transfer data", zap.Error(err))
		util.Error(c, http.StatusBadRequest, "INVALID_DATA", "Failed to parse transfer data")
		return
	}

	h.logger.Warn("Flutterwave transfer failed webhook received",
		zap.String("reference", transferData.Reference),
		zap.Float64("amount", transferData.Amount),
		zap.String("complete_message", transferData.CompleteMessage),
	)
	if err := h.fundingService.ProcessFlutterwaveWithdrawalWebhook(
		c.Request.Context(),
		transferData.Reference,
		domain.WithdrawalStatusFailed,
		transferData.CompleteMessage,
	); err != nil {
		h.logger.Error("failed to process Flutterwave transfer failed webhook",
			zap.String("reference", transferData.Reference),
			zap.Error(err),
		)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Transfer failed acknowledged",
		"reference": transferData.Reference,
	})
}
