package repository

import (
	"context"

	"github.com/google/uuid"

	"kipa/internal/domain"
)

type DeviceTokenRepository interface {
	Save(ctx context.Context, token *domain.DeviceToken) error
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.DeviceToken, error)

	Delete(ctx context.Context, token string) error
	DeleteByUserID(ctx context.Context, userID uuid.UUID) error
	UpdateLastUsed(ctx context.Context, token string) error
}

type NotificationLogRepository interface {
	Create(ctx context.Context, log *domain.NotificationLog) error
	CreateBulk(ctx context.Context, logs []*domain.NotificationLog) error
	GetByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.NotificationLog, error)
}
