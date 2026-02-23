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
type UserRepo struct {
	db *sqlx.DB
}

func NewUserRepo(db *sqlx.DB) repository.UserRepository {
	return &UserRepo{db: db}
}

func (r *UserRepo) Create(ctx context.Context, user *domain.User) error {
	query := `
		INSERT INTO users (id, phone_number, phone_verified, email, first_name, last_name, profile_photo_url, is_seller, is_buyer, is_rider)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING created_at, updated_at`

	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}

	return r.db.QueryRowxContext(ctx, query,
		user.ID,
		user.PhoneNumber,
		user.PhoneVerified,
		user.Email,
		user.FirstName,
		user.LastName,
		user.ProfilePhotoURL,
		user.IsSeller,
		user.IsBuyer,
		user.IsRider,
	).Scan(&user.CreatedAt, &user.UpdatedAt)
}

func (r *UserRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	var user domain.User
	query := `SELECT * FROM users WHERE id = $1`

	err := r.db.GetContext(ctx, &user, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}

	return &user, nil
}

func (r *UserRepo) GetByPhoneNumber(ctx context.Context, phoneNumber string) (*domain.User, error) {
	var user domain.User
	query := `SELECT * FROM users WHERE phone_number = $1`

	err := r.db.GetContext(ctx, &user, query, phoneNumber)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}

	return &user, nil
}

func (r *UserRepo) Update(ctx context.Context, user *domain.User) error {
	query := `
		UPDATE users
		SET first_name = $2, last_name = $3, email = $4, profile_photo_url = $5,
		    is_seller = $6, is_buyer = $7, is_rider = $8, updated_at = NOW()
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query,
		user.ID,
		user.FirstName,
		user.LastName,
		user.Email,
		user.ProfilePhotoURL,
		user.IsSeller,
		user.IsBuyer,
		user.IsRider,
	)
	return err
}

func (r *UserRepo) UpdateLastLogin(ctx context.Context, userID uuid.UUID) error {
	query := `UPDATE users SET last_login_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, userID)
	return err
}

func (r *UserRepo) SetPhoneVerified(ctx context.Context, userID uuid.UUID) error {
	query := `UPDATE users SET phone_verified = true WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, userID)
	return err
}

func (r *UserRepo) SetIsSeller(ctx context.Context, userID uuid.UUID) error {
	query := `UPDATE users SET is_seller = true, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, userID)
	return err
}

func (r *UserRepo) SetIsBuyer(ctx context.Context, userID uuid.UUID) error {
	query := `UPDATE users SET is_buyer = true, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, userID)
	return err
}

func (r *UserRepo) SetIsRider(ctx context.Context, userID uuid.UUID) error {
	query := `UPDATE users SET is_rider = true, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, userID)
	return err
}

// Admin methods

func (r *UserRepo) Suspend(ctx context.Context, userID uuid.UUID, reason string) error {
	query := `UPDATE users SET is_suspended = true, suspension_reason = $2, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, userID, reason)
	return err
}

func (r *UserRepo) Unsuspend(ctx context.Context, userID uuid.UUID) error {
	query := `UPDATE users SET is_suspended = false, suspension_reason = NULL, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, userID)
	return err
}

func (r *UserRepo) VerifySeller(ctx context.Context, userID uuid.UUID) error {
	query := `UPDATE users SET seller_verified = true, seller_verified_at = NOW(), updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, userID)
	return err
}

func (r *UserRepo) List(ctx context.Context, filters interface{}, limit, offset int) ([]*domain.User, error) {
	query := `SELECT * FROM users ORDER BY created_at DESC LIMIT $1 OFFSET $2`
	var users []*domain.User
	err := r.db.SelectContext(ctx, &users, query, limit, offset)
	if err != nil {
		return nil, err
	}
	return users, nil
}

func (r *UserRepo) GetStatistics(ctx context.Context) (*repository.UserStatistics, error) {
	query := `
		SELECT
			COUNT(*) as total_users,
			COUNT(*) FILTER (WHERE is_seller = true) as total_sellers,
			COUNT(*) FILTER (WHERE is_buyer = true) as total_buyers,
			COUNT(*) FILTER (WHERE is_rider = true) as total_riders,
			COUNT(*) FILTER (WHERE is_suspended = true) as suspended_users
		FROM users
	`
	var stats repository.UserStatistics
	err := r.db.GetContext(ctx, &stats, query)
	if err != nil {
		return nil, err
	}
	return &stats, nil
}

func (r *UserRepo) SoftDelete(ctx context.Context, userID uuid.UUID) error {
	query := `
		UPDATE users SET
			first_name = 'Deleted',
			last_name = 'User',
			email = NULL,
			profile_photo_url = NULL,
			is_suspended = true,
			suspension_reason = 'Account deleted by user',
			updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, userID)
	return err
}

