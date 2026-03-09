package service

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"kipa/internal/config"
	"kipa/internal/domain"
	"kipa/internal/integration"
	"kipa/internal/repository"
)
type WalletService interface {
	GetWallet(ctx context.Context, userID uuid.UUID) (*domain.Wallet, error)
	Credit(ctx context.Context, userID uuid.UUID, amount float64, txType domain.WalletTransactionType, refType string, refID uuid.UUID, description, idempotencyKey string) (*domain.WalletTransaction, error)
	Debit(ctx context.Context, userID uuid.UUID, amount float64, txType domain.WalletTransactionType, refType string, refID uuid.UUID, description, idempotencyKey string) (*domain.WalletTransaction, error)
	Lock(ctx context.Context, userID uuid.UUID, amount float64, idempotencyKey string) error
	Unlock(ctx context.Context, userID uuid.UUID, amount float64, idempotencyKey string) error
	DeductLocked(ctx context.Context, userID uuid.UUID, amount float64, txType domain.WalletTransactionType, refType string, refID uuid.UUID, description, idempotencyKey string) (*domain.WalletTransaction, error)
	GetTransactions(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.WalletTransaction, error)
	GetBalance(ctx context.Context, userID uuid.UUID) (*WalletBalance, error)
	CreatePin(ctx context.Context, userID uuid.UUID, pin string) error
	VerifyPin(ctx context.Context, userID uuid.UUID, pin string) error
	ChangePin(ctx context.Context, userID uuid.UUID, oldPin, newPin string) error
	HasPin(ctx context.Context, userID uuid.UUID) (bool, error)
	GetPinStatus(ctx context.Context, userID uuid.UUID) (*PinStatus, error)
	RequestPinReset(ctx context.Context, userID uuid.UUID) (*PinResetResponse, error)
	ConfirmPinReset(ctx context.Context, userID uuid.UUID, otpCode, newPin string) error
}

type WalletBalance struct {
	AvailableBalance float64 `json:"available_balance"`
	LockedBalance    float64 `json:"locked_balance"`
	TotalEarned      float64 `json:"total_earned"`
	TotalWithdrawn   float64 `json:"total_withdrawn"`
}

type PinStatus struct {
	HasPin         bool       `json:"has_pin"`
	IsLocked       bool       `json:"is_locked"`
	LockedUntil    *time.Time `json:"locked_until,omitempty"`
	AttemptsLeft   int        `json:"attempts_left"`
	PinCreatedAt   *time.Time `json:"pin_created_at,omitempty"`
}

type PinResetResponse struct {
	Message           string    `json:"message"`
	ExpiresAt         time.Time `json:"expires_at"`
	RetryAfterSeconds int       `json:"retry_after_seconds"`
}

const (
	maxPinAttempts       = 5
	pinLockDuration      = 30 * time.Minute
	pinResetPendingTTL   = 10 * time.Minute
)

type WalletServiceImpl struct {
	walletRepo          repository.WalletRepository
	txRepo              repository.WalletTransactionRepository
	userRepo            repository.UserRepository
	sessionRepo         repository.SessionRepository
	verifyProvider      integration.VerifyProvider
	notificationService NotificationService
	config              *config.Config
	logger              *zap.Logger
}

