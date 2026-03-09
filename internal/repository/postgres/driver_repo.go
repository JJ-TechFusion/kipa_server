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
	"github.com/lib/pq"
)

type DriverProfileRepo struct {
	db *sqlx.DB
}

func NewDriverProfileRepo(db *sqlx.DB) repository.DriverProfileRepository {
	return &DriverProfileRepo{db: db}
}

func (r *DriverProfileRepo) Create(ctx context.Context, profile *domain.DriverProfile) error {
	query := `
		INSERT INTO driver_profiles (
			id, user_id, status, max_carrying_value, current_carry_value,
			max_package_count, current_package_count, vehicle_type, license_plate,
			is_verified, nin_number, nin_photo_url, license_number, license_photo_url,
			vehicle_photo_url, face_photo_url
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		RETURNING created_at, updated_at`

	if profile.ID == uuid.Nil {
		profile.ID = uuid.New()
	}

	return r.db.QueryRowxContext(ctx, query,
		profile.ID,
		profile.UserID,
		profile.Status,
		profile.MaxCarryingValue,
		profile.CurrentCarryValue,
		profile.MaxPackageCount,
		profile.CurrentPackageCount,
		profile.VehicleType,
		profile.LicensePlate,
		profile.IsVerified,
		profile.NINNumber,
		profile.NINPhotoURL,
		profile.LicenseNumber,
		profile.LicensePhotoURL,
		profile.VehiclePhotoURL,
		profile.FacePhotoURL,
	).Scan(&profile.CreatedAt, &profile.UpdatedAt)
}

func (r *DriverProfileRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.DriverProfile, error) {
	var profile domain.DriverProfile
	query := `SELECT * FROM driver_profiles WHERE id = $1`

	err := r.db.GetContext(ctx, &profile, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrDriverProfileNotFound
		}
		return nil, err
	}

	return &profile, nil
}

func (r *DriverProfileRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.DriverProfile, error) {
	var profile domain.DriverProfile
	query := `SELECT * FROM driver_profiles WHERE user_id = $1`

	err := r.db.GetContext(ctx, &profile, query, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrDriverProfileNotFound
		}
		return nil, err
	}

	return &profile, nil
}

func (r *DriverProfileRepo) Update(ctx context.Context, profile *domain.DriverProfile) error {
	query := `
		UPDATE driver_profiles SET
			status = $2,
			max_carrying_value = $3,
			current_carry_value = $4,
			max_package_count = $5,
			current_package_count = $6,
			vehicle_type = $7,
			license_plate = $8,
			is_verified = $9,
			total_deliveries = $10,
			total_earnings = $11,
			rating = $12,
			current_trip_id = $13,
			origin_city = $14,
			destination_city = $15,
			nin_number = $16,
			nin_photo_url = $17,
			license_number = $18,
			license_photo_url = $19,
			vehicle_photo_url = $20,
			face_photo_url = $21,
			updated_at = NOW()
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query,
		profile.ID,
		profile.Status,
		profile.MaxCarryingValue,
		profile.CurrentCarryValue,
		profile.MaxPackageCount,
		profile.CurrentPackageCount,
		profile.VehicleType,
		profile.LicensePlate,
		profile.IsVerified,
		profile.TotalDeliveries,
		profile.TotalEarnings,
		profile.Rating,
		profile.CurrentTripID,
		profile.OriginCity,
		profile.DestinationCity,
		profile.NINNumber,
		profile.NINPhotoURL,
		profile.LicenseNumber,
		profile.LicensePhotoURL,
		profile.VehiclePhotoURL,
		profile.FacePhotoURL,
	)

	return err
}

func (r *DriverProfileRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.DriverStatus) error {
	query := `UPDATE driver_profiles SET status = $2, updated_at = NOW() WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, id, status)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrDriverProfileNotFound
	}

	return nil
}

func (r *DriverProfileRepo) UpdateCapacity(ctx context.Context, id uuid.UUID, carryValue float64, packageCount int) error {
	query := `
		UPDATE driver_profiles SET
			current_carry_value = $2,
			current_package_count = $3,
			updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, carryValue, packageCount)
	return err
}

func (r *DriverProfileRepo) ResetCapacity(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE driver_profiles SET
			current_carry_value = 0,
			current_package_count = 0,
			updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

func (r *DriverProfileRepo) SetCurrentTrip(ctx context.Context, id uuid.UUID, tripID *uuid.UUID, origin, destination *string) error {
	query := `
		UPDATE driver_profiles SET
			current_trip_id = $2,
			origin_city = $3,
			destination_city = $4,
			updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, tripID, origin, destination)
	return err
}

