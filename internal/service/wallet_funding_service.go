package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/domain"
	"kipa/internal/integration/paystack"
	"kipa/internal/repository"
)

// WalletFundingService handles wallet funding operations
type WalletFundingService interface {
	// Bank Account Management
	AddBankAccount(ctx context.Context, userID uuid.UUID, bankCode, accountNumber string) (*domain.BankAccount, error)
	GetBankAccounts(ctx context.Context, userID uuid.UUID) ([]*domain.BankAccount, error)
	GetBankAccountByID(ctx context.Context, userID, accountID uuid.UUID) (*domain.BankAccount, error)
	SetDefaultBankAccount(ctx context.Context, userID, accountID uuid.UUID) error
	DeleteBankAccount(ctx context.Context, userID, accountID uuid.UUID) error
	ListBanks(ctx context.Context) ([]paystack.BankData, error)

	// Virtual Account (DVA)
	GetOrCreateVirtualAccount(ctx context.Context, userID uuid.UUID, user *domain.User) (*domain.VirtualAccount, error)
	GetVirtualAccount(ctx context.Context, userID uuid.UUID) (*domain.VirtualAccount, error)

	// Top-up
	InitializeTopUp(ctx context.Context, userID uuid.UUID, amount float64, email, callbackURL string) (*TopUpInitResult, error)
	ProcessTopUp(ctx context.Context, reference string) error
	GetTopUpHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.TopUp, error)

	// Withdrawal
	RequestWithdrawal(ctx context.Context, userID, bankAccountID uuid.UUID, amount float64) (*domain.Withdrawal, error)
	ProcessWithdrawalWebhook(ctx context.Context, reference string, status domain.WithdrawalStatus, failureReason string) error
	GetWithdrawalHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.Withdrawal, error)
	GetWithdrawalByID(ctx context.Context, userID, withdrawalID uuid.UUID) (*domain.Withdrawal, error)
}

// TopUpInitResult contains the result of initializing a top-up
type TopUpInitResult struct {
	TopUpID          uuid.UUID `json:"top_up_id"`
	PaymentReference string    `json:"payment_reference"`
	AuthorizationURL string    `json:"authorization_url"`
	AccessCode       string    `json:"access_code"`
}

// WalletFundingConfig contains configuration for wallet funding
type WalletFundingConfig struct {
	MinTopUpAmount     float64 `json:"min_top_up_amount"`
	MinWithdrawalAmount float64 `json:"min_withdrawal_amount"`
	WithdrawalFee      float64 `json:"withdrawal_fee"`
	WithdrawalFeeType  string  `json:"withdrawal_fee_type"` // "fixed" or "percentage"
}

type WalletFundingServiceImpl struct {
	bankAccountRepo    repository.BankAccountRepository
	virtualAccountRepo repository.VirtualAccountRepository
	withdrawalRepo     repository.WithdrawalRepository
	topUpRepo          repository.TopUpRepository
	walletRepo         repository.WalletRepository
	walletTxRepo       repository.WalletTransactionRepository
	userRepo           repository.UserRepository
	paystackClient     *paystack.Client
	config             WalletFundingConfig
	logger             *zap.Logger
}

func NewWalletFundingService(
	bankAccountRepo repository.BankAccountRepository,
	virtualAccountRepo repository.VirtualAccountRepository,
	withdrawalRepo repository.WithdrawalRepository,
	topUpRepo repository.TopUpRepository,
	walletRepo repository.WalletRepository,
	walletTxRepo repository.WalletTransactionRepository,
	userRepo repository.UserRepository,
	paystackClient *paystack.Client,
	logger *zap.Logger,
) WalletFundingService {
	return &WalletFundingServiceImpl{
		bankAccountRepo:    bankAccountRepo,
		virtualAccountRepo: virtualAccountRepo,
		withdrawalRepo:     withdrawalRepo,
		topUpRepo:          topUpRepo,
		walletRepo:         walletRepo,
		walletTxRepo:       walletTxRepo,
		userRepo:           userRepo,
		paystackClient:     paystackClient,
		config: WalletFundingConfig{
			MinTopUpAmount:      100,    // ₦100 minimum
			MinWithdrawalAmount: 1000,   // ₦1000 minimum
			WithdrawalFee:       50,     // ₦50 fixed fee
			WithdrawalFeeType:   "fixed",
		},
		logger: logger,
	}
}

