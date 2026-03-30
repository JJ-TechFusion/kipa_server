package service

import (
	"context"
	"testing"
	"time"

	"kipa/internal/domain"
	"kipa/internal/repository"

	"github.com/google/uuid"
)

type authTestUserRepo struct {
	getByPhoneNumber func(ctx context.Context, phoneNumber string) (*domain.User, error)
}

func (r *authTestUserRepo) Create(ctx context.Context, user *domain.User) error { return nil }
func (r *authTestUserRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	return nil, domain.ErrUserNotFound
}
func (r *authTestUserRepo) GetByPhoneNumber(ctx context.Context, phoneNumber string) (*domain.User, error) {
	if r.getByPhoneNumber != nil {
		return r.getByPhoneNumber(ctx, phoneNumber)
	}
	return nil, domain.ErrUserNotFound
}
func (r *authTestUserRepo) Update(ctx context.Context, user *domain.User) error          { return nil }
func (r *authTestUserRepo) UpdateLastLogin(ctx context.Context, userID uuid.UUID) error  { return nil }
func (r *authTestUserRepo) SetPhoneVerified(ctx context.Context, userID uuid.UUID) error { return nil }
func (r *authTestUserRepo) SetIsSeller(ctx context.Context, userID uuid.UUID) error      { return nil }
func (r *authTestUserRepo) SetIsBuyer(ctx context.Context, userID uuid.UUID) error       { return nil }
func (r *authTestUserRepo) SetIsRider(ctx context.Context, userID uuid.UUID) error       { return nil }
func (r *authTestUserRepo) Suspend(ctx context.Context, userID uuid.UUID, reason string) error {
	return nil
}
func (r *authTestUserRepo) Unsuspend(ctx context.Context, userID uuid.UUID) error    { return nil }
func (r *authTestUserRepo) VerifySeller(ctx context.Context, userID uuid.UUID) error { return nil }
func (r *authTestUserRepo) List(ctx context.Context, filters interface{}, limit, offset int) ([]*domain.User, error) {
	return nil, nil
}
func (r *authTestUserRepo) GetStatistics(ctx context.Context) (*repository.UserStatistics, error) {
	return nil, nil
}
func (r *authTestUserRepo) SoftDelete(ctx context.Context, userID uuid.UUID, reason string) error {
	return nil
}
func (r *authTestUserRepo) GetAllActiveUserIDs(ctx context.Context) ([]uuid.UUID, error) {
	return nil, nil
}
func (r *authTestUserRepo) GetActiveUserIDsByRole(ctx context.Context, role string) ([]uuid.UUID, error) {
	return nil, nil
}

type authTestSupportStaffRepo struct {
	getByUserID func(ctx context.Context, userID uuid.UUID) (*domain.SupportStaff, error)
}

func (r *authTestSupportStaffRepo) Create(ctx context.Context, staff *domain.SupportStaff) error {
	return nil
}
func (r *authTestSupportStaffRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*domain.SupportStaff, error) {
	if r.getByUserID != nil {
		return r.getByUserID(ctx, userID)
	}
	return nil, domain.ErrSupportStaffNotFound
}
func (r *authTestSupportStaffRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.SupportStaff, error) {
	return nil, domain.ErrSupportStaffNotFound
}
func (r *authTestSupportStaffRepo) Update(ctx context.Context, staff *domain.SupportStaff) error {
	return nil
}
func (r *authTestSupportStaffRepo) Delete(ctx context.Context, userID uuid.UUID) error { return nil }
func (r *authTestSupportStaffRepo) List(ctx context.Context, activeOnly bool, limit, offset int) ([]*domain.SupportStaff, int, error) {
	return nil, 0, nil
}
func (r *authTestSupportStaffRepo) ExistsByUserID(ctx context.Context, userID uuid.UUID) (bool, error) {
	return false, nil
}

func TestGetPortalUserKeepsSupportStaffNonAdmin(t *testing.T) {
	userID := uuid.New()
	user := &domain.User{
		ID:            userID,
		PhoneNumber:   "+2348000000000",
		PhoneVerified: true,
		IsAdmin:       false,
		CreatedAt:     time.Now(),
	}

	svc := &AuthServiceImpl{
		userRepo: &authTestUserRepo{
			getByPhoneNumber: func(ctx context.Context, phoneNumber string) (*domain.User, error) {
				return user, nil
			},
		},
		supportStaffRepo: &authTestSupportStaffRepo{
			getByUserID: func(ctx context.Context, userID uuid.UUID) (*domain.SupportStaff, error) {
				return &domain.SupportStaff{
					UserID:        userID,
					CanReplyChats: true,
					IsActive:      true,
				}, nil
			},
		},
	}

	got, isNewUser, err := svc.getPortalUser(context.Background(), user.PhoneNumber)
	if err != nil {
		t.Fatalf("getPortalUser returned error: %v", err)
	}
	if isNewUser {
		t.Fatalf("expected existing user, got isNewUser=true")
	}
	if got.IsAdmin {
		t.Fatalf("expected support staff login not to promote user to admin")
	}
}

func TestGetPortalUserRejectsInactiveSupportStaff(t *testing.T) {
	userID := uuid.New()
	user := &domain.User{
		ID:            userID,
		PhoneNumber:   "+2348111111111",
		PhoneVerified: true,
		IsAdmin:       false,
	}

	svc := &AuthServiceImpl{
		userRepo: &authTestUserRepo{
			getByPhoneNumber: func(ctx context.Context, phoneNumber string) (*domain.User, error) {
				return user, nil
			},
		},
		supportStaffRepo: &authTestSupportStaffRepo{
			getByUserID: func(ctx context.Context, userID uuid.UUID) (*domain.SupportStaff, error) {
				return &domain.SupportStaff{
					UserID:   userID,
					IsActive: false,
				}, nil
			},
		},
	}

	_, _, err := svc.getPortalUser(context.Background(), user.PhoneNumber)
	if err != domain.ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}
