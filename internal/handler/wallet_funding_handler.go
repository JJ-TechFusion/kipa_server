package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/domain"
	"kipa/internal/handler/middleware"
	"kipa/internal/integration/paystack"
	"kipa/internal/repository"
	"kipa/internal/service"
	"kipa/internal/util"
)

// WalletFundingHandler handles wallet funding operations
type WalletFundingHandler struct {
	fundingService service.WalletFundingService
	walletService  service.WalletService
	authService    service.AuthService
	userRepo       repository.UserRepository
	paystackClient *paystack.Client
	logger         *zap.Logger
}

func NewWalletFundingHandler(
	fundingService service.WalletFundingService,
	walletService service.WalletService,
	authService service.AuthService,
	userRepo repository.UserRepository,
	paystackClient *paystack.Client,
	logger *zap.Logger,
) *WalletFundingHandler {
	return &WalletFundingHandler{
		fundingService: fundingService,
		walletService:  walletService,
		authService:    authService,
		userRepo:       userRepo,
		paystackClient: paystackClient,
		logger:         logger,
	}
}

// RegisterRoutes registers wallet funding routes
func (h *WalletFundingHandler) RegisterRoutes(r *gin.RouterGroup, authMiddleware *middleware.AuthMiddleware) {
	wallet := r.Group("/wallet")
	wallet.Use(authMiddleware.RequireAuth())
	{
		// Top-up
		wallet.POST("/top-up", h.InitializeTopUp)
		wallet.GET("/top-up/history", h.GetTopUpHistory)
		wallet.POST("/top-up/verify", h.VerifyTopUp)

		// Virtual Account
		wallet.GET("/virtual-account", h.GetVirtualAccount)
		wallet.POST("/virtual-account", h.CreateVirtualAccount)

		// Withdrawal (using new funding service)
		wallet.POST("/withdraw/v2", h.RequestWithdrawal)
		wallet.GET("/withdrawals", h.GetWithdrawalHistory)
		wallet.GET("/withdrawals/:id", h.GetWithdrawalByID)

		// Bank Accounts
		wallet.GET("/bank-accounts", h.GetBankAccounts)
		wallet.POST("/bank-accounts", h.AddBankAccount)
		wallet.PUT("/bank-accounts/:id/default", h.SetDefaultBankAccount)
		wallet.DELETE("/bank-accounts/:id", h.DeleteBankAccount)
	}

	// Banks list (public)
	r.GET("/banks", h.ListBanks)
}

// ==================== Top-up ====================

type InitializeTopUpRequest struct {
	Amount      float64 `json:"amount" binding:"required,gt=0"`
	CallbackURL string  `json:"callback_url"`
}

// InitializeTopUp initializes a wallet top-up
// @Summary Initialize wallet top-up
// @Tags Wallet
// @Security BearerAuth
// @Param request body InitializeTopUpRequest true "Top-up request"
// @Success 200 {object} map[string]interface{}
// @Router /wallet/top-up [post]
func (h *WalletFundingHandler) InitializeTopUp(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req InitializeTopUpRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	// Get user for email
	user, err := h.userRepo.GetByID(c.Request.Context(), userID)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to get user")
		return
	}

	email := ""
	if user.Email != nil {
		email = *user.Email
	} else {
		email = user.PhoneNumber + "@kipa.ng"
	}

	result, err := h.fundingService.InitializeTopUp(c.Request.Context(), userID, req.Amount, email, req.CallbackURL)
	if err != nil {
		if errors.Is(err, domain.ErrTopUpMinimum) {
			util.Error(c, http.StatusBadRequest, "TOP_UP_MINIMUM", "Amount below minimum top-up amount (₦100)")
			return
		}
		h.logger.Error("failed to initialize top-up", zap.Error(err))
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to initialize top-up")
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"top_up_id":         result.TopUpID,
		"payment_reference": result.PaymentReference,
		"authorization_url": result.AuthorizationURL,
		"access_code":       result.AccessCode,
		"public_key":        h.paystackClient.GetPublicKey(),
	})
}

type VerifyTopUpRequest struct {
	Reference string `json:"reference" binding:"required"`
}