func (r *DriverProfileRepo) IncrementDeliveries(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE driver_profiles SET total_deliveries = total_deliveries + 1, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

func (r *DriverProfileRepo) AddEarnings(ctx context.Context, id uuid.UUID, amount float64) error {
	query := `UPDATE driver_profiles SET total_earnings = total_earnings + $2, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, amount)
	return err
}

func (r *DriverProfileRepo) GetAvailableForRoute(ctx context.Context, originCity, destCity string) ([]*domain.DriverProfile, error) {
	var profiles []*domain.DriverProfile
	query := `
		SELECT * FROM driver_profiles
		WHERE is_verified = true
		AND status IN ('idle', 'loading')
		AND (origin_city = $1 OR origin_city IS NULL)
		AND (destination_city = $2 OR destination_city IS NULL)
		ORDER BY rating DESC, total_deliveries DESC`

	err := r.db.SelectContext(ctx, &profiles, query, originCity, destCity)
	if err != nil {
		return nil, err
	}

	return profiles, nil
}

func (r *DriverProfileRepo) GetByStatus(ctx context.Context, status domain.DriverStatus, limit, offset int) ([]*domain.DriverProfile, error) {
	var profiles []*domain.DriverProfile
	query := `
		SELECT * FROM driver_profiles
		WHERE status = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	err := r.db.SelectContext(ctx, &profiles, query, status, limit, offset)
	if err != nil {
		return nil, err
	}

	return profiles, nil
}

func (r *DriverProfileRepo) GetPending(ctx context.Context, limit, offset int) ([]*domain.DriverProfile, error) {
	var profiles []*domain.DriverProfile
	query := `
		SELECT * FROM driver_profiles
		WHERE is_verified = false
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2`

	err := r.db.SelectContext(ctx, &profiles, query, limit, offset)
	if err != nil {
		return nil, err
	}

	return profiles, nil
}

func (r *DriverProfileRepo) GetStatistics(ctx context.Context) (*repository.DriverStatistics, error) {
	query := `
		SELECT
			COUNT(*) as total_drivers,
			COUNT(*) FILTER (WHERE is_verified = true) as verified_drivers,
			COUNT(*) FILTER (WHERE status IN ('idle', 'loading')) as online_drivers,
			COUNT(*) FILTER (WHERE status = 'in_transit') as in_transit_drivers,
			COALESCE(SUM(total_deliveries), 0) as total_deliveries,
			COALESCE(AVG(rating) FILTER (WHERE total_deliveries > 0), 0) as average_rating
		FROM driver_profiles`

	var stats repository.DriverStatistics
	err := r.db.GetContext(ctx, &stats, query)
	if err != nil {
		return nil, err
	}

	return &stats, nil
}

type DriverTripRepo struct {
	db *sqlx.DB
}

func NewDriverTripRepo(db *sqlx.DB) repository.DriverTripRepository {
	return &DriverTripRepo{db: db}
}

func (r *DriverTripRepo) Create(ctx context.Context, trip *domain.DriverTrip) error {
	query := `
		INSERT INTO driver_trips (
			id, driver_id, origin_city, destination_city, status,
			total_packages, total_value, total_driver_fee, accepting_until
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING created_at, updated_at`

	if trip.ID == uuid.Nil {
		trip.ID = uuid.New()
	}

	return r.db.QueryRowxContext(ctx, query,
		trip.ID,
		trip.DriverID,
		trip.OriginCity,
		trip.DestinationCity,
		trip.Status,
		trip.TotalPackages,
		trip.TotalValue,
		trip.TotalDriverFee,
		trip.AcceptingUntil,
	).Scan(&trip.CreatedAt, &trip.UpdatedAt)
}

func (r *DriverTripRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.DriverTrip, error) {
	var trip domain.DriverTrip
	query := `SELECT * FROM driver_trips WHERE id = $1`

	err := r.db.GetContext(ctx, &trip, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrDriverTripNotFound
		}
		return nil, err
	}

	return &trip, nil
}