// ==================== Bank Account Management ====================

// AddBankAccount verifies and adds a bank account for a user
func (s *WalletFundingServiceImpl) AddBankAccount(ctx context.Context, userID uuid.UUID, bankCode, accountNumber string) (*domain.BankAccount, error) {
	// Check if account already exists
	existingAccount, err := s.bankAccountRepo.GetByAccountNumber(ctx, userID, accountNumber, bankCode)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing account: %w", err)
	}
	if existingAccount != nil {
		return nil, domain.ErrBankAccountExists
	}

	// Verify account with Paystack
	resolveResp, err := s.paystackClient.ResolveAccountNumber(ctx, accountNumber, bankCode)
	if err != nil {
		s.logger.Error("failed to resolve bank account",
			zap.String("account_number", accountNumber),
			zap.String("bank_code", bankCode),
			zap.Error(err),
		)
		return nil, domain.ErrInvalidBankAccount
	}

	// Get bank name from list
	banksResp, err := s.paystackClient.ListBanks(ctx, "nigeria")
	if err != nil {
		return nil, fmt.Errorf("failed to get banks list: %w", err)
	}

	var bankName string
	for _, bank := range banksResp.Data {
		if bank.Code == bankCode {
			bankName = bank.Name
			break
		}
	}

	// Create transfer recipient for future withdrawals
	recipientResp, err := s.paystackClient.CreateTransferRecipient(ctx, &paystack.CreateTransferRecipientRequest{
		Type:          "nuban",
		Name:          resolveResp.Data.AccountName,
		AccountNumber: accountNumber,
		BankCode:      bankCode,
		Currency:      "NGN",
		Metadata: map[string]interface{}{
			"user_id": userID.String(),
		},
	})
	if err != nil {
		s.logger.Error("failed to create transfer recipient",
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to setup bank account: %w", err)
	}

	// Check if this is the first account (make it default)
	existingAccounts, err := s.bankAccountRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing accounts: %w", err)
	}
	isDefault := len(existingAccounts) == 0

	// Create the bank account record
	account := &domain.BankAccount{
		UserID:        userID,
		BankCode:      bankCode,
		BankName:      bankName,
		AccountNumber: accountNumber,
		AccountName:   resolveResp.Data.AccountName,
		RecipientCode: recipientResp.Data.RecipientCode,
		IsDefault:     isDefault,
		IsVerified:    true,
	}

	if err := s.bankAccountRepo.Create(ctx, account); err != nil {
		return nil, fmt.Errorf("failed to create bank account: %w", err)
	}

	s.logger.Info("bank account added",
		zap.String("user_id", userID.String()),
		zap.String("bank_name", bankName),
		zap.String("account_name", account.AccountName),
	)

	return account, nil
}

// GetBankAccounts returns all bank accounts for a user
func (s *WalletFundingServiceImpl) GetBankAccounts(ctx context.Context, userID uuid.UUID) ([]*domain.BankAccount, error) {
	return s.bankAccountRepo.GetByUserID(ctx, userID)
}

// GetBankAccountByID returns a specific bank account
func (s *WalletFundingServiceImpl) GetBankAccountByID(ctx context.Context, userID, accountID uuid.UUID) (*domain.BankAccount, error) {
	account, err := s.bankAccountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}

	// Verify ownership
	if account.UserID != userID {
		return nil, domain.ErrBankAccountNotFound
	}

	return account, nil
}

// SetDefaultBankAccount sets a bank account as the default
func (s *WalletFundingServiceImpl) SetDefaultBankAccount(ctx context.Context, userID, accountID uuid.UUID) error {
	// Verify ownership
	account, err := s.bankAccountRepo.GetByID(ctx, accountID)
	if err != nil {
		return err
	}
	if account.UserID != userID {
		return domain.ErrBankAccountNotFound
	}

	return s.bankAccountRepo.SetDefault(ctx, userID, accountID)
}

// DeleteBankAccount deletes a bank account
func (s *WalletFundingServiceImpl) DeleteBankAccount(ctx context.Context, userID, accountID uuid.UUID) error {
	// Verify ownership
	account, err := s.bankAccountRepo.GetByID(ctx, accountID)
	if err != nil {
		return err
	}
	if account.UserID != userID {
		return domain.ErrBankAccountNotFound
	}

	return s.bankAccountRepo.Delete(ctx, accountID)
}

