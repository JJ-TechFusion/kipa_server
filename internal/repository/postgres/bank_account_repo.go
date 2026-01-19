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
type BankAccountRepo struct {
	db *sqlx.DB
}

func NewBankAccountRepo(db *sqlx.DB) repository.BankAccountRepository {
	return &BankAccountRepo{db: db}
}

func (r *BankAccountRepo) Create(ctx context.Context, account *domain.BankAccount) error {
	query := `
		INSERT INTO bank_accounts (id, user_id, bank_code, bank_name, account_number, account_name, recipient_code, is_default, is_verified)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING created_at, updated_at`

	if account.ID == uuid.Nil {
		account.ID = uuid.New()
	}

	return r.db.QueryRowxContext(ctx, query,
		account.ID,
		account.UserID,
		account.BankCode,
		account.BankName,
		account.AccountNumber,
		account.AccountName,
		account.RecipientCode,
		account.IsDefault,
		account.IsVerified,
	).Scan(&account.CreatedAt, &account.UpdatedAt)
}

func (r *BankAccountRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.BankAccount, error) {
	var account domain.BankAccount
	query := `SELECT * FROM bank_accounts WHERE id = $1`

	err := r.db.GetContext(ctx, &account, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrBankAccountNotFound
		}
		return nil, err
	}

	return &account, nil
}

func (r *BankAccountRepo) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.BankAccount, error) {
	var accounts []*domain.BankAccount
	query := `SELECT * FROM bank_accounts WHERE user_id = $1 ORDER BY is_default DESC, created_at DESC`

	err := r.db.SelectContext(ctx, &accounts, query, userID)
	if err != nil {
		return nil, err
	}

	return accounts, nil
}

func (r *BankAccountRepo) GetDefaultByUserID(ctx context.Context, userID uuid.UUID) (*domain.BankAccount, error) {
	var account domain.BankAccount
	query := `SELECT * FROM bank_accounts WHERE user_id = $1 AND is_default = true LIMIT 1`

	err := r.db.GetContext(ctx, &account, query, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrBankAccountNotFound
		}
		return nil, err
	}

	return &account, nil
}

func (r *BankAccountRepo) GetByAccountNumber(ctx context.Context, userID uuid.UUID, accountNumber, bankCode string) (*domain.BankAccount, error) {
	var account domain.BankAccount
	query := `SELECT * FROM bank_accounts WHERE user_id = $1 AND account_number = $2 AND bank_code = $3`

	err := r.db.GetContext(ctx, &account, query, userID, accountNumber, bankCode)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &account, nil
}

func (r *BankAccountRepo) Update(ctx context.Context, account *domain.BankAccount) error {
	query := `
		UPDATE bank_accounts
		SET bank_name = $2, account_name = $3, recipient_code = $4, is_default = $5, is_verified = $6, updated_at = NOW()
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query,
		account.ID,
		account.BankName,
		account.AccountName,
		account.RecipientCode,
		account.IsDefault,
		account.IsVerified,
	)
	return err
}

func (r *BankAccountRepo) SetDefault(ctx context.Context, userID, accountID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `UPDATE bank_accounts SET is_default = false WHERE user_id = $1`, userID)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `UPDATE bank_accounts SET is_default = true WHERE id = $1`, accountID)
	return err
}

func (r *BankAccountRepo) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM bank_accounts WHERE id = $1`, id)
	return err
}
type VirtualAccountRepo struct {
	db *sqlx.DB
}

func NewVirtualAccountRepo(db *sqlx.DB) repository.VirtualAccountRepository {
	return &VirtualAccountRepo{db: db}
}