// VerifyTopUp verifies a top-up payment
// @Summary Verify top-up payment
// @Tags Wallet
// @Security BearerAuth
// @Param request body VerifyTopUpRequest true "Verify request"
// @Success 200 {object} map[string]interface{}
// @Router /wallet/top-up/verify [post]
func (h *WalletFundingHandler) VerifyTopUp(c *gin.Context) {
	var req VerifyTopUpRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	if err := h.fundingService.ProcessTopUp(c.Request.Context(), req.Reference); err != nil {
		if errors.Is(err, domain.ErrTopUpNotFound) {
			util.Error(c, http.StatusNotFound, "TOP_UP_NOT_FOUND", "Top-up record not found")
			return
		}
		if errors.Is(err, domain.ErrPaymentNotSuccessful) {
			util.Error(c, http.StatusBadRequest, "PAYMENT_FAILED", "Payment was not successful")
			return
		}
		if errors.Is(err, domain.ErrPaymentAmountMismatch) {
			util.Error(c, http.StatusBadRequest, "AMOUNT_MISMATCH", "Payment amount mismatch")
			return
		}
		h.logger.Error("failed to process top-up", zap.Error(err))
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to process top-up")
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"message": "Top-up processed successfully",
	})
}

// GetTopUpHistory returns top-up history
// @Summary Get top-up history
// @Tags Wallet
// @Security BearerAuth
// @Param limit query int false "Limit" default(20)
// @Param offset query int false "Offset" default(0)
// @Success 200 {object} map[string]interface{}
// @Router /wallet/top-up/history [get]
func (h *WalletFundingHandler) GetTopUpHistory(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	limit := getIntQuery(c, "limit", 20)
	offset := getIntQuery(c, "offset", 0)

	topUps, err := h.fundingService.GetTopUpHistory(c.Request.Context(), userID, limit, offset)
	if err != nil {
		h.logger.Error("failed to get top-up history", zap.Error(err))
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to get top-up history")
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"top_ups": topUps,
		"limit":   limit,
		"offset":  offset,
	})
}

// ==================== Virtual Account ====================

// GetVirtualAccount returns the user's virtual account
// @Summary Get virtual account
// @Tags Wallet
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /wallet/virtual-account [get]
func (h *WalletFundingHandler) GetVirtualAccount(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	va, err := h.fundingService.GetVirtualAccount(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("failed to get virtual account", zap.Error(err))
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to get virtual account")
		return
	}

	if va == nil {
		util.Success(c, http.StatusOK, gin.H{
			"virtual_account": nil,
			"message":         "No virtual account found. Create one to fund wallet via bank transfer.",
		})
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"virtual_account": gin.H{
			"id":             va.ID,
			"bank_name":      va.BankName,
			"account_number": va.AccountNumber,
			"account_name":   va.AccountName,
			"is_active":      va.IsActive,
		},
	})
}

// CreateVirtualAccount creates a virtual account for the user
// @Summary Create virtual account
// @Tags Wallet
// @Security BearerAuth
// @Success 201 {object} map[string]interface{}
// @Router /wallet/virtual-account [post]
func (h *WalletFundingHandler) CreateVirtualAccount(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	// Get user
	user, err := h.userRepo.GetByID(c.Request.Context(), userID)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to get user")
		return
	}

	va, err := h.fundingService.GetOrCreateVirtualAccount(c.Request.Context(), userID, user)
	if err != nil {
		if errors.Is(err, domain.ErrProfileIncomplete) {
			util.Error(c, http.StatusBadRequest, "PROFILE_INCOMPLETE", "Please update your profile with first name, last name, and email before creating a virtual account")
			return
		}
		h.logger.Error("failed to create virtual account", zap.Error(err))
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to create virtual account")
		return
	}

	util.Success(c, http.StatusCreated, gin.H{
		"virtual_account": gin.H{
			"id":             va.ID,
			"bank_name":      va.BankName,
			"account_number": va.AccountNumber,
			"account_name":   va.AccountName,
			"is_active":      va.IsActive,
		},
		"message": "Virtual account created. Transfer funds to this account to top up your wallet.",
	})
}

// ==================== Withdrawal ====================

type FundingWithdrawalRequest struct {
	BankAccountID string  `json:"bank_account_id" binding:"required,uuid"`
	Amount        float64 `json:"amount" binding:"required,gt=0"`
}

