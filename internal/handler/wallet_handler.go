package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"kipa/internal/domain"
	"kipa/internal/handler/middleware"
	"kipa/internal/repository"
	"kipa/internal/service"
	"kipa/internal/util"
)

type WalletHandler struct {
	walletService service.WalletService
	escrowService service.EscrowService
}

func NewWalletHandler(walletService service.WalletService, escrowService service.EscrowService) *WalletHandler {
	return &WalletHandler{
		walletService: walletService,
		escrowService: escrowService,
	}
}

func (h *WalletHandler) RegisterRoutes(router *gin.RouterGroup, authMiddleware *middleware.AuthMiddleware) {
	wallet := router.Group("/wallet")
	wallet.Use(authMiddleware.RequireAuth())
	{
		wallet.GET("", h.GetWallet)
		wallet.GET("/transactions", h.GetTransactions)
		wallet.GET("/pending", h.GetPendingFunds)
		wallet.POST("/withdraw", h.RequestWithdrawal)
		wallet.GET("/pin/status", h.GetPinStatus)
		wallet.POST("/pin", h.CreatePin)
		wallet.POST("/pin/verify", h.VerifyPin)
		wallet.PUT("/pin", h.ChangePin)
		wallet.POST("/pin/reset/request", h.RequestPinReset)
		wallet.POST("/pin/reset/confirm", h.ConfirmPinReset)
	}
}
func (h *WalletHandler) GetWallet(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	balance, err := h.walletService.GetBalance(c.Request.Context(), userID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	pendingEscrows, err := h.escrowService.GetPendingForUser(c.Request.Context(), userID, 1000, 0)
	if err != nil {
		h.handleError(c, err)
		return
	}
	var pendingAsBuyer float64  // Money buyer paid, held in escrow (potential refund)
	var pendingAsSeller float64 // Money seller will receive when escrow releases

	for _, escrow := range pendingEscrows {
		if escrow.IsBuyer {
			pendingAsBuyer += escrow.TotalLocked
		} else {
			pendingAsSeller += escrow.ItemAmount
		}
	}
	hasPin, _ := h.walletService.HasPin(c.Request.Context(), userID)

	util.Success(c, http.StatusOK, WalletBalanceResponse{
		AvailableBalance:      balance.AvailableBalance,
		LockedBalance:         balance.LockedBalance,
		TotalEarned:           balance.TotalEarned,
		TotalWithdrawn:        balance.TotalWithdrawn,
		PendingEscrowAsBuyer:  pendingAsBuyer,
		PendingEscrowAsSeller: pendingAsSeller,
		TotalPendingEscrow:    pendingAsBuyer + pendingAsSeller,
		HasPin:                hasPin,
	})
}
func (h *WalletHandler) GetTransactions(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	limit := 20
	offset := 0

	transactions, err := h.walletService.GetTransactions(c.Request.Context(), userID, limit, offset)
	if err != nil {
		h.handleError(c, err)
		return
	}

	responses := make([]WalletTransactionResponse, len(transactions))
	for i, tx := range transactions {
		responses[i] = walletTransactionToResponse(tx)
	}

	util.Success(c, http.StatusOK, gin.H{
		"transactions": responses,
		"count":        len(responses),
	})
}
func (h *WalletHandler) RequestWithdrawal(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req WithdrawalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}
	balance, err := h.walletService.GetBalance(c.Request.Context(), userID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	if balance.AvailableBalance < req.Amount {
		util.Error(c, http.StatusBadRequest, "INSUFFICIENT_BALANCE", "Insufficient balance for withdrawal")
		return
	}
	idempotencyKey := uuid.New().String()
	_, err = h.walletService.Debit(
		c.Request.Context(),
		userID,
		req.Amount,
		domain.WalletTxWithdrawal,
		"withdrawal",
		uuid.Nil,
		"Withdrawal to bank account",
		idempotencyKey,
	)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"message":         "Withdrawal request submitted successfully",
		"amount":          req.Amount,
		"status":          "processing",
		"idempotency_key": idempotencyKey,
	})
}

func (h *WalletHandler) handleError(c *gin.Context, err error) {
	status, code, message := mapWalletErrorToResponse(err)
	util.Error(c, status, code, message)
}

func mapWalletErrorToResponse(err error) (int, string, string) {
	switch {
	case errors.Is(err, domain.ErrWalletNotFound):
		return http.StatusNotFound, "WALLET_NOT_FOUND", "Wallet not found"
	case errors.Is(err, domain.ErrInsufficientBalance):
		return http.StatusBadRequest, "INSUFFICIENT_BALANCE", "Insufficient balance"
	case errors.Is(err, domain.ErrInsufficientLockedBalance):
		return http.StatusBadRequest, "INSUFFICIENT_LOCKED_BALANCE", "Insufficient locked balance"
	case errors.Is(err, domain.ErrIdempotencyKeyExists):
		return http.StatusConflict, "DUPLICATE_REQUEST", "This request has already been processed"
	default:
		return http.StatusInternalServerError, domain.CodeInternalError, "An unexpected error occurred"
	}
}