// ListBanks returns the list of supported banks
func (s *WalletFundingServiceImpl) ListBanks(ctx context.Context) ([]paystack.BankData, error) {
	resp, err := s.paystackClient.ListBanks(ctx, "nigeria")
	if err != nil {
		return nil, fmt.Errorf("failed to list banks: %w", err)
	}
	return resp.Data, nil
}

// ==================== Virtual Account (DVA) ====================

// GetOrCreateVirtualAccount gets or creates a virtual account for a user
func (s *WalletFundingServiceImpl) GetOrCreateVirtualAccount(ctx context.Context, userID uuid.UUID, user *domain.User) (*domain.VirtualAccount, error) {
	// Check if user already has a virtual account
	existingVA, err := s.virtualAccountRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing VA: %w", err)
	}
	if existingVA != nil {
		return existingVA, nil
	}

	// Validate required fields for Paystack customer creation
	if user.FirstName == nil || *user.FirstName == "" {
		return nil, domain.ErrProfileIncomplete
	}
	if user.LastName == nil || *user.LastName == "" {
		return nil, domain.ErrProfileIncomplete
	}
	if user.Email == nil || *user.Email == "" {
		return nil, domain.ErrProfileIncomplete
	}

	email := *user.Email
	firstName := *user.FirstName
	lastName := *user.LastName

	customerResp, err := s.paystackClient.CreateCustomer(ctx, &paystack.CreateCustomerRequest{
		Email:     email,
		FirstName: firstName,
		LastName:  lastName,
		Phone:     user.PhoneNumber,
		Metadata: map[string]interface{}{
			"user_id": userID.String(),
		},
	})
	if err != nil {
		s.logger.Error("failed to create Paystack customer",
			zap.String("user_id", userID.String()),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to create customer: %w", err)
	}

	// Create dedicated virtual account
	dvaResp, err := s.paystackClient.CreateDedicatedVirtualAccount(ctx, &paystack.CreateDVARequest{
		Customer:      customerResp.Data.CustomerCode,
		PreferredBank: "wema-bank", // Default to Wema Bank
	})
	if err != nil {
		s.logger.Error("failed to create DVA",
			zap.String("user_id", userID.String()),
			zap.String("customer_code", customerResp.Data.CustomerCode),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to create virtual account: %w", err)
	}

	// Save virtual account
	va := &domain.VirtualAccount{
		UserID:          userID,
		BankName:        dvaResp.Data.Bank.Name,
		BankSlug:        dvaResp.Data.Bank.Slug,
		AccountNumber:   dvaResp.Data.AccountNumber,
		AccountName:     dvaResp.Data.AccountName,
		CustomerCode:    customerResp.Data.CustomerCode,
		DedicatedAcctID: dvaResp.Data.ID,
		IsActive:        true,
	}

	if err := s.virtualAccountRepo.Create(ctx, va); err != nil {
		return nil, fmt.Errorf("failed to save virtual account: %w", err)
	}

	s.logger.Info("virtual account created",
		zap.String("user_id", userID.String()),
		zap.String("bank_name", va.BankName),
		zap.String("account_number", va.AccountNumber),
	)

	return va, nil
}

// GetVirtualAccount returns the virtual account for a user
func (s *WalletFundingServiceImpl) GetVirtualAccount(ctx context.Context, userID uuid.UUID) (*domain.VirtualAccount, error) {
	return s.virtualAccountRepo.GetByUserID(ctx, userID)
}

// ==================== Top-up ====================

// InitializeTopUp initializes a top-up transaction via card payment
func (s *WalletFundingServiceImpl) InitializeTopUp(ctx context.Context, userID uuid.UUID, amount float64, email, callbackURL string) (*TopUpInitResult, error) {
	if amount < s.config.MinTopUpAmount {
		return nil, domain.ErrTopUpMinimum
	}

	// Get or create wallet
	wallet, err := s.walletRepo.GetOrCreate(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet: %w", err)
	}

	// Generate unique reference
	reference := fmt.Sprintf("topup_%s_%d", uuid.New().String()[:8], time.Now().Unix())

	// Initialize Paystack transaction
	initResp, err := s.paystackClient.InitializeTransaction(ctx, &paystack.InitializeTransactionRequest{
		Email:       email,
		Amount:      paystack.AmountToKobo(amount),
		Reference:   reference,
		CallbackURL: callbackURL,
		Metadata: map[string]interface{}{
			"user_id":   userID.String(),
			"wallet_id": wallet.ID.String(),
			"type":      "top_up",
		},
		Channels: []string{"card", "bank", "ussd", "bank_transfer"},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize payment: %w", err)
	}

	// Create top-up record
	topUp := &domain.TopUp{
		UserID:          userID,
		WalletID:        wallet.ID,
		Amount:          amount,
		PaymentMethod:   "card",
		PaymentRef:      reference,
		PaymentProvider: "paystack",
		Status:          domain.TopUpStatusPending,
	}

	if err := s.topUpRepo.Create(ctx, topUp); err != nil {
		return nil, fmt.Errorf("failed to create top-up record: %w", err)
	}

	return &TopUpInitResult{
		TopUpID:          topUp.ID,
		PaymentReference: reference,
		AuthorizationURL: initResp.Data.AuthorizationURL,
		AccessCode:       initResp.Data.AccessCode,
	}, nil
}

// ProcessTopUp processes a successful top-up payment
func (s *WalletFundingServiceImpl) ProcessTopUp(ctx context.Context, reference string) error {
	// Get top-up record
	topUp, err := s.topUpRepo.GetByReference(ctx, reference)
	if err != nil {
		return fmt.Errorf("failed to get top-up: %w", err)
	}
	if topUp == nil {
		return domain.ErrTopUpNotFound
	}

	// Skip if already processed
	if topUp.Status == domain.TopUpStatusSuccess {
		return nil
	}

	// Verify with Paystack
	verifyResp, err := s.paystackClient.VerifyTransaction(ctx, reference)
	if err != nil {
		s.logger.Error("failed to verify top-up transaction",
			zap.String("reference", reference),
			zap.Error(err),
		)
		return fmt.Errorf("failed to verify payment: %w", err)
	}

	if verifyResp.Data.Status != paystack.TransactionStatusSuccess {
		// Update status to failed
		topUp.Status = domain.TopUpStatusFailed
		s.topUpRepo.Update(ctx, topUp)
		return domain.ErrPaymentNotSuccessful
	}

	// Verify amount
	expectedKobo := paystack.AmountToKobo(topUp.Amount)
	if verifyResp.Data.Amount != expectedKobo {
		s.logger.Error("top-up amount mismatch",
			zap.Int64("expected", expectedKobo),
			zap.Int64("received", verifyResp.Data.Amount),
		)
		return domain.ErrPaymentAmountMismatch
	}

	// Credit wallet
	wallet, err := s.walletRepo.GetByID(ctx, topUp.WalletID)
	if err != nil {
		return fmt.Errorf("failed to get wallet: %w", err)
	}

	if err := s.walletRepo.Credit(ctx, wallet.ID, topUp.Amount); err != nil {
		return fmt.Errorf("failed to credit wallet: %w", err)
	}

	// Get updated balance
	wallet, _ = s.walletRepo.GetByID(ctx, wallet.ID)

	// Create transaction record
	idempotencyKey := fmt.Sprintf("topup_%s", reference)
	refType := "top_up"
	description := fmt.Sprintf("Wallet top-up via %s", topUp.PaymentMethod)
	tx := &domain.WalletTransaction{
		WalletID:       wallet.ID,
		Type:           domain.WalletTxTopUp,
		Amount:         topUp.Amount,
		BalanceAfter:   wallet.AvailableBalance,
		ReferenceType:  &refType,
		ReferenceID:    &topUp.ID,
		Description:    &description,
		IdempotencyKey: &idempotencyKey,
	}
	s.walletTxRepo.Create(ctx, tx)

	// Update top-up status
	now := time.Now()
	topUp.Status = domain.TopUpStatusSuccess
	topUp.PaidAt = &now
	if err := s.topUpRepo.Update(ctx, topUp); err != nil {
		s.logger.Error("failed to update top-up status", zap.Error(err))
	}

	s.logger.Info("top-up processed successfully",
		zap.String("user_id", topUp.UserID.String()),
		zap.Float64("amount", topUp.Amount),
		zap.String("reference", reference),
	)

	return nil
}

// GetTopUpHistory returns top-up history for a user
func (s *WalletFundingServiceImpl) GetTopUpHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.TopUp, error) {
	return s.topUpRepo.GetByUserID(ctx, userID, limit, offset)
}

// ==================== Withdrawal ====================

// RequestWithdrawal initiates a withdrawal to a bank account
func (s *WalletFundingServiceImpl) RequestWithdrawal(ctx context.Context, userID, bankAccountID uuid.UUID, amount float64) (*domain.Withdrawal, error) {
	if amount < s.config.MinWithdrawalAmount {
		return nil, domain.ErrWithdrawalMinimum
	}

	// Check for pending withdrawal
	pendingWithdrawal, err := s.withdrawalRepo.GetPendingByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to check pending withdrawals: %w", err)
	}
	if pendingWithdrawal != nil {
		return nil, domain.ErrWithdrawalPending
	}

	// Get bank account
	bankAccount, err := s.bankAccountRepo.GetByID(ctx, bankAccountID)
	if err != nil {
		return nil, err
	}
	if bankAccount.UserID != userID {
		return nil, domain.ErrBankAccountNotFound
	}
	if bankAccount.RecipientCode == "" {
		return nil, domain.ErrBankAccountNotVerified
	}

	// Calculate fee
	var fee float64
	if s.config.WithdrawalFeeType == "percentage" {
		fee = amount * (s.config.WithdrawalFee / 100)
	} else {
		fee = s.config.WithdrawalFee
	}
	netAmount := amount - fee

	// Get wallet and check balance
	wallet, err := s.walletRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet: %w", err)
	}
	if wallet.AvailableBalance < amount {
		return nil, domain.ErrInsufficientBalance
	}

	// Generate transfer reference
	transferRef := fmt.Sprintf("wd_%s_%d", uuid.New().String()[:8], time.Now().Unix())

	// Debit wallet
	if err := s.walletRepo.Debit(ctx, wallet.ID, amount); err != nil {
		return nil, fmt.Errorf("failed to debit wallet: %w", err)
	}

	// Create withdrawal record
	withdrawal := &domain.Withdrawal{
		UserID:        userID,
		WalletID:      wallet.ID,
		BankAccountID: bankAccountID,
		Amount:        amount,
		Fee:           fee,
		NetAmount:     netAmount,
		Status:        domain.WithdrawalStatusPending,
		TransferRef:   transferRef,
	}

	if err := s.withdrawalRepo.Create(ctx, withdrawal); err != nil {
		// Rollback wallet debit
		s.walletRepo.Credit(ctx, wallet.ID, amount)
		return nil, fmt.Errorf("failed to create withdrawal: %w", err)
	}

	// Get updated balance for transaction record
	wallet, _ = s.walletRepo.GetByID(ctx, wallet.ID)

	// Create wallet transaction
	idempotencyKey := fmt.Sprintf("withdrawal_%s", transferRef)
	refType := "withdrawal"
	description := fmt.Sprintf("Withdrawal to %s - %s", bankAccount.BankName, bankAccount.AccountNumber)
	tx := &domain.WalletTransaction{
		WalletID:       wallet.ID,
		Type:           domain.WalletTxWithdrawal,
		Amount:         -amount,
		BalanceAfter:   wallet.AvailableBalance,
		ReferenceType:  &refType,
		ReferenceID:    &withdrawal.ID,
		Description:    &description,
		IdempotencyKey: &idempotencyKey,
	}
	s.walletTxRepo.Create(ctx, tx)

	// Initiate Paystack transfer
	transferResp, err := s.paystackClient.InitiateTransfer(ctx, &paystack.InitiateTransferRequest{
		Source:    "balance",
		Amount:    paystack.AmountToKobo(netAmount),
		Recipient: bankAccount.RecipientCode,
		Reason:    "Kipa wallet withdrawal",
		Reference: transferRef,
	})
	if err != nil {
		s.logger.Error("failed to initiate transfer",
			zap.String("reference", transferRef),
			zap.Error(err),
		)
		// Update withdrawal status to failed
		failureReason := err.Error()
		withdrawal.Status = domain.WithdrawalStatusFailed
		withdrawal.FailureReason = &failureReason
		s.withdrawalRepo.Update(ctx, withdrawal)

		// Refund wallet
		s.walletRepo.Credit(ctx, wallet.ID, amount)

		return nil, domain.ErrTransferFailed
	}

	// Update withdrawal with transfer code
	withdrawal.TransferCode = &transferResp.Data.TransferCode
	withdrawal.Status = domain.WithdrawalStatusProcessing
	s.withdrawalRepo.Update(ctx, withdrawal)

	s.logger.Info("withdrawal initiated",
		zap.String("user_id", userID.String()),
		zap.Float64("amount", amount),
		zap.Float64("net_amount", netAmount),
		zap.String("reference", transferRef),
	)

	return withdrawal, nil
}

