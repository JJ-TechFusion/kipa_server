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

type LogisticsDeliveryRepo struct {
	db *sqlx.DB
}

func NewLogisticsDeliveryRepo(db *sqlx.DB) repository.LogisticsDeliveryRepository {
	return &LogisticsDeliveryRepo{db: db}
}

func (r *LogisticsDeliveryRepo) Create(ctx context.Context, delivery *domain.LogisticsDelivery) error {
	query := `
		INSERT INTO logistics_deliveries (
			id, payment_request_id, escrow_id, seller_id, buyer_id,
			carrier, carrier_other_name, tracking_number, tracking_url,
			shipment_receipt_url, estimated_delivery_at,
			pickup_address, pickup_state, dropoff_address, dropoff_state,
			status, ship_deadline, requires_admin_approval
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18
		)
		RETURNING created_at, updated_at`

	if delivery.ID == uuid.Nil {
		delivery.ID = uuid.New()
	}

	return r.db.QueryRowxContext(ctx, query,
		delivery.ID,
		delivery.PaymentRequestID,
		delivery.EscrowID,
		delivery.SellerID,
		delivery.BuyerID,
		delivery.Carrier,
		delivery.CarrierOtherName,
		delivery.TrackingNumber,
		delivery.TrackingURL,
		delivery.ShipmentReceiptURL,
		delivery.EstimatedDeliveryAt,
		delivery.PickupAddress,
		delivery.PickupState,
		delivery.DropoffAddress,
		delivery.DropoffState,
		delivery.Status,
		delivery.ShipDeadline,
		delivery.RequiresAdminApproval,
	).Scan(&delivery.CreatedAt, &delivery.UpdatedAt)
}

func (r *LogisticsDeliveryRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.LogisticsDelivery, error) {
	var delivery domain.LogisticsDelivery
	query := `SELECT * FROM logistics_deliveries WHERE id = $1`

	err := r.db.GetContext(ctx, &delivery, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrLogisticsDeliveryNotFound
		}
		return nil, err
	}

	return &delivery, nil
}

func (r *LogisticsDeliveryRepo) GetByPaymentRequestID(ctx context.Context, prID uuid.UUID) (*domain.LogisticsDelivery, error) {
	var delivery domain.LogisticsDelivery
	query := `SELECT * FROM logistics_deliveries WHERE payment_request_id = $1`

	err := r.db.GetContext(ctx, &delivery, query, prID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrLogisticsDeliveryNotFound
		}
		return nil, err
	}

	return &delivery, nil
}

func (r *LogisticsDeliveryRepo) GetByEscrowID(ctx context.Context, escrowID uuid.UUID) (*domain.LogisticsDelivery, error) {
	var delivery domain.LogisticsDelivery
	query := `SELECT * FROM logistics_deliveries WHERE escrow_id = $1`

	err := r.db.GetContext(ctx, &delivery, query, escrowID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrLogisticsDeliveryNotFound
		}
		return nil, err
	}

	return &delivery, nil
}