// RequestWithdrawal initiates a withdrawal
// @Summary Request withdrawal
// @Tags Wallet
// @Security BearerAuth
// @Param request body FundingWithdrawalRequest true "Withdrawal request"
// @Success 200 {object} map[string]interface{}
// @Router /wallet/withdraw/v2 [post]
func (h *WalletFundingHandler) RequestWithdrawal(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req FundingWithdrawalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	bankAccountID, _ := uuid.Parse(req.BankAccountID)

	withdrawal, err := h.fundingService.RequestWithdrawal(c.Request.Context(), userID, bankAccountID, req.Amount)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrWithdrawalMinimum):
			util.Error(c, http.StatusBadRequest, "WITHDRAWAL_MINIMUM", "Amount below minimum withdrawal amount (₦1000)")
		case errors.Is(err, domain.ErrWithdrawalPending):
			util.Error(c, http.StatusBadRequest, "WITHDRAWAL_PENDING", "You already have a pending withdrawal")
		case errors.Is(err, domain.ErrBankAccountNotFound):
			util.Error(c, http.StatusNotFound, "BANK_ACCOUNT_NOT_FOUND", "Bank account not found")
		case errors.Is(err, domain.ErrBankAccountNotVerified):
			util.Error(c, http.StatusBadRequest, "BANK_ACCOUNT_NOT_VERIFIED", "Bank account not verified")
		case errors.Is(err, domain.ErrInsufficientBalance):
			util.Error(c, http.StatusBadRequest, "INSUFFICIENT_BALANCE", "Insufficient wallet balance")
		case errors.Is(err, domain.ErrTransferFailed):
			util.Error(c, http.StatusBadRequest, "TRANSFER_FAILED", "Failed to initiate transfer. Please try again.")
		default:
			h.logger.Error("failed to request withdrawal", zap.Error(err))
			util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to request withdrawal")
		}
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"withdrawal": gin.H{
			"id":           withdrawal.ID,
			"amount":       withdrawal.Amount,
			"fee":          withdrawal.Fee,
			"net_amount":   withdrawal.NetAmount,
			"status":       withdrawal.Status,
			"transfer_ref": withdrawal.TransferRef,
			"created_at":   withdrawal.CreatedAt,
		},
		"message": "Withdrawal initiated. You will receive the funds shortly.",
	})
}

// GetWithdrawalHistory returns withdrawal history
// @Summary Get withdrawal history
// @Tags Wallet
// @Security BearerAuth
// @Param limit query int false "Limit" default(20)
// @Param offset query int false "Offset" default(0)
// @Success 200 {object} map[string]interface{}
// @Router /wallet/withdrawals [get]
func (h *WalletFundingHandler) GetWithdrawalHistory(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	limit := getIntQuery(c, "limit", 20)
	offset := getIntQuery(c, "offset", 0)

	withdrawals, err := h.fundingService.GetWithdrawalHistory(c.Request.Context(), userID, limit, offset)
	if err != nil {
		h.logger.Error("failed to get withdrawal history", zap.Error(err))
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to get withdrawal history")
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"withdrawals": withdrawals,
		"limit":       limit,
		"offset":      offset,
	})
}

// GetWithdrawalByID returns a specific withdrawal
// @Summary Get withdrawal by ID
// @Tags Wallet
// @Security BearerAuth
// @Param id path string true "Withdrawal ID"
// @Success 200 {object} map[string]interface{}
// @Router /wallet/withdrawals/{id} [get]
func (h *WalletFundingHandler) GetWithdrawalByID(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	withdrawalID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid withdrawal ID")
		return
	}

	withdrawal, err := h.fundingService.GetWithdrawalByID(c.Request.Context(), userID, withdrawalID)
	if err != nil {
		if errors.Is(err, domain.ErrWithdrawalNotFound) {
			util.Error(c, http.StatusNotFound, "WITHDRAWAL_NOT_FOUND", "Withdrawal not found")
			return
		}
		h.logger.Error("failed to get withdrawal", zap.Error(err))
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to get withdrawal")
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"withdrawal": withdrawal,
	})
}

// ==================== Bank Accounts ====================

type AddBankAccountRequest struct {
	BankCode      string `json:"bank_code" binding:"required"`
	AccountNumber string `json:"account_number" binding:"required,len=10"`
}