func (r *DriverTripRepo) GetByDriverID(ctx context.Context, driverID uuid.UUID) (*domain.DriverTrip, error) {
	var trip domain.DriverTrip
	query := `SELECT * FROM driver_trips WHERE driver_id = $1 ORDER BY created_at DESC LIMIT 1`

	err := r.db.GetContext(ctx, &trip, query, driverID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrDriverTripNotFound
		}
		return nil, err
	}

	return &trip, nil
}

func (r *DriverTripRepo) GetActiveByDriverID(ctx context.Context, driverID uuid.UUID) (*domain.DriverTrip, error) {
	var trip domain.DriverTrip
	query := `
		SELECT * FROM driver_trips
		WHERE driver_id = $1
		AND status NOT IN ('completed', 'cancelled')
		ORDER BY created_at DESC LIMIT 1`

	err := r.db.GetContext(ctx, &trip, query, driverID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrDriverTripNotFound
		}
		return nil, err
	}

	return &trip, nil
}

func (r *DriverTripRepo) GetByStatus(ctx context.Context, status domain.DriverTripStatus, limit, offset int) ([]*domain.DriverTrip, error) {
	var trips []*domain.DriverTrip
	query := `
		SELECT * FROM driver_trips
		WHERE status = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	err := r.db.SelectContext(ctx, &trips, query, status, limit, offset)
	if err != nil {
		return nil, err
	}

	return trips, nil
}

func (r *DriverTripRepo) GetCompletedByDriverID(ctx context.Context, driverID uuid.UUID, limit, offset int) ([]*domain.DriverTrip, error) {
	var trips []*domain.DriverTrip
	query := `
		SELECT * FROM driver_trips
		WHERE driver_id = $1 AND status = 'completed'
		ORDER BY completed_at DESC
		LIMIT $2 OFFSET $3`

	err := r.db.SelectContext(ctx, &trips, query, driverID, limit, offset)
	if err != nil {
		return nil, err
	}

	return trips, nil
}

func (r *DriverTripRepo) GetByRoute(ctx context.Context, originCity, destCity string, status *domain.DriverTripStatus, limit, offset int) ([]*domain.DriverTrip, error) {
	var trips []*domain.DriverTrip
	var query string
	var args []interface{}

	if status != nil {
		query = `
			SELECT * FROM driver_trips
			WHERE origin_city = $1 AND destination_city = $2 AND status = $3
			ORDER BY created_at DESC
			LIMIT $4 OFFSET $5`
		args = []interface{}{originCity, destCity, *status, limit, offset}
	} else {
		query = `
			SELECT * FROM driver_trips
			WHERE origin_city = $1 AND destination_city = $2
			ORDER BY created_at DESC
			LIMIT $3 OFFSET $4`
		args = []interface{}{originCity, destCity, limit, offset}
	}

	err := r.db.SelectContext(ctx, &trips, query, args...)
	if err != nil {
		return nil, err
	}

	return trips, nil
}

func (r *DriverTripRepo) Update(ctx context.Context, trip *domain.DriverTrip) error {
	query := `
		UPDATE driver_trips SET
			status = $2,
			total_packages = $3,
			total_value = $4,
			total_driver_fee = $5,
			accepting_until = $6,
			departed_at = $7,
			arrived_at = $8,
			completed_at = $9,
			updated_at = NOW()
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query,
		trip.ID,
		trip.Status,
		trip.TotalPackages,
		trip.TotalValue,
		trip.TotalDriverFee,
		trip.AcceptingUntil,
		trip.DepartedAt,
		trip.ArrivedAt,
		trip.CompletedAt,
	)

	return err
}

func (r *DriverTripRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.DriverTripStatus) error {
	query := `UPDATE driver_trips SET status = $2, updated_at = NOW() WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, id, status)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrDriverTripNotFound
	}

	return nil
}

func (r *DriverTripRepo) UpdateAggregates(ctx context.Context, id uuid.UUID, totalPackages int, totalValue, totalDriverFee float64) error {
	query := `
		UPDATE driver_trips SET
			total_packages = $2,
			total_value = $3,
			total_driver_fee = $4,
			updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, totalPackages, totalValue, totalDriverFee)
	return err
}

