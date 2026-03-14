package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"kipa/internal/domain"
	"kipa/internal/repository"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)
type DeliveryJobRepo struct {
	db *sqlx.DB
}

func NewDeliveryJobRepo(db *sqlx.DB) repository.DeliveryJobRepository {
	return &DeliveryJobRepo{db: db}
}

func (r *DeliveryJobRepo) Create(ctx context.Context, job *domain.DeliveryJob) error {
	var paymentRequestID *uuid.UUID
	if job.PaymentRequestID != uuid.Nil {
		paymentRequestID = &job.PaymentRequestID
	}

	query := `
		INSERT INTO delivery_jobs (
			id, payment_request_id, errand_id, source_type,
			pickup_address, pickup_location, dropoff_address, dropoff_location,
			distance_km, current_eta_minutes, delivery_fee, rider_payout, platform_delivery_fee,
			required_vehicle_type, status, search_radius_km, search_started_at, search_timeout_at,
			pickup_code, dropoff_code, is_forced_return, rider_id,
			is_return, original_job_id, dispute_id
		) VALUES (
			$1, $2, $3, $4,
			$5, ST_SetSRID(ST_MakePoint($6, $7), 4326)::geography,
			$8, ST_SetSRID(ST_MakePoint($9, $10), 4326)::geography,
			$11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24,
			$25, $26, $27
		)
		RETURNING created_at, updated_at`

	if job.ID == uuid.Nil {
		job.ID = uuid.New()
	}

	return r.db.QueryRowxContext(ctx, query,
		job.ID,
		paymentRequestID,
		job.ErrandID,
		job.SourceType,
		job.PickupAddress,
		job.PickupLng, job.PickupLat,
		job.DropoffAddress,
		job.DropoffLng, job.DropoffLat,
		job.DistanceKM,
		job.CurrentETAMinutes,
		job.DeliveryFee,
		job.RiderPayout,
		job.PlatformDeliveryFee,
		job.RequiredVehicleType,
		job.Status,
		job.SearchRadiusKM,
		job.SearchStartedAt,
		job.SearchTimeoutAt,
		job.PickupCode,
		job.DropoffCode,
		job.IsForcedReturn,
		job.RiderID,
		job.IsReturn,
		job.OriginalJobID,
		job.DisputeID,
	).Scan(&job.CreatedAt, &job.UpdatedAt)
}

func (r *DeliveryJobRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.DeliveryJob, error) {
	var job domain.DeliveryJob
	query := `
		SELECT *,
			COALESCE(ST_Y(pickup_location::geometry), 0) as pickup_lat,
			COALESCE(ST_X(pickup_location::geometry), 0) as pickup_lng,
			COALESCE(ST_Y(dropoff_location::geometry), 0) as dropoff_lat,
			COALESCE(ST_X(dropoff_location::geometry), 0) as dropoff_lng
		FROM delivery_jobs WHERE id = $1`

	err := r.db.GetContext(ctx, &job, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrDeliveryJobNotFound
		}
		return nil, err
	}

	return &job, nil
}

func (r *DeliveryJobRepo) GetByPaymentRequestID(ctx context.Context, prID uuid.UUID) (*domain.DeliveryJob, error) {
	var job domain.DeliveryJob
	query := `
		SELECT *,
			COALESCE(ST_Y(pickup_location::geometry), 0) as pickup_lat,
			COALESCE(ST_X(pickup_location::geometry), 0) as pickup_lng,
			COALESCE(ST_Y(dropoff_location::geometry), 0) as dropoff_lat,
			COALESCE(ST_X(dropoff_location::geometry), 0) as dropoff_lng
		FROM delivery_jobs WHERE payment_request_id = $1`

	err := r.db.GetContext(ctx, &job, query, prID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrDeliveryJobNotFound
		}
		return nil, err
	}

	return &job, nil
}

