package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"kipa/internal/domain"
	"kipa/internal/repository"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)
type RiderCommissionRepo struct {
	db *sqlx.DB
}

func NewRiderCommissionRepo(db *sqlx.DB) repository.RiderCommissionRepository {
	return &RiderCommissionRepo{db: db}
}

func (r *RiderCommissionRepo) Create(ctx context.Context, commission *domain.RiderCommission) error {
	query := `
		INSERT INTO rider_commissions (
			id, rider_profile_id, delivery_job_id,
			delivery_fee, commission_rate, commission_amount,
			status, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	if commission.ID == uuid.Nil {
		commission.ID = uuid.New()
	}
	if commission.CreatedAt.IsZero() {
		commission.CreatedAt = time.Now()
	}

	_, err := r.db.ExecContext(ctx, query,
		commission.ID,
		commission.RiderProfileID,
		commission.DeliveryJobID,
		commission.DeliveryFee,
		commission.CommissionRate,
		commission.CommissionAmount,
		commission.Status,
		commission.CreatedAt,
	)

	return err
}

func (r *RiderCommissionRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.RiderCommission, error) {
	var commission domain.RiderCommission
	query := `SELECT * FROM rider_commissions WHERE id = $1`

	err := r.db.GetContext(ctx, &commission, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrCommissionNotFound
		}
		return nil, err
	}

	return &commission, nil
}

func (r *RiderCommissionRepo) GetByDeliveryJobID(ctx context.Context, jobID uuid.UUID) (*domain.RiderCommission, error) {
	var commission domain.RiderCommission
	query := `SELECT * FROM rider_commissions WHERE delivery_job_id = $1`

	err := r.db.GetContext(ctx, &commission, query, jobID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrCommissionNotFound
		}
		return nil, err
	}

	return &commission, nil
}

func (r *RiderCommissionRepo) GetPendingByRiderID(ctx context.Context, riderProfileID uuid.UUID) ([]*domain.RiderCommission, error) {
	var commissions []*domain.RiderCommission
	query := `
		SELECT * FROM rider_commissions 
		WHERE rider_profile_id = $1 AND status = 'pending'
		ORDER BY created_at ASC`

	err := r.db.SelectContext(ctx, &commissions, query, riderProfileID)
	if err != nil {
		return nil, err
	}

	return commissions, nil
}

func (r *RiderCommissionRepo) GetByRiderID(ctx context.Context, riderProfileID uuid.UUID, limit, offset int) ([]*domain.RiderCommission, error) {
	var commissions []*domain.RiderCommission
	query := `
		SELECT * FROM rider_commissions 
		WHERE rider_profile_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	err := r.db.SelectContext(ctx, &commissions, query, riderProfileID, limit, offset)
	if err != nil {
		return nil, err
	}

	return commissions, nil
}

func (r *RiderCommissionRepo) GetPendingTotals(ctx context.Context, riderProfileID uuid.UUID) (float64, int, error) {
	var result struct {
		TotalAmount float64 `db:"total_amount"`
		Count       int     `db:"count"`
	}

	query := `
		SELECT 
			COALESCE(SUM(commission_amount), 0) as total_amount,
			COUNT(*) as count
		FROM rider_commissions 
		WHERE rider_profile_id = $1 AND status = 'pending'`

	err := r.db.GetContext(ctx, &result, query, riderProfileID)
	if err != nil {
		return 0, 0, err
	}

	return result.TotalAmount, result.Count, nil
}

func (r *RiderCommissionRepo) MarkAllPendingAsPaid(ctx context.Context, riderProfileID uuid.UUID, paidVia string) (int, error) {
	now := time.Now()
	query := `
		UPDATE rider_commissions SET 
			status = 'paid',
			paid_at = $2,
			paid_via = $3
		WHERE rider_profile_id = $1 AND status = 'pending'`

	result, err := r.db.ExecContext(ctx, query, riderProfileID, now, paidVia)
	if err != nil {
		return 0, err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}

	return int(rows), nil
}

func (r *RiderCommissionRepo) MarkAsWaived(ctx context.Context, riderProfileID uuid.UUID, paidVia string) (int, error) {
	now := time.Now()
	query := `
		UPDATE rider_commissions SET 
			status = 'waived',
			paid_at = $2,
			paid_via = $3
		WHERE rider_profile_id = $1 AND status = 'pending'`

	result, err := r.db.ExecContext(ctx, query, riderProfileID, now, paidVia)
	if err != nil {
		return 0, err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}

	return int(rows), nil
}
type CommissionPaymentRepo struct {
	db *sqlx.DB
}

func NewCommissionPaymentRepo(db *sqlx.DB) repository.CommissionPaymentRepository {
	return &CommissionPaymentRepo{db: db}
}

func (r *CommissionPaymentRepo) Create(ctx context.Context, payment *domain.CommissionPayment) error {
	query := `
		INSERT INTO commission_payments (
			id, rider_profile_id, amount, commissions_paid,
			payment_method, wallet_transaction_id, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)`

	if payment.ID == uuid.Nil {
		payment.ID = uuid.New()
	}
	if payment.CreatedAt.IsZero() {
		payment.CreatedAt = time.Now()
	}

	_, err := r.db.ExecContext(ctx, query,
		payment.ID,
		payment.RiderProfileID,
		payment.Amount,
		payment.CommissionsPaid,
		payment.PaymentMethod,
		payment.WalletTransactionID,
		payment.CreatedAt,
	)

	return err
}

func (r *CommissionPaymentRepo) GetByRiderID(ctx context.Context, riderProfileID uuid.UUID, limit, offset int) ([]*domain.CommissionPayment, error) {
	var payments []*domain.CommissionPayment
	query := `
		SELECT * FROM commission_payments 
		WHERE rider_profile_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	err := r.db.SelectContext(ctx, &payments, query, riderProfileID, limit, offset)
	if err != nil {
		return nil, err
	}

	return payments, nil
}
