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
	"kipa/internal/integration/flutterwave"
	"kipa/internal/integration/paystack"
	"kipa/internal/repository"
	"kipa/internal/service"
	"kipa/internal/util"
)
type WalletFundingHandler struct {
	fundingService    service.WalletFundingService
	walletService     service.WalletService
	authService       service.AuthService
	userRepo          repository.UserRepository
	paystackClient    *paystack.Client
	flutterwaveClient *flutterwave.Client
	logger            *zap.Logger
}

func NewWalletFundingHandler(
	fundingService service.WalletFundingService,
	walletService service.WalletService,
	authService service.AuthService,
	userRepo repository.UserRepository,
	paystackClient *paystack.Client,
	flutterwaveClient *flutterwave.Client,
	logger *zap.Logger,
) *WalletFundingHandler {
	return &WalletFundingHandler{
		fundingService:    fundingService,
		walletService:     walletService,
		authService:       authService,
		userRepo:          userRepo,
		paystackClient:    paystackClient,
		flutterwaveClient: flutterwaveClient,
		logger:            logger,
	}
}
func (h *WalletFundingHandler) RegisterRoutes(r *gin.RouterGroup, authMiddleware *middleware.AuthMiddleware) {
	wallet := r.Group("/wallet")
	wallet.Use(authMiddleware.RequireAuth())
	{
		// Paystack routes
		wallet.POST("/top-up", h.InitializeTopUp)
		wallet.GET("/top-up/history", h.GetTopUpHistory)
		wallet.POST("/top-up/verify", h.VerifyTopUp)
		wallet.GET("/virtual-account", h.GetVirtualAccount)
		wallet.POST("/virtual-account", h.CreateVirtualAccount)
		wallet.POST("/withdraw/v2", h.RequestWithdrawal)
		wallet.GET("/withdrawals", h.GetWithdrawalHistory)
		wallet.GET("/withdrawals/:id", h.GetWithdrawalByID)
		wallet.GET("/bank-accounts", h.GetBankAccounts)
		wallet.POST("/bank-accounts", h.AddBankAccount)
		wallet.PUT("/bank-accounts/:id/default", h.SetDefaultBankAccount)
		wallet.DELETE("/bank-accounts/:id", h.DeleteBankAccount)

		// Flutterwave routes
		fw := wallet.Group("/flutterwave")
		{
			// Virtual account for bank transfer funding
			fw.GET("/virtual-account/status", h.GetFlutterwaveVirtualAccountStatus)
			fw.GET("/virtual-account", h.GetFlutterwaveVirtualAccount)
			fw.POST("/virtual-account", h.CreateFlutterwaveVirtualAccount)
			fw.POST("/virtual-account/decline", h.DeclineFlutterwaveVirtualAccount)

			fw.POST("/topup/initialize", h.InitializeFlutterwaveTopUp)
			fw.POST("/topup/verify", h.VerifyFlutterwaveTopUp)
			fw.POST("/withdraw", h.RequestFlutterwaveWithdrawal)
			fw.GET("/banks", h.ListFlutterwaveBanks)
			fw.POST("/resolve-account", h.ResolveFlutterwaveAccount)
		}
	}
	r.GET("/banks", h.ListBanks)
}

type InitializeTopUpRequest struct {
	Amount      float64 `json:"amount" binding:"required,gt=0"`
	CallbackURL string  `json:"callback_url"`
}
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
	})
}

type VerifyTopUpRequest struct {
	Reference string `json:"reference" binding:"required"`
}
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
func (h *WalletFundingHandler) CreateVirtualAccount(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}
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

type FundingWithdrawalRequest struct {
	BankAccountID string  `json:"bank_account_id" binding:"required,uuid"`
	Amount        float64 `json:"amount" binding:"required,gt=0"`
}
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

