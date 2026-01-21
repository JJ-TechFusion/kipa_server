package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"kipa/internal/domain"
)

type PlatformSettingsRepo struct {
	db *sqlx.DB
}

func NewPlatformSettingsRepo(db *sqlx.DB) *PlatformSettingsRepo {
	return &PlatformSettingsRepo{db: db}
}

func (r *PlatformSettingsRepo) Get(ctx context.Context, key string) (*domain.PlatformSettings, error) {
	var settings domain.PlatformSettings
	query := `
		SELECT id, setting_key, setting_value, description, updated_by, created_at, updated_at
		FROM platform_settings
		WHERE setting_key = $1
	`
	err := r.db.GetContext(ctx, &settings, query, key)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("setting not found: %s", key)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get setting: %w", err)
	}
	return &settings, nil
}

func (r *PlatformSettingsRepo) Set(ctx context.Context, key string, value []byte, description *string, updatedBy *uuid.UUID) error {
	query := `
		INSERT INTO platform_settings (setting_key, setting_value, description, updated_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (setting_key)
		DO UPDATE SET
			setting_value = EXCLUDED.setting_value,
			description = EXCLUDED.description,
			updated_by = EXCLUDED.updated_by,
			updated_at = NOW()
	`
	_, err := r.db.ExecContext(ctx, query, key, value, description, updatedBy)
	if err != nil {
		return fmt.Errorf("failed to set setting: %w", err)
	}
	return nil
}

func (r *PlatformSettingsRepo) GetAll(ctx context.Context) ([]*domain.PlatformSettings, error) {
	var settings []*domain.PlatformSettings
	query := `
		SELECT id, setting_key, setting_value, description, updated_by, created_at, updated_at
		FROM platform_settings
		ORDER BY setting_key
	`
	err := r.db.SelectContext(ctx, &settings, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all settings: %w", err)
	}
	return settings, nil
}

func (r *PlatformSettingsRepo) Delete(ctx context.Context, key string) error {
	query := `DELETE FROM platform_settings WHERE setting_key = $1`
	result, err := r.db.ExecContext(ctx, query, key)
	if err != nil {
		return fmt.Errorf("failed to delete setting: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("setting not found: %s", key)
	}
	return nil
}
