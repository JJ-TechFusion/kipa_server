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
type RiderProfileRepo struct {
	db *sqlx.DB
}

func NewRiderProfileRepo(db *sqlx.DB) repository.RiderProfileRepository {
	return &RiderProfileRepo{db: db}
}

func (r *RiderProfileRepo) Create(ctx context.Context, profile *domain.RiderProfile) error {
	query := `
		INSERT INTO rider_profiles (
			id, user_id, license_number, nin_number, license_photo_url, nin_photo_url,
			face_verification_url, vehicle_type, vehicle_plate, vehicle_photo_url, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING created_at, updated_at`

	if profile.ID == uuid.Nil {
		profile.ID = uuid.New()
	}

	return r.db.QueryRowxContext(ctx, query,
		profile.ID,
		profile.UserID,
		profile.LicenseNumber,
		profile.NINNumber,
		profile.LicensePhotoURL,
		profile.NINPhotoURL,
		profile.FaceVerificationURL,
		profile.VehicleType,
		profile.VehiclePlate,
		profile.VehiclePhotoURL,
		profile.Status,
	).Scan(&profile.CreatedAt, &profile.UpdatedAt)
}

func (r *RiderProfileRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.RiderProfile, error) {
	var profile domain.RiderProfile
	query := `SELECT * FROM rider_profiles WHERE id = $1`

	err := r.db.GetContext(ctx, &profile, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrRiderNotFound
		}
		return nil, err
	}

	return &profile, nil
}

func (r *RiderProfileRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.RiderProfile, error) {
	var profile domain.RiderProfile
	query := `SELECT * FROM rider_profiles WHERE user_id = $1`

	err := r.db.GetContext(ctx, &profile, query, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrRiderNotFound
		}
		return nil, err
	}

	return &profile, nil
}

func (r *RiderProfileRepo) Update(ctx context.Context, profile *domain.RiderProfile) error {
	query := `
		UPDATE rider_profiles SET
			license_number = $2,
			nin_number = $3,
			license_photo_url = $4,
			nin_photo_url = $5,
			face_verification_url = $6,
			vehicle_type = $7,
			vehicle_plate = $8,
			vehicle_photo_url = $9,
			status = $10,
			rating = $11,
			total_deliveries = $12,
			total_earnings = $13
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query,
		profile.ID,
		profile.LicenseNumber,
		profile.NINNumber,
		profile.LicensePhotoURL,
		profile.NINPhotoURL,
		profile.FaceVerificationURL,
		profile.VehicleType,
		profile.VehiclePlate,
		profile.VehiclePhotoURL,
		profile.Status,
		profile.Rating,
		profile.TotalDeliveries,
		profile.TotalEarnings,
	)

	return err
}

func (r *RiderProfileRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.RiderStatus) error {
	query := `UPDATE rider_profiles SET status = $2 WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, id, status)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrRiderNotFound
	}

	return nil
}

func (r *RiderProfileRepo) SetOnline(ctx context.Context, id uuid.UUID, isOnline bool) error {
	query := `UPDATE rider_profiles SET is_online = $2 WHERE id = $1 AND status = 'verified'`
	result, err := r.db.ExecContext(ctx, query, id, isOnline)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrRiderNotVerified
	}

	return nil
}

func (r *RiderProfileRepo) UpdateLocation(ctx context.Context, id uuid.UUID, lat, lng float64) error {
	now := time.Now()
	query := `
		UPDATE rider_profiles SET
			current_location = ST_SetSRID(ST_MakePoint($2, $3), 4326)::geography,
			last_location_update = $4
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query, id, lng, lat, now)
	return err
}

func (r *RiderProfileRepo) GetOnlineRidersInRadius(ctx context.Context, lat, lng float64, radiusKM int) ([]*domain.RiderProfile, error) {
	var profiles []*domain.RiderProfile

	query := `
		SELECT * FROM rider_profiles
		WHERE is_online = true
		AND status = 'verified'
		AND current_location IS NOT NULL
		AND ST_DWithin(
			current_location,
			ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography,
			$3
		)
		ORDER BY ST_Distance(current_location, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography) ASC`

	radiusMeters := radiusKM * 1000

	err := r.db.SelectContext(ctx, &profiles, query, lng, lat, radiusMeters)
	if err != nil {
		return nil, err
	}

	return profiles, nil
}

func (r *RiderProfileRepo) IncrementDeliveries(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE rider_profiles SET total_deliveries = total_deliveries + 1 WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

func (r *RiderProfileRepo) AddEarnings(ctx context.Context, id uuid.UUID, amount float64) error {
	query := `UPDATE rider_profiles SET total_earnings = total_earnings + $2 WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, amount)
	return err
}
func (r *RiderProfileRepo) UpdateCommissionStats(ctx context.Context, id uuid.UUID, unpaidCommission float64, unpaidCount int) error {
	query := `
		UPDATE rider_profiles SET 
			unpaid_commission = $2,
			unpaid_delivery_count = $3
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, unpaidCommission, unpaidCount)
	return err
}
func (r *RiderProfileRepo) SetCommissionBlocked(ctx context.Context, id uuid.UUID, blocked bool) error {
	query := `UPDATE rider_profiles SET is_commission_blocked = $2 WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, blocked)
	return err
}
func (r *RiderProfileRepo) ClearCommissionDebt(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	query := `
		UPDATE rider_profiles SET 
			unpaid_commission = 0,
			unpaid_delivery_count = 0,
			is_commission_blocked = false,
			last_commission_paid_at = $2
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, now)
	return err
}
func (r *RiderProfileRepo) RecordCommissionPayment(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	query := `UPDATE rider_profiles SET last_commission_paid_at = $2 WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, now)
	return err
}
type DeliveryZoneRepo struct {
	db *sqlx.DB
}