func (r *DeliveryJobRepo) GetByRiderID(ctx context.Context, riderID uuid.UUID, statuses []domain.DeliveryJobStatus, limit, offset int) ([]*domain.DeliveryJob, error) {
	var jobs []*domain.DeliveryJob

	query := `SELECT * FROM delivery_jobs WHERE rider_id = $1`
	args := []interface{}{riderID}

	if len(statuses) > 0 {
		statusStrings := make([]string, len(statuses))
		for i, s := range statuses {
			statusStrings[i] = string(s)
		}
		query += ` AND status = ANY($2)`
		args = append(args, pq.Array(statusStrings))
	}
	query += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2)
	args = append(args, limit, offset)

	err := r.db.SelectContext(ctx, &jobs, query, args...)
	if err != nil {
		return nil, err
	}

	return jobs, nil
}

func (r *DeliveryJobRepo) GetActiveByRiderID(ctx context.Context, riderID uuid.UUID) (*domain.DeliveryJob, error) {
	var job domain.DeliveryJob
	query := `
		SELECT *,
			COALESCE(ST_Y(pickup_location::geometry), 0) as pickup_lat,
			COALESCE(ST_X(pickup_location::geometry), 0) as pickup_lng,
			COALESCE(ST_Y(dropoff_location::geometry), 0) as dropoff_lat,
			COALESCE(ST_X(dropoff_location::geometry), 0) as dropoff_lng
		FROM delivery_jobs
		WHERE rider_id = $1
		AND status IN ('accepted', 'en_route_pickup', 'picked_up', 'in_transit', 'buyer_unavailable', 'forced_return')
		ORDER BY created_at DESC
		LIMIT 1`

	err := r.db.GetContext(ctx, &job, query, riderID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &job, nil
}

func (r *DeliveryJobRepo) GetSearchingJobs(ctx context.Context, limit int) ([]*domain.DeliveryJob, error) {
	var jobs []*domain.DeliveryJob
	query := `
		SELECT *,
			COALESCE(ST_Y(pickup_location::geometry), 0) as pickup_lat,
			COALESCE(ST_X(pickup_location::geometry), 0) as pickup_lng,
			COALESCE(ST_Y(dropoff_location::geometry), 0) as dropoff_lat,
			COALESCE(ST_X(dropoff_location::geometry), 0) as dropoff_lng
		FROM delivery_jobs
		WHERE status = 'searching'
		AND (search_timeout_at IS NULL OR search_timeout_at > NOW())
		ORDER BY created_at ASC
		LIMIT $1`

	err := r.db.SelectContext(ctx, &jobs, query, limit)
	if err != nil {
		return nil, err
	}

	return jobs, nil
}

func (r *DeliveryJobRepo) GetSearchingJobsByVehicleType(ctx context.Context, vehicleType domain.VehicleType, limit int) ([]*domain.DeliveryJob, error) {
	var jobs []*domain.DeliveryJob
	query := `
		SELECT *,
			COALESCE(ST_Y(pickup_location::geometry), 0) as pickup_lat,
			COALESCE(ST_X(pickup_location::geometry), 0) as pickup_lng,
			COALESCE(ST_Y(dropoff_location::geometry), 0) as dropoff_lat,
			COALESCE(ST_X(dropoff_location::geometry), 0) as dropoff_lng
		FROM delivery_jobs
		WHERE status = 'searching'
		AND required_vehicle_type = $1
		AND (search_timeout_at IS NULL OR search_timeout_at > NOW())
		ORDER BY created_at ASC
		LIMIT $2`

	err := r.db.SelectContext(ctx, &jobs, query, vehicleType, limit)
	if err != nil {
		return nil, err
	}

	return jobs, nil
}

func (r *DeliveryJobRepo) GetTimedOutJobs(ctx context.Context) ([]*domain.DeliveryJob, error) {
	var jobs []*domain.DeliveryJob
	query := `
		SELECT * FROM delivery_jobs
		WHERE status = 'searching'
		AND search_timeout_at IS NOT NULL
		AND search_timeout_at <= NOW()
		ORDER BY search_timeout_at ASC
		LIMIT 100`

	err := r.db.SelectContext(ctx, &jobs, query)
	if err != nil {
		return nil, err
	}

	return jobs, nil
}

func (r *DeliveryJobRepo) GetJobsForRadiusExpansion(ctx context.Context, expandAfterMins int) ([]*domain.DeliveryJob, error) {
	var jobs []*domain.DeliveryJob
	query := `
		SELECT * FROM delivery_jobs
		WHERE status = 'searching'
		AND search_timeout_at > NOW()
		AND (
			last_radius_expand_at IS NULL 
			OR last_radius_expand_at <= NOW() - INTERVAL '1 minute' * $1
		)
		ORDER BY created_at ASC
		LIMIT 50`

	err := r.db.SelectContext(ctx, &jobs, query, expandAfterMins)
	if err != nil {
		return nil, err
	}

	return jobs, nil
}

func (r *DeliveryJobRepo) UpdateSearchRadius(ctx context.Context, id uuid.UUID, radiusKM int) error {
	query := `
		UPDATE delivery_jobs 
		SET search_radius_km = $2, last_radius_expand_at = NOW()
		WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query, id, radiusKM)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrDeliveryJobNotFound
	}

	return nil
}

func (r *DeliveryJobRepo) Update(ctx context.Context, job *domain.DeliveryJob) error {
	query := `
		UPDATE delivery_jobs SET
			rider_id = $2,
			delivery_fee = $3,
			rider_payout = $4,
			platform_delivery_fee = $5,
			pickup_code = $6,
			dropoff_code = $7,
			status = $8,
			accepted_at = $9,
			picked_up_at = $10,
			delivered_at = $11,
			dropoff_address = $12,
			dropoff_location = ST_SetSRID(ST_MakePoint($13, $14), 4326)::geography
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query,
		job.ID,
		job.RiderID,
		job.DeliveryFee,
		job.RiderPayout,
		job.PlatformDeliveryFee,
		job.PickupCode,
		job.DropoffCode,
		job.Status,
		job.AcceptedAt,
		job.PickedUpAt,
		job.DeliveredAt,
		job.DropoffAddress,
		job.DropoffLng, job.DropoffLat,
	)

	return err
}

