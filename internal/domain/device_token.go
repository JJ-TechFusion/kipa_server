package domain

import (
	"time"

	"github.com/google/uuid"
)

type DevicePlatform string

const (
	PlatformAndroid DevicePlatform = "android"
	PlatformIOS     DevicePlatform = "ios"
	PlatformWeb     DevicePlatform = "web"
)

type DeviceToken struct {
	ID         uuid.UUID      `db:"id" json:"id"`
	UserID     uuid.UUID      `db:"user_id" json:"user_id"`
	Token      string         `db:"token" json:"token"`
	Platform   DevicePlatform `db:"platform" json:"platform"`
	LastUsedAt time.Time      `db:"last_used_at" json:"last_used_at"`
	CreatedAt  time.Time      `db:"created_at" json:"created_at"`
}
