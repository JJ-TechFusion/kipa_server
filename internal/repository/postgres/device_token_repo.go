package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"kipa/internal/domain"
	"kipa/internal/repository"
)

type DeviceTokenRepo struct {
	db *sqlx.DB
}

func NewDeviceTokenRepo(db *sqlx.DB) repository.DeviceTokenRepository {
	return &DeviceTokenRepo{db: db}
}

func (r *DeviceTokenRepo) Save(ctx context.Context, token *domain.DeviceToken) error {
	if token.ID == uuid.Nil {
		token.ID = uuid.New()
	}
	token.CreatedAt = time.Now()
	token.LastUsedAt = time.Now()

	query := `
		INSERT INTO user_device_tokens (id, user_id, token, platform, last_used_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id, token) DO UPDATE SET
			platform = EXCLUDED.platform,
			last_used_at = NOW()
	`

	_, err := r.db.ExecContext(ctx, query,
		token.ID,
		token.UserID,
		token.Token,
		token.Platform,
		token.LastUsedAt,
		token.CreatedAt,
	)
	return err
}

func (r *DeviceTokenRepo) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.DeviceToken, error) {
	var tokens []*domain.DeviceToken
	query := `SELECT * FROM user_device_tokens WHERE user_id = $1 ORDER BY last_used_at DESC`

	err := r.db.SelectContext(ctx, &tokens, query, userID)
	if err != nil {
		return nil, err
	}
	return tokens, nil
}

func (r *DeviceTokenRepo) Delete(ctx context.Context, token string) error {
	query := `DELETE FROM user_device_tokens WHERE token = $1`
	_, err := r.db.ExecContext(ctx, query, token)
	return err
}

func (r *DeviceTokenRepo) DeleteByUserID(ctx context.Context, userID uuid.UUID) error {
	query := `DELETE FROM user_device_tokens WHERE user_id = $1`
	_, err := r.db.ExecContext(ctx, query, userID)
	return err
}

func (r *DeviceTokenRepo) UpdateLastUsed(ctx context.Context, token string) error {
	query := `UPDATE user_device_tokens SET last_used_at = NOW() WHERE token = $1`
	_, err := r.db.ExecContext(ctx, query, token)
	return err
}

type NotificationLogRepo struct {
	db *sqlx.DB
}

func NewNotificationLogRepo(db *sqlx.DB) repository.NotificationLogRepository {
	return &NotificationLogRepo{db: db}
}

func (r *NotificationLogRepo) Create(ctx context.Context, log *domain.NotificationLog) error {
	if log.ID == uuid.Nil {
		log.ID = uuid.New()
	}
	log.SentAt = time.Now()

	query := `
		INSERT INTO notification_logs (id, user_id, title, body, data, notification_type, sent_at, delivered)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err := r.db.ExecContext(ctx, query,
		log.ID,
		log.UserID,
		log.Title,
		log.Body,
		nil, // Data stored as JSONB - handle separately if needed
		log.NotificationType,
		log.SentAt,
		log.Delivered,
	)
	return err
}

func (r *NotificationLogRepo) CreateBulk(ctx context.Context, logs []*domain.NotificationLog) error {
	if len(logs) == 0 {
		return nil
	}

	now := time.Now()
	query := `
		INSERT INTO notification_logs (id, user_id, title, body, notification_type, sent_at, delivered)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	// Use a transaction for bulk insert
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, log := range logs {
		if log.ID == uuid.Nil {
			log.ID = uuid.New()
		}
		log.SentAt = now
		log.Delivered = true // Stored in DB = delivered

		_, err := tx.ExecContext(ctx, query,
			log.ID,
			log.UserID,
			log.Title,
			log.Body,
			log.NotificationType,
			log.SentAt,
			log.Delivered,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *NotificationLogRepo) GetByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.NotificationLog, error) {
	var logs []*domain.NotificationLog
	query := `SELECT * FROM notification_logs WHERE user_id = $1 ORDER BY sent_at DESC LIMIT $2 OFFSET $3`

	err := r.db.SelectContext(ctx, &logs, query, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	return logs, nil
}

var _ repository.DeviceTokenRepository = (*DeviceTokenRepo)(nil)
var _ repository.NotificationLogRepository = (*NotificationLogRepo)(nil)
