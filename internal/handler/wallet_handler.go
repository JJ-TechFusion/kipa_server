package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"kipa/internal/domain"
	"kipa/internal/handler/middleware"
	"kipa/internal/service"
	"kipa/internal/util"
)

type WalletHandler struct {
	walletService service.WalletService
}

func NewWalletHandler(walletService service.WalletService) *WalletHandler {
	return &WalletHandler{walletService: walletService}
}

func (h *WalletHandler) RegisterRoutes(router *gin.RouterGroup, authMiddleware *middleware.AuthMiddleware) {
	wallet := router.Group("/wallet")
	wallet.Use(authMiddleware.RequireAuth())
	{
		wallet.GET("", h.GetWallet)
		wallet.GET("/transactions", h.GetTransactions)
		wallet.POST("/withdraw", h.RequestWithdrawal)
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

	util.Success(c, http.StatusOK, WalletBalanceResponse{
		AvailableBalance: balance.AvailableBalance,
		LockedBalance:    balance.LockedBalance,
		TotalEarned:      balance.TotalEarned,
		TotalWithdrawn:   balance.TotalWithdrawn,
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
	AvailableBalance float64 `json:"available_balance"`
	LockedBalance    float64 `json:"locked_balance"`
	TotalEarned      float64 `json:"total_earned"`
	TotalWithdrawn   float64 `json:"total_withdrawn"`
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