type WithdrawalRequest struct {
	Amount          float64 `json:"amount" binding:"required,gt=0"`
	BankAccountID   string  `json:"bank_account_id"`
	BankAccountName string  `json:"bank_account_name"`
	BankCode        string  `json:"bank_code"`
	AccountNumber   string  `json:"account_number"`
}

type WalletBalanceResponse struct {
	AvailableBalance      float64 `json:"available_balance"`
	LockedBalance         float64 `json:"locked_balance"`
	TotalEarned           float64 `json:"total_earned"`
	TotalWithdrawn        float64 `json:"total_withdrawn"`
	PendingEscrowAsBuyer  float64 `json:"pending_escrow_as_buyer"`  // Money paid by user, held in escrow
	PendingEscrowAsSeller float64 `json:"pending_escrow_as_seller"` // Money user will receive when escrow releases
	TotalPendingEscrow    float64 `json:"total_pending_escrow"`     // Combined pending escrow
	HasPin                bool    `json:"has_pin"`                  // Whether wallet PIN is set
}

type WalletTransactionResponse struct {
	ID            uuid.UUID `json:"id"`
	Type          string    `json:"type"`
	Amount        float64   `json:"amount"`
	BalanceAfter  float64   `json:"balance_after"`
	ReferenceType *string   `json:"reference_type,omitempty"`
	ReferenceID   *uuid.UUID `json:"reference_id,omitempty"`
	Description   *string   `json:"description,omitempty"`
	CreatedAt     string    `json:"created_at"`
}

func walletTransactionToResponse(tx *domain.WalletTransaction) WalletTransactionResponse {
	return WalletTransactionResponse{
		ID:            tx.ID,
		Type:          string(tx.Type),
		Amount:        tx.Amount,
		BalanceAfter:  tx.BalanceAfter,
		ReferenceType: tx.ReferenceType,
		ReferenceID:   tx.ReferenceID,
		Description:   tx.Description,
		CreatedAt:     tx.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func (h *WalletHandler) GetPendingFunds(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	limit := 50
	offset := 0

	pendingEscrows, err := h.escrowService.GetPendingForUser(c.Request.Context(), userID, limit, offset)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to fetch pending funds")
		return
	}

	responses := make([]PendingFundResponse, len(pendingEscrows))
	var totalPending float64

	for i, escrow := range pendingEscrows {
		responses[i] = pendingEscrowToResponse(escrow)
		totalPending += escrow.ItemAmount
	}

	util.Success(c, http.StatusOK, gin.H{
		"pending_funds":  responses,
		"count":          len(responses),
		"total_pending":  totalPending,
	})
}

type PendingFundResponse struct {
	EscrowID            uuid.UUID `json:"escrow_id"`
	PaymentRequestID    uuid.UUID `json:"payment_request_id"`
	Status              string    `json:"status"`
	ItemAmount          float64   `json:"item_amount"`
	TotalLocked         float64   `json:"total_locked"`
	ItemName            string    `json:"item_name"`
	ItemDescription     *string   `json:"item_description,omitempty"`
	ProcessingTimeHours *int      `json:"processing_time_hours,omitempty"`
	Role                string    `json:"role"` // "buyer" or "seller"
	CounterParty        CounterPartyInfo `json:"counter_party"`
	FundedAt            *string   `json:"funded_at,omitempty"`
	CreatedAt           string    `json:"created_at"`
}

type CounterPartyInfo struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	PhotoURL *string   `json:"photo_url,omitempty"`
}

