package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"kipa/internal/domain"
	"kipa/internal/repository"
)
type WalletRepo struct {
	db *sqlx.DB
}

func NewWalletRepo(db *sqlx.DB) repository.WalletRepository {
	return &WalletRepo{db: db}
}

func (r *WalletRepo) Create(ctx context.Context, wallet *domain.Wallet) error {
	query := `
		INSERT INTO wallets (id, user_id, available_balance, locked_balance, total_earned, total_withdrawn)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at, updated_at`

	if wallet.ID == uuid.Nil {
		wallet.ID = uuid.New()
	}

	return r.db.QueryRowxContext(ctx, query,
		wallet.ID,
		wallet.UserID,
		wallet.AvailableBalance,
		wallet.LockedBalance,
		wallet.TotalEarned,
		wallet.TotalWithdrawn,
	).Scan(&wallet.CreatedAt, &wallet.UpdatedAt)
}

func (r *WalletRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Wallet, error) {
	var wallet domain.Wallet
	query := `SELECT * FROM wallets WHERE id = $1`

	err := r.db.GetContext(ctx, &wallet, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrWalletNotFound
		}
		return nil, err
	}

	return &wallet, nil
}

func (r *WalletRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.Wallet, error) {
	var wallet domain.Wallet
	query := `SELECT * FROM wallets WHERE user_id = $1`

	err := r.db.GetContext(ctx, &wallet, query, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrWalletNotFound
		}
		return nil, err
	}

	return &wallet, nil
}

func (r *WalletRepo) GetOrCreate(ctx context.Context, userID uuid.UUID) (*domain.Wallet, error) {
	wallet, err := r.GetByUserID(ctx, userID)
	if err == nil {
		return wallet, nil
	}

	if !errors.Is(err, domain.ErrWalletNotFound) {
		return nil, err
	}
	newWallet := &domain.Wallet{
		ID:               uuid.New(),
		UserID:           userID,
		AvailableBalance: 0,
		LockedBalance:    0,
		TotalEarned:      0,
		TotalWithdrawn:   0,
	}

	if err := r.Create(ctx, newWallet); err != nil {
		if isUniqueViolation(err) {
			return r.GetByUserID(ctx, userID)
		}
		return nil, err
	}

	return newWallet, nil
}

func (r *WalletRepo) Update(ctx context.Context, wallet *domain.Wallet) error {
	query := `
		UPDATE wallets SET
			available_balance = $2,
			locked_balance = $3,
			total_earned = $4,
			total_withdrawn = $5
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query,
		wallet.ID,
		wallet.AvailableBalance,
		wallet.LockedBalance,
		wallet.TotalEarned,
		wallet.TotalWithdrawn,
	)

	return err
}

func (r *WalletRepo) Credit(ctx context.Context, walletID uuid.UUID, amount float64) error {
	query := `
		UPDATE wallets SET
			available_balance = available_balance + $2,
			total_earned = total_earned + $2
		WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query, walletID, amount)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrWalletNotFound
	}

	return nil
}

func (r *WalletRepo) Debit(ctx context.Context, walletID uuid.UUID, amount float64) error {
	query := `
		UPDATE wallets SET
			available_balance = available_balance - $2
		WHERE id = $1 AND available_balance >= $2`

	result, err := r.db.ExecContext(ctx, query, walletID, amount)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrInsufficientBalance
	}

	return nil
}
type WalletTransactionRepo struct {
	db *sqlx.DB
}

func NewWalletTransactionRepo(db *sqlx.DB) repository.WalletTransactionRepository {
	return &WalletTransactionRepo{db: db}
}

func (r *WalletTransactionRepo) Create(ctx context.Context, tx *domain.WalletTransaction) error {
	query := `
		INSERT INTO wallet_transactions (
			id, wallet_id, type, amount, balance_after,
			reference_type, reference_id, description, idempotency_key
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING created_at`

	if tx.ID == uuid.Nil {
		tx.ID = uuid.New()
	}

	err := r.db.QueryRowxContext(ctx, query,
		tx.ID,
		tx.WalletID,
		tx.Type,
		tx.Amount,
		tx.BalanceAfter,
		tx.ReferenceType,
		tx.ReferenceID,
		tx.Description,
		tx.IdempotencyKey,
	).Scan(&tx.CreatedAt)

	if err != nil {
		if isUniqueViolation(err) {
			return domain.ErrIdempotencyKeyExists
		}
		return err
	}

	return nil
}

func (r *WalletTransactionRepo) GetByWalletID(ctx context.Context, walletID uuid.UUID, limit, offset int) ([]*domain.WalletTransaction, error) {
	var transactions []*domain.WalletTransaction
	query := `SELECT * FROM wallet_transactions WHERE wallet_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`

	err := r.db.SelectContext(ctx, &transactions, query, walletID, limit, offset)
	if err != nil {
		return nil, err
	}

	return transactions, nil
}

func (r *WalletTransactionRepo) GetByIdempotencyKey(ctx context.Context, key string) (*domain.WalletTransaction, error) {
	var tx domain.WalletTransaction
	query := `SELECT * FROM wallet_transactions WHERE idempotency_key = $1`

	err := r.db.GetContext(ctx, &tx, query, key)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &tx, nil
}

func (r *WalletTransactionRepo) IdempotencyKeyExists(ctx context.Context, key string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM wallet_transactions WHERE idempotency_key = $1)`
	err := r.db.GetContext(ctx, &exists, query, key)
	return exists, err
}