// ProcessWithdrawalWebhook processes withdrawal status updates from webhooks
func (s *WalletFundingServiceImpl) ProcessWithdrawalWebhook(ctx context.Context, reference string, status domain.WithdrawalStatus, failureReason string) error {
	withdrawal, err := s.withdrawalRepo.GetByReference(ctx, reference)
	if err != nil {
		return fmt.Errorf("failed to get withdrawal: %w", err)
	}

	// Skip if already in final state
	if withdrawal.Status == domain.WithdrawalStatusSuccess ||
		withdrawal.Status == domain.WithdrawalStatusFailed ||
		withdrawal.Status == domain.WithdrawalStatusReversed {
		return nil
	}

	now := time.Now()
	withdrawal.Status = status
	withdrawal.ProcessedAt = &now

	if failureReason != "" {
		withdrawal.FailureReason = &failureReason
	}

	// Handle failed/reversed transfers - refund the wallet
	if status == domain.WithdrawalStatusFailed || status == domain.WithdrawalStatusReversed {
		if err := s.walletRepo.Credit(ctx, withdrawal.WalletID, withdrawal.Amount); err != nil {
			s.logger.Error("failed to refund wallet after failed transfer",
				zap.String("reference", reference),
				zap.Error(err),
			)
		}

		// Create refund transaction
		wallet, _ := s.walletRepo.GetByID(ctx, withdrawal.WalletID)
		idempotencyKey := fmt.Sprintf("withdrawal_refund_%s", reference)
		refType := "withdrawal"
		description := fmt.Sprintf("Refund for failed withdrawal %s", reference)
		tx := &domain.WalletTransaction{
			WalletID:       wallet.ID,
			Type:           domain.WalletTxRefund,
			Amount:         withdrawal.Amount,
			BalanceAfter:   wallet.AvailableBalance,
			ReferenceType:  &refType,
			ReferenceID:    &withdrawal.ID,
			Description:    &description,
			IdempotencyKey: &idempotencyKey,
		}
		s.walletTxRepo.Create(ctx, tx)
	}

	// Update total withdrawn on success
	if status == domain.WithdrawalStatusSuccess {
		wallet, _ := s.walletRepo.GetByID(ctx, withdrawal.WalletID)
		wallet.TotalWithdrawn += withdrawal.NetAmount
		s.walletRepo.Update(ctx, wallet)
	}

	if err := s.withdrawalRepo.Update(ctx, withdrawal); err != nil {
		return fmt.Errorf("failed to update withdrawal: %w", err)
	}

	s.logger.Info("withdrawal webhook processed",
		zap.String("reference", reference),
		zap.String("status", string(status)),
	)

	return nil
}

// GetWithdrawalHistory returns withdrawal history for a user
func (s *WalletFundingServiceImpl) GetWithdrawalHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.Withdrawal, error) {
	return s.withdrawalRepo.GetByUserID(ctx, userID, limit, offset)
}

// GetWithdrawalByID returns a specific withdrawal
func (s *WalletFundingServiceImpl) GetWithdrawalByID(ctx context.Context, userID, withdrawalID uuid.UUID) (*domain.Withdrawal, error) {
	withdrawal, err := s.withdrawalRepo.GetByID(ctx, withdrawalID)
	if err != nil {
		return nil, err
	}

	// Verify ownership
	if withdrawal.UserID != userID {
		return nil, domain.ErrWithdrawalNotFound
	}

	return withdrawal, nil
}

// Ensure WalletFundingServiceImpl implements WalletFundingService
var _ WalletFundingService = (*WalletFundingServiceImpl)(nil)