func NewDeliveryZoneRepo(db *sqlx.DB) repository.DeliveryZoneRepository {
	return &DeliveryZoneRepo{db: db}
}

func (r *DeliveryZoneRepo) Create(ctx context.Context, zone *domain.DeliveryZone) error {
	query := `
		INSERT INTO delivery_zones (
			id, name, city, state, polygon,
			initial_search_radius_km, max_search_radius_km, radius_expand_interval_seconds,
			base_delivery_fee, per_km_fee, is_active
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING created_at`

	if zone.ID == uuid.Nil {
		zone.ID = uuid.New()
	}

	return r.db.QueryRowxContext(ctx, query,
		zone.ID,
		zone.Name,
		zone.City,
		zone.State,
		zone.Polygon,
		zone.InitialSearchRadiusKM,
		zone.MaxSearchRadiusKM,
		zone.RadiusExpandIntervalSeconds,
		zone.BaseDeliveryFee,
		zone.PerKMFee,
		zone.IsActive,
	).Scan(&zone.CreatedAt)
}

func (r *DeliveryZoneRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.DeliveryZone, error) {
	var zone domain.DeliveryZone
	query := `SELECT * FROM delivery_zones WHERE id = $1`

	err := r.db.GetContext(ctx, &zone, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrZoneNotFound
		}
		return nil, err
	}

	return &zone, nil
}

func (r *DeliveryZoneRepo) GetByLocation(ctx context.Context, lat, lng float64) (*domain.DeliveryZone, error) {
	var zone domain.DeliveryZone

	query := `
		SELECT * FROM delivery_zones
		WHERE is_active = true
		AND ST_Contains(
			polygon::geometry,
			ST_SetSRID(ST_MakePoint($1, $2), 4326)
		)
		LIMIT 1`

	err := r.db.GetContext(ctx, &zone, query, lng, lat)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrZoneNotFound
		}
		return nil, err
	}

	return &zone, nil
}

func (r *DeliveryZoneRepo) GetAll(ctx context.Context) ([]*domain.DeliveryZone, error) {
	var zones []*domain.DeliveryZone
	query := `SELECT * FROM delivery_zones ORDER BY name ASC`

	err := r.db.SelectContext(ctx, &zones, query)
	if err != nil {
		return nil, err
	}

	return zones, nil
}

func (r *DeliveryZoneRepo) GetActive(ctx context.Context) ([]*domain.DeliveryZone, error) {
	var zones []*domain.DeliveryZone
	query := `SELECT * FROM delivery_zones WHERE is_active = true ORDER BY name ASC`

	err := r.db.SelectContext(ctx, &zones, query)
	if err != nil {
		return nil, err
	}

	return zones, nil
}

func (r *DeliveryZoneRepo) Update(ctx context.Context, zone *domain.DeliveryZone) error {
	query := `
		UPDATE delivery_zones SET
			name = $2,
			city = $3,
			state = $4,
			initial_search_radius_km = $5,
			max_search_radius_km = $6,
			radius_expand_interval_seconds = $7,
			base_delivery_fee = $8,
			per_km_fee = $9,
			is_active = $10
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query,
		zone.ID,
		zone.Name,
		zone.City,
		zone.State,
		zone.InitialSearchRadiusKM,
		zone.MaxSearchRadiusKM,
		zone.RadiusExpandIntervalSeconds,
		zone.BaseDeliveryFee,
		zone.PerKMFee,
		zone.IsActive,
	)

	return err
}
