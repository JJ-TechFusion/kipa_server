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

type FlutterwaveVirtualAccountRepo struct {
	db *sqlx.DB
}

func NewFlutterwaveVirtualAccountRepo(db *sqlx.DB) repository.FlutterwaveVirtualAccountRepository {
	return &FlutterwaveVirtualAccountRepo{db: db}
}

func (r *FlutterwaveVirtualAccountRepo) Create(ctx context.Context, va *domain.FlutterwaveVirtualAccount) error {
	query := `
		INSERT INTO flutterwave_virtual_accounts (id, user_id, account_number, bank_name, tx_ref, flw_ref, order_ref, email, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING created_at, updated_at`

	if va.ID == uuid.Nil {
		va.ID = uuid.New()
	}

	return r.db.QueryRowxContext(ctx, query,
		va.ID,
		va.UserID,
		va.AccountNumber,
		va.BankName,
		va.TxRef,
		va.FlwRef,
		va.OrderRef,
		va.Email,
		va.IsActive,
	).Scan(&va.CreatedAt, &va.UpdatedAt)
}

func (r *FlutterwaveVirtualAccountRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.FlutterwaveVirtualAccount, error) {
	var va domain.FlutterwaveVirtualAccount
	query := `SELECT * FROM flutterwave_virtual_accounts WHERE id = $1`

	err := r.db.GetContext(ctx, &va, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrVirtualAccountNotFound
		}
		return nil, err
	}

	return &va, nil
}

func (r *FlutterwaveVirtualAccountRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.FlutterwaveVirtualAccount, error) {
	var va domain.FlutterwaveVirtualAccount
	query := `SELECT * FROM flutterwave_virtual_accounts WHERE user_id = $1 AND is_active = true LIMIT 1`

	err := r.db.GetContext(ctx, &va, query, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // Return nil, nil when not found (will create new one)
		}
		return nil, err
	}

	return &va, nil
}

func (r *FlutterwaveVirtualAccountRepo) GetByTxRef(ctx context.Context, txRef string) (*domain.FlutterwaveVirtualAccount, error) {
	var va domain.FlutterwaveVirtualAccount
	query := `SELECT * FROM flutterwave_virtual_accounts WHERE tx_ref = $1`

	err := r.db.GetContext(ctx, &va, query, txRef)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // Return nil, nil for webhook processing
		}
		return nil, err
	}

	return &va, nil
}

func (r *FlutterwaveVirtualAccountRepo) GetByAccountNumber(ctx context.Context, accountNumber string) (*domain.FlutterwaveVirtualAccount, error) {
	var va domain.FlutterwaveVirtualAccount
	query := `SELECT * FROM flutterwave_virtual_accounts WHERE account_number = $1 AND is_active = true`

	err := r.db.GetContext(ctx, &va, query, accountNumber)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &va, nil
}

func (r *FlutterwaveVirtualAccountRepo) Update(ctx context.Context, va *domain.FlutterwaveVirtualAccount) error {
	query := `
		UPDATE flutterwave_virtual_accounts
		SET account_number = $2, bank_name = $3, flw_ref = $4, order_ref = $5, email = $6, is_active = $7, updated_at = NOW()
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query,
		va.ID,
		va.AccountNumber,
		va.BankName,
		va.FlwRef,
		va.OrderRef,
		va.Email,
		va.IsActive,
	)
	return err
}
