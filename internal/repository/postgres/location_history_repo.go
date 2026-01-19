package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"kipa/internal/domain"
)

// LocationHistoryRepo handles location history persistence
type LocationHistoryRepo struct {
	db *sqlx.DB
}

// NewLocationHistoryRepo creates a new location history repo
func NewLocationHistoryRepo(db *sqlx.DB) *LocationHistoryRepo {
	return &LocationHistoryRepo{db: db}
}

// Create saves a location record
func (r *LocationHistoryRepo) Create(ctx context.Context, record *domain.LocationRecord) error {
	query := `
		INSERT INTO rider_location_history (
			id, rider_id, delivery_job_id, lat, lng, heading, speed,
			eta_minutes, distance_remaining_km, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	if record.ID == uuid.Nil {
		record.ID = uuid.New()
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now()
	}

	_, err := r.db.ExecContext(ctx, query,
		record.ID,
		record.RiderID,
		record.DeliveryJobID,
		record.Lat,
		record.Lng,
		record.Heading,
		record.Speed,
		record.ETAMinutes,
		record.DistanceRemainingKM,
		record.CreatedAt,
	)
	return err
}

// GetByJobID gets location history for a job
func (r *LocationHistoryRepo) GetByJobID(ctx context.Context, jobID uuid.UUID, limit int) ([]*domain.LocationRecord, error) {
	var records []*domain.LocationRecord
	query := `
		SELECT id, rider_id, delivery_job_id, lat, lng, heading, speed,
		       eta_minutes, distance_remaining_km, created_at
		FROM rider_location_history
		WHERE delivery_job_id = $1
		ORDER BY created_at DESC
		LIMIT $2`

	rows, err := r.db.QueryxContext(ctx, query, jobID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var rec domain.LocationRecord
		if err := rows.StructScan(&rec); err != nil {
			return nil, err
		}
		records = append(records, &rec)
	}

	return records, nil
}

// GetLatestByJobID gets the most recent location for a job
func (r *LocationHistoryRepo) GetLatestByJobID(ctx context.Context, jobID uuid.UUID) (*domain.LocationRecord, error) {
	var rec domain.LocationRecord
	query := `
		SELECT id, rider_id, delivery_job_id, lat, lng, heading, speed,
		       eta_minutes, distance_remaining_km, created_at
		FROM rider_location_history
		WHERE delivery_job_id = $1
		ORDER BY created_at DESC
		LIMIT 1`

	err := r.db.GetContext(ctx, &rec, query, jobID)
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// DeleteOlderThan deletes records older than the given time
func (r *LocationHistoryRepo) DeleteOlderThan(ctx context.Context, before time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx,
		"DELETE FROM rider_location_history WHERE created_at < $1",
		before,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
