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

type SupportStaffRepo struct {
	db *sqlx.DB
}

func NewSupportStaffRepo(db *sqlx.DB) repository.SupportStaffRepository {
	return &SupportStaffRepo{db: db}
}

func (r *SupportStaffRepo) Create(ctx context.Context, staff *domain.SupportStaff) error {
	query := `
		INSERT INTO support_staff (
			id, user_id, added_by, can_reply_chats, can_view_disputes,
			can_resolve_chats, can_message_users, is_active
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at, updated_at`

	if staff.ID == uuid.Nil {
		staff.ID = uuid.New()
	}

	return r.db.QueryRowxContext(ctx, query,
		staff.ID,
		staff.UserID,
		staff.AddedBy,
		staff.CanReplyChats,
		staff.CanViewDisputes,
		staff.CanResolveChats,
		staff.CanMessageUsers,
		staff.IsActive,
	).Scan(&staff.CreatedAt, &staff.UpdatedAt)
}

func (r *SupportStaffRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.SupportStaff, error) {
	var staff domain.SupportStaff
	query := `SELECT * FROM support_staff WHERE user_id = $1`

	err := r.db.GetContext(ctx, &staff, query, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrSupportStaffNotFound
		}
		return nil, err
	}

	return &staff, nil
}

func (r *SupportStaffRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.SupportStaff, error) {
	var staff domain.SupportStaff
	query := `SELECT * FROM support_staff WHERE id = $1`

	err := r.db.GetContext(ctx, &staff, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrSupportStaffNotFound
		}
		return nil, err
	}

	return &staff, nil
}

func (r *SupportStaffRepo) Update(ctx context.Context, staff *domain.SupportStaff) error {
	query := `
		UPDATE support_staff
		SET can_reply_chats = $2, can_view_disputes = $3, can_resolve_chats = $4,
		    can_message_users = $5, is_active = $6, updated_at = NOW()
		WHERE user_id = $1`

	result, err := r.db.ExecContext(ctx, query,
		staff.UserID,
		staff.CanReplyChats,
		staff.CanViewDisputes,
		staff.CanResolveChats,
		staff.CanMessageUsers,
		staff.IsActive,
	)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrSupportStaffNotFound
	}

	return nil
}

func (r *SupportStaffRepo) Delete(ctx context.Context, userID uuid.UUID) error {
	query := `DELETE FROM support_staff WHERE user_id = $1`

	result, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrSupportStaffNotFound
	}

	return nil
}

func (r *SupportStaffRepo) List(ctx context.Context, activeOnly bool, limit, offset int) ([]*domain.SupportStaff, int, error) {
	var staff []*domain.SupportStaff
	var total int

	baseQuery := `FROM support_staff`
	if activeOnly {
		baseQuery += ` WHERE is_active = TRUE`
	}

	countQuery := `SELECT COUNT(*) ` + baseQuery
	if err := r.db.GetContext(ctx, &total, countQuery); err != nil {
		return nil, 0, err
	}

	selectQuery := `SELECT * ` + baseQuery + ` ORDER BY created_at DESC LIMIT $1 OFFSET $2`
	if err := r.db.SelectContext(ctx, &staff, selectQuery, limit, offset); err != nil {
		return nil, 0, err
	}

	return staff, total, nil
}

func (r *SupportStaffRepo) ExistsByUserID(ctx context.Context, userID uuid.UUID) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM support_staff WHERE user_id = $1)`

	err := r.db.GetContext(ctx, &exists, query, userID)
	return exists, err
}
