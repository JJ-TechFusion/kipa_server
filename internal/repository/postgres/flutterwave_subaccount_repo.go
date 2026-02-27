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

type FlutterwaveSubaccountRepo struct {
	db *sqlx.DB
}

func NewFlutterwaveSubaccountRepo(db *sqlx.DB) repository.FlutterwaveSubaccountRepository {
	return &FlutterwaveSubaccountRepo{db: db}
}

func (r *FlutterwaveSubaccountRepo) Create(ctx context.Context, subaccount *domain.FlutterwaveSubaccount) error {
	query := `
		INSERT INTO flutterwave_subaccounts (id, user_id, account_reference, account_bank, account_number, barter_id, business_name, email, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING created_at, updated_at`

	if subaccount.ID == uuid.Nil {
		subaccount.ID = uuid.New()
	}

	return r.db.QueryRowxContext(ctx, query,
		subaccount.ID,
		subaccount.UserID,
		subaccount.AccountReference,
		subaccount.AccountBank,
		subaccount.AccountNumber,
		subaccount.BarterID,
		subaccount.BusinessName,
		subaccount.Email,
		subaccount.IsActive,
	).Scan(&subaccount.CreatedAt, &subaccount.UpdatedAt)
}

func (r *FlutterwaveSubaccountRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.FlutterwaveSubaccount, error) {
	var subaccount domain.FlutterwaveSubaccount
	query := `SELECT * FROM flutterwave_subaccounts WHERE id = $1`

	err := r.db.GetContext(ctx, &subaccount, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrFlutterwaveSubaccountNotFound
		}
		return nil, err
	}

	return &subaccount, nil
}

func (r *FlutterwaveSubaccountRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.FlutterwaveSubaccount, error) {
	var subaccount domain.FlutterwaveSubaccount
	query := `SELECT * FROM flutterwave_subaccounts WHERE user_id = $1 AND is_active = true LIMIT 1`

	err := r.db.GetContext(ctx, &subaccount, query, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // Return nil, nil when not found (will create new one)
		}
		return nil, err
	}

	return &subaccount, nil
}

func (r *FlutterwaveSubaccountRepo) GetByAccountRef(ctx context.Context, accountRef string) (*domain.FlutterwaveSubaccount, error) {
	var subaccount domain.FlutterwaveSubaccount
	query := `SELECT * FROM flutterwave_subaccounts WHERE account_reference = $1`

	err := r.db.GetContext(ctx, &subaccount, query, accountRef)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrFlutterwaveSubaccountNotFound
		}
		return nil, err
	}

	return &subaccount, nil
}

func (r *FlutterwaveSubaccountRepo) GetByAccountNumber(ctx context.Context, accountNumber string) (*domain.FlutterwaveSubaccount, error) {
	var subaccount domain.FlutterwaveSubaccount
	query := `SELECT * FROM flutterwave_subaccounts WHERE account_number = $1 AND is_active = true`

	err := r.db.GetContext(ctx, &subaccount, query, accountNumber)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // Return nil, nil to allow caller to handle "not found" gracefully
		}
		return nil, err
	}

	return &subaccount, nil
}

func (r *FlutterwaveSubaccountRepo) Update(ctx context.Context, subaccount *domain.FlutterwaveSubaccount) error {
	query := `
		UPDATE flutterwave_subaccounts
		SET account_bank = $2, account_number = $3, barter_id = $4, business_name = $5, email = $6, is_active = $7, updated_at = NOW()
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query,
		subaccount.ID,
		subaccount.AccountBank,
		subaccount.AccountNumber,
		subaccount.BarterID,
		subaccount.BusinessName,
		subaccount.Email,
		subaccount.IsActive,
	)
	return err
}

func (r *FlutterwaveSubaccountRepo) GetAllActiveUserIDs(ctx context.Context) ([]uuid.UUID, error) {
	var userIDs []uuid.UUID
	query := `SELECT user_id FROM flutterwave_subaccounts WHERE is_active = true`

	err := r.db.SelectContext(ctx, &userIDs, query)
	if err != nil {
		return nil, err
	}

	return userIDs, nil
}
