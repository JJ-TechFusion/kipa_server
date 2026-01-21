package postgres

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"kipa/internal/domain"
	"kipa/internal/repository"
)

type AuditLogRepo struct {
	db *sqlx.DB
}

func NewAuditLogRepo(db *sqlx.DB) *AuditLogRepo {
	return &AuditLogRepo{db: db}
}

func (r *AuditLogRepo) Create(ctx context.Context, log *domain.AuditLog) error {
	var detailsJSON []byte
	var err error
	if log.Details != nil {
		detailsJSON, err = json.Marshal(log.Details)
		if err != nil {
			return err
		}
	}

	query := `
		INSERT INTO admin_audit_logs (id, admin_id, action, target_type, target_id, details, ip_address, user_agent, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err = r.db.ExecContext(ctx, query,
		log.ID,
		log.AdminID,
		log.Action,
		log.TargetType,
		log.TargetID,
		detailsJSON,
		log.IPAddress,
		log.UserAgent,
		log.CreatedAt,
	)

	return err
}

func (r *AuditLogRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.AuditLog, error) {
	query := `
		SELECT id, admin_id, action, target_type, target_id, details, ip_address, user_agent, created_at
		FROM admin_audit_logs
		WHERE id = $1
	`

	var log domain.AuditLog
	var detailsJSON []byte
	var ipAddress, userAgent sql.NullString

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&log.ID,
		&log.AdminID,
		&log.Action,
		&log.TargetType,
		&log.TargetID,
		&detailsJSON,
		&ipAddress,
		&userAgent,
		&log.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if len(detailsJSON) > 0 {
		if err := json.Unmarshal(detailsJSON, &log.Details); err != nil {
			return nil, err
		}
	}

	log.IPAddress = ipAddress.String
	log.UserAgent = userAgent.String

	return &log, nil
}

func (r *AuditLogRepo) List(ctx context.Context, filters *repository.AuditLogFilters, limit, offset int) ([]*domain.AuditLog, error) {
	query := `
		SELECT id, admin_id, action, target_type, target_id, details, ip_address, user_agent, created_at
		FROM admin_audit_logs
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanRows(rows)
}

func (r *AuditLogRepo) GetByAdmin(ctx context.Context, adminID uuid.UUID, limit, offset int) ([]*domain.AuditLog, error) {
	query := `
		SELECT id, admin_id, action, target_type, target_id, details, ip_address, user_agent, created_at
		FROM admin_audit_logs
		WHERE admin_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryContext(ctx, query, adminID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanRows(rows)
}

func (r *AuditLogRepo) GetByTarget(ctx context.Context, targetType domain.AuditTargetType, targetID uuid.UUID, limit, offset int) ([]*domain.AuditLog, error) {
	query := `
		SELECT id, admin_id, action, target_type, target_id, details, ip_address, user_agent, created_at
		FROM admin_audit_logs
		WHERE target_type = $1 AND target_id = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`

	rows, err := r.db.QueryContext(ctx, query, targetType, targetID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanRows(rows)
}

func (r *AuditLogRepo) scanRows(rows *sql.Rows) ([]*domain.AuditLog, error) {
	var logs []*domain.AuditLog

	for rows.Next() {
		var log domain.AuditLog
		var detailsJSON []byte
		var ipAddress, userAgent sql.NullString

		err := rows.Scan(
			&log.ID,
			&log.AdminID,
			&log.Action,
			&log.TargetType,
			&log.TargetID,
			&detailsJSON,
			&ipAddress,
			&userAgent,
			&log.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		if len(detailsJSON) > 0 {
			if err := json.Unmarshal(detailsJSON, &log.Details); err != nil {
				return nil, err
			}
		}

		log.IPAddress = ipAddress.String
		log.UserAgent = userAgent.String

		logs = append(logs, &log)
	}

	return logs, rows.Err()
}

var _ repository.AuditLogRepository = (*AuditLogRepo)(nil)
