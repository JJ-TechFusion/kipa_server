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
			processing_time_hours, delivery_type, status, is_reusable, max_uses, current_uses,
			pickup_address, pickup_state, pickup_location,
			dropoff_address, dropoff_state, dropoff_location
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, ST_SetSRID(ST_MakePoint($15, $16), 4326)::geography, $17, $18, ST_SetSRID(ST_MakePoint($19, $20), 4326)::geography)
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
		pr.PickupAddress,              // $13
		pr.PickupState,                // $14
		pr.PickupLng, pr.PickupLat,    // $15, $16
		pr.DropoffAddress,             // $17
		pr.DropoffState,               // $18
		pr.DropoffLng, pr.DropoffLat,  // $19, $20
	).Scan(&pr.CreatedAt, &pr.UpdatedAt)
}

func (r *PaymentRequestRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.PaymentRequest, error) {
	var pr domain.PaymentRequest
	query := `
		SELECT 
			*,
			COALESCE(ST_Y(pickup_location::geometry), 0) as pickup_lat,
			COALESCE(ST_X(pickup_location::geometry), 0) as pickup_lng,
			COALESCE(ST_Y(dropoff_location::geometry), 0) as dropoff_lat,
			COALESCE(ST_X(dropoff_location::geometry), 0) as dropoff_lng
		FROM payment_requests 
		WHERE id = $1`

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
	query := `
		SELECT 
			*,
			COALESCE(ST_Y(pickup_location::geometry), 0) as pickup_lat,
			COALESCE(ST_X(pickup_location::geometry), 0) as pickup_lng,
			COALESCE(ST_Y(dropoff_location::geometry), 0) as dropoff_lat,
			COALESCE(ST_X(dropoff_location::geometry), 0) as dropoff_lng
		FROM payment_requests 
		WHERE payment_code = $1`

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
	query := `
		SELECT
			*,
			COALESCE(ST_Y(pickup_location::geometry), 0) as pickup_lat,
			COALESCE(ST_X(pickup_location::geometry), 0) as pickup_lng,
			COALESCE(ST_Y(dropoff_location::geometry), 0) as dropoff_lat,
			COALESCE(ST_X(dropoff_location::geometry), 0) as dropoff_lng
		FROM payment_requests
		WHERE seller_id = $1 AND status != 'cancelled'
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`

	err := r.db.SelectContext(ctx, &requests, query, sellerID, limit, offset)
	if err != nil {
		return nil, err
	}

	return requests, nil
}

func (r *PaymentRequestRepo) GetByBuyerID(ctx context.Context, buyerID uuid.UUID, limit, offset int) ([]*domain.PaymentRequest, error) {
	var requests []*domain.PaymentRequest
	query := `
		SELECT
			*,
			COALESCE(ST_Y(pickup_location::geometry), 0) as pickup_lat,
			COALESCE(ST_X(pickup_location::geometry), 0) as pickup_lng,
			COALESCE(ST_Y(dropoff_location::geometry), 0) as dropoff_lat,
			COALESCE(ST_X(dropoff_location::geometry), 0) as dropoff_lng
		FROM payment_requests
		WHERE buyer_id = $1 AND status != 'cancelled'
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`

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
			pickup_state = $10,
			pickup_location = ST_SetSRID(ST_MakePoint($11, $12), 4326)::geography,
			dropoff_address = $13,
			dropoff_state = $14,
			dropoff_location = ST_SetSRID(ST_MakePoint($15, $16), 4326)::geography,
			estimated_delivery_fee = $17,
			buyer_service_fee = $18,
			seller_platform_fee = $19,
			estimated_total = $20,
			actual_delivery_fee = $21,
			actual_total = $22,
			payment_code = $23,
			link_expires_at = $24,
			logistics_carrier = $25,
			tracking_number = $26,
			tracking_url = $27,
			status = $28,
			link_generated_at = $29,
			paid_at = $30,
			ready_for_pickup_at = $31,
			delivered_at = $32,
			completed_at = $33,
			forced_return_job_id = $34,
			rebook_deadline = $35,
			rebook_attempts = $36,
			unavailability_fee = $37,
			buyer_id = $38,
			escrow_id = $39,
			logistics_delivery_id = $40,
			delivery_job_id = $41,
			return_delivery_job_id = $42,
			dispute_outcome = $43,
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
		pr.PickupState,               // $10
		pr.PickupLng, pr.PickupLat,   // $11, $12
		pr.DropoffAddress,
		pr.DropoffState,              // $14
		pr.DropoffLng, pr.DropoffLat, // $15, $16
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
		pr.ForcedReturnJobID,
		pr.RebookDeadline,
		pr.RebookAttempts,
		pr.UnavailabilityFee,
		pr.BuyerID,
		pr.EscrowID,
		pr.LogisticsDeliveryID,
		pr.DeliveryJobID,
		pr.ReturnDeliveryJobID,
		pr.DisputeOutcome,
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
func (r *PaymentRequestRepo) GetExpiredRebookRequests(ctx context.Context) ([]*domain.PaymentRequest, error) {
	var requests []*domain.PaymentRequest
	query := `
		SELECT * FROM payment_requests
		WHERE status = 'pending_rebook'
		AND rebook_deadline IS NOT NULL
		AND rebook_deadline < NOW()
		ORDER BY rebook_deadline ASC
		LIMIT 100`

	err := r.db.SelectContext(ctx, &requests, query)
	if err != nil {
		return nil, err
	}

	return requests, nil
}
func (r *PaymentRequestRepo) SetUnavailabilityFee(ctx context.Context, id uuid.UUID, fee float64) error {
	query := `UPDATE payment_requests SET unavailability_fee = $1, updated_at = NOW() WHERE id = $2`
	result, err := r.db.ExecContext(ctx, query, fee, id)
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