func pendingEscrowToResponse(e *repository.PendingEscrowDetails) PendingFundResponse {
	role := "seller"
	if e.IsBuyer {
		role = "buyer"
	}

	resp := PendingFundResponse{
		EscrowID:            e.EscrowID,
		PaymentRequestID:    e.PaymentRequestID,
		Status:              string(e.EscrowStatus),
		ItemAmount:          e.ItemAmount,
		TotalLocked:         e.TotalLocked,
		ItemName:            e.ItemName,
		ItemDescription:     e.ItemDescription,
		ProcessingTimeHours: e.ProcessingTimeHours,
		Role:                role,
		CounterParty: CounterPartyInfo{
			ID:       e.CounterPartyID,
			Name:     e.CounterPartyName,
			PhotoURL: e.CounterPartyPhotoURL,
		},
		CreatedAt: e.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	if e.FundedAt != nil {
		fundedAt := e.FundedAt.Format("2006-01-02T15:04:05Z07:00")
		resp.FundedAt = &fundedAt
	}

	return resp
}

type CreatePinRequest struct {
	Pin string `json:"pin" binding:"required"`
}

type VerifyPinRequest struct {
	Pin string `json:"pin" binding:"required"`
}

type ChangePinRequest struct {
	OldPin string `json:"old_pin" binding:"required"`
	NewPin string `json:"new_pin" binding:"required"`
}

type ConfirmPinResetRequest struct {
	OTPCode string `json:"otp_code" binding:"required"`
	NewPin  string `json:"new_pin" binding:"required"`
}

func (h *WalletHandler) GetPinStatus(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	status, err := h.walletService.GetPinStatus(c.Request.Context(), userID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, status)
}

func (h *WalletHandler) CreatePin(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req CreatePinRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	if err := h.walletService.CreatePin(c.Request.Context(), userID, req.Pin); err != nil {
		h.handlePinError(c, err)
		return
	}

	util.Success(c, http.StatusCreated, gin.H{
		"message": "PIN created successfully",
	})
}

func (h *WalletHandler) VerifyPin(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req VerifyPinRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	if err := h.walletService.VerifyPin(c.Request.Context(), userID, req.Pin); err != nil {
		h.handlePinError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"message": "PIN verified successfully",
		"valid":   true,
	})
}

func (h *WalletHandler) ChangePin(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req ChangePinRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	if err := h.walletService.ChangePin(c.Request.Context(), userID, req.OldPin, req.NewPin); err != nil {
		h.handlePinError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"message": "PIN changed successfully",
	})
}

func (h *WalletHandler) handlePinError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrPinAlreadySet):
		util.Error(c, http.StatusConflict, "PIN_ALREADY_SET", "Wallet PIN already set. Use change PIN to update.")
	case errors.Is(err, domain.ErrPinNotSet):
		util.Error(c, http.StatusBadRequest, "PIN_NOT_SET", "Wallet PIN not set. Please create a PIN first.")
	case errors.Is(err, domain.ErrInvalidPin):
		util.Error(c, http.StatusUnauthorized, "INVALID_PIN", "Invalid PIN")
	case errors.Is(err, domain.ErrPinLocked):
		util.Error(c, http.StatusTooManyRequests, "PIN_LOCKED", "Wallet locked due to too many failed attempts. Please try again later.")
	case errors.Is(err, domain.ErrPinTooShort):
		util.Error(c, http.StatusBadRequest, "PIN_TOO_SHORT", "PIN must be at least 4 digits")
	case errors.Is(err, domain.ErrPinTooLong):
		util.Error(c, http.StatusBadRequest, "PIN_TOO_LONG", "PIN must be at most 6 digits")
	case errors.Is(err, domain.ErrPinMustBeNumeric):
		util.Error(c, http.StatusBadRequest, "PIN_MUST_BE_NUMERIC", "PIN must contain only digits")
	case errors.Is(err, domain.ErrPinResetNotRequested):
		util.Error(c, http.StatusBadRequest, "PIN_RESET_NOT_REQUESTED", "PIN reset not requested. Call request endpoint first.")
	case errors.Is(err, domain.ErrOTPRateLimitExceeded):
		util.Error(c, http.StatusTooManyRequests, "OTP_RATE_LIMIT_EXCEEDED", "Too many OTP requests. Please try again later.")
	case errors.Is(err, domain.ErrOTPResendCooldown):
		util.Error(c, http.StatusTooManyRequests, "OTP_RESEND_COOLDOWN", "Please wait before requesting another OTP.")
	case errors.Is(err, domain.ErrSMSDeliveryFailed):
		util.Error(c, http.StatusServiceUnavailable, "SMS_DELIVERY_FAILED", "Failed to send OTP. Please try again.")
	case errors.Is(err, domain.ErrInvalidOTP):
		util.Error(c, http.StatusBadRequest, "INVALID_OTP", "Invalid or expired OTP code.")
	default:
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "An unexpected error occurred")
	}
}

func (h *WalletHandler) RequestPinReset(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	response, err := h.walletService.RequestPinReset(c.Request.Context(), userID)
	if err != nil {
		h.handlePinError(c, err)
		return
	}

	util.Success(c, http.StatusOK, response)
}

func (h *WalletHandler) ConfirmPinReset(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req ConfirmPinResetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	if err := h.walletService.ConfirmPinReset(c.Request.Context(), userID, req.OTPCode, req.NewPin); err != nil {
		h.handlePinError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"message": "PIN reset successfully",
	})
}