type AddBankAccountRequest struct {
	BankCode      string `json:"bank_code" binding:"required"`
	AccountNumber string `json:"account_number" binding:"required,len=10"`
}
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
func (h *WalletFundingHandler) ListBanks(c *gin.Context) {
	banks, err := h.fundingService.ListBanks(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to list banks", zap.Error(err))
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to list banks")
		return
	}
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
func getIntQuery(c *gin.Context, key string, defaultValue int) int {
	if v := c.Query(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultValue
}

// ============================================
// FLUTTERWAVE HANDLERS
// ============================================

// GetFlutterwaveVirtualAccountStatus returns the user's virtual account status
// Frontend uses this to determine whether to show the VA creation prompt
func (h *WalletFundingHandler) GetFlutterwaveVirtualAccountStatus(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	status, err := h.fundingService.GetFlutterwaveVirtualAccountStatus(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("failed to get virtual account status", zap.Error(err))
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to get virtual account status")
		return
	}

	util.Success(c, http.StatusOK, status)
}

// GetFlutterwaveVirtualAccount returns the user's existing virtual account details
func (h *WalletFundingHandler) GetFlutterwaveVirtualAccount(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	va, err := h.fundingService.GetFlutterwaveVirtualAccount(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("failed to get Flutterwave virtual account", zap.Error(err))
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to get virtual account")
		return
	}

	if va == nil {
		util.Error(c, http.StatusNotFound, "VIRTUAL_ACCOUNT_NOT_FOUND", "No virtual account found. Create one first.")
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"virtual_account": gin.H{
			"account_number": va.AccountNumber,
			"bank_name":      va.BankName,
			"email":          va.Email,
			"is_active":      va.IsActive,
			"created_at":     va.CreatedAt,
		},
		"message": "Transfer any amount to this account to fund your wallet",
	})
}

type CreateFlutterwaveVARequest struct {
	BVN string `json:"bvn" binding:"required,len=11"`
}

// CreateFlutterwaveVirtualAccount creates a static virtual account for the user
// Requires BVN for CBN compliance
func (h *WalletFundingHandler) CreateFlutterwaveVirtualAccount(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req CreateFlutterwaveVARequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	va, err := h.fundingService.CreateFlutterwaveVirtualAccount(c.Request.Context(), userID, req.BVN)
	if err != nil {
		if errors.Is(err, domain.ErrProfileIncomplete) {
			util.Error(c, http.StatusBadRequest, "PROFILE_INCOMPLETE", "Please complete your profile (email, first name, last name) before creating a virtual account")
			return
		}
		h.logger.Error("failed to create Flutterwave virtual account", zap.Error(err))
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to create virtual account")
		return
	}

	util.Success(c, http.StatusCreated, gin.H{
		"virtual_account": gin.H{
			"account_number": va.AccountNumber,
			"bank_name":      va.BankName,
			"email":          va.Email,
			"is_active":      va.IsActive,
			"created_at":     va.CreatedAt,
		},
		"message": "Virtual account created successfully. Transfer any amount to fund your wallet.",
	})
}

// DeclineFlutterwaveVirtualAccount marks the user as having declined the VA prompt
func (h *WalletFundingHandler) DeclineFlutterwaveVirtualAccount(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	if err := h.fundingService.DeclineFlutterwaveVirtualAccount(c.Request.Context(), userID); err != nil {
		h.logger.Error("failed to decline virtual account", zap.Error(err))
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to save preference")
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"message": "Virtual account creation declined",
	})
}

func (h *WalletFundingHandler) InitializeFlutterwaveTopUp(c *gin.Context) {
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

	user, err := h.userRepo.GetByID(c.Request.Context(), userID)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to get user")
		return
	}

	email := user.PhoneNumber + "@kipa.ng"
	if user.Email != nil {
		email = *user.Email
	}

	result, err := h.fundingService.InitializeFlutterwaveTopUp(c.Request.Context(), userID, req.Amount, email, req.CallbackURL)
	if err != nil {
		if errors.Is(err, domain.ErrTopUpMinimum) {
			util.Error(c, http.StatusBadRequest, "TOP_UP_MINIMUM", "Amount below minimum top-up amount (₦100)")
			return
		}
		h.logger.Error("failed to initialize Flutterwave top-up", zap.Error(err))
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to initialize top-up")
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"top_up_id":         result.TopUpID,
		"payment_reference": result.PaymentReference,
		"authorization_url": result.AuthorizationURL,
	})
}