func (r *DriverTripRepo) AddDelivery(ctx context.Context, id uuid.UUID, itemValue, driverFee float64) error {
	query := `
		UPDATE driver_trips SET
			total_packages = total_packages + 1,
			total_value = total_value + $2,
			total_driver_fee = total_driver_fee + $3,
			updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, itemValue, driverFee)
	return err
}

func (r *DriverTripRepo) RemoveDelivery(ctx context.Context, id uuid.UUID, itemValue, driverFee float64) error {
	query := `
		UPDATE driver_trips SET
			total_packages = GREATEST(total_packages - 1, 0),
			total_value = GREATEST(total_value - $2, 0),
			total_driver_fee = GREATEST(total_driver_fee - $3, 0),
			updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, itemValue, driverFee)
	return err
}

func (r *DriverTripRepo) MarkDeparted(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	query := `
		UPDATE driver_trips SET
			status = 'in_transit',
			departed_at = $2,
			updated_at = NOW()
		WHERE id = $1 AND status = 'loading'`
	result, err := r.db.ExecContext(ctx, query, id, now)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrTripCannotDepart
	}

	return nil
}

func (r *DriverTripRepo) MarkArrived(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	query := `
		UPDATE driver_trips SET
			status = 'arrived',
			arrived_at = $2,
			updated_at = NOW()
		WHERE id = $1 AND status = 'in_transit'`
	result, err := r.db.ExecContext(ctx, query, id, now)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrTripNotInTransit
	}

	return nil
}