func (r *DeliveryJobRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.DeliveryJobStatus) error {
	query := `UPDATE delivery_jobs SET status = $2 WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, id, status)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrDeliveryJobNotFound
	}

	return nil
}

func (r *DeliveryJobRepo) AssignRider(ctx context.Context, jobID, riderID uuid.UUID) error {
	now := time.Now()
	query := `UPDATE delivery_jobs SET rider_id = $2, status = 'accepted', accepted_at = $3 WHERE id = $1 AND status = 'searching'`
	result, err := r.db.ExecContext(ctx, query, jobID, riderID, now)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrJobAlreadyAccepted
	}

	return nil
}
type Coordinates struct {
	Lat float64
	Lng float64
}

func (r *DeliveryJobRepo) GetDropoffCoordinates(ctx context.Context, jobID uuid.UUID) (*Coordinates, error) {
	var coords Coordinates
	query := `
		SELECT
			COALESCE(ST_Y(dropoff_location::geometry), 0) as lat,
			COALESCE(ST_X(dropoff_location::geometry), 0) as lng
		FROM delivery_jobs
		WHERE id = $1`

	err := r.db.QueryRowxContext(ctx, query, jobID).Scan(&coords.Lat, &coords.Lng)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrDeliveryJobNotFound
		}
		return nil, err
	}

	return &coords, nil
}

func (r *DeliveryJobRepo) GetJobCoordinates(ctx context.Context, jobID uuid.UUID) (*repository.JobCoordinates, error) {
	var coords repository.JobCoordinates
	query := `
		SELECT
			COALESCE(ST_Y(pickup_location::geometry), 0) as pickup_lat,
			COALESCE(ST_X(pickup_location::geometry), 0) as pickup_lng,
			COALESCE(ST_Y(dropoff_location::geometry), 0) as dropoff_lat,
			COALESCE(ST_X(dropoff_location::geometry), 0) as dropoff_lng
		FROM delivery_jobs
		WHERE id = $1`

	err := r.db.QueryRowxContext(ctx, query, jobID).Scan(
		&coords.PickupLat, &coords.PickupLng,
		&coords.DropoffLat, &coords.DropoffLng,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrDeliveryJobNotFound
		}
		return nil, err
	}

	return &coords, nil
}

func (r *DeliveryJobRepo) GetCompletedTodayByRiderID(ctx context.Context, riderID uuid.UUID) (int, error) {
	var count int
	query := `
		SELECT COUNT(*) FROM delivery_jobs
		WHERE rider_id = $1
		AND status = 'delivered'
		AND delivered_at >= CURRENT_DATE
		AND delivered_at < CURRENT_DATE + INTERVAL '1 day'`

	err := r.db.GetContext(ctx, &count, query, riderID)
	if err != nil {
		return 0, err
	}

	return count, nil
}

func (r *DeliveryJobRepo) GetByStatus(ctx context.Context, status domain.DeliveryJobStatus, limit, offset int) ([]*domain.DeliveryJob, int, error) {
	var jobs []*domain.DeliveryJob
	var total int

	countQuery := `SELECT COUNT(*) FROM delivery_jobs WHERE status = $1`
	if err := r.db.GetContext(ctx, &total, countQuery, status); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT * FROM delivery_jobs
		WHERE status = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	if err := r.db.SelectContext(ctx, &jobs, query, status, limit, offset); err != nil {
		return nil, 0, err
	}

	return jobs, total, nil
}

func (r *DeliveryJobRepo) GetReturnJobByOriginalID(ctx context.Context, originalJobID uuid.UUID) (*domain.DeliveryJob, error) {
	var job domain.DeliveryJob
	query := `
		SELECT *,
			COALESCE(ST_Y(pickup_location::geometry), 0) as pickup_lat,
			COALESCE(ST_X(pickup_location::geometry), 0) as pickup_lng,
			COALESCE(ST_Y(dropoff_location::geometry), 0) as dropoff_lat,
			COALESCE(ST_X(dropoff_location::geometry), 0) as dropoff_lng
		FROM delivery_jobs
		WHERE original_job_id = $1 AND is_return = true
		ORDER BY created_at DESC LIMIT 1`

	err := r.db.GetContext(ctx, &job, query, originalJobID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &job, nil
}

func (r *DeliveryJobRepo) UpdateForcedReturn(ctx context.Context, job *domain.DeliveryJob) error {
	query := `
		UPDATE delivery_jobs SET
			status = $2,
			is_forced_return = $3,
			dropoff_code = $4,
			pickup_address = $5,
			pickup_location = ST_SetSRID(ST_MakePoint($6, $7), 4326)::geography,
			dropoff_address = $8,
			dropoff_location = ST_SetSRID(ST_MakePoint($9, $10), 4326)::geography
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query,
		job.ID,
		job.Status,
		job.IsForcedReturn,
		job.DropoffCode,
		job.PickupAddress,
		job.PickupLng, job.PickupLat,
		job.DropoffAddress,
		job.DropoffLng, job.DropoffLat,
	)

	return err
}