func NewWalletService(
	walletRepo repository.WalletRepository,
	txRepo repository.WalletTransactionRepository,
	userRepo repository.UserRepository,
	sessionRepo repository.SessionRepository,
	verifyProvider integration.VerifyProvider,
	notificationService NotificationService,
	cfg *config.Config,
	logger *zap.Logger,
) WalletService {
	return &WalletServiceImpl{
		walletRepo:          walletRepo,
		txRepo:              txRepo,
		userRepo:            userRepo,
		sessionRepo:         sessionRepo,
		verifyProvider:      verifyProvider,
		notificationService: notificationService,
		config:              cfg,
		logger:              logger,
	}
}
func (s *WalletServiceImpl) GetWallet(ctx context.Context, userID uuid.UUID) (*domain.Wallet, error) {
	return s.walletRepo.GetOrCreate(ctx, userID)
}
func (s *WalletServiceImpl) Credit(ctx context.Context, userID uuid.UUID, amount float64, txType domain.WalletTransactionType, refType string, refID uuid.UUID, description, idempotencyKey string) (*domain.WalletTransaction, error) {
	if idempotencyKey != "" {
		existingTx, err := s.txRepo.GetByIdempotencyKey(ctx, idempotencyKey)
		if err != nil {
			return nil, fmt.Errorf("failed to check idempotency key: %w", err)
		}
		if existingTx != nil {
			s.logger.Info("idempotent credit request - returning existing transaction",
				zap.String("idempotency_key", idempotencyKey),
				zap.String("tx_id", existingTx.ID.String()),
			)
			return existingTx, nil
		}
	}
	wallet, err := s.walletRepo.GetOrCreate(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet: %w", err)
	}
	if err := s.walletRepo.Credit(ctx, wallet.ID, amount); err != nil {
		return nil, fmt.Errorf("failed to credit wallet: %w", err)
	}
	updatedWallet, err := s.walletRepo.GetByID(ctx, wallet.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get updated wallet: %w", err)
	}
	tx := &domain.WalletTransaction{
		WalletID:       wallet.ID,
		Type:           txType,
		Amount:         amount,
		BalanceAfter:   updatedWallet.AvailableBalance,
		ReferenceType:  &refType,
		ReferenceID:    &refID,
		Description:    &description,
		IdempotencyKey: &idempotencyKey,
	}

	if err := s.txRepo.Create(ctx, tx); err != nil {
		s.logger.Error("failed to create transaction record",
			zap.Error(err),
			zap.String("idempotency_key", idempotencyKey),
		)
	}

	s.logger.Info("wallet credited",
		zap.String("user_id", userID.String()),
		zap.Float64("amount", amount),
		zap.String("type", string(txType)),
	)

	return tx, nil
}
func (s *WalletServiceImpl) Debit(ctx context.Context, userID uuid.UUID, amount float64, txType domain.WalletTransactionType, refType string, refID uuid.UUID, description, idempotencyKey string) (*domain.WalletTransaction, error) {
	if idempotencyKey != "" {
		existingTx, err := s.txRepo.GetByIdempotencyKey(ctx, idempotencyKey)
		if err != nil {
			return nil, fmt.Errorf("failed to check idempotency key: %w", err)
		}
		if existingTx != nil {
			s.logger.Info("idempotent debit request - returning existing transaction",
				zap.String("idempotency_key", idempotencyKey),
				zap.String("tx_id", existingTx.ID.String()),
			)
			return existingTx, nil
		}
	}
	wallet, err := s.walletRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet: %w", err)
	}
	if wallet.AvailableBalance < amount {
		return nil, domain.ErrInsufficientBalance
	}
	if err := s.walletRepo.Debit(ctx, wallet.ID, amount); err != nil {
		return nil, fmt.Errorf("failed to debit wallet: %w", err)
	}
	updatedWallet, err := s.walletRepo.GetByID(ctx, wallet.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get updated wallet: %w", err)
	}
	tx := &domain.WalletTransaction{
		WalletID:       wallet.ID,
		Type:           txType,
		Amount:         -amount, // Negative for debit
		BalanceAfter:   updatedWallet.AvailableBalance,
		ReferenceType:  &refType,
		ReferenceID:    &refID,
		Description:    &description,
		IdempotencyKey: &idempotencyKey,
	}

	if err := s.txRepo.Create(ctx, tx); err != nil {
		s.logger.Error("failed to create transaction record",
			zap.Error(err),
			zap.String("idempotency_key", idempotencyKey),
		)
	}

	s.logger.Info("wallet debited",
		zap.String("user_id", userID.String()),
		zap.Float64("amount", amount),
		zap.String("type", string(txType)),
	)

	return tx, nil
}
func (s *WalletServiceImpl) Lock(ctx context.Context, userID uuid.UUID, amount float64, idempotencyKey string) error {
	wallet, err := s.walletRepo.GetByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get wallet: %w", err)
	}

	if err := wallet.Lock(amount); err != nil {
		return err
	}

	return s.walletRepo.Update(ctx, wallet)
}
func (s *WalletServiceImpl) Unlock(ctx context.Context, userID uuid.UUID, amount float64, idempotencyKey string) error {
	wallet, err := s.walletRepo.GetByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get wallet: %w", err)
	}

	if err := wallet.Unlock(amount); err != nil {
		return err
	}

	return s.walletRepo.Update(ctx, wallet)
}
func (s *WalletServiceImpl) DeductLocked(ctx context.Context, userID uuid.UUID, amount float64, txType domain.WalletTransactionType, refType string, refID uuid.UUID, description, idempotencyKey string) (*domain.WalletTransaction, error) {
	if idempotencyKey != "" {
		existingTx, err := s.txRepo.GetByIdempotencyKey(ctx, idempotencyKey)
		if err != nil {
			return nil, fmt.Errorf("failed to check idempotency key: %w", err)
		}
		if existingTx != nil {
			return existingTx, nil
		}
	}

	wallet, err := s.walletRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet: %w", err)
	}

	if err := wallet.DeductLocked(amount); err != nil {
		return nil, err
	}

	if err := s.walletRepo.Update(ctx, wallet); err != nil {
		return nil, fmt.Errorf("failed to update wallet: %w", err)
	}
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

	if err := s.txRepo.Create(ctx, tx); err != nil {
		s.logger.Error("failed to create transaction record", zap.Error(err))
	}

	return tx, nil
}
func (s *WalletServiceImpl) GetTransactions(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.WalletTransaction, error) {
	wallet, err := s.walletRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet: %w", err)
	}

	return s.txRepo.GetByWalletID(ctx, wallet.ID, limit, offset)
}
func (s *WalletServiceImpl) GetBalance(ctx context.Context, userID uuid.UUID) (*WalletBalance, error) {
	wallet, err := s.walletRepo.GetOrCreate(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet: %w", err)
	}

	return &WalletBalance{
		AvailableBalance: wallet.AvailableBalance,
		LockedBalance:    wallet.LockedBalance,
		TotalEarned:      wallet.TotalEarned,
		TotalWithdrawn:   wallet.TotalWithdrawn,
	}, nil
}
func validatePin(pin string) error {
	if len(pin) < 4 {
		return domain.ErrPinTooShort
	}
	if len(pin) > 6 {
		return domain.ErrPinTooLong
	}
	if matched, _ := regexp.MatchString(`^\d+$`, pin); !matched {
		return domain.ErrPinMustBeNumeric
	}
	return nil
}