func (r *DriverTripRepo) MarkCompleted(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	query := `
		UPDATE driver_trips SET
			status = 'completed',
			completed_at = $2,
			updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, now)
	return err
}

func (r *DriverTripRepo) GetStaleDepartures(ctx context.Context, hoursThreshold int) ([]*domain.DriverTrip, error) {
	var trips []*domain.DriverTrip
	query := `
		SELECT * FROM driver_trips
		WHERE status = 'loading'
		AND created_at < NOW() - INTERVAL '1 hour' * $1
		ORDER BY created_at ASC
		LIMIT 100`

	err := r.db.SelectContext(ctx, &trips, query, hoursThreshold)
	if err != nil {
		return nil, err
	}

	return trips, nil
}

func (r *DriverTripRepo) GetAbandonedTrips(ctx context.Context, hoursThreshold int) ([]*domain.DriverTrip, error) {
	var trips []*domain.DriverTrip
	query := `
		SELECT * FROM driver_trips
		WHERE status = 'in_transit'
		AND departed_at < NOW() - INTERVAL '1 hour' * $1
		ORDER BY departed_at ASC
		LIMIT 100`

	err := r.db.SelectContext(ctx, &trips, query, hoursThreshold)
	if err != nil {
		return nil, err
	}

	return trips, nil
}

type DriverDeliveryRepo struct {
	db *sqlx.DB
}

func NewDriverDeliveryRepo(db *sqlx.DB) repository.DriverDeliveryRepository {
	return &DriverDeliveryRepo{db: db}
}

func (r *DriverDeliveryRepo) Create(ctx context.Context, delivery *domain.DriverDelivery) error {
	query := `
		INSERT INTO driver_deliveries (
			id, payment_request_id, trip_id, driver_id,
			origin_city, destination_city, pickup_address, dropoff_address,
			item_value, driver_fee, status, seller_id, buyer_id, escrow_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING created_at, updated_at`

	if delivery.ID == uuid.Nil {
		delivery.ID = uuid.New()
	}

	return r.db.QueryRowxContext(ctx, query,
		delivery.ID,
		delivery.PaymentRequestID,
		delivery.TripID,
		delivery.DriverID,
		delivery.OriginCity,
		delivery.DestinationCity,
		delivery.PickupAddress,
		delivery.DropoffAddress,
		delivery.ItemValue,
		delivery.DriverFee,
		delivery.Status,
		delivery.SellerID,
		delivery.BuyerID,
		delivery.EscrowID,
	).Scan(&delivery.CreatedAt, &delivery.UpdatedAt)
}

func (r *DriverDeliveryRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.DriverDelivery, error) {
	var delivery domain.DriverDelivery
	query := `SELECT * FROM driver_deliveries WHERE id = $1`

	err := r.db.GetContext(ctx, &delivery, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrDriverDeliveryNotFound
		}
		return nil, err
	}

	return &delivery, nil
}

func (r *DriverDeliveryRepo) GetByPaymentRequestID(ctx context.Context, prID uuid.UUID) (*domain.DriverDelivery, error) {
	var delivery domain.DriverDelivery
	query := `SELECT * FROM driver_deliveries WHERE payment_request_id = $1`

	err := r.db.GetContext(ctx, &delivery, query, prID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrDriverDeliveryNotFound
		}
		return nil, err
	}

	return &delivery, nil
}

func (r *DriverDeliveryRepo) GetByTripID(ctx context.Context, tripID uuid.UUID) ([]*domain.DriverDelivery, error) {
	var deliveries []*domain.DriverDelivery
	query := `SELECT * FROM driver_deliveries WHERE trip_id = $1 ORDER BY created_at ASC`

	err := r.db.SelectContext(ctx, &deliveries, query, tripID)
	if err != nil {
		return nil, err
	}

	return deliveries, nil
}

func (r *DriverDeliveryRepo) GetByDriverID(ctx context.Context, driverID uuid.UUID, statuses []domain.DriverDeliveryStatus, limit, offset int) ([]*domain.DriverDelivery, error) {
	var deliveries []*domain.DriverDelivery
	var query string
	var args []interface{}

	if len(statuses) > 0 {
		query = `
			SELECT * FROM driver_deliveries
			WHERE driver_id = $1 AND status = ANY($2)
			ORDER BY created_at DESC
			LIMIT $3 OFFSET $4`
		args = []interface{}{driverID, pq.Array(statuses), limit, offset}
	} else {
		query = `
			SELECT * FROM driver_deliveries
			WHERE driver_id = $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3`
		args = []interface{}{driverID, limit, offset}
	}

	err := r.db.SelectContext(ctx, &deliveries, query, args...)
	if err != nil {
		return nil, err
	}

	return deliveries, nil
}

func (r *DriverDeliveryRepo) GetBySellerID(ctx context.Context, sellerID uuid.UUID, limit, offset int) ([]*domain.DriverDelivery, error) {
	var deliveries []*domain.DriverDelivery
	query := `
		SELECT * FROM driver_deliveries
		WHERE seller_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	err := r.db.SelectContext(ctx, &deliveries, query, sellerID, limit, offset)
	if err != nil {
		return nil, err
	}

	return deliveries, nil
}

func (r *DriverDeliveryRepo) GetByBuyerID(ctx context.Context, buyerID uuid.UUID, limit, offset int) ([]*domain.DriverDelivery, error) {
	var deliveries []*domain.DriverDelivery
	query := `
		SELECT * FROM driver_deliveries
		WHERE buyer_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	err := r.db.SelectContext(ctx, &deliveries, query, buyerID, limit, offset)
	if err != nil {
		return nil, err
	}

	return deliveries, nil
}

func (r *DriverDeliveryRepo) Update(ctx context.Context, delivery *domain.DriverDelivery) error {
	query := `
		UPDATE driver_deliveries SET
			trip_id = $2,
			driver_id = $3,
			pickup_code = $4,
			pickup_code_generated_at = $5,
			pickup_attempts = $6,
			dropoff_code = $7,
			dropoff_code_generated_at = $8,
			dropoff_attempts = $9,
			status = $10,
			driver_assigned_at = $11,
			at_pickup_at = $12,
			picked_up_at = $13,
			departed_at = $14,
			arrived_at = $15,
			delivered_at = $16,
			completed_at = $17,
			buyer_unreachable_at = $18,
			unreachable_deadline = $19,
			buyer_id = $20,
			escrow_id = $21,
			updated_at = NOW()
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query,
		delivery.ID,
		delivery.TripID,
		delivery.DriverID,
		delivery.PickupCode,
		delivery.PickupCodeGeneratedAt,
		delivery.PickupAttempts,
		delivery.DropoffCode,
		delivery.DropoffCodeGeneratedAt,
		delivery.DropoffAttempts,
		delivery.Status,
		delivery.DriverAssignedAt,
		delivery.AtPickupAt,
		delivery.PickedUpAt,
		delivery.DepartedAt,
		delivery.ArrivedAt,
		delivery.DeliveredAt,
		delivery.CompletedAt,
		delivery.BuyerUnreachableAt,
		delivery.UnreachableDeadline,
		delivery.BuyerID,
		delivery.EscrowID,
	)

	return err
}

