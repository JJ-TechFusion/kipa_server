package postgres

import (
	"context"
	"database/sql"
	"errors"

	"kipa/internal/domain"
	"kipa/internal/repository"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)
type PaymentRequestRepo struct {
	db *sqlx.DB
}

func NewPaymentRequestRepo(db *sqlx.DB) repository.PaymentRequestRepository {
	return &PaymentRequestRepo{db: db}
}

func (r *PaymentRequestRepo) Create(ctx context.Context, pr *domain.PaymentRequest) error {
	query := `
		INSERT INTO payment_requests (
			id, seller_id, item_name, item_description, item_price, item_images,
			processing_time_hours, delivery_type, status, is_reusable, max_uses, current_uses
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING created_at, updated_at`

	if pr.ID == uuid.Nil {
		pr.ID = uuid.New()
	}

	return r.db.QueryRowxContext(ctx, query,
		pr.ID,
		pr.SellerID,
		pr.ItemName,
		pr.ItemDescription,
		pr.ItemPrice,
		pr.ItemImages,
		pr.ProcessingTimeHours,
		pr.DeliveryType,
		pr.Status,
		pr.IsReusable,
		pr.MaxUses,
		pr.CurrentUses,
	).Scan(&pr.CreatedAt, &pr.UpdatedAt)
}

func (r *PaymentRequestRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.PaymentRequest, error) {
	var pr domain.PaymentRequest
	query := `SELECT * FROM payment_requests WHERE id = $1`

	err := r.db.GetContext(ctx, &pr, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrPaymentRequestNotFound
		}
		return nil, err
	}

	return &pr, nil
}

func (r *PaymentRequestRepo) GetByPaymentCode(ctx context.Context, code string) (*domain.PaymentRequest, error) {
	var pr domain.PaymentRequest
	query := `SELECT * FROM payment_requests WHERE payment_code = $1`

	err := r.db.GetContext(ctx, &pr, query, code)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrPaymentRequestNotFound
		}
		return nil, err
	}

	return &pr, nil
}

func (r *PaymentRequestRepo) GetBySellerID(ctx context.Context, sellerID uuid.UUID, limit, offset int) ([]*domain.PaymentRequest, error) {
	var requests []*domain.PaymentRequest
	query := `SELECT * FROM payment_requests WHERE seller_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`

	err := r.db.SelectContext(ctx, &requests, query, sellerID, limit, offset)
	if err != nil {
		return nil, err
	}

	return requests, nil
}

func (r *PaymentRequestRepo) GetByBuyerID(ctx context.Context, buyerID uuid.UUID, limit, offset int) ([]*domain.PaymentRequest, error) {
	var requests []*domain.PaymentRequest
	query := `SELECT * FROM payment_requests WHERE buyer_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`

	err := r.db.SelectContext(ctx, &requests, query, buyerID, limit, offset)
	if err != nil {
		return nil, err
	}

	return requests, nil
}

func (r *PaymentRequestRepo) GetHistoryBySellerID(ctx context.Context, sellerID uuid.UUID, limit, offset int) ([]*domain.PaymentRequest, error) {
	var requests []*domain.PaymentRequest
	query := `SELECT * FROM payment_requests WHERE seller_id = $1 AND status IN ('expired', 'cancelled', 'completed', 'refunded') ORDER BY updated_at DESC LIMIT $2 OFFSET $3`

	err := r.db.SelectContext(ctx, &requests, query, sellerID, limit, offset)
	if err != nil {
		return nil, err
	}

	return requests, nil
}

func (r *PaymentRequestRepo) GetExpiredLinks(ctx context.Context) ([]*domain.PaymentRequest, error) {
	var requests []*domain.PaymentRequest
	query := `
		SELECT * FROM payment_requests 
		WHERE status = 'link_active'
		AND link_expires_at IS NOT NULL
		AND link_expires_at <= NOW()
		ORDER BY link_expires_at ASC
		LIMIT 100`

	err := r.db.SelectContext(ctx, &requests, query)
	if err != nil {
		return nil, err
	}

	return requests, nil
}

func (r *PaymentRequestRepo) GetOverdueProcessingOrders(ctx context.Context, reminderCooldownMins int) ([]*domain.PaymentRequest, error) {
	var requests []*domain.PaymentRequest
	query := `
		SELECT * FROM payment_requests
		WHERE status = 'paid_awaiting_fulfillment'
		AND paid_at IS NOT NULL
		AND processing_time_hours IS NOT NULL
		AND paid_at + (processing_time_hours * INTERVAL '1 hour') <= NOW()
		AND (
			last_reminder_sent_at IS NULL
			OR last_reminder_sent_at <= NOW() - INTERVAL '1 minute' * $1
		)
		ORDER BY paid_at ASC
		LIMIT 100`

	err := r.db.SelectContext(ctx, &requests, query, reminderCooldownMins)
	if err != nil {
		return nil, err
	}

	return requests, nil
}

func (r *PaymentRequestRepo) UpdateReminderSent(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE payment_requests
		SET last_reminder_sent_at = NOW(),
			reminder_count = COALESCE(reminder_count, 0) + 1,
			updated_at = NOW()
		WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrPaymentRequestNotFound
	}

	return nil
}