func (r *LogisticsDeliveryRepo) GetBySellerID(ctx context.Context, sellerID uuid.UUID, status *domain.LogisticsDeliveryStatus, limit, offset int) ([]*domain.LogisticsDelivery, int, error) {
	var deliveries []*domain.LogisticsDelivery
	var total int

	baseQuery := `FROM logistics_deliveries WHERE seller_id = $1`
	args := []interface{}{sellerID}
	argIndex := 2

	if status != nil {
		baseQuery += ` AND status = $` + string(rune('0'+argIndex))
		args = append(args, *status)
		argIndex++
	}
	countQuery := `SELECT COUNT(*) ` + baseQuery
	err := r.db.GetContext(ctx, &total, countQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	dataQuery := `SELECT * ` + baseQuery + ` ORDER BY created_at DESC LIMIT $` + string(rune('0'+argIndex)) + ` OFFSET $` + string(rune('0'+argIndex+1))
	args = append(args, limit, offset)

	err = r.db.SelectContext(ctx, &deliveries, dataQuery, args...)
	if err != nil {
		return nil, 0, err
	}

	return deliveries, total, nil
}

func (r *LogisticsDeliveryRepo) GetByBuyerID(ctx context.Context, buyerID uuid.UUID, status *domain.LogisticsDeliveryStatus, limit, offset int) ([]*domain.LogisticsDelivery, int, error) {
	var deliveries []*domain.LogisticsDelivery
	var total int

	baseQuery := `FROM logistics_deliveries WHERE buyer_id = $1`
	args := []interface{}{buyerID}
	argIndex := 2

	if status != nil {
		baseQuery += ` AND status = $` + string(rune('0'+argIndex))
		args = append(args, *status)
		argIndex++
	}
	countQuery := `SELECT COUNT(*) ` + baseQuery
	err := r.db.GetContext(ctx, &total, countQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	dataQuery := `SELECT * ` + baseQuery + ` ORDER BY created_at DESC LIMIT $` + string(rune('0'+argIndex)) + ` OFFSET $` + string(rune('0'+argIndex+1))
	args = append(args, limit, offset)

	err = r.db.SelectContext(ctx, &deliveries, dataQuery, args...)
	if err != nil {
		return nil, 0, err
	}

	return deliveries, total, nil
}

func (r *LogisticsDeliveryRepo) GetByStatus(ctx context.Context, status domain.LogisticsDeliveryStatus, limit, offset int) ([]*domain.LogisticsDelivery, int, error) {
	var deliveries []*domain.LogisticsDelivery
	var total int

	countQuery := `SELECT COUNT(*) FROM logistics_deliveries WHERE status = $1`
	err := r.db.GetContext(ctx, &total, countQuery, status)
	if err != nil {
		return nil, 0, err
	}

	dataQuery := `SELECT * FROM logistics_deliveries WHERE status = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	err = r.db.SelectContext(ctx, &deliveries, dataQuery, status, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	return deliveries, total, nil
}

func (r *LogisticsDeliveryRepo) Update(ctx context.Context, delivery *domain.LogisticsDelivery) error {
	query := `
		UPDATE logistics_deliveries SET
			carrier = $2,
			carrier_other_name = $3,
			tracking_number = $4,
			tracking_url = $5,
			shipment_receipt_url = $6,
			estimated_delivery_at = $7,
			status = $8,
			confirmation_deadline = $9,
			dispute_auto_escalation_at = $10,
			return_tracking_number = $11,
			return_carrier = $12,
			return_cost_buyer_share = $13,
			return_cost_seller_share = $14,
			return_condition = $15,
			damage_claim_reason = $16,
			return_received_at = $17,
			dispute_id = $18,
			dispute_outcome = $19,
			last_reminder_sent_at = $20,
			reminder_count = $21,
			shipped_at = $22,
			delivery_claimed_at = $23,
			confirmed_at = $24,
			released_at = $25,
			cancelled_at = $26
		WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query,
		delivery.ID,
		delivery.Carrier,
		delivery.CarrierOtherName,
		delivery.TrackingNumber,
		delivery.TrackingURL,
		delivery.ShipmentReceiptURL,
		delivery.EstimatedDeliveryAt,
		delivery.Status,
		delivery.ConfirmationDeadline,
		delivery.DisputeAutoEscalationAt,
		delivery.ReturnTrackingNumber,
		delivery.ReturnCarrier,
		delivery.ReturnCostBuyerShare,
		delivery.ReturnCostSellerShare,
		delivery.ReturnCondition,
		delivery.DamageClaimReason,
		delivery.ReturnReceivedAt,
		delivery.DisputeID,
		delivery.DisputeOutcome,
		delivery.LastReminderSentAt,
		delivery.ReminderCount,
		delivery.ShippedAt,
		delivery.DeliveryClaimedAt,
		delivery.ConfirmedAt,
		delivery.ReleasedAt,
		delivery.CancelledAt,
	)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrLogisticsDeliveryNotFound
	}

	return nil
}

