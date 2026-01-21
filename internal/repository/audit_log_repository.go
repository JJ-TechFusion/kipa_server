package repository

import (
	"context"

	"github.com/google/uuid"

	"kipa/internal/domain"
)

type AuditLogRepository interface {
	Create(ctx context.Context, log *domain.AuditLog) error

	GetByID(ctx context.Context, id uuid.UUID) (*domain.AuditLog, error)

	List(ctx context.Context, filters *AuditLogFilters, limit, offset int) ([]*domain.AuditLog, error)

	GetByAdmin(ctx context.Context, adminID uuid.UUID, limit, offset int) ([]*domain.AuditLog, error)
	GetByTarget(ctx context.Context, targetType domain.AuditTargetType, targetID uuid.UUID, limit, offset int) ([]*domain.AuditLog, error)
}

type AuditLogFilters struct {
	AdminID    *uuid.UUID
	Action     *domain.AuditAction
	TargetType *domain.AuditTargetType
	TargetID   *uuid.UUID
	StartDate  *string
	EndDate    *string
}
