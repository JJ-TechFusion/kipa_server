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

	// Flutterwave subaccount methods
	GetOrCreateFlutterwaveSubaccount(ctx context.Context, userID uuid.UUID, email, phone string) (*domain.FlutterwaveSubaccount, error)
	GetFlutterwaveSubaccount(ctx context.Context, userID uuid.UUID) (*domain.FlutterwaveSubaccount, error)
	GetFlutterwaveSubaccountBalance(ctx context.Context, userID uuid.UUID) (float64, error)

	// Flutterwave top-up methods
	InitializeFlutterwaveTopUp(ctx context.Context, userID uuid.UUID, amount float64, email, callbackURL string) (*TopUpInitResult, error)
	ProcessFlutterwaveTopUp(ctx context.Context, txRef string) error

	// Flutterwave withdrawal methods
	RequestFlutterwaveWithdrawal(ctx context.Context, userID, bankAccountID uuid.UUID, amount float64) (*domain.Withdrawal, error)
	ProcessFlutterwaveWithdrawalWebhook(ctx context.Context, reference string, status domain.WithdrawalStatus, failureReason string) error

	// Flutterwave wallet funding (bank transfer to virtual account)
	ProcessWalletFundingWebhook(ctx context.Context, accountNumber string, amount float64, reference string) error
	ProcessWalletFundingByAccountRef(ctx context.Context, accountRef string, amount float64, reference string) error

	// Flutterwave synced wallet operations (credit/debit wallet + subaccount atomically)
	CreditWalletWithFlutterwave(ctx context.Context, userID uuid.UUID, amount float64, txType domain.WalletTransactionType, refType string, refID uuid.UUID, idempotencyKey string) error
	DebitWalletWithFlutterwave(ctx context.Context, userID uuid.UUID, amount float64, txType domain.WalletTransactionType, refType string, refID uuid.UUID, idempotencyKey string) error

	// Flutterwave banks
	ListFlutterwaveBanks(ctx context.Context) ([]flutterwave.BankData, error)
	ResolveFlutterwaveAccount(ctx context.Context, accountNumber, bankCode string) (*flutterwave.ResolveAccountData, error)

	// Sync wallet with Flutterwave subaccount balance (for bank transfers that don't trigger webhooks)
	SyncFlutterwaveWallet(ctx context.Context, userID uuid.UUID) (*WalletSyncResult, error)
	GetAllActiveFlutterwaveSubaccounts(ctx context.Context) ([]uuid.UUID, error)
}

// WalletSyncResult contains the result of syncing wallet with Flutterwave
type WalletSyncResult struct {
	PreviousBalance   float64 `json:"previous_balance"`
	FlutterwaveBalance float64 `json:"flutterwave_balance"`
	NewBalance        float64 `json:"new_balance"`
	AmountSynced      float64 `json:"amount_synced"`
	WasSynced         bool    `json:"was_synced"`
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
	bankAccountRepo      repository.BankAccountRepository
	virtualAccountRepo   repository.VirtualAccountRepository
	withdrawalRepo       repository.WithdrawalRepository
	topUpRepo            repository.TopUpRepository
	walletRepo           repository.WalletRepository
	walletTxRepo         repository.WalletTransactionRepository
	userRepo             repository.UserRepository
	flwSubaccountRepo    repository.FlutterwaveSubaccountRepository
	paystackClient       *paystack.Client
	flutterwaveClient    *flutterwave.Client
	notificationService  NotificationService
	config               WalletFundingConfig
	logger               *zap.Logger
}