func (r *VirtualAccountRepo) Create(ctx context.Context, account *domain.VirtualAccount) error {
	query := `
		INSERT INTO virtual_accounts (id, user_id, bank_name, bank_slug, account_number, account_name, customer_code, dedicated_acct_id, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING created_at, updated_at`

	if account.ID == uuid.Nil {
		account.ID = uuid.New()
	}

	return r.db.QueryRowxContext(ctx, query,
		account.ID,
		account.UserID,
		account.BankName,
		account.BankSlug,
		account.AccountNumber,
		account.AccountName,
		account.CustomerCode,
		account.DedicatedAcctID,
		account.IsActive,
	).Scan(&account.CreatedAt, &account.UpdatedAt)
}

func (r *VirtualAccountRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.VirtualAccount, error) {
	var account domain.VirtualAccount
	query := `SELECT * FROM virtual_accounts WHERE id = $1`

	err := r.db.GetContext(ctx, &account, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrVirtualAccountNotFound
		}
		return nil, err
	}

	return &account, nil
}

func (r *VirtualAccountRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.VirtualAccount, error) {
	var account domain.VirtualAccount
	query := `SELECT * FROM virtual_accounts WHERE user_id = $1 AND is_active = true LIMIT 1`

	err := r.db.GetContext(ctx, &account, query, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // Return nil, nil when not found (will create new one)
		}
		return nil, err
	}

	return &account, nil
}

func (r *VirtualAccountRepo) GetByAccountNumber(ctx context.Context, accountNumber string) (*domain.VirtualAccount, error) {
	var account domain.VirtualAccount
	query := `SELECT * FROM virtual_accounts WHERE account_number = $1`

	err := r.db.GetContext(ctx, &account, query, accountNumber)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrVirtualAccountNotFound
		}
		return nil, err
	}

	return &account, nil
}

func (r *VirtualAccountRepo) GetByCustomerCode(ctx context.Context, customerCode string) (*domain.VirtualAccount, error) {
	var account domain.VirtualAccount
	query := `SELECT * FROM virtual_accounts WHERE customer_code = $1`

	err := r.db.GetContext(ctx, &account, query, customerCode)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrVirtualAccountNotFound
		}
		return nil, err
	}

	return &account, nil
}

func (r *VirtualAccountRepo) Update(ctx context.Context, account *domain.VirtualAccount) error {
	query := `
		UPDATE virtual_accounts
		SET bank_name = $2, bank_slug = $3, account_name = $4, is_active = $5, updated_at = NOW()
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query,
		account.ID,
		account.BankName,
		account.BankSlug,
		account.AccountName,
		account.IsActive,
	)
	return err
}
type WithdrawalRepo struct {
	db *sqlx.DB
}

func NewWithdrawalRepo(db *sqlx.DB) repository.WithdrawalRepository {
	return &WithdrawalRepo{db: db}
}

func (r *WithdrawalRepo) Create(ctx context.Context, withdrawal *domain.Withdrawal) error {
	query := `
		INSERT INTO withdrawals (id, user_id, wallet_id, bank_account_id, amount, fee, net_amount, status, transfer_ref)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING created_at, updated_at`

	if withdrawal.ID == uuid.Nil {
		withdrawal.ID = uuid.New()
	}

	return r.db.QueryRowxContext(ctx, query,
		withdrawal.ID,
		withdrawal.UserID,
		withdrawal.WalletID,
		withdrawal.BankAccountID,
		withdrawal.Amount,
		withdrawal.Fee,
		withdrawal.NetAmount,
		withdrawal.Status,
		withdrawal.TransferRef,
	).Scan(&withdrawal.CreatedAt, &withdrawal.UpdatedAt)
}

func (r *WithdrawalRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Withdrawal, error) {
	var withdrawal domain.Withdrawal
	query := `SELECT * FROM withdrawals WHERE id = $1`

	err := r.db.GetContext(ctx, &withdrawal, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrWithdrawalNotFound
		}
		return nil, err
	}

	return &withdrawal, nil
}

func (r *WithdrawalRepo) GetByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.Withdrawal, error) {
	var withdrawals []*domain.Withdrawal
	query := `SELECT * FROM withdrawals WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`

	err := r.db.SelectContext(ctx, &withdrawals, query, userID, limit, offset)
	if err != nil {
		return nil, err
	}

	return withdrawals, nil
}

func (r *WithdrawalRepo) GetByReference(ctx context.Context, reference string) (*domain.Withdrawal, error) {
	var withdrawal domain.Withdrawal
	query := `SELECT * FROM withdrawals WHERE transfer_ref = $1`

	err := r.db.GetContext(ctx, &withdrawal, query, reference)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrWithdrawalNotFound
		}
		return nil, err
	}

	return &withdrawal, nil
}

func (r *WithdrawalRepo) GetPendingByUserID(ctx context.Context, userID uuid.UUID) (*domain.Withdrawal, error) {
	var withdrawal domain.Withdrawal
	query := `SELECT * FROM withdrawals WHERE user_id = $1 AND status IN ('pending', 'processing') LIMIT 1`

	err := r.db.GetContext(ctx, &withdrawal, query, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &withdrawal, nil
}

func (r *WithdrawalRepo) Update(ctx context.Context, withdrawal *domain.Withdrawal) error {
	query := `
		UPDATE withdrawals
		SET status = $2, transfer_code = $3, failure_reason = $4, processed_at = $5, updated_at = NOW()
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query,
		withdrawal.ID,
		withdrawal.Status,
		withdrawal.TransferCode,
		withdrawal.FailureReason,
		withdrawal.ProcessedAt,
	)
	return err
}