func (r *PaymentRequestRepo) Update(ctx context.Context, pr *domain.PaymentRequest) error {
	query := `
		UPDATE payment_requests SET
			item_name = $2,
			item_description = $3,
			item_price = $4,
			item_images = $5,
			processing_time_hours = $6,
			delivery_type = $7,
			vehicle_type = $8,
			pickup_address = $9,
			pickup_location = ST_SetSRID(ST_MakePoint($10, $11), 4326)::geography,
			dropoff_address = $12,
			dropoff_location = ST_SetSRID(ST_MakePoint($13, $14), 4326)::geography,
			estimated_delivery_fee = $15,
			buyer_service_fee = $16,
			seller_platform_fee = $17,
			estimated_total = $18,
			actual_delivery_fee = $19,
			actual_total = $20,
			payment_code = $21,
			link_expires_at = $22,
			logistics_carrier = $23,
			tracking_number = $24,
			tracking_url = $25,
			status = $26,
			link_generated_at = $27,
			paid_at = $28,
			ready_for_pickup_at = $29,
			delivered_at = $30,
			completed_at = $31,
			updated_at = NOW()
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query,
		pr.ID,
		pr.ItemName,
		pr.ItemDescription,
		pr.ItemPrice,
		pr.ItemImages,
		pr.ProcessingTimeHours,
		pr.DeliveryType,
		pr.VehicleType,
		pr.PickupAddress,
		0.0, 0.0, // Placeholder for lat/lng - actual values should be extracted
		pr.DropoffAddress,
		0.0, 0.0,
		pr.EstimatedDeliveryFee,
		pr.BuyerServiceFee,
		pr.SellerPlatformFee,
		pr.EstimatedTotal,
		pr.ActualDeliveryFee,
		pr.ActualTotal,
		pr.PaymentCode,
		pr.LinkExpiresAt,
		pr.LogisticsCarrier,
		pr.TrackingNumber,
		pr.TrackingURL,
		pr.Status,
		pr.LinkGeneratedAt,
		pr.PaidAt,
		pr.ReadyForPickupAt,
		pr.DeliveredAt,
		pr.CompletedAt,
	)

	return err
}

func (r *PaymentRequestRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.PaymentRequestStatus) error {
	query := `UPDATE payment_requests SET status = $2 WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, id, status)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrPaymentRequestNotFound
	}

	return nil
}

func (r *PaymentRequestRepo) SetBuyer(ctx context.Context, id uuid.UUID, buyerID uuid.UUID) error {
	query := `UPDATE payment_requests SET buyer_id = $2 WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, buyerID)
	return err
}

func (r *PaymentRequestRepo) SetEscrowID(ctx context.Context, id uuid.UUID, escrowID uuid.UUID) error {
	query := `UPDATE payment_requests SET escrow_id = $2 WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, escrowID)
	return err
}

func (r *PaymentRequestRepo) SetDeliveryJobID(ctx context.Context, id uuid.UUID, jobID uuid.UUID) error {
	query := `UPDATE payment_requests SET delivery_job_id = $2 WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, jobID)
	return err
}

func (r *PaymentRequestRepo) PaymentCodeExists(ctx context.Context, code string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM payment_requests WHERE payment_code = $1)`
	err := r.db.GetContext(ctx, &exists, query, code)
	return exists, err
}

func (r *PaymentRequestRepo) IncrementCurrentUses(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE payment_requests SET current_uses = current_uses + 1, updated_at = NOW() WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrPaymentRequestNotFound
	}

	return nil
}

func (r *PaymentRequestRepo) GetAll(ctx context.Context, limit, offset int) ([]*domain.PaymentRequest, error) {
	var requests []*domain.PaymentRequest
	query := `SELECT * FROM payment_requests ORDER BY created_at DESC LIMIT $1 OFFSET $2`

	err := r.db.SelectContext(ctx, &requests, query, limit, offset)
	if err != nil {
		return nil, err
	}

	return requests, nil
}

type PaymentRequestEventRepo struct {

	db *sqlx.DB
}

func NewPaymentRequestEventRepo(db *sqlx.DB) repository.PaymentRequestEventRepository {
	return &PaymentRequestEventRepo{db: db}
}

func (r *PaymentRequestEventRepo) Create(ctx context.Context, event *domain.PaymentRequestEvent) error {
	query := `
		INSERT INTO payment_request_events (id, payment_request_id, from_status, to_status, actor_id, actor_type, reason, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at`

	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}

	return r.db.QueryRowxContext(ctx, query,
		event.ID,
		event.PaymentRequestID,
		event.FromStatus,
		event.ToStatus,
		event.ActorID,
		event.ActorType,
		event.Reason,
		event.Metadata,
	).Scan(&event.CreatedAt)
}

func (r *PaymentRequestEventRepo) GetByPaymentRequestID(ctx context.Context, prID uuid.UUID) ([]*domain.PaymentRequestEvent, error) {
	var events []*domain.PaymentRequestEvent
	query := `SELECT * FROM payment_request_events WHERE payment_request_id = $1 ORDER BY created_at ASC`

	err := r.db.SelectContext(ctx, &events, query, prID)
	if err != nil {
		return nil, err
	}

	return events, nil
}