func (r *DeliveryJobRepo) HasActiveByRiderUserID(ctx context.Context, userID uuid.UUID) (bool, error) {
	var exists bool
	query := `
		SELECT EXISTS(
			SELECT 1 FROM delivery_jobs dj
			JOIN rider_profiles rp ON dj.rider_id = rp.id
			WHERE rp.user_id = $1
			AND dj.status IN ('accepted', 'en_route_pickup', 'picked_up', 'in_transit', 'buyer_unavailable', 'forced_return')
		)`
	err := r.db.GetContext(ctx, &exists, query, userID)
	return exists, err
}

func (r *DeliveryJobRepo) GetExpiredErrandUnavailableJobs(ctx context.Context, waitMinutes int) ([]*domain.DeliveryJob, error) {
	var jobs []*domain.DeliveryJob
	query := `
		SELECT
			id, payment_request_id, errand_id, source_type, rider_id,
			is_return, original_job_id, is_forced_return, unavailable_marked_at,
			pickup_address,
			COALESCE(ST_Y(pickup_location::geometry), 0) as pickup_lat,
			COALESCE(ST_X(pickup_location::geometry), 0) as pickup_lng,
			dropoff_address,
			COALESCE(ST_Y(dropoff_location::geometry), 0) as dropoff_lat,
			COALESCE(ST_X(dropoff_location::geometry), 0) as dropoff_lng,
			distance_km, delivery_fee, rider_payout, platform_delivery_fee,
			required_vehicle_type, pickup_code, dropoff_code,
			search_radius_km, search_started_at, search_timeout_at, last_radius_expand_at,
			status, current_eta_minutes, eta_updated_at,
			accepted_at, picked_up_at, delivered_at, created_at, updated_at
		FROM delivery_jobs
		WHERE source_type = 'errand'
		AND status = 'buyer_unavailable'
		AND unavailable_marked_at IS NOT NULL
		AND unavailable_marked_at + ($1 * INTERVAL '1 minute') < NOW()`

	err := r.db.SelectContext(ctx, &jobs, query, waitMinutes)
	if err != nil {
		return nil, err
	}

	return jobs, nil
}