func (r *LogisticsDeliveryRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.LogisticsDeliveryStatus) error {
	query := `UPDATE logistics_deliveries SET status = $2 WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, id, status)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrLogisticsDeliveryNotFound
	}

	return nil
}

func (r *LogisticsDeliveryRepo) GetExceededShipDeadlines(ctx context.Context) ([]*domain.LogisticsDelivery, error) {
	var deliveries []*domain.LogisticsDelivery
	query := `
		SELECT * FROM logistics_deliveries
		WHERE status = 'awaiting_shipment'
		AND ship_deadline <= $1
		ORDER BY ship_deadline ASC`

	err := r.db.SelectContext(ctx, &deliveries, query, time.Now())
	if err != nil {
		return nil, err
	}

	return deliveries, nil
}

func (r *LogisticsDeliveryRepo) GetPendingConfirmationReminders(ctx context.Context, reminderIntervalHours int) ([]*domain.LogisticsDelivery, error) {
	var deliveries []*domain.LogisticsDelivery
	query := `
		SELECT * FROM logistics_deliveries
		WHERE status = 'delivery_claimed'
		AND (
			last_reminder_sent_at IS NULL
			OR last_reminder_sent_at <= $1
		)
		ORDER BY delivery_claimed_at ASC`

	reminderThreshold := time.Now().Add(-time.Duration(reminderIntervalHours) * time.Hour)
	err := r.db.SelectContext(ctx, &deliveries, query, reminderThreshold)
	if err != nil {
		return nil, err
	}

	return deliveries, nil
}

func (r *LogisticsDeliveryRepo) GetReadyForAutoEscalation(ctx context.Context) ([]*domain.LogisticsDelivery, error) {
	var deliveries []*domain.LogisticsDelivery
	query := `
		SELECT * FROM logistics_deliveries
		WHERE status = 'delivery_claimed'
		AND dispute_auto_escalation_at IS NOT NULL
		AND dispute_auto_escalation_at <= $1
		ORDER BY dispute_auto_escalation_at ASC`

	err := r.db.SelectContext(ctx, &deliveries, query, time.Now())
	if err != nil {
		return nil, err
	}

	return deliveries, nil
}

func (r *LogisticsDeliveryRepo) GetReadyForReturnAutoEscalation(ctx context.Context) ([]*domain.LogisticsDelivery, error) {
	var deliveries []*domain.LogisticsDelivery
	query := `
		SELECT * FROM logistics_deliveries
		WHERE status = 'return_delivery_claimed'
		AND return_auto_escalation_at IS NOT NULL
		AND return_auto_escalation_at <= $1
		ORDER BY return_auto_escalation_at ASC`

	err := r.db.SelectContext(ctx, &deliveries, query, time.Now())
	if err != nil {
		return nil, err
	}

	return deliveries, nil
}

func (r *LogisticsDeliveryRepo) GetPendingAdminApproval(ctx context.Context, limit, offset int) ([]*domain.LogisticsDelivery, int, error) {
	var deliveries []*domain.LogisticsDelivery
	var total int

	countQuery := `SELECT COUNT(*) FROM logistics_deliveries WHERE status = 'pending_admin_approval'`
	err := r.db.GetContext(ctx, &total, countQuery)
	if err != nil {
		return nil, 0, err
	}

	query := `
		SELECT * FROM logistics_deliveries
		WHERE status = 'pending_admin_approval'
		ORDER BY confirmed_at ASC
		LIMIT $1 OFFSET $2`

	err = r.db.SelectContext(ctx, &deliveries, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	return deliveries, total, nil
}

func (r *LogisticsDeliveryRepo) SetShipped(ctx context.Context, id uuid.UUID, carrier *domain.LogisticsCarrier, carrierOtherName, trackingNumber *string, trackingURL, shipmentReceiptURL *string, estimatedDeliveryAt *time.Time) error {
	query := `
		UPDATE logistics_deliveries SET
			carrier = $2,
			carrier_other_name = $3,
			tracking_number = $4,
			tracking_url = $5,
			shipment_receipt_url = $6,
			estimated_delivery_at = $7,
			status = 'shipped',
			shipped_at = $8
		WHERE id = $1 AND status = 'awaiting_shipment'`

	result, err := r.db.ExecContext(ctx, query, id, carrier, carrierOtherName, trackingNumber, trackingURL, shipmentReceiptURL, estimatedDeliveryAt, time.Now())
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrCannotShipYet
	}

	return nil
}

func (r *LogisticsDeliveryRepo) SetDeliveryClaimed(ctx context.Context, id uuid.UUID, confirmationDeadline, autoEscalationAt time.Time) error {
	query := `
		UPDATE logistics_deliveries SET
			status = 'delivery_claimed',
			delivery_claimed_at = $2,
			confirmation_deadline = $3,
			dispute_auto_escalation_at = $4
		WHERE id = $1 AND status = 'shipped'`

	result, err := r.db.ExecContext(ctx, query, id, time.Now(), confirmationDeadline, autoEscalationAt)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrCannotClaimDelivery
	}

	return nil
}

func (r *LogisticsDeliveryRepo) SetConfirmed(ctx context.Context, id uuid.UUID, requiresAdminApproval bool) error {
	var newStatus domain.LogisticsDeliveryStatus
	if requiresAdminApproval {
		newStatus = domain.LDStatusPendingAdminApproval
	} else {
		newStatus = domain.LDStatusConfirmed
	}

	query := `
		UPDATE logistics_deliveries SET
			status = $2,
			confirmed_at = $3
		WHERE id = $1 AND status = 'delivery_claimed'`

	result, err := r.db.ExecContext(ctx, query, id, newStatus, time.Now())
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrCannotConfirmDelivery
	}

	return nil
}

func (r *LogisticsDeliveryRepo) SetDispute(ctx context.Context, id uuid.UUID, disputeID uuid.UUID) error {
	query := `
		UPDATE logistics_deliveries SET
			status = 'disputed',
			dispute_id = $2
		WHERE id = $1 AND status IN ('shipped', 'delivery_claimed', 'pending_admin_approval')`

	result, err := r.db.ExecContext(ctx, query, id, disputeID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrCannotOpenDisputeInState
	}

	return nil
}

func (r *LogisticsDeliveryRepo) SetReturnInfo(ctx context.Context, id uuid.UUID, buyerShare, sellerShare float64) error {
	query := `
		UPDATE logistics_deliveries SET
			status = 'return_required',
			return_cost_buyer_share = $2,
			return_cost_seller_share = $3
		WHERE id = $1 AND status = 'disputed'`

	result, err := r.db.ExecContext(ctx, query, id, buyerShare, sellerShare)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrLogisticsDeliveryNotFound
	}

	return nil
}

func (r *LogisticsDeliveryRepo) SetReturnShipped(ctx context.Context, id uuid.UUID, carrier, trackingNumber string) error {
	query := `
		UPDATE logistics_deliveries SET
			status = 'return_shipped',
			return_carrier = $2,
			return_tracking_number = $3
		WHERE id = $1 AND status = 'return_required'`

	result, err := r.db.ExecContext(ctx, query, id, carrier, trackingNumber)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrLogisticsDeliveryNotFound
	}

	return nil
}

func (r *LogisticsDeliveryRepo) UpdateReminderSent(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE logistics_deliveries SET
			last_reminder_sent_at = $2,
			reminder_count = reminder_count + 1
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query, id, time.Now())
	return err
}

var _ repository.LogisticsDeliveryRepository = (*LogisticsDeliveryRepo)(nil)

type LogisticsDeliveryEventRepo struct {
	db *sqlx.DB
}

func NewLogisticsDeliveryEventRepo(db *sqlx.DB) repository.LogisticsDeliveryEventRepository {
	return &LogisticsDeliveryEventRepo{db: db}
}

func (r *LogisticsDeliveryEventRepo) Create(ctx context.Context, event *domain.LogisticsDeliveryEvent) error {
	query := `
		INSERT INTO logistics_delivery_events (
			id, logistics_delivery_id, from_status, to_status,
			actor_id, actor_type, reason, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at`

	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}

	return r.db.QueryRowxContext(ctx, query,
		event.ID,
		event.LogisticsDeliveryID,
		event.FromStatus,
		event.ToStatus,
		event.ActorID,
		event.ActorType,
		event.Reason,
		event.Metadata,
	).Scan(&event.CreatedAt)
}

func (r *LogisticsDeliveryEventRepo) GetByDeliveryID(ctx context.Context, deliveryID uuid.UUID) ([]*domain.LogisticsDeliveryEvent, error) {
	var events []*domain.LogisticsDeliveryEvent
	query := `
		SELECT * FROM logistics_delivery_events
		WHERE logistics_delivery_id = $1
		ORDER BY created_at ASC`

	err := r.db.SelectContext(ctx, &events, query, deliveryID)
	if err != nil {
		return nil, err
	}

	return events, nil
}

var _ repository.LogisticsDeliveryEventRepository = (*LogisticsDeliveryEventRepo)(nil)

type LogisticsEvidenceRepo struct {
	db *sqlx.DB
}

func NewLogisticsEvidenceRepo(db *sqlx.DB) repository.LogisticsEvidenceRepository {
	return &LogisticsEvidenceRepo{db: db}
}

func (r *LogisticsEvidenceRepo) Create(ctx context.Context, evidence *domain.LogisticsEvidence) error {
	query := `
		INSERT INTO logistics_evidence (
			id, logistics_delivery_id, uploaded_by, uploader_type,
			evidence_type, file_url, description
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING uploaded_at`

	if evidence.ID == uuid.Nil {
		evidence.ID = uuid.New()
	}

	return r.db.QueryRowxContext(ctx, query,
		evidence.ID,
		evidence.LogisticsDeliveryID,
		evidence.UploadedBy,
		evidence.UploaderType,
		evidence.EvidenceType,
		evidence.FileURL,
		evidence.Description,
	).Scan(&evidence.UploadedAt)
}

func (r *LogisticsEvidenceRepo) GetByDeliveryID(ctx context.Context, deliveryID uuid.UUID) ([]*domain.LogisticsEvidence, error) {
	var evidence []*domain.LogisticsEvidence
	query := `
		SELECT * FROM logistics_evidence
		WHERE logistics_delivery_id = $1
		ORDER BY uploaded_at ASC`

	err := r.db.SelectContext(ctx, &evidence, query, deliveryID)
	if err != nil {
		return nil, err
	}

	return evidence, nil
}

func (r *LogisticsEvidenceRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.LogisticsEvidence, error) {
	var evidence domain.LogisticsEvidence
	query := `SELECT * FROM logistics_evidence WHERE id = $1`

	err := r.db.GetContext(ctx, &evidence, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &evidence, nil
}

var _ repository.LogisticsEvidenceRepository = (*LogisticsEvidenceRepo)(nil)