func (s *WalletServiceImpl) CreatePin(ctx context.Context, userID uuid.UUID, pin string) error {
	if err := validatePin(pin); err != nil {
		return err
	}

	wallet, err := s.walletRepo.GetOrCreate(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get wallet: %w", err)
	}

	if wallet.HasPin() {
		return domain.ErrPinAlreadySet
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pin), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash PIN: %w", err)
	}

	if err := s.walletRepo.SetPin(ctx, wallet.ID, string(hash)); err != nil {
		return fmt.Errorf("failed to set PIN: %w", err)
	}

	s.logger.Info("wallet PIN created",
		zap.String("user_id", userID.String()),
	)

	return nil
}

func (s *WalletServiceImpl) VerifyPin(ctx context.Context, userID uuid.UUID, pin string) error {
	wallet, err := s.walletRepo.GetByUserID(ctx, userID)
	if err != nil {
		return err
	}

	if !wallet.HasPin() {
		return domain.ErrPinNotSet
	}

	if wallet.IsPinLocked() {
		return domain.ErrPinLocked
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*wallet.PinHash), []byte(pin)); err != nil {
		wallet.IncrementPinAttempts(maxPinAttempts, pinLockDuration)
		if err := s.walletRepo.UpdatePinAttempts(ctx, wallet.ID, wallet.PinAttempts, wallet.PinLockedUntil); err != nil {
			s.logger.Error("failed to update PIN attempts", zap.Error(err))
		}

		if wallet.IsPinLocked() {
			s.logger.Warn("wallet locked due to too many failed PIN attempts",
				zap.String("user_id", userID.String()),
			)
			return domain.ErrPinLocked
		}

		return domain.ErrInvalidPin
	}
	if wallet.PinAttempts > 0 {
		if err := s.walletRepo.ResetPinAttempts(ctx, wallet.ID); err != nil {
			s.logger.Error("failed to reset PIN attempts", zap.Error(err))
		}
	}

	return nil
}

func (s *WalletServiceImpl) ChangePin(ctx context.Context, userID uuid.UUID, oldPin, newPin string) error {
	if err := s.VerifyPin(ctx, userID, oldPin); err != nil {
		return err
	}
	if err := validatePin(newPin); err != nil {
		return err
	}

	wallet, err := s.walletRepo.GetByUserID(ctx, userID)
	if err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPin), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash PIN: %w", err)
	}

	if err := s.walletRepo.SetPin(ctx, wallet.ID, string(hash)); err != nil {
		return fmt.Errorf("failed to update PIN: %w", err)
	}

	s.logger.Info("wallet PIN changed",
		zap.String("user_id", userID.String()),
	)

	return nil
}

func (s *WalletServiceImpl) HasPin(ctx context.Context, userID uuid.UUID) (bool, error) {
	wallet, err := s.walletRepo.GetOrCreate(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("failed to get wallet: %w", err)
	}

	return wallet.HasPin(), nil
}

