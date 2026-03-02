package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/domain"
	"kipa/internal/integration/flutterwave"
	"kipa/internal/integration/paystack"
	"kipa/internal/repository"
)
type WalletFundingService interface {
	// Bank account methods (Paystack)
	AddBankAccount(ctx context.Context, userID uuid.UUID, bankCode, accountNumber string) (*domain.BankAccount, error)
	GetBankAccounts(ctx context.Context, userID uuid.UUID) ([]*domain.BankAccount, error)
	GetBankAccountByID(ctx context.Context, userID, accountID uuid.UUID) (*domain.BankAccount, error)
	SetDefaultBankAccount(ctx context.Context, userID, accountID uuid.UUID) error
	DeleteBankAccount(ctx context.Context, userID, accountID uuid.UUID) error
	ListBanks(ctx context.Context) ([]paystack.BankData, error)

	// Virtual account methods (Paystack)
	GetOrCreateVirtualAccount(ctx context.Context, userID uuid.UUID, user *domain.User) (*domain.VirtualAccount, error)
	GetVirtualAccount(ctx context.Context, userID uuid.UUID) (*domain.VirtualAccount, error)

	// Top-up methods (Paystack)
	InitializeTopUp(ctx context.Context, userID uuid.UUID, amount float64, email, callbackURL string) (*TopUpInitResult, error)
	ProcessTopUp(ctx context.Context, reference string) error
	GetTopUpHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.TopUp, error)

	// Withdrawal methods (Paystack)
	RequestWithdrawal(ctx context.Context, userID, bankAccountID uuid.UUID, amount float64) (*domain.Withdrawal, error)
	ProcessWithdrawalWebhook(ctx context.Context, reference string, status domain.WithdrawalStatus, failureReason string) error
	GetWithdrawalHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.Withdrawal, error)
	GetWithdrawalByID(ctx context.Context, userID, withdrawalID uuid.UUID) (*domain.Withdrawal, error)

	// Flutterwave top-up methods
	InitializeFlutterwaveTopUp(ctx context.Context, userID uuid.UUID, amount float64, email, callbackURL string) (*TopUpInitResult, error)
	ProcessFlutterwaveTopUp(ctx context.Context, txRef string) error

	// Flutterwave withdrawal methods
	RequestFlutterwaveWithdrawal(ctx context.Context, userID, bankAccountID uuid.UUID, amount float64) (*domain.Withdrawal, error)
	ProcessFlutterwaveWithdrawalWebhook(ctx context.Context, reference string, status domain.WithdrawalStatus, failureReason string) error

	// Flutterwave banks
	ListFlutterwaveBanks(ctx context.Context) ([]flutterwave.BankData, error)
	ResolveFlutterwaveAccount(ctx context.Context, accountNumber, bankCode string) (*flutterwave.ResolveAccountData, error)

	// Flutterwave Virtual Account (for bank transfer funding)
	CreateFlutterwaveVirtualAccount(ctx context.Context, userID uuid.UUID, bvn string) (*domain.FlutterwaveVirtualAccount, error)
	GetFlutterwaveVirtualAccount(ctx context.Context, userID uuid.UUID) (*domain.FlutterwaveVirtualAccount, error)
	GetFlutterwaveVirtualAccountStatus(ctx context.Context, userID uuid.UUID) (*FlutterwaveVAStatus, error)
	DeclineFlutterwaveVirtualAccount(ctx context.Context, userID uuid.UUID) error
	ProcessVirtualAccountPayment(ctx context.Context, txRef string, amount float64, reference string) error
}

// FlutterwaveVAStatus represents the user's virtual account status
type FlutterwaveVAStatus struct {
	HasAccount bool                               `json:"has_account"`
	Declined   bool                               `json:"declined"`
	Account    *domain.FlutterwaveVirtualAccount  `json:"account,omitempty"`
}
type TopUpInitResult struct {
	TopUpID          uuid.UUID `json:"top_up_id"`
	PaymentReference string    `json:"payment_reference"`
	AuthorizationURL string    `json:"authorization_url"`
	AccessCode       string    `json:"access_code"`
}
type WalletFundingConfig struct {
	MinTopUpAmount      float64 `json:"min_top_up_amount"`
	MinWithdrawalAmount float64 `json:"min_withdrawal_amount"`
	WithdrawalFee       float64 `json:"withdrawal_fee"`
	WithdrawalFeeType   string  `json:"withdrawal_fee_type"` // "fixed" or "percentage"
}

