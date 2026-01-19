package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/domain"
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
}

type WalletBalance struct {
	AvailableBalance float64 `json:"available_balance"`
	LockedBalance    float64 `json:"locked_balance"`
	TotalEarned      float64 `json:"total_earned"`
	TotalWithdrawn   float64 `json:"total_withdrawn"`
}

type WalletServiceImpl struct {
	walletRepo repository.WalletRepository
	txRepo     repository.WalletTransactionRepository
	logger     *zap.Logger
}

func NewWalletService(
	walletRepo repository.WalletRepository,
	txRepo repository.WalletTransactionRepository,
	logger *zap.Logger,
) WalletService {
	return &WalletServiceImpl{
		walletRepo: walletRepo,
		txRepo:     txRepo,
		logger:     logger,
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
var _ WalletService = (*WalletServiceImpl)(nil)