type OTPRepo struct {
	db *sqlx.DB
}

func NewOTPRepo(db *sqlx.DB) repository.OTPRepository {
	return &OTPRepo{db: db}
}

func (r *OTPRepo) Create(ctx context.Context, otp *domain.OTPVerification) error {
	query := `
		INSERT INTO otp_verifications (id, phone_number, verification_id, code_hash, purpose, expires_at, attempts)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at`

	if otp.ID == uuid.Nil {
		otp.ID = uuid.New()
	}

	return r.db.QueryRowxContext(ctx, query,
		otp.ID,
		otp.PhoneNumber,
		otp.VerificationID,
		otp.CodeHash,
		otp.Purpose,
		otp.ExpiresAt,
		otp.Attempts,
	).Scan(&otp.CreatedAt)
}

func (r *OTPRepo) GetByVerificationID(ctx context.Context, phoneNumber string, verificationID uuid.UUID) (*domain.OTPVerification, error) {
	var otp domain.OTPVerification
	query := `SELECT * FROM otp_verifications WHERE phone_number = $1 AND verification_id = $2`

	err := r.db.GetContext(ctx, &otp, query, phoneNumber, verificationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrOTPNotFound
		}
		return nil, err
	}

	return &otp, nil
}

func (r *OTPRepo) IncrementAttempts(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE otp_verifications SET attempts = attempts + 1 WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

func (r *OTPRepo) MarkVerified(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE otp_verifications SET verified_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

func (r *OTPRepo) GetRecentCount(ctx context.Context, phoneNumber string, since time.Time) (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM otp_verifications WHERE phone_number = $1 AND created_at >= $2`
	err := r.db.GetContext(ctx, &count, query, phoneNumber, since)
	return count, err
}

func (r *OTPRepo) DeleteExpired(ctx context.Context) (int64, error) {
	query := `DELETE FROM otp_verifications WHERE expires_at < NOW() AND verified_at IS NULL`
	result, err := r.db.ExecContext(ctx, query)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
type IdempotencyRepo struct {
	db *sqlx.DB
}

func NewIdempotencyRepo(db *sqlx.DB) repository.IdempotencyRepository {
	return &IdempotencyRepo{db: db}
}

func (r *IdempotencyRepo) Create(ctx context.Context, key *domain.IdempotencyKey) error {
	query := `
		INSERT INTO idempotency_keys (id, key, operation, request_hash)
		VALUES ($1, $2, $3, $4)
		RETURNING created_at`

	if key.ID == uuid.Nil {
		key.ID = uuid.New()
	}

	err := r.db.QueryRowxContext(ctx, query,
		key.ID,
		key.Key,
		key.Operation,
		key.RequestHash,
	).Scan(&key.CreatedAt)

	if err != nil {
		if isUniqueViolation(err) {
			return domain.ErrIdempotencyKeyExists
		}
		return err
	}

	return nil
}

func (r *IdempotencyRepo) Exists(ctx context.Context, key string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM idempotency_keys WHERE key = $1)`
	err := r.db.GetContext(ctx, &exists, query, key)
	return exists, err
}

func (r *IdempotencyRepo) GetByKey(ctx context.Context, key string) (*domain.IdempotencyKey, error) {
	var idempotencyKey domain.IdempotencyKey
	query := `SELECT * FROM idempotency_keys WHERE key = $1`

	err := r.db.GetContext(ctx, &idempotencyKey, query, key)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &idempotencyKey, nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return len(errStr) >= 23 && errStr[:23] == "pq: duplicate key value"
}