type VerifyFlutterwaveTopUpRequest struct {
	TxRef string `json:"tx_ref" binding:"required"`
}

func (h *WalletFundingHandler) VerifyFlutterwaveTopUp(c *gin.Context) {
	var req VerifyFlutterwaveTopUpRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	if err := h.fundingService.ProcessFlutterwaveTopUp(c.Request.Context(), req.TxRef); err != nil {
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
		h.logger.Error("failed to process Flutterwave top-up", zap.Error(err))
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to process top-up")
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"message": "Top-up processed successfully",
	})
}

func (h *WalletFundingHandler) RequestFlutterwaveWithdrawal(c *gin.Context) {
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

	withdrawal, err := h.fundingService.RequestFlutterwaveWithdrawal(c.Request.Context(), userID, bankAccountID, req.Amount)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrWithdrawalMinimum):
			util.Error(c, http.StatusBadRequest, "WITHDRAWAL_MINIMUM", "Amount below minimum withdrawal amount (₦1000)")
		case errors.Is(err, domain.ErrWithdrawalPending):
			util.Error(c, http.StatusBadRequest, "WITHDRAWAL_PENDING", "You already have a pending withdrawal")
		case errors.Is(err, domain.ErrBankAccountNotFound):
			util.Error(c, http.StatusNotFound, "BANK_ACCOUNT_NOT_FOUND", "Bank account not found")
		case errors.Is(err, domain.ErrInsufficientBalance):
			util.Error(c, http.StatusBadRequest, "INSUFFICIENT_BALANCE", "Insufficient wallet balance")
		case errors.Is(err, domain.ErrTransferFailed):
			util.Error(c, http.StatusBadRequest, "TRANSFER_FAILED", "Failed to initiate transfer. Please try again.")
		default:
			h.logger.Error("failed to request Flutterwave withdrawal", zap.Error(err))
			util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to request withdrawal")
		}
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"withdrawal": gin.H{
			"id":               withdrawal.ID,
			"amount":           withdrawal.Amount,
			"fee":              withdrawal.Fee,
			"net_amount":       withdrawal.NetAmount,
			"status":           withdrawal.Status,
			"transfer_ref":     withdrawal.TransferRef,
			"payment_provider": withdrawal.PaymentProvider,
			"created_at":       withdrawal.CreatedAt,
		},
		"message": "Withdrawal initiated via Flutterwave. You will receive the funds shortly.",
	})
}

func (h *WalletFundingHandler) ListFlutterwaveBanks(c *gin.Context) {
	banks, err := h.fundingService.ListFlutterwaveBanks(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to list Flutterwave banks", zap.Error(err))
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to list banks")
		return
	}

	bankList := make([]gin.H, 0, len(banks))
	for _, bank := range banks {
		bankList = append(bankList, gin.H{
			"code": bank.Code,
			"name": bank.Name,
		})
	}

	util.Success(c, http.StatusOK, gin.H{
		"banks": bankList,
	})
}

type ResolveFlutterwaveAccountRequest struct {
	AccountNumber string `json:"account_number" binding:"required,len=10"`
	BankCode      string `json:"bank_code" binding:"required"`
}

func (h *WalletFundingHandler) ResolveFlutterwaveAccount(c *gin.Context) {
	var req ResolveFlutterwaveAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	account, err := h.fundingService.ResolveFlutterwaveAccount(c.Request.Context(), req.AccountNumber, req.BankCode)
	if err != nil {
		h.logger.Error("failed to resolve Flutterwave account", zap.Error(err))
		util.Error(c, http.StatusBadRequest, "INVALID_ACCOUNT", "Could not verify account details")
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"account_number": account.AccountNumber,
		"account_name":   account.AccountName,
	})
}