type RiderJobOfferRepo struct {
	db *sqlx.DB
}

func NewRiderJobOfferRepo(db *sqlx.DB) repository.RiderJobOfferRepository {
	return &RiderJobOfferRepo{db: db}
}

func (r *RiderJobOfferRepo) Create(ctx context.Context, offer *domain.RiderJobOffer) error {
	query := `
		INSERT INTO rider_job_offers (id, delivery_job_id, rider_id, distance_km, response)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING offered_at`

	if offer.ID == uuid.Nil {
		offer.ID = uuid.New()
	}

	return r.db.QueryRowxContext(ctx, query,
		offer.ID,
		offer.DeliveryJobID,
		offer.RiderID,
		offer.DistanceKM,
		offer.Response,
	).Scan(&offer.OfferedAt)
}

func (r *RiderJobOfferRepo) GetByJobID(ctx context.Context, jobID uuid.UUID) ([]*domain.RiderJobOffer, error) {
	var offers []*domain.RiderJobOffer
	query := `SELECT * FROM rider_job_offers WHERE delivery_job_id = $1 ORDER BY offered_at ASC`

	err := r.db.SelectContext(ctx, &offers, query, jobID)
	if err != nil {
		return nil, err
	}

	return offers, nil
}

func (r *RiderJobOfferRepo) GetByRiderID(ctx context.Context, riderID uuid.UUID, response *domain.RiderOfferResponse) ([]*domain.RiderJobOffer, error) {
	var offers []*domain.RiderJobOffer

	query := `SELECT * FROM rider_job_offers WHERE rider_id = $1`
	args := []interface{}{riderID}

	if response != nil {
		query += ` AND response = $2`
		args = append(args, *response)
	}

	query += ` ORDER BY offered_at DESC`

	err := r.db.SelectContext(ctx, &offers, query, args...)
	if err != nil {
		return nil, err
	}

	return offers, nil
}

func (r *RiderJobOfferRepo) UpdateResponse(ctx context.Context, id uuid.UUID, response domain.RiderOfferResponse) error {
	now := time.Now()
	query := `UPDATE rider_job_offers SET response = $2, responded_at = $3 WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, response, now)
	return err
}

func (r *RiderJobOfferRepo) ExpirePendingOffers(ctx context.Context, jobID uuid.UUID) error {
	now := time.Now()
	query := `UPDATE rider_job_offers SET response = 'expired', responded_at = $2 WHERE delivery_job_id = $1 AND response = 'pending'`
	_, err := r.db.ExecContext(ctx, query, jobID, now)
	return err
}

func (r *RiderJobOfferRepo) ExpireAllStaleOffers(ctx context.Context, olderThanMins int) (int64, error) {
	now := time.Now()
	query := `
		UPDATE rider_job_offers 
		SET response = 'expired', responded_at = $1 
		WHERE response = 'pending' 
		AND offered_at <= NOW() - INTERVAL '1 minute' * $2`

	result, err := r.db.ExecContext(ctx, query, now, olderThanMins)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}
