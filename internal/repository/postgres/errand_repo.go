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
type ErrandRepo struct {
	db *sqlx.DB
}

func NewErrandRepo(db *sqlx.DB) repository.ErrandRepository {
	return &ErrandRepo{db: db}
}

func (r *ErrandRepo) Create(ctx context.Context, errand *domain.Errand) error {
	query := `
		INSERT INTO errands (
			id, user_id,
			pickup_address, pickup_lat, pickup_lng,
			pickup_contact_name, pickup_contact_phone, pickup_instructions,
			dropoff_address, dropoff_lat, dropoff_lng,
			dropoff_contact_name, dropoff_contact_phone,
			package_description, package_weight_kg, package_photo_url,
			vehicle_type, distance_km, delivery_fee,
			status, dropoff_code
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
		RETURNING created_at, updated_at`

	if errand.ID == uuid.Nil {
		errand.ID = uuid.New()
	}

	return r.db.QueryRowxContext(ctx, query,
		errand.ID,
		errand.UserID,
		errand.PickupAddress,
		errand.PickupLat,
		errand.PickupLng,
		errand.PickupContactName,
		errand.PickupContactPhone,
		errand.PickupInstructions,
		errand.DropoffAddress,
		errand.DropoffLat,
		errand.DropoffLng,
		errand.DropoffContactName,
		errand.DropoffContactPhone,
		errand.PackageDescription,
		errand.PackageWeightKg,
		errand.PackagePhotoURL,
		errand.VehicleType,
		errand.DistanceKM,
		errand.DeliveryFee,
		errand.Status,
		errand.DropoffCode,
	).Scan(&errand.CreatedAt, &errand.UpdatedAt)
}

func (r *ErrandRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Errand, error) {
	var errand domain.Errand
	query := `SELECT * FROM errands WHERE id = $1`

	err := r.db.GetContext(ctx, &errand, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrErrandNotFound
		}
		return nil, err
	}

	return &errand, nil
}

func (r *ErrandRepo) GetByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.Errand, error) {
	var errands []*domain.Errand
	query := `
		SELECT * FROM errands 
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	err := r.db.SelectContext(ctx, &errands, query, userID, limit, offset)
	if err != nil {
		return nil, err
	}

	return errands, nil
}

func (r *ErrandRepo) GetActiveByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.Errand, error) {
	var errands []*domain.Errand
	query := `
		SELECT * FROM errands 
		WHERE user_id = $1 
		AND status IN ('searching', 'accepted', 'picked_up', 'in_transit')
		ORDER BY created_at DESC`

	err := r.db.SelectContext(ctx, &errands, query, userID)
	if err != nil {
		return nil, err
	}

	return errands, nil
}

func (r *ErrandRepo) Update(ctx context.Context, errand *domain.Errand) error {
	errand.UpdatedAt = time.Now()
	query := `
		UPDATE errands SET
			pickup_address = $2,
			pickup_lat = $3,
			pickup_lng = $4,
			pickup_contact_name = $5,
			pickup_contact_phone = $6,
			pickup_instructions = $7,
			dropoff_address = $8,
			dropoff_lat = $9,
			dropoff_lng = $10,
			dropoff_contact_name = $11,
			dropoff_contact_phone = $12,
			package_description = $13,
			package_weight_kg = $14,
			package_photo_url = $15,
			vehicle_type = $16,
			distance_km = $17,
			delivery_fee = $18,
			status = $19,
			dropoff_code = $20,
			rider_id = $21,
			delivery_job_id = $22,
			confirmed_at = $23,
			accepted_at = $24,
			picked_up_at = $25,
			delivered_at = $26,
			cancelled_at = $27,
			updated_at = $28
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query,
		errand.ID,
		errand.PickupAddress,
		errand.PickupLat,
		errand.PickupLng,
		errand.PickupContactName,
		errand.PickupContactPhone,
		errand.PickupInstructions,
		errand.DropoffAddress,
		errand.DropoffLat,
		errand.DropoffLng,
		errand.DropoffContactName,
		errand.DropoffContactPhone,
		errand.PackageDescription,
		errand.PackageWeightKg,
		errand.PackagePhotoURL,
		errand.VehicleType,
		errand.DistanceKM,
		errand.DeliveryFee,
		errand.Status,
		errand.DropoffCode,
		errand.RiderID,
		errand.DeliveryJobID,
		errand.ConfirmedAt,
		errand.AcceptedAt,
		errand.PickedUpAt,
		errand.DeliveredAt,
		errand.CancelledAt,
		errand.UpdatedAt,
	)

	return err
}

func (r *ErrandRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.ErrandStatus) error {
	query := `UPDATE errands SET status = $2, updated_at = NOW() WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, id, status)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrErrandNotFound
	}

	return nil
}

func (r *ErrandRepo) AssignRider(ctx context.Context, errandID, riderID, jobID uuid.UUID) error {
	now := time.Now()
	query := `
		UPDATE errands SET 
			rider_id = $2,
			delivery_job_id = $3,
			status = 'accepted',
			accepted_at = $4,
			updated_at = $4
		WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query, errandID, riderID, jobID, now)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrErrandNotFound
	}

	return nil
}

func (r *ErrandRepo) SetDeliveryJobID(ctx context.Context, errandID, jobID uuid.UUID) error {
	query := `UPDATE errands SET delivery_job_id = $2, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, errandID, jobID)
	return err
}