// AddBankAccount adds a new bank account
// @Summary Add bank account
// @Tags Wallet
// @Security BearerAuth
// @Param request body AddBankAccountRequest true "Bank account request"
// @Success 201 {object} map[string]interface{}
// @Router /wallet/bank-accounts [post]
func (h *WalletFundingHandler) AddBankAccount(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req AddBankAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	account, err := h.fundingService.AddBankAccount(c.Request.Context(), userID, req.BankCode, req.AccountNumber)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrBankAccountExists):
			util.Error(c, http.StatusConflict, "BANK_ACCOUNT_EXISTS", "This bank account is already added")
		case errors.Is(err, domain.ErrInvalidBankAccount):
			util.Error(c, http.StatusBadRequest, "INVALID_BANK_ACCOUNT", "Invalid bank account details")
		default:
			h.logger.Error("failed to add bank account", zap.Error(err))
			util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to add bank account")
		}
		return
	}

	util.Success(c, http.StatusCreated, gin.H{
		"bank_account": gin.H{
			"id":             account.ID,
			"bank_code":      account.BankCode,
			"bank_name":      account.BankName,
			"account_number": account.AccountNumber,
			"account_name":   account.AccountName,
			"is_default":     account.IsDefault,
			"is_verified":    account.IsVerified,
		},
	})
}

// GetBankAccounts returns all bank accounts
// @Summary Get bank accounts
// @Tags Wallet
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /wallet/bank-accounts [get]
func (h *WalletFundingHandler) GetBankAccounts(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	accounts, err := h.fundingService.GetBankAccounts(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("failed to get bank accounts", zap.Error(err))
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to get bank accounts")
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"bank_accounts": accounts,
	})
}

// SetDefaultBankAccount sets a bank account as default
// @Summary Set default bank account
// @Tags Wallet
// @Security BearerAuth
// @Param id path string true "Bank Account ID"
// @Success 200 {object} map[string]interface{}
// @Router /wallet/bank-accounts/{id}/default [put]
func (h *WalletFundingHandler) SetDefaultBankAccount(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	accountID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid bank account ID")
		return
	}

	if err := h.fundingService.SetDefaultBankAccount(c.Request.Context(), userID, accountID); err != nil {
		if errors.Is(err, domain.ErrBankAccountNotFound) {
			util.Error(c, http.StatusNotFound, "BANK_ACCOUNT_NOT_FOUND", "Bank account not found")
			return
		}
		h.logger.Error("failed to set default bank account", zap.Error(err))
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to set default bank account")
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"message": "Default bank account updated",
	})
}

// DeleteBankAccount deletes a bank account
// @Summary Delete bank account
// @Tags Wallet
// @Security BearerAuth
// @Param id path string true "Bank Account ID"
// @Success 200 {object} map[string]interface{}
// @Router /wallet/bank-accounts/{id} [delete]
func (h *WalletFundingHandler) DeleteBankAccount(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	accountID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid bank account ID")
		return
	}

	if err := h.fundingService.DeleteBankAccount(c.Request.Context(), userID, accountID); err != nil {
		if errors.Is(err, domain.ErrBankAccountNotFound) {
			util.Error(c, http.StatusNotFound, "BANK_ACCOUNT_NOT_FOUND", "Bank account not found")
			return
		}
		h.logger.Error("failed to delete bank account", zap.Error(err))
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to delete bank account")
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"message": "Bank account deleted",
	})
}

// ==================== Banks List ====================

// ListBanks returns the list of supported banks
// @Summary List supported banks
// @Tags Banks
// @Success 200 {object} map[string]interface{}
// @Router /banks [get]
func (h *WalletFundingHandler) ListBanks(c *gin.Context) {
	banks, err := h.fundingService.ListBanks(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to list banks", zap.Error(err))
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to list banks")
		return
	}

	// Map to simpler format for frontend
	bankList := make([]gin.H, 0, len(banks))
	for _, bank := range banks {
		if bank.Active {
			bankList = append(bankList, gin.H{
				"code": bank.Code,
				"name": bank.Name,
				"slug": bank.Slug,
			})
		}
	}

	util.Success(c, http.StatusOK, gin.H{
		"banks": bankList,
	})
}

// Helper function
func getIntQuery(c *gin.Context, key string, defaultValue int) int {
	if v := c.Query(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultValue
}
