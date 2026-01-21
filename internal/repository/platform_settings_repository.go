package repository

import (
	"context"

	"github.com/google/uuid"

	"kipa/internal/domain"
)

type PlatformSettingsRepository interface {
	Get(ctx context.Context, key string) (*domain.PlatformSettings, error)

	Set(ctx context.Context, key string, value []byte, description *string, updatedBy *uuid.UUID) error

	GetAll(ctx context.Context) ([]*domain.PlatformSettings, error)

	Delete(ctx context.Context, key string) error
}