type WalletFundingServiceImpl struct {
	bankAccountRepo     repository.BankAccountRepository
	virtualAccountRepo  repository.VirtualAccountRepository
	withdrawalRepo      repository.WithdrawalRepository
	topUpRepo           repository.TopUpRepository
	walletRepo          repository.WalletRepository
	walletTxRepo        repository.WalletTransactionRepository
	userRepo            repository.UserRepository
	flwVirtualAcctRepo  repository.FlutterwaveVirtualAccountRepository
	paystackClient      *paystack.Client
	flutterwaveClient   *flutterwave.Client
	notificationService NotificationService
	config              WalletFundingConfig
	logger              *zap.Logger
}

func NewWalletFundingService(
	bankAccountRepo repository.BankAccountRepository,
	virtualAccountRepo repository.VirtualAccountRepository,
	withdrawalRepo repository.WithdrawalRepository,
	topUpRepo repository.TopUpRepository,
	walletRepo repository.WalletRepository,
	walletTxRepo repository.WalletTransactionRepository,
	userRepo repository.UserRepository,
	flwVirtualAcctRepo repository.FlutterwaveVirtualAccountRepository,
	paystackClient *paystack.Client,
	flutterwaveClient *flutterwave.Client,
	notificationService NotificationService,
	config WalletFundingConfig,
	logger *zap.Logger,
) WalletFundingService {
	// Set defaults if not provided
	if config.MinTopUpAmount == 0 {
		config.MinTopUpAmount = 100 // ₦100 minimum
	}
	if config.MinWithdrawalAmount == 0 {
		config.MinWithdrawalAmount = 1000 // ₦1000 minimum
	}
	if config.WithdrawalFee == 0 {
		config.WithdrawalFee = 50 // ₦50 fixed fee
	}
	if config.WithdrawalFeeType == "" {
		config.WithdrawalFeeType = "fixed"
	}

	return &WalletFundingServiceImpl{
		bankAccountRepo:     bankAccountRepo,
		virtualAccountRepo:  virtualAccountRepo,
		withdrawalRepo:      withdrawalRepo,
		topUpRepo:           topUpRepo,
		walletRepo:          walletRepo,
		walletTxRepo:        walletTxRepo,
		userRepo:            userRepo,
		flwVirtualAcctRepo:  flwVirtualAcctRepo,
		paystackClient:      paystackClient,
		flutterwaveClient:   flutterwaveClient,
		notificationService: notificationService,
		config:              config,
		logger:              logger,
	}
}
func (s *WalletFundingServiceImpl) AddBankAccount(ctx context.Context, userID uuid.UUID, bankCode, accountNumber string) (*domain.BankAccount, error) {
	existingAccount, err := s.bankAccountRepo.GetByAccountNumber(ctx, userID, accountNumber, bankCode)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing account: %w", err)
	}
	if existingAccount != nil {
		return nil, domain.ErrBankAccountExists
	}
	resolveResp, err := s.paystackClient.ResolveAccountNumber(ctx, accountNumber, bankCode)
	if err != nil {
		s.logger.Error("failed to resolve bank account",
			zap.String("account_number", accountNumber),
			zap.String("bank_code", bankCode),
			zap.Error(err),
		)
		return nil, domain.ErrInvalidBankAccount
	}
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
	existingAccounts, err := s.bankAccountRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing accounts: %w", err)
	}
	isDefault := len(existingAccounts) == 0
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
func (s *WalletFundingServiceImpl) GetBankAccounts(ctx context.Context, userID uuid.UUID) ([]*domain.BankAccount, error) {
	return s.bankAccountRepo.GetByUserID(ctx, userID)
}
func (s *WalletFundingServiceImpl) GetBankAccountByID(ctx context.Context, userID, accountID uuid.UUID) (*domain.BankAccount, error) {
	account, err := s.bankAccountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if account.UserID != userID {
		return nil, domain.ErrBankAccountNotFound
	}

	return account, nil
}
func (s *WalletFundingServiceImpl) SetDefaultBankAccount(ctx context.Context, userID, accountID uuid.UUID) error {
	account, err := s.bankAccountRepo.GetByID(ctx, accountID)
	if err != nil {
		return err
	}
	if account.UserID != userID {
		return domain.ErrBankAccountNotFound
	}

	return s.bankAccountRepo.SetDefault(ctx, userID, accountID)
}
func (s *WalletFundingServiceImpl) DeleteBankAccount(ctx context.Context, userID, accountID uuid.UUID) error {
	account, err := s.bankAccountRepo.GetByID(ctx, accountID)
	if err != nil {
		return err
	}
	if account.UserID != userID {
		return domain.ErrBankAccountNotFound
	}

	return s.bankAccountRepo.Delete(ctx, accountID)
}
func (s *WalletFundingServiceImpl) ListBanks(ctx context.Context) ([]paystack.BankData, error) {
	resp, err := s.paystackClient.ListBanks(ctx, "nigeria")
	if err != nil {
		return nil, fmt.Errorf("failed to list banks: %w", err)
	}
	return resp.Data, nil
}
func (s *WalletFundingServiceImpl) GetOrCreateVirtualAccount(ctx context.Context, userID uuid.UUID, user *domain.User) (*domain.VirtualAccount, error) {
	existingVA, err := s.virtualAccountRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing VA: %w", err)
	}
	if existingVA != nil {
		return existingVA, nil
	}
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
func (s *WalletFundingServiceImpl) GetVirtualAccount(ctx context.Context, userID uuid.UUID) (*domain.VirtualAccount, error) {
	return s.virtualAccountRepo.GetByUserID(ctx, userID)
}
func (s *WalletFundingServiceImpl) InitializeTopUp(ctx context.Context, userID uuid.UUID, amount float64, email, callbackURL string) (*TopUpInitResult, error) {
	if amount < s.config.MinTopUpAmount {
		return nil, domain.ErrTopUpMinimum
	}
	wallet, err := s.walletRepo.GetOrCreate(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet: %w", err)
	}
	reference := fmt.Sprintf("topup_%s_%d", uuid.New().String()[:8], time.Now().Unix())
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
func (s *WalletFundingServiceImpl) ProcessTopUp(ctx context.Context, reference string) error {
	topUp, err := s.topUpRepo.GetByReference(ctx, reference)
	if err != nil {
		return fmt.Errorf("failed to get top-up: %w", err)
	}
	if topUp == nil {
		return domain.ErrTopUpNotFound
	}
	if topUp.Status == domain.TopUpStatusSuccess {
		return nil
	}
	verifyResp, err := s.paystackClient.VerifyTransaction(ctx, reference)
	if err != nil {
		s.logger.Error("failed to verify top-up transaction",
			zap.String("reference", reference),
			zap.Error(err),
		)
		return fmt.Errorf("failed to verify payment: %w", err)
	}

	if verifyResp.Data.Status != paystack.TransactionStatusSuccess {
		topUp.Status = domain.TopUpStatusFailed
		s.topUpRepo.Update(ctx, topUp)
		return domain.ErrPaymentNotSuccessful
	}
	expectedKobo := paystack.AmountToKobo(topUp.Amount)
	if verifyResp.Data.Amount != expectedKobo {
		s.logger.Error("top-up amount mismatch",
			zap.Int64("expected", expectedKobo),
			zap.Int64("received", verifyResp.Data.Amount),
		)
		return domain.ErrPaymentAmountMismatch
	}
	wallet, err := s.walletRepo.GetByID(ctx, topUp.WalletID)
	if err != nil {
		return fmt.Errorf("failed to get wallet: %w", err)
	}

	if err := s.walletRepo.Credit(ctx, wallet.ID, topUp.Amount); err != nil {
		return fmt.Errorf("failed to credit wallet: %w", err)
	}
	wallet, _ = s.walletRepo.GetByID(ctx, wallet.ID)
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
func (s *WalletFundingServiceImpl) GetTopUpHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.TopUp, error) {
	return s.topUpRepo.GetByUserID(ctx, userID, limit, offset)
}
func (s *WalletFundingServiceImpl) RequestWithdrawal(ctx context.Context, userID, bankAccountID uuid.UUID, amount float64) (*domain.Withdrawal, error) {
	if amount < s.config.MinWithdrawalAmount {
		return nil, domain.ErrWithdrawalMinimum
	}
	pendingWithdrawal, err := s.withdrawalRepo.GetPendingByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to check pending withdrawals: %w", err)
	}
	if pendingWithdrawal != nil {
		return nil, domain.ErrWithdrawalPending
	}
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
	var fee float64
	if s.config.WithdrawalFeeType == "percentage" {
		fee = amount * (s.config.WithdrawalFee / 100)
	} else {
		fee = s.config.WithdrawalFee
	}
	netAmount := amount - fee
	wallet, err := s.walletRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet: %w", err)
	}
	if wallet.AvailableBalance < amount {
		return nil, domain.ErrInsufficientBalance
	}
	transferRef := fmt.Sprintf("wd_%s_%d", uuid.New().String()[:8], time.Now().Unix())
	if err := s.walletRepo.Debit(ctx, wallet.ID, amount); err != nil {
		return nil, fmt.Errorf("failed to debit wallet: %w", err)
	}
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
		s.walletRepo.Credit(ctx, wallet.ID, amount)
		return nil, fmt.Errorf("failed to create withdrawal: %w", err)
	}
	wallet, _ = s.walletRepo.GetByID(ctx, wallet.ID)
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
		failureReason := err.Error()
		withdrawal.Status = domain.WithdrawalStatusFailed
		withdrawal.FailureReason = &failureReason
		s.withdrawalRepo.Update(ctx, withdrawal)
		s.walletRepo.Credit(ctx, wallet.ID, amount)

		return nil, domain.ErrTransferFailed
	}
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
func (s *WalletFundingServiceImpl) ProcessWithdrawalWebhook(ctx context.Context, reference string, status domain.WithdrawalStatus, failureReason string) error {
	withdrawal, err := s.withdrawalRepo.GetByReference(ctx, reference)
	if err != nil {
		return fmt.Errorf("failed to get withdrawal: %w", err)
	}
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
	if status == domain.WithdrawalStatusFailed || status == domain.WithdrawalStatusReversed {
		if err := s.walletRepo.Credit(ctx, withdrawal.WalletID, withdrawal.Amount); err != nil {
			s.logger.Error("failed to refund wallet after failed transfer",
				zap.String("reference", reference),
				zap.Error(err),
			)
		}
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
func (s *WalletFundingServiceImpl) GetWithdrawalHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.Withdrawal, error) {
	return s.withdrawalRepo.GetByUserID(ctx, userID, limit, offset)
}
func (s *WalletFundingServiceImpl) GetWithdrawalByID(ctx context.Context, userID, withdrawalID uuid.UUID) (*domain.Withdrawal, error) {
	withdrawal, err := s.withdrawalRepo.GetByID(ctx, withdrawalID)
	if err != nil {
		return nil, err
	}
	if withdrawal.UserID != userID {
		return nil, domain.ErrWithdrawalNotFound
	}

	return withdrawal, nil
}

// ============================================
// FLUTTERWAVE TOP-UP METHODS
// ============================================

func (s *WalletFundingServiceImpl) InitializeFlutterwaveTopUp(ctx context.Context, userID uuid.UUID, amount float64, email, callbackURL string) (*TopUpInitResult, error) {
	if amount < s.config.MinTopUpAmount {
		return nil, domain.ErrTopUpMinimum
	}

	wallet, err := s.walletRepo.GetOrCreate(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet: %w", err)
	}

	reference := fmt.Sprintf("fw_topup_%s_%d", uuid.New().String()[:8], time.Now().Unix())

	// Set default redirect URL if not provided
	redirectURL := callbackURL
	if redirectURL == "" {
		redirectURL = fmt.Sprintf("https://getkipa.com/wallet/topup/verify?tx_ref=%s", reference)
	}

	// Initialize payment with Flutterwave
	resp, err := s.flutterwaveClient.InitializePayment(ctx, &flutterwave.InitializePaymentRequest{
		TxRef:       reference,
		Amount:      amount,
		Currency:    "NGN",
		RedirectURL: redirectURL,
		Customer: flutterwave.CustomerInfo{
			Email: email,
		},
		Meta: map[string]interface{}{
			"user_id":   userID.String(),
			"wallet_id": wallet.ID.String(),
			"type":      "top_up",
		},
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
		PaymentProvider: "flutterwave",
		Status:          domain.TopUpStatusPending,
	}

	if err := s.topUpRepo.Create(ctx, topUp); err != nil {
		return nil, fmt.Errorf("failed to create top-up record: %w", err)
	}

	return &TopUpInitResult{
		TopUpID:          topUp.ID,
		PaymentReference: reference,
		AuthorizationURL: resp.Data.Link,
	}, nil
}

func (s *WalletFundingServiceImpl) ProcessFlutterwaveTopUp(ctx context.Context, txRef string) error {
	topUp, err := s.topUpRepo.GetByReference(ctx, txRef)
	if err != nil {
		return fmt.Errorf("failed to get top-up: %w", err)
	}
	if topUp == nil {
		return domain.ErrTopUpNotFound
	}

	// Already processed
	if topUp.Status == domain.TopUpStatusSuccess {
		return nil
	}

	// Verify transaction with Flutterwave
	verifyResp, err := s.flutterwaveClient.VerifyTransaction(ctx, txRef)
	if err != nil {
		s.logger.Error("failed to verify Flutterwave transaction",
			zap.String("reference", txRef),
			zap.Error(err),
		)
		return fmt.Errorf("failed to verify payment: %w", err)
	}

	if verifyResp.Data.Status != "successful" {
		topUp.Status = domain.TopUpStatusFailed
		s.topUpRepo.Update(ctx, topUp)
		return domain.ErrPaymentNotSuccessful
	}

	// Verify amount
	if verifyResp.Data.Amount != topUp.Amount {
		s.logger.Error("Flutterwave top-up amount mismatch",
			zap.Float64("expected", topUp.Amount),
			zap.Float64("received", verifyResp.Data.Amount),
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

	// Create transaction record
	wallet, _ = s.walletRepo.GetByID(ctx, wallet.ID)
	idempotencyKey := fmt.Sprintf("fw_topup_%s", txRef)
	refType := "top_up"
	description := "Wallet top-up via Flutterwave"
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

	s.logger.Info("Flutterwave top-up processed successfully",
		zap.String("user_id", topUp.UserID.String()),
		zap.Float64("amount", topUp.Amount),
		zap.String("reference", txRef),
	)

	// Send push notification
	if s.notificationService != nil {
		go func() {
			_ = s.notificationService.SendToUser(
				context.Background(),
				topUp.UserID,
				"Wallet Funded",
				fmt.Sprintf("Your wallet has been credited with ₦%.2f", topUp.Amount),
				map[string]string{
					"amount":    fmt.Sprintf("%.2f", topUp.Amount),
					"reference": txRef,
				},
				domain.NotificationTopUpSuccess,
			)
		}()
	}

	return nil
}

// ============================================
// FLUTTERWAVE WITHDRAWAL METHODS
// ============================================

func (s *WalletFundingServiceImpl) RequestFlutterwaveWithdrawal(ctx context.Context, userID, bankAccountID uuid.UUID, amount float64) (*domain.Withdrawal, error) {
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

	// Calculate fee
	var fee float64
	if s.config.WithdrawalFeeType == "percentage" {
		fee = amount * (s.config.WithdrawalFee / 100)
	} else {
		fee = s.config.WithdrawalFee
	}
	netAmount := amount - fee

	// Check wallet balance
	wallet, err := s.walletRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet: %w", err)
	}
	if wallet.AvailableBalance < amount {
		return nil, domain.ErrInsufficientBalance
	}

	transferRef := fmt.Sprintf("fw_wd_%s_%d", uuid.New().String()[:8], time.Now().Unix())

	// Debit wallet first
	if err := s.walletRepo.Debit(ctx, wallet.ID, amount); err != nil {
		return nil, fmt.Errorf("failed to debit wallet: %w", err)
	}

	// Create withdrawal record
	withdrawal := &domain.Withdrawal{
		UserID:          userID,
		WalletID:        wallet.ID,
		BankAccountID:   bankAccountID,
		Amount:          amount,
		Fee:             fee,
		NetAmount:       netAmount,
		Status:          domain.WithdrawalStatusPending,
		TransferRef:     transferRef,
		PaymentProvider: "flutterwave",
	}

	if err := s.withdrawalRepo.Create(ctx, withdrawal); err != nil {
		// Refund wallet on failure
		s.walletRepo.Credit(ctx, wallet.ID, amount)
		return nil, fmt.Errorf("failed to create withdrawal: %w", err)
	}

	// Record transaction
	wallet, _ = s.walletRepo.GetByID(ctx, wallet.ID)
	idempotencyKey := fmt.Sprintf("fw_withdrawal_%s", transferRef)
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

	// Initiate transfer via Flutterwave (from platform's balance to user's bank)
	transferResp, err := s.flutterwaveClient.InitiateTransfer(ctx, &flutterwave.InitiateTransferRequest{
		AccountBank:     bankAccount.BankCode,
		AccountNumber:   bankAccount.AccountNumber,
		Amount:          netAmount,
		Currency:        "NGN",
		Narration:       "Kipa wallet withdrawal",
		Reference:       transferRef,
		BeneficiaryName: bankAccount.AccountName,
	})
	if err != nil {
		s.logger.Error("failed to initiate Flutterwave transfer",
			zap.String("reference", transferRef),
			zap.Error(err),
		)
		failureReason := err.Error()
		withdrawal.Status = domain.WithdrawalStatusFailed
		withdrawal.FailureReason = &failureReason
		s.withdrawalRepo.Update(ctx, withdrawal)
		s.walletRepo.Credit(ctx, wallet.ID, amount)
		return nil, domain.ErrTransferFailed
	}

	// Update withdrawal with transfer code
	transferCode := fmt.Sprintf("%d", transferResp.Data.ID)
	withdrawal.TransferCode = &transferCode
	withdrawal.Status = domain.WithdrawalStatusProcessing
	s.withdrawalRepo.Update(ctx, withdrawal)

	s.logger.Info("Flutterwave withdrawal initiated",
		zap.String("user_id", userID.String()),
		zap.Float64("amount", amount),
		zap.Float64("net_amount", netAmount),
		zap.String("reference", transferRef),
	)

	// Send push notification
	if s.notificationService != nil {
		go func() {
			_ = s.notificationService.SendToUser(
				context.Background(),
				userID,
				"Withdrawal Processing",
				fmt.Sprintf("Your withdrawal of ₦%.2f is being processed", netAmount),
				map[string]string{
					"amount":      fmt.Sprintf("%.2f", netAmount),
					"reference":   transferRef,
					"bank_name":   bankAccount.BankName,
					"account":     bankAccount.AccountNumber,
				},
				domain.NotificationWithdrawalInitiated,
			)
		}()
	}

	return withdrawal, nil
}

func (s *WalletFundingServiceImpl) ProcessFlutterwaveWithdrawalWebhook(ctx context.Context, reference string, status domain.WithdrawalStatus, failureReason string) error {
	withdrawal, err := s.withdrawalRepo.GetByReference(ctx, reference)
	if err != nil {
		return fmt.Errorf("failed to get withdrawal: %w", err)
	}

	// Already processed
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

	// Refund wallet on failure
	if status == domain.WithdrawalStatusFailed || status == domain.WithdrawalStatusReversed {
		if err := s.walletRepo.Credit(ctx, withdrawal.WalletID, withdrawal.Amount); err != nil {
			s.logger.Error("failed to refund wallet after failed Flutterwave transfer",
				zap.String("reference", reference),
				zap.Error(err),
			)
		}

		wallet, _ := s.walletRepo.GetByID(ctx, withdrawal.WalletID)
		idempotencyKey := fmt.Sprintf("fw_withdrawal_refund_%s", reference)
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

	s.logger.Info("Flutterwave withdrawal webhook processed",
		zap.String("reference", reference),
		zap.String("status", string(status)),
	)

	// Send push notification
	if s.notificationService != nil {
		go func() {
			switch status {
case domain.WithdrawalStatusSuccess:
				_ = s.notificationService.SendToUser(
					context.Background(),
					withdrawal.UserID,
					"Withdrawal Successful",
					fmt.Sprintf("₦%.2f has been sent to your bank account", withdrawal.NetAmount),
					map[string]string{
						"amount":    fmt.Sprintf("%.2f", withdrawal.NetAmount),
						"reference": reference,
					},
					domain.NotificationWithdrawalSuccess,
				)
			case domain.WithdrawalStatusFailed, domain.WithdrawalStatusReversed:
				_ = s.notificationService.SendToUser(
					context.Background(),
					withdrawal.UserID,
					"Withdrawal Failed",
					fmt.Sprintf("Your withdrawal of ₦%.2f failed. Amount has been refunded to your wallet.", withdrawal.NetAmount),
					map[string]string{
						"amount":    fmt.Sprintf("%.2f", withdrawal.Amount),
						"reference": reference,
						"reason":    failureReason,
					},
					domain.NotificationWithdrawalFailed,
				)
			}
		}()
	}

	return nil
}

// ============================================
// FLUTTERWAVE BANK METHODS
// ============================================

func (s *WalletFundingServiceImpl) ListFlutterwaveBanks(ctx context.Context) ([]flutterwave.BankData, error) {
	resp, err := s.flutterwaveClient.ListBanks(ctx, "NG")
	if err != nil {
		return nil, fmt.Errorf("failed to list banks: %w", err)
	}
	return resp.Data, nil
}

func (s *WalletFundingServiceImpl) ResolveFlutterwaveAccount(ctx context.Context, accountNumber, bankCode string) (*flutterwave.ResolveAccountData, error) {
	resp, err := s.flutterwaveClient.ResolveBankAccount(ctx, accountNumber, bankCode)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve account: %w", err)
	}
	return &resp.Data, nil
}

// ============================================
// FLUTTERWAVE VIRTUAL ACCOUNT METHODS
// ============================================

// CreateFlutterwaveVirtualAccount creates a static virtual account for a user
// BVN is required by CBN regulations for Nigerian virtual accounts
func (s *WalletFundingServiceImpl) CreateFlutterwaveVirtualAccount(ctx context.Context, userID uuid.UUID, bvn string) (*domain.FlutterwaveVirtualAccount, error) {
	// Check if user already has a virtual account
	existing, err := s.flwVirtualAcctRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing virtual account: %w", err)
	}
	if existing != nil {
		return existing, nil
	}

	// Get user details
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Validate required fields
	if user.Email == nil || *user.Email == "" {
		return nil, domain.ErrProfileIncomplete
	}
	if user.FirstName == nil || *user.FirstName == "" {
		return nil, domain.ErrProfileIncomplete
	}
	if user.LastName == nil || *user.LastName == "" {
		return nil, domain.ErrProfileIncomplete
	}

	email := *user.Email
	firstName := *user.FirstName
	lastName := *user.LastName

	// Generate unique tx_ref for this user's virtual account
	txRef := fmt.Sprintf("va-%s", userID.String())

	// Create static virtual account via Flutterwave
	resp, err := s.flutterwaveClient.CreateStaticVirtualAccount(ctx, &flutterwave.CreateVirtualAccountRequest{
		Email:       email,
		IsPermanent: true,
		TxRef:       txRef,
		BVN:         bvn,
		PhoneNumber: user.PhoneNumber,
		FirstName:   firstName,
		LastName:    lastName,
		Narration:   fmt.Sprintf("Kipa Wallet - %s %s", firstName, lastName),
	})
	if err != nil {
		s.logger.Error("failed to create Flutterwave virtual account",
			zap.String("user_id", userID.String()),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to create virtual account: %w", err)
	}

	// Save virtual account to database
	va := &domain.FlutterwaveVirtualAccount{
		UserID:        userID,
		AccountNumber: resp.Data.AccountNumber,
		BankName:      resp.Data.BankName,
		TxRef:         txRef,
		FlwRef:        resp.Data.FlwRef,
		OrderRef:      resp.Data.OrderRef,
		Email:         email,
		IsActive:      true,
	}

	if err := s.flwVirtualAcctRepo.Create(ctx, va); err != nil {
		return nil, fmt.Errorf("failed to save virtual account: %w", err)
	}

	s.logger.Info("Flutterwave virtual account created",
		zap.String("user_id", userID.String()),
		zap.String("account_number", va.AccountNumber),
		zap.String("bank_name", va.BankName),
	)

	return va, nil
}

// GetFlutterwaveVirtualAccount retrieves the user's virtual account details
func (s *WalletFundingServiceImpl) GetFlutterwaveVirtualAccount(ctx context.Context, userID uuid.UUID) (*domain.FlutterwaveVirtualAccount, error) {
	return s.flwVirtualAcctRepo.GetByUserID(ctx, userID)
}

// GetFlutterwaveVirtualAccountStatus returns the user's virtual account status
// Used by frontend to determine whether to show the VA prompt
func (s *WalletFundingServiceImpl) GetFlutterwaveVirtualAccountStatus(ctx context.Context, userID uuid.UUID) (*FlutterwaveVAStatus, error) {
	// Check if user has an existing virtual account
	va, err := s.flwVirtualAcctRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to check virtual account: %w", err)
	}

	if va != nil {
		return &FlutterwaveVAStatus{
			HasAccount: true,
			Declined:   false,
			Account:    va,
		}, nil
	}

	// Check if user has declined
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &FlutterwaveVAStatus{
		HasAccount: false,
		Declined:   user.FlwVirtualAccountDeclinedAt != nil,
		Account:    nil,
	}, nil
}

// DeclineFlutterwaveVirtualAccount marks the user as having declined the VA prompt
func (s *WalletFundingServiceImpl) DeclineFlutterwaveVirtualAccount(ctx context.Context, userID uuid.UUID) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	now := time.Now()
	user.FlwVirtualAccountDeclinedAt = &now

	if err := s.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	s.logger.Info("user declined Flutterwave virtual account",
		zap.String("user_id", userID.String()),
	)

	return nil
}

// ProcessVirtualAccountPayment handles incoming bank transfers to a virtual account
// This is called from the webhook handler when a charge.completed event is received
func (s *WalletFundingServiceImpl) ProcessVirtualAccountPayment(ctx context.Context, txRef string, amount float64, reference string) error {
	// Idempotency check
	idempotencyKey := fmt.Sprintf("va_payment_%s", reference)
	existingTx, err := s.walletTxRepo.GetByIdempotencyKey(ctx, idempotencyKey)
	if err != nil {
		return fmt.Errorf("failed to check idempotency: %w", err)
	}
	if existingTx != nil {
		s.logger.Info("virtual account payment already processed (idempotent)",
			zap.String("reference", reference),
		)
		return nil
	}

	// Find virtual account by tx_ref
	va, err := s.flwVirtualAcctRepo.GetByTxRef(ctx, txRef)
	if err != nil {
		return fmt.Errorf("failed to find virtual account: %w", err)
	}
	if va == nil {
		s.logger.Warn("virtual account payment for unknown tx_ref",
			zap.String("tx_ref", txRef),
			zap.String("reference", reference),
		)
		return fmt.Errorf("virtual account not found for tx_ref: %s", txRef)
	}

	// Get or create wallet for user
	wallet, err := s.walletRepo.GetOrCreate(ctx, va.UserID)
	if err != nil {
		return fmt.Errorf("failed to get wallet: %w", err)
	}

	// Credit the wallet
	if err := s.walletRepo.Credit(ctx, wallet.ID, amount); err != nil {
		return fmt.Errorf("failed to credit wallet: %w", err)
	}

	// Create transaction record
	wallet, _ = s.walletRepo.GetByID(ctx, wallet.ID)
	refType := "virtual_account_funding"
	description := fmt.Sprintf("Bank transfer to %s", va.AccountNumber)
	tx := &domain.WalletTransaction{
		WalletID:       wallet.ID,
		Type:           domain.WalletTxTopUp,
		Amount:         amount,
		BalanceAfter:   wallet.AvailableBalance,
		ReferenceType:  &refType,
		Description:    &description,
		IdempotencyKey: &idempotencyKey,
	}
	s.walletTxRepo.Create(ctx, tx)

	s.logger.Info("virtual account payment processed",
		zap.String("user_id", va.UserID.String()),
		zap.Float64("amount", amount),
		zap.String("reference", reference),
		zap.String("account_number", va.AccountNumber),
	)

	// Send push notification
	if s.notificationService != nil {
		go func() {
			_ = s.notificationService.SendToUser(
				context.Background(),
				va.UserID,
				"Wallet Funded",
				fmt.Sprintf("Your wallet has been credited with ₦%.2f via bank transfer", amount),
				map[string]string{
					"amount":         fmt.Sprintf("%.2f", amount),
					"reference":      reference,
					"account_number": va.AccountNumber,
				},
				domain.NotificationTopUpSuccess,
			)
		}()
	}

	return nil
}

var _ WalletFundingService = (*WalletFundingServiceImpl)(nil)
