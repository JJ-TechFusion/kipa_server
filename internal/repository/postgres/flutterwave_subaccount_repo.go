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
		INSERT INTO flutterwave_subaccounts (id, user_id, account_reference, account_bank, account_number, business_name, email, mobile_number, is_active)
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
		subaccount.BusinessName,
		subaccount.Email,
		subaccount.MobileNumber,
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

func (r *FlutterwaveSubaccountRepo) Update(ctx context.Context, subaccount *domain.FlutterwaveSubaccount) error {
	query := `
		UPDATE flutterwave_subaccounts
		SET account_bank = $2, account_number = $3, business_name = $4, email = $5, mobile_number = $6, is_active = $7, updated_at = NOW()
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query,
		subaccount.ID,
		subaccount.AccountBank,
		subaccount.AccountNumber,
		subaccount.BusinessName,
		subaccount.Email,
		subaccount.MobileNumber,
		subaccount.IsActive,
	)
	return err
}