func (s *WalletServiceImpl) GetPinStatus(ctx context.Context, userID uuid.UUID) (*PinStatus, error) {
	wallet, err := s.walletRepo.GetOrCreate(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet: %w", err)
	}

	status := &PinStatus{
		HasPin:       wallet.HasPin(),
		IsLocked:     wallet.IsPinLocked(),
		AttemptsLeft: maxPinAttempts - wallet.PinAttempts,
		PinCreatedAt: wallet.PinCreatedAt,
	}

	if wallet.IsPinLocked() {
		status.LockedUntil = wallet.PinLockedUntil
		status.AttemptsLeft = 0
	}

	return status, nil
}

func (s *WalletServiceImpl) RequestPinReset(ctx context.Context, userID uuid.UUID) (*PinResetResponse, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	count, err := s.sessionRepo.GetRateLimitCount(ctx, user.PhoneNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to check rate limit: %w", err)
	}

	if count >= s.config.OTP.RateLimitCount {
		s.logger.Warn("PIN reset rate limit exceeded",
			zap.String("user_id", userID.String()),
			zap.Int("count", count),
		)
		return nil, domain.ErrOTPRateLimitExceeded
	}
	lastSent, err := s.sessionRepo.GetLastOTPSentTime(ctx, user.PhoneNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to check cooldown: %w", err)
	}

	if !lastSent.IsZero() {
		elapsed := time.Since(lastSent)
		if elapsed < s.config.OTP.ResendCooldown {
			return nil, domain.ErrOTPResendCooldown
		}
	}
	result, err := s.verifyProvider.SendVerification(ctx, user.PhoneNumber, "sms")
	if err != nil || !result.Success {
		s.logger.Error("failed to send PIN reset verification",
			zap.String("phone", user.PhoneNumber),
			zap.Error(err),
		)
		return nil, domain.ErrSMSDeliveryFailed
	}
	if err := s.sessionRepo.SetPinResetPending(ctx, userID, pinResetPendingTTL); err != nil {
		s.logger.Error("failed to set PIN reset pending", zap.Error(err))
	}
	if err := s.sessionRepo.IncrementRateLimitCount(ctx, user.PhoneNumber, s.config.OTP.RateLimitWindow); err != nil {
		s.logger.Error("failed to increment rate limit", zap.Error(err))
	}
	if err := s.sessionRepo.SetLastOTPSentTime(ctx, user.PhoneNumber, s.config.OTP.RateLimitWindow); err != nil {
		s.logger.Error("failed to set last OTP sent time", zap.Error(err))
	}

	s.logger.Info("PIN reset OTP sent",
		zap.String("user_id", userID.String()),
	)

	return &PinResetResponse{
		Message:           "OTP sent to your phone",
		ExpiresAt:         time.Now().Add(pinResetPendingTTL),
		RetryAfterSeconds: int(s.config.OTP.ResendCooldown.Seconds()),
	}, nil
}

func (s *WalletServiceImpl) ConfirmPinReset(ctx context.Context, userID uuid.UUID, otpCode, newPin string) error {
	pending, err := s.sessionRepo.GetPinResetPending(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to check PIN reset pending: %w", err)
	}

	if !pending {
		return domain.ErrPinResetNotRequested
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	result, err := s.verifyProvider.CheckVerification(ctx, user.PhoneNumber, otpCode)
	if err != nil {
		s.logger.Error("PIN reset verification check failed",
			zap.String("phone", user.PhoneNumber),
			zap.Error(err),
		)
		return domain.ErrInvalidOTP
	}

	if !result.Success {
		s.logger.Warn("invalid PIN reset OTP attempt",
			zap.String("user_id", userID.String()),
		)
		return domain.ErrInvalidOTP
	}
	if err := validatePin(newPin); err != nil {
		return err
	}
	wallet, err := s.walletRepo.GetOrCreate(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get wallet: %w", err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPin), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash PIN: %w", err)
	}
	if err := s.walletRepo.SetPin(ctx, wallet.ID, string(hash)); err != nil {
		return fmt.Errorf("failed to set PIN: %w", err)
	}
	if err := s.sessionRepo.DeletePinResetPending(ctx, userID); err != nil {
		s.logger.Error("failed to delete PIN reset pending", zap.Error(err))
	}
	if s.notificationService != nil {
		if err := s.notificationService.SendToUser(
			ctx,
			userID,
			"Wallet PIN Reset",
			"Your wallet PIN was reset successfully",
			nil,
			domain.NotificationPinReset,
		); err != nil {
			s.logger.Error("failed to send PIN reset notification", zap.Error(err))
		}
	}

	s.logger.Info("wallet PIN reset successfully",
		zap.String("user_id", userID.String()),
	)

	return nil
}

var _ WalletService = (*WalletServiceImpl)(nil)
