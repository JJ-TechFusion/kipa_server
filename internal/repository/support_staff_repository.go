package repository

import (
	"context"

	"kipa/internal/domain"

	"github.com/google/uuid"
)

type SupportStaffRepository interface {
	Create(ctx context.Context, staff *domain.SupportStaff) error
	GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.SupportStaff, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.SupportStaff, error)
	Update(ctx context.Context, staff *domain.SupportStaff) error
	Delete(ctx context.Context, userID uuid.UUID) error
	List(ctx context.Context, activeOnly bool, limit, offset int) ([]*domain.SupportStaff, int, error)
	ExistsByUserID(ctx context.Context, userID uuid.UUID) (bool, error)
}
