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

type DriverOfferRepo struct {
	db *sqlx.DB
}

func NewDriverOfferRepo(db *sqlx.DB) repository.DriverOfferRepository {
	return &DriverOfferRepo{db: db}
}

func (r *DriverOfferRepo) Create(ctx context.Context, offer *domain.DriverOffer) error {
	query := `
		INSERT INTO driver_offers (
			id, delivery_id, driver_id, offered_amount, counter_amount,
			agreed_amount, status, expires_at, counter_expires_at,
			countered_at, responded_at, driver_note, seller_note, offer_count
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING created_at, updated_at`

	if offer.ID == uuid.Nil {
		offer.ID = uuid.New()
	}

	return r.db.QueryRowxContext(ctx, query,
		offer.ID,
		offer.DeliveryID,
		offer.DriverID,
		offer.OfferedAmount,
		offer.CounterAmount,
		offer.AgreedAmount,
		offer.Status,
		offer.ExpiresAt,
		offer.CounterExpiresAt,
		offer.CounteredAt,
		offer.RespondedAt,
		offer.DriverNote,
		offer.SellerNote,
		offer.OfferCount,
	).Scan(&offer.CreatedAt, &offer.UpdatedAt)
}

func (r *DriverOfferRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.DriverOffer, error) {
	var offer domain.DriverOffer
	query := `SELECT * FROM driver_offers WHERE id = $1`

	err := r.db.GetContext(ctx, &offer, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrOfferNotFound
		}
		return nil, err
	}

	return &offer, nil
}

func (r *DriverOfferRepo) Update(ctx context.Context, offer *domain.DriverOffer) error {
	query := `
		UPDATE driver_offers SET
			offered_amount = $2,
			counter_amount = $3,
			agreed_amount = $4,
			status = $5,
			expires_at = $6,
			counter_expires_at = $7,
			countered_at = $8,
			responded_at = $9,
			driver_note = $10,
			seller_note = $11,
			offer_count = $12,
			updated_at = NOW()
		WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query,
		offer.ID,
		offer.OfferedAmount,
		offer.CounterAmount,
		offer.AgreedAmount,
		offer.Status,
		offer.ExpiresAt,
		offer.CounterExpiresAt,
		offer.CounteredAt,
		offer.RespondedAt,
		offer.DriverNote,
		offer.SellerNote,
		offer.OfferCount,
	)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrOfferNotFound
	}

	return nil
}

func (r *DriverOfferRepo) GetByDeliveryID(ctx context.Context, deliveryID uuid.UUID) ([]*domain.DriverOffer, error) {
	var offers []*domain.DriverOffer
	query := `
		SELECT * FROM driver_offers
		WHERE delivery_id = $1
		ORDER BY created_at DESC`

	err := r.db.SelectContext(ctx, &offers, query, deliveryID)
	if err != nil {
		return nil, err
	}

	return offers, nil
}

func (r *DriverOfferRepo) GetByDriverID(ctx context.Context, driverID uuid.UUID, status *domain.DriverOfferStatus) ([]*domain.DriverOffer, error) {
	var offers []*domain.DriverOffer
	var query string
	var args []interface{}

	if status != nil {
		query = `
			SELECT * FROM driver_offers
			WHERE driver_id = $1 AND status = $2
			ORDER BY created_at DESC`
		args = []interface{}{driverID, *status}
	} else {
		query = `
			SELECT * FROM driver_offers
			WHERE driver_id = $1
			ORDER BY created_at DESC`
		args = []interface{}{driverID}
	}

	err := r.db.SelectContext(ctx, &offers, query, args...)
	if err != nil {
		return nil, err
	}

	return offers, nil
}

func (r *DriverOfferRepo) GetPendingByDeliveryAndDriver(ctx context.Context, deliveryID, driverID uuid.UUID) (*domain.DriverOffer, error) {
	var offer domain.DriverOffer
	query := `
		SELECT * FROM driver_offers
		WHERE delivery_id = $1 AND driver_id = $2
		AND status IN ('pending', 'countered')
		ORDER BY created_at DESC
		LIMIT 1`

	err := r.db.GetContext(ctx, &offer, query, deliveryID, driverID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // No pending offer found is not an error
		}
		return nil, err
	}

	return &offer, nil
}

func (r *DriverOfferRepo) GetOfferCountByDeliveryAndDriver(ctx context.Context, deliveryID, driverID uuid.UUID) (int, error) {
	var count int
	query := `
		SELECT COALESCE(MAX(offer_count), 0)
		FROM driver_offers
		WHERE delivery_id = $1 AND driver_id = $2`

	err := r.db.GetContext(ctx, &count, query, deliveryID, driverID)
	if err != nil {
		return 0, err
	}

	return count, nil
}

func (r *DriverOfferRepo) RejectAllPendingExcept(ctx context.Context, deliveryID uuid.UUID, exceptOfferID uuid.UUID) error {
	query := `
		UPDATE driver_offers SET
			status = 'rejected',
			responded_at = NOW(),
			seller_note = 'Another offer was accepted',
			updated_at = NOW()
		WHERE delivery_id = $1
		AND id != $2
		AND status = 'pending'`

	_, err := r.db.ExecContext(ctx, query, deliveryID, exceptOfferID)
	return err
}

func (r *DriverOfferRepo) ExpireStaleOffers(ctx context.Context) (int64, error) {
	query := `
		UPDATE driver_offers SET
			status = 'expired',
			updated_at = NOW()
		WHERE status = 'pending'
		AND expires_at < NOW()`

	result, err := r.db.ExecContext(ctx, query)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

func (r *DriverOfferRepo) ExpireStaleCounterOffers(ctx context.Context) (int64, error) {
	query := `
		UPDATE driver_offers SET
			status = 'expired',
			updated_at = NOW()
		WHERE status = 'countered'
		AND counter_expires_at IS NOT NULL
		AND counter_expires_at < NOW()`

	result, err := r.db.ExecContext(ctx, query)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}
