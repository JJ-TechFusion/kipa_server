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
type EscrowRepo struct {
	db *sqlx.DB
}

func NewEscrowRepo(db *sqlx.DB) repository.EscrowRepository {
	return &EscrowRepo{db: db}
}

func (r *EscrowRepo) Create(ctx context.Context, escrow *domain.Escrow) error {
	query := `
		INSERT INTO escrows (
			id, payment_request_id, buyer_id, seller_id,
			item_amount, delivery_fee_estimate, buyer_fee, total_locked,
			payment_reference, payment_provider, status, funded_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING created_at`

	if escrow.ID == uuid.Nil {
		escrow.ID = uuid.New()
	}

	return r.db.QueryRowxContext(ctx, query,
		escrow.ID,
		escrow.PaymentRequestID,
		escrow.BuyerID,
		escrow.SellerID,
		escrow.ItemAmount,
		escrow.DeliveryFeeEstimate,
		escrow.BuyerFee,
		escrow.TotalLocked,
		escrow.PaymentReference,
		escrow.PaymentProvider,
		escrow.Status,
		escrow.FundedAt,
	).Scan(&escrow.CreatedAt)
}

func (r *EscrowRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Escrow, error) {
	var escrow domain.Escrow
	query := `SELECT * FROM escrows WHERE id = $1`

	err := r.db.GetContext(ctx, &escrow, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrEscrowNotFound
		}
		return nil, err
	}

	return &escrow, nil
}

func (r *EscrowRepo) GetByPaymentRequestID(ctx context.Context, prID uuid.UUID) (*domain.Escrow, error) {
	var escrow domain.Escrow
	query := `SELECT * FROM escrows WHERE payment_request_id = $1`

	err := r.db.GetContext(ctx, &escrow, query, prID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrEscrowNotFound
		}
		return nil, err
	}

	return &escrow, nil
}

func (r *EscrowRepo) GetByBuyerID(ctx context.Context, buyerID uuid.UUID, limit, offset int) ([]*domain.Escrow, error) {
	var escrows []*domain.Escrow
	query := `SELECT * FROM escrows WHERE buyer_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`

	err := r.db.SelectContext(ctx, &escrows, query, buyerID, limit, offset)
	if err != nil {
		return nil, err
	}

	return escrows, nil
}

func (r *EscrowRepo) GetBySellerID(ctx context.Context, sellerID uuid.UUID, limit, offset int) ([]*domain.Escrow, error) {
	var escrows []*domain.Escrow
	query := `SELECT * FROM escrows WHERE seller_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`

	err := r.db.SelectContext(ctx, &escrows, query, sellerID, limit, offset)
	if err != nil {
		return nil, err
	}

	return escrows, nil
}

func (r *EscrowRepo) Update(ctx context.Context, escrow *domain.Escrow) error {
	query := `
		UPDATE escrows SET
			actual_delivery_fee = $2,
			delivery_fee_adjustment = $3,
			seller_payout = $4,
			rider_payout = $5,
			platform_revenue = $6,
			status = $7,
			funded_at = $8,
			delivery_confirmed_at = $9,
			confirmation_window_ends_at = $10,
			released_at = $11
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query,
		escrow.ID,
		escrow.ActualDeliveryFee,
		escrow.DeliveryFeeAdjustment,
		escrow.SellerPayout,
		escrow.RiderPayout,
		escrow.PlatformRevenue,
		escrow.Status,
		escrow.FundedAt,
		escrow.DeliveryConfirmedAt,
		escrow.ConfirmationWindowEndsAt,
		escrow.ReleasedAt,
	)

	return err
}

func (r *EscrowRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.EscrowStatus) error {
	query := `UPDATE escrows SET status = $2 WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, id, status)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrEscrowNotFound
	}

	return nil
}

func (r *EscrowRepo) GetPendingConfirmationWindow(ctx context.Context) ([]*domain.Escrow, error) {
	var escrows []*domain.Escrow
	query := `
		SELECT * FROM escrows
		WHERE status = 'confirmation_window'
		AND confirmation_window_ends_at <= $1
		ORDER BY confirmation_window_ends_at ASC`

	err := r.db.SelectContext(ctx, &escrows, query, time.Now())
	if err != nil {
		return nil, err
	}

	return escrows, nil
}

func (r *EscrowRepo) GetPendingForUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*repository.PendingEscrowDetails, error) {
	var results []*repository.PendingEscrowDetails

	query := `
		SELECT
			e.id as escrow_id,
			e.status as escrow_status,
			e.item_amount,
			e.total_locked,
			e.funded_at,
			e.created_at,
			pr.id as payment_request_id,
			pr.item_name,
			pr.item_description,
			pr.processing_time_hours,
			CASE WHEN e.buyer_id = $1 THEN true ELSE false END as is_buyer,
			CASE WHEN e.buyer_id = $1 THEN e.seller_id ELSE e.buyer_id END as counter_party_id,
			CASE WHEN e.buyer_id = $1
				THEN COALESCE(CONCAT(seller.first_name, ' ', seller.last_name), 'Unknown')
				ELSE COALESCE(CONCAT(buyer.first_name, ' ', buyer.last_name), 'Unknown')
			END as counter_party_name,
			CASE WHEN e.buyer_id = $1
				THEN seller.profile_photo_url
				ELSE buyer.profile_photo_url
			END as counter_party_photo_url
		FROM escrows e
		JOIN payment_requests pr ON e.payment_request_id = pr.id
		JOIN users buyer ON e.buyer_id = buyer.id
		JOIN users seller ON e.seller_id = seller.id
		WHERE (e.buyer_id = $1 OR e.seller_id = $1)
			AND e.status IN ('funded', 'delivery_in_progress', 'delivered', 'confirmation_window', 'disputed')
		ORDER BY e.created_at DESC
		LIMIT $2 OFFSET $3`

	err := r.db.SelectContext(ctx, &results, query, userID, limit, offset)
	if err != nil {
		return nil, err
	}

	return results, nil
}

func (r *EscrowRepo) HasActiveByUserID(ctx context.Context, userID uuid.UUID) (bool, error) {
	var exists bool
	query := `
		SELECT EXISTS(
			SELECT 1 FROM escrows
			WHERE (buyer_id = $1 OR seller_id = $1)
			AND status IN ('funded', 'delivery_in_progress', 'delivered', 'confirmation_window', 'disputed')
		)`
	err := r.db.GetContext(ctx, &exists, query, userID)
	return exists, err
}