func (r *DriverDeliveryRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.DriverDeliveryStatus) error {
	query := `UPDATE driver_deliveries SET status = $2, updated_at = NOW() WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, id, status)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrDriverDeliveryNotFound
	}

	return nil
}

func (r *DriverDeliveryRepo) GetAwaitingDriver(ctx context.Context, originCity, destCity string, limit int) ([]*domain.DriverDelivery, error) {
	var deliveries []*domain.DriverDelivery
	query := `
		SELECT * FROM driver_deliveries
		WHERE status = 'awaiting_driver'
		AND origin_city = $1 AND destination_city = $2
		ORDER BY created_at ASC
		LIMIT $3`

	err := r.db.SelectContext(ctx, &deliveries, query, originCity, destCity, limit)
	if err != nil {
		return nil, err
	}

	return deliveries, nil
}

func (r *DriverDeliveryRepo) GetAvailableForRoute(ctx context.Context, originCity, destCity string, maxValue float64, limit int) ([]*domain.DriverDelivery, error) {
	var deliveries []*domain.DriverDelivery
	query := `
		SELECT * FROM driver_deliveries
		WHERE status = 'awaiting_driver'
		AND origin_city = $1 AND destination_city = $2
		AND item_value <= $3
		ORDER BY item_value DESC, created_at ASC
		LIMIT $4`

	err := r.db.SelectContext(ctx, &deliveries, query, originCity, destCity, maxValue, limit)
	if err != nil {
		return nil, err
	}

	return deliveries, nil
}

func (r *DriverDeliveryRepo) AssignDriver(ctx context.Context, id uuid.UUID, driverID, tripID uuid.UUID) error {
	now := time.Now()
	query := `
		UPDATE driver_deliveries SET
			driver_id = $2,
			trip_id = $3,
			status = 'driver_assigned',
			driver_assigned_at = $4,
			updated_at = NOW()
		WHERE id = $1 AND status = 'awaiting_driver'`
	result, err := r.db.ExecContext(ctx, query, id, driverID, tripID, now)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrDriverDeliveryNotFound
	}

	return nil
}

func (r *DriverDeliveryRepo) SetPickupCode(ctx context.Context, id uuid.UUID, code string) error {
	now := time.Now()
	query := `
		UPDATE driver_deliveries SET
			pickup_code = $2,
			pickup_code_generated_at = $3,
			updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, code, now)
	return err
}

func (r *DriverDeliveryRepo) SetDropoffCode(ctx context.Context, id uuid.UUID, code string) error {
	now := time.Now()
	query := `
		UPDATE driver_deliveries SET
			dropoff_code = $2,
			dropoff_code_generated_at = $3,
			updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, code, now)
	return err
}

func (r *DriverDeliveryRepo) IncrementPickupAttempts(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE driver_deliveries SET pickup_attempts = pickup_attempts + 1, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

func (r *DriverDeliveryRepo) IncrementDropoffAttempts(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE driver_deliveries SET dropoff_attempts = dropoff_attempts + 1, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

func (r *DriverDeliveryRepo) MarkAtPickup(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	query := `
		UPDATE driver_deliveries SET
			status = 'awaiting_pickup',
			at_pickup_at = $2,
			updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, now)
	return err
}

func (r *DriverDeliveryRepo) MarkPickedUp(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	query := `
		UPDATE driver_deliveries SET
			status = 'picked_up',
			picked_up_at = $2,
			updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, now)
	return err
}

func (r *DriverDeliveryRepo) MarkDeparted(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	query := `
		UPDATE driver_deliveries SET
			status = 'in_transit',
			departed_at = $2,
			updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, now)
	return err
}

func (r *DriverDeliveryRepo) MarkArrived(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	query := `
		UPDATE driver_deliveries SET
			status = 'arrived',
			arrived_at = $2,
			updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, now)
	return err
}

func (r *DriverDeliveryRepo) MarkDelivered(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	query := `
		UPDATE driver_deliveries SET
			status = 'confirmation_window',
			delivered_at = $2,
			updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, now)
	return err
}

func (r *DriverDeliveryRepo) MarkCompleted(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	query := `
		UPDATE driver_deliveries SET
			status = 'completed',
			completed_at = $2,
			updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, now)
	return err
}

func (r *DriverDeliveryRepo) MarkBuyerUnreachable(ctx context.Context, id uuid.UUID, deadline time.Time) error {
	now := time.Now()
	query := `
		UPDATE driver_deliveries SET
			status = 'buyer_unreachable',
			buyer_unreachable_at = $2,
			unreachable_deadline = $3,
			updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, now, deadline)
	return err
}