func NewWalletFundingService(
	bankAccountRepo repository.BankAccountRepository,
	virtualAccountRepo repository.VirtualAccountRepository,
	withdrawalRepo repository.WithdrawalRepository,
	topUpRepo repository.TopUpRepository,
	walletRepo repository.WalletRepository,
	walletTxRepo repository.WalletTransactionRepository,
	userRepo repository.UserRepository,
	flwSubaccountRepo repository.FlutterwaveSubaccountRepository,
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
		flwSubaccountRepo:   flwSubaccountRepo,
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
// FLUTTERWAVE SUBACCOUNT METHODS
// ============================================

func (s *WalletFundingServiceImpl) GetOrCreateFlutterwaveSubaccount(ctx context.Context, userID uuid.UUID, email, phone string) (*domain.FlutterwaveSubaccount, error) {
	// Check if subaccount already exists
	existing, err := s.flwSubaccountRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing subaccount: %w", err)
	}
	if existing != nil {
		return existing, nil
	}

	// Get user for business name
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	var businessName string
	if user.FirstName != nil && user.LastName != nil {
		businessName = fmt.Sprintf("%s %s", *user.FirstName, *user.LastName)
	} else {
		businessName = phone
	}

	// Create subaccount in Flutterwave
	resp, err := s.flutterwaveClient.CreateSubaccount(ctx, &flutterwave.CreateSubaccountRequest{
		AccountName: businessName,
		Email:       email,
		Country:     "NG",
		BankCode:    "035", // WEMA Bank
	})
	if err != nil {
		s.logger.Error("failed to create Flutterwave subaccount",
			zap.String("user_id", userID.String()),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to create subaccount: %w", err)
	}

	// Save subaccount locally
	subaccount := &domain.FlutterwaveSubaccount{
		UserID:           userID,
		AccountReference: resp.Data.AccountReference,
		AccountBank:      &resp.Data.BankName,
		AccountNumber:    &resp.Data.Nuban,
		BarterID:         &resp.Data.BarterID,
		BusinessName:     &businessName,
		Email:            &email,
		IsActive:         true,
	}

	if err := s.flwSubaccountRepo.Create(ctx, subaccount); err != nil {
		return nil, fmt.Errorf("failed to save subaccount: %w", err)
	}

	s.logger.Info("Flutterwave subaccount created",
		zap.String("user_id", userID.String()),
		zap.String("account_reference", resp.Data.AccountReference),
	)

	return subaccount, nil
}

func (s *WalletFundingServiceImpl) GetFlutterwaveSubaccount(ctx context.Context, userID uuid.UUID) (*domain.FlutterwaveSubaccount, error) {
	return s.flwSubaccountRepo.GetByUserID(ctx, userID)
}

func (s *WalletFundingServiceImpl) GetFlutterwaveSubaccountBalance(ctx context.Context, userID uuid.UUID) (float64, error) {
	subaccount, err := s.flwSubaccountRepo.GetByUserID(ctx, userID)
	if err != nil {
		return 0, err
	}
	if subaccount == nil {
		return 0, domain.ErrFlutterwaveSubaccountNotFound
	}

	resp, err := s.flutterwaveClient.GetSubaccountBalance(ctx, subaccount.AccountReference)
	if err != nil {
		return 0, fmt.Errorf("failed to get subaccount balance: %w", err)
	}

	return resp.Data.AvailableBalance, nil
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

	// Fund user's Flutterwave subaccount (transfer from platform to user's wallet)
	subaccount, err := s.flwSubaccountRepo.GetByUserID(ctx, topUp.UserID)
	if err != nil {
		s.logger.Error("failed to get Flutterwave subaccount for top-up sync",
			zap.String("user_id", topUp.UserID.String()),
			zap.Error(err),
		)
	} else if subaccount != nil && subaccount.BarterID != nil && *subaccount.BarterID != "" {
		fundRef := fmt.Sprintf("topup_%s", txRef)
		_, fundErr := s.flutterwaveClient.FundWallet(ctx, *subaccount.BarterID, topUp.Amount, fundRef, "Wallet top-up")
		if fundErr != nil {
			s.logger.Error("failed to fund Flutterwave subaccount after top-up",
				zap.String("user_id", topUp.UserID.String()),
				zap.String("barter_id", *subaccount.BarterID),
				zap.Float64("amount", topUp.Amount),
				zap.Error(fundErr),
			)
			// Note: Internal wallet is credited but subaccount funding failed
			// This creates a sync issue - should be handled by reconciliation job
		} else {
			s.logger.Info("Flutterwave subaccount funded after top-up",
				zap.String("user_id", topUp.UserID.String()),
				zap.Float64("amount", topUp.Amount),
			)
		}
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

// ProcessWalletFundingWebhook handles bank transfers to the user's virtual account (nuban)
// This is called when Flutterwave sends a webhook for transfers into a payout subaccount
func (s *WalletFundingServiceImpl) ProcessWalletFundingWebhook(ctx context.Context, accountNumber string, amount float64, reference string) error {
	// Idempotency check
	idempotencyKey := fmt.Sprintf("wallet_funding_%s", reference)
	existingTx, err := s.walletTxRepo.GetByIdempotencyKey(ctx, idempotencyKey)
	if err != nil {
		return fmt.Errorf("failed to check idempotency: %w", err)
	}
	if existingTx != nil {
		s.logger.Info("wallet funding already processed (idempotent)",
			zap.String("reference", reference),
		)
		return nil
	}

	// Find subaccount by account number (nuban)
	subaccount, err := s.flwSubaccountRepo.GetByAccountNumber(ctx, accountNumber)
	if err != nil {
		return fmt.Errorf("failed to find subaccount: %w", err)
	}
	if subaccount == nil {
		s.logger.Warn("wallet funding webhook for unknown account",
			zap.String("account_number", accountNumber),
			zap.String("reference", reference),
		)
		return fmt.Errorf("subaccount not found for account_number: %s", accountNumber)
	}

	// Get or create wallet for user
	wallet, err := s.walletRepo.GetOrCreate(ctx, subaccount.UserID)
	if err != nil {
		return fmt.Errorf("failed to get wallet: %w", err)
	}

	// Credit the internal wallet
	if err := s.walletRepo.Credit(ctx, wallet.ID, amount); err != nil {
		return fmt.Errorf("failed to credit wallet: %w", err)
	}

	// Create transaction record
	wallet, _ = s.walletRepo.GetByID(ctx, wallet.ID)
	refType := "wallet_funding"
	description := "Bank transfer to virtual account"
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

	s.logger.Info("wallet funding processed from bank transfer",
		zap.String("user_id", subaccount.UserID.String()),
		zap.Float64("amount", amount),
		zap.String("reference", reference),
	)

	// Send push notification
	if s.notificationService != nil {
		go func() {
			_ = s.notificationService.SendToUser(
				context.Background(),
				subaccount.UserID,
				"Bank Transfer Received",
				fmt.Sprintf("Your wallet has been credited with ₦%.2f via bank transfer", amount),
				map[string]string{
					"amount":    fmt.Sprintf("%.2f", amount),
					"reference": reference,
				},
				domain.NotificationDVATransfer,
			)
		}()
	}

	return nil
}

// ProcessWalletFundingByAccountRef handles bank transfers to the user's virtual account
// using the account_reference to look up the subaccount
func (s *WalletFundingServiceImpl) ProcessWalletFundingByAccountRef(ctx context.Context, accountRef string, amount float64, reference string) error {
	// Idempotency check
	idempotencyKey := fmt.Sprintf("wallet_funding_%s", reference)
	existingTx, err := s.walletTxRepo.GetByIdempotencyKey(ctx, idempotencyKey)
	if err != nil {
		return fmt.Errorf("failed to check idempotency: %w", err)
	}
	if existingTx != nil {
		s.logger.Info("wallet funding already processed (idempotent)",
			zap.String("reference", reference),
		)
		return nil
	}

	// Find subaccount by account_reference
	subaccount, err := s.flwSubaccountRepo.GetByAccountRef(ctx, accountRef)
	if err != nil {
		return fmt.Errorf("failed to find subaccount: %w", err)
	}
	if subaccount == nil {
		s.logger.Warn("wallet funding webhook for unknown account_ref",
			zap.String("account_reference", accountRef),
			zap.String("reference", reference),
		)
		return fmt.Errorf("subaccount not found for account_reference: %s", accountRef)
	}

	// Get or create wallet for user
	wallet, err := s.walletRepo.GetOrCreate(ctx, subaccount.UserID)
	if err != nil {
		return fmt.Errorf("failed to get wallet: %w", err)
	}

	// Credit the internal wallet
	if err := s.walletRepo.Credit(ctx, wallet.ID, amount); err != nil {
		return fmt.Errorf("failed to credit wallet: %w", err)
	}

	// Create transaction record
	wallet, _ = s.walletRepo.GetByID(ctx, wallet.ID)
	refType := "wallet_funding"
	description := "Bank transfer to virtual account"
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

	s.logger.Info("wallet funding processed from bank transfer (via account_ref)",
		zap.String("user_id", subaccount.UserID.String()),
		zap.Float64("amount", amount),
		zap.String("reference", reference),
		zap.String("account_reference", accountRef),
	)

	// Send push notification
	if s.notificationService != nil {
		go func() {
			_ = s.notificationService.SendToUser(
				context.Background(),
				subaccount.UserID,
				"Bank Transfer Received",
				fmt.Sprintf("Your wallet has been credited with ₦%.2f via bank transfer", amount),
				map[string]string{
					"amount":    fmt.Sprintf("%.2f", amount),
					"reference": reference,
				},
				domain.NotificationDVATransfer,
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

	// Get or create Flutterwave subaccount (lazy creation)
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	email := user.PhoneNumber + "@kipa.ng"
	if user.Email != nil {
		email = *user.Email
	}

	subaccount, err := s.GetOrCreateFlutterwaveSubaccount(ctx, userID, email, user.PhoneNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get/create subaccount: %w", err)
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

	// Initiate transfer via Flutterwave (from subaccount)
	transferResp, err := s.flutterwaveClient.InitiateTransfer(ctx, &flutterwave.InitiateTransferRequest{
		AccountBank:     bankAccount.BankCode,
		AccountNumber:   bankAccount.AccountNumber,
		Amount:          netAmount,
		Currency:        "NGN",
		Narration:       "Kipa wallet withdrawal",
		Reference:       transferRef,
		DebitSubaccount: subaccount.AccountReference,
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
			if status == domain.WithdrawalStatusSuccess {
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
			} else if status == domain.WithdrawalStatusFailed || status == domain.WithdrawalStatusReversed {
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
// FLUTTERWAVE SYNCED WALLET OPERATIONS
// ============================================

// CreditWalletWithFlutterwave credits internal wallet AND funds the Flutterwave subaccount
func (s *WalletFundingServiceImpl) CreditWalletWithFlutterwave(ctx context.Context, userID uuid.UUID, amount float64, txType domain.WalletTransactionType, refType string, refID uuid.UUID, idempotencyKey string) error {
	// Check idempotency
	if idempotencyKey != "" {
		existingTx, err := s.walletTxRepo.GetByIdempotencyKey(ctx, idempotencyKey)
		if err != nil {
			return fmt.Errorf("failed to check idempotency key: %w", err)
		}
		if existingTx != nil {
			s.logger.Info("idempotent credit request - already processed",
				zap.String("idempotency_key", idempotencyKey),
			)
			return nil
		}
	}

	// Get or create subaccount (lazy creation)
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	email := user.PhoneNumber + "@kipa.ng"
	if user.Email != nil {
		email = *user.Email
	}

	subaccount, err := s.GetOrCreateFlutterwaveSubaccount(ctx, userID, email, user.PhoneNumber)
	if err != nil {
		return fmt.Errorf("failed to get/create subaccount: %w", err)
	}

	if subaccount.BarterID == nil || *subaccount.BarterID == "" {
		return fmt.Errorf("subaccount missing barter_id for wallet transfer")
	}

	// Fund wallet using barter_id (wallet-to-wallet transfer)
	fundRef := fmt.Sprintf("fund_%s_%d", uuid.New().String()[:8], time.Now().Unix())
	narration := fmt.Sprintf("Credit: %s", refType)

	_, err = s.flutterwaveClient.FundWallet(ctx, *subaccount.BarterID, amount, fundRef, narration)
	if err != nil {
		s.logger.Error("failed to fund Flutterwave subaccount",
			zap.String("user_id", userID.String()),
			zap.Float64("amount", amount),
			zap.Error(err),
		)
		return fmt.Errorf("failed to sync with Flutterwave: %w", err)
	}

	// Credit internal wallet
	wallet, err := s.walletRepo.GetOrCreate(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get wallet: %w", err)
	}

	if err := s.walletRepo.Credit(ctx, wallet.ID, amount); err != nil {
		// Note: Flutterwave is already funded - this is a consistency issue
		// In production, we should handle this with a reconciliation job
		s.logger.Error("CRITICAL: Flutterwave funded but wallet credit failed",
			zap.String("user_id", userID.String()),
			zap.Float64("amount", amount),
			zap.Error(err),
		)
		return fmt.Errorf("failed to credit wallet: %w", err)
	}

	// Record transaction
	wallet, _ = s.walletRepo.GetByID(ctx, wallet.ID)
	description := fmt.Sprintf("Credit via Flutterwave: %s", refType)
	tx := &domain.WalletTransaction{
		WalletID:       wallet.ID,
		Type:           txType,
		Amount:         amount,
		BalanceAfter:   wallet.AvailableBalance,
		ReferenceType:  &refType,
		ReferenceID:    &refID,
		Description:    &description,
		IdempotencyKey: &idempotencyKey,
	}
	s.walletTxRepo.Create(ctx, tx)

	s.logger.Info("wallet credited with Flutterwave sync",
		zap.String("user_id", userID.String()),
		zap.Float64("amount", amount),
		zap.String("type", string(txType)),
	)

	return nil
}

// DebitWalletWithFlutterwave debits internal wallet AND transfers from Flutterwave subaccount back to platform
// This is used when user pays from their wallet (e.g., paying for a delivery)
// The funds are transferred from their subaccount to the platform's collection account
func (s *WalletFundingServiceImpl) DebitWalletWithFlutterwave(ctx context.Context, userID uuid.UUID, amount float64, txType domain.WalletTransactionType, refType string, refID uuid.UUID, idempotencyKey string) error {
	// Check idempotency
	if idempotencyKey != "" {
		existingTx, err := s.walletTxRepo.GetByIdempotencyKey(ctx, idempotencyKey)
		if err != nil {
			return fmt.Errorf("failed to check idempotency key: %w", err)
		}
		if existingTx != nil {
			s.logger.Info("idempotent debit request - already processed",
				zap.String("idempotency_key", idempotencyKey),
			)
			return nil
		}
	}

	// Verify user has a Flutterwave subaccount
	subaccount, err := s.flwSubaccountRepo.GetByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get subaccount: %w", err)
	}
	if subaccount == nil {
		return domain.ErrFlutterwaveSubaccountNotFound
	}

	// Check wallet balance
	wallet, err := s.walletRepo.GetByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get wallet: %w", err)
	}
	if wallet.AvailableBalance < amount {
		return domain.ErrInsufficientBalance
	}

	// Debit internal wallet
	// Note: Actual Flutterwave subaccount debit (transfer) is handled by the specific operation
	// (e.g., withdrawal to bank uses InitiateTransfer with DebitSubaccount)
	// This method tracks the internal wallet state for Flutterwave users
	if err := s.walletRepo.Debit(ctx, wallet.ID, amount); err != nil {
		return fmt.Errorf("failed to debit wallet: %w", err)
	}

	// Record transaction
	wallet, _ = s.walletRepo.GetByID(ctx, wallet.ID)
	description := fmt.Sprintf("Wallet debit: %s", refType)
	tx := &domain.WalletTransaction{
		WalletID:       wallet.ID,
		Type:           txType,
		Amount:         -amount,
		BalanceAfter:   wallet.AvailableBalance,
		ReferenceType:  &refType,
		ReferenceID:    &refID,
		Description:    &description,
		IdempotencyKey: &idempotencyKey,
	}
	s.walletTxRepo.Create(ctx, tx)

	s.logger.Info("wallet debited for Flutterwave user",
		zap.String("user_id", userID.String()),
		zap.Float64("amount", amount),
		zap.String("type", string(txType)),
		zap.String("subaccount_ref", subaccount.AccountReference),
	)

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

// SyncFlutterwaveWallet syncs the internal wallet balance with the Flutterwave subaccount balance
// This is used when bank transfers are made to the virtual account (no webhook is sent)
func (s *WalletFundingServiceImpl) SyncFlutterwaveWallet(ctx context.Context, userID uuid.UUID) (*WalletSyncResult, error) {
	// Get subaccount
	subaccount, err := s.flwSubaccountRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get subaccount: %w", err)
	}
	if subaccount == nil {
		return nil, domain.ErrFlutterwaveSubaccountNotFound
	}

	// Get Flutterwave balance
	balanceResp, err := s.flutterwaveClient.GetSubaccountBalance(ctx, subaccount.AccountReference)
	if err != nil {
		return nil, fmt.Errorf("failed to get Flutterwave balance: %w", err)
	}
	flwBalance := balanceResp.Data.AvailableBalance

	// Get internal wallet
	wallet, err := s.walletRepo.GetOrCreate(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet: %w", err)
	}
	previousBalance := wallet.AvailableBalance

	// Calculate difference
	// If Flutterwave has more money than our internal wallet, credit the difference
	diff := flwBalance - previousBalance

	result := &WalletSyncResult{
		PreviousBalance:    previousBalance,
		FlutterwaveBalance: flwBalance,
		NewBalance:         previousBalance,
		AmountSynced:       0,
		WasSynced:          false,
	}

	if diff > 0 {
		// Flutterwave has more - credit internal wallet
		idempotencyKey := fmt.Sprintf("sync_%s_%d", userID.String(), time.Now().Unix())

		// Check idempotency (prevent rapid duplicate syncs)
		existingTx, err := s.walletTxRepo.GetByIdempotencyKey(ctx, idempotencyKey)
		if err != nil {
			s.logger.Warn("idempotency check failed, proceeding anyway", zap.Error(err))
		}
		if existingTx != nil {
			// Already synced very recently, return current state
			result.NewBalance = wallet.AvailableBalance
			return result, nil
		}

		// Credit wallet
		if err := s.walletRepo.Credit(ctx, wallet.ID, diff); err != nil {
			return nil, fmt.Errorf("failed to credit wallet: %w", err)
		}

		// Record transaction
		wallet, _ = s.walletRepo.GetByID(ctx, wallet.ID)
		refType := "wallet_sync"
		description := "Sync with Flutterwave subaccount (bank transfer)"
		tx := &domain.WalletTransaction{
			WalletID:       wallet.ID,
			Type:           domain.WalletTxTopUp,
			Amount:         diff,
			BalanceAfter:   wallet.AvailableBalance,
			ReferenceType:  &refType,
			Description:    &description,
			IdempotencyKey: &idempotencyKey,
		}
		s.walletTxRepo.Create(ctx, tx)

		result.NewBalance = wallet.AvailableBalance
		result.AmountSynced = diff
		result.WasSynced = true

		s.logger.Info("wallet synced with Flutterwave",
			zap.String("user_id", userID.String()),
			zap.Float64("previous_balance", previousBalance),
			zap.Float64("flw_balance", flwBalance),
			zap.Float64("amount_synced", diff),
		)

		// Send push notification
		if s.notificationService != nil {
			go func() {
				_ = s.notificationService.SendToUser(
					context.Background(),
					userID,
					"Wallet Synced",
					fmt.Sprintf("Your wallet has been credited with ₦%.2f from bank transfer", diff),
					map[string]string{
						"amount":      fmt.Sprintf("%.2f", diff),
						"sync_type":   "bank_transfer",
					},
					domain.NotificationDVATransfer,
				)
			}()
		}
	} else if diff < 0 {
		// Internal wallet has more than Flutterwave
		// This could happen if:
		// 1. User did a card top-up that was credited to internal wallet but Flutterwave funding failed
		// 2. User spent from Flutterwave side directly (shouldn't happen normally)
		// For safety, we log this but don't debit the user
		s.logger.Warn("wallet has more than Flutterwave subaccount",
			zap.String("user_id", userID.String()),
			zap.Float64("wallet_balance", previousBalance),
			zap.Float64("flw_balance", flwBalance),
			zap.Float64("difference", diff),
		)
	}

	return result, nil
}

// GetAllActiveFlutterwaveSubaccounts returns all user IDs with active Flutterwave subaccounts
// Used by the background wallet sync job
func (s *WalletFundingServiceImpl) GetAllActiveFlutterwaveSubaccounts(ctx context.Context) ([]uuid.UUID, error) {
	return s.flwSubaccountRepo.GetAllActiveUserIDs(ctx)
}

var _ WalletFundingService = (*WalletFundingServiceImpl)(nil)