func (r *WithdrawalRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.WithdrawalStatus) error {
	query := `UPDATE withdrawals SET status = $2, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, status)
	return err
}
type TopUpRepo struct {
	db *sqlx.DB
}

func NewTopUpRepo(db *sqlx.DB) repository.TopUpRepository {
	return &TopUpRepo{db: db}
}

func (r *TopUpRepo) Create(ctx context.Context, topUp *domain.TopUp) error {
	query := `
		INSERT INTO top_ups (id, user_id, wallet_id, amount, payment_method, payment_ref, payment_provider, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at, updated_at`

	if topUp.ID == uuid.Nil {
		topUp.ID = uuid.New()
	}

	return r.db.QueryRowxContext(ctx, query,
		topUp.ID,
		topUp.UserID,
		topUp.WalletID,
		topUp.Amount,
		topUp.PaymentMethod,
		topUp.PaymentRef,
		topUp.PaymentProvider,
		topUp.Status,
	).Scan(&topUp.CreatedAt, &topUp.UpdatedAt)
}

func (r *TopUpRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.TopUp, error) {
	var topUp domain.TopUp
	query := `SELECT * FROM top_ups WHERE id = $1`

	err := r.db.GetContext(ctx, &topUp, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrTopUpNotFound
		}
		return nil, err
	}

	return &topUp, nil
}

func (r *TopUpRepo) GetByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.TopUp, error) {
	var topUps []*domain.TopUp
	query := `SELECT * FROM top_ups WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`

	err := r.db.SelectContext(ctx, &topUps, query, userID, limit, offset)
	if err != nil {
		return nil, err
	}

	return topUps, nil
}

func (r *TopUpRepo) GetByReference(ctx context.Context, reference string) (*domain.TopUp, error) {
	var topUp domain.TopUp
	query := `SELECT * FROM top_ups WHERE payment_ref = $1`

	err := r.db.GetContext(ctx, &topUp, query, reference)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // Return nil when not found
		}
		return nil, err
	}

	return &topUp, nil
}

func (r *TopUpRepo) Update(ctx context.Context, topUp *domain.TopUp) error {
	query := `
		UPDATE top_ups
		SET status = $2, paid_at = $3, updated_at = NOW()
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query,
		topUp.ID,
		topUp.Status,
		topUp.PaidAt,
	)
	return err
}

func (r *TopUpRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.TopUpStatus) error {
	query := `UPDATE top_ups SET status = $2, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, status)
	return err
}