func (r *DriverDeliveryRepo) GetExpiredUnreachable(ctx context.Context) ([]*domain.DriverDelivery, error) {
	var deliveries []*domain.DriverDelivery
	query := `
		SELECT * FROM driver_deliveries
		WHERE status = 'buyer_unreachable'
		AND unreachable_deadline IS NOT NULL
		AND unreachable_deadline < NOW()
		ORDER BY unreachable_deadline ASC
		LIMIT 100`

	err := r.db.SelectContext(ctx, &deliveries, query)
	if err != nil {
		return nil, err
	}

	return deliveries, nil
}

func (r *DriverDeliveryRepo) UpdateStatusByTripID(ctx context.Context, tripID uuid.UUID, fromStatuses []domain.DriverDeliveryStatus, toStatus domain.DriverDeliveryStatus) error {
	query := `
		UPDATE driver_deliveries SET
			status = $2,
			updated_at = NOW()
		WHERE trip_id = $1 AND status = ANY($3)`
	_, err := r.db.ExecContext(ctx, query, tripID, toStatus, pq.Array(fromStatuses))
	return err
}

func (r *DriverDeliveryRepo) GetExpiredAwaiting(ctx context.Context, hoursThreshold int) ([]*domain.DriverDelivery, error) {
	var deliveries []*domain.DriverDelivery
	query := `
		SELECT * FROM driver_deliveries
		WHERE status = 'awaiting_driver'
		AND created_at < NOW() - INTERVAL '1 hour' * $1
		ORDER BY created_at ASC
		LIMIT 100`

	err := r.db.SelectContext(ctx, &deliveries, query, hoursThreshold)
	if err != nil {
		return nil, err
	}

	return deliveries, nil
}

func (r *DriverDeliveryRepo) GetStaleInTransit(ctx context.Context, hoursThreshold int) ([]*domain.DriverDelivery, error) {
	var deliveries []*domain.DriverDelivery
	query := `
		SELECT * FROM driver_deliveries
		WHERE status = 'in_transit'
		AND departed_at < NOW() - INTERVAL '1 hour' * $1
		ORDER BY departed_at ASC
		LIMIT 100`

	err := r.db.SelectContext(ctx, &deliveries, query, hoursThreshold)
	if err != nil {
		return nil, err
	}

	return deliveries, nil
}

func (r *DriverDeliveryRepo) GetExpiredConfirmationWindow(ctx context.Context) ([]*domain.DriverDelivery, error) {
	var deliveries []*domain.DriverDelivery
	query := `
		SELECT * FROM driver_deliveries
		WHERE status = 'confirmation_window'
		AND delivered_at < NOW() - INTERVAL '7 days'
		ORDER BY delivered_at ASC
		LIMIT 100`

	err := r.db.SelectContext(ctx, &deliveries, query)
	if err != nil {
		return nil, err
	}

	return deliveries, nil
}

type DriverDeliveryEventRepo struct {
	db *sqlx.DB
}

func NewDriverDeliveryEventRepo(db *sqlx.DB) repository.DriverDeliveryEventRepository {
	return &DriverDeliveryEventRepo{db: db}
}

func (r *DriverDeliveryEventRepo) Create(ctx context.Context, event *domain.DriverDeliveryEvent) error {
	query := `
		INSERT INTO driver_delivery_events (
			id, delivery_id, from_status, to_status, actor_type, actor_id, reason, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at`

	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}

	return r.db.QueryRowxContext(ctx, query,
		event.ID,
		event.DeliveryID,
		event.FromStatus,
		event.ToStatus,
		event.ActorType,
		event.ActorID,
		event.Reason,
		event.Metadata,
	).Scan(&event.CreatedAt)
}

func (r *DriverDeliveryEventRepo) GetByDeliveryID(ctx context.Context, deliveryID uuid.UUID) ([]*domain.DriverDeliveryEvent, error) {
	var events []*domain.DriverDeliveryEvent
	query := `SELECT * FROM driver_delivery_events WHERE delivery_id = $1 ORDER BY created_at ASC`

	err := r.db.SelectContext(ctx, &events, query, deliveryID)
	if err != nil {
		return nil, err
	}

	return events, nil
}
