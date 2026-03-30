package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"kipa/internal/domain"
	"kipa/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type adminTestUserRepo struct {
	user *domain.User
}

func (r *adminTestUserRepo) Create(ctx context.Context, user *domain.User) error { return nil }
func (r *adminTestUserRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	if r.user == nil {
		return nil, domain.ErrUserNotFound
	}
	return r.user, nil
}
func (r *adminTestUserRepo) GetByPhoneNumber(ctx context.Context, phoneNumber string) (*domain.User, error) {
	return nil, domain.ErrUserNotFound
}
func (r *adminTestUserRepo) Update(ctx context.Context, user *domain.User) error          { return nil }
func (r *adminTestUserRepo) UpdateLastLogin(ctx context.Context, userID uuid.UUID) error  { return nil }
func (r *adminTestUserRepo) SetPhoneVerified(ctx context.Context, userID uuid.UUID) error { return nil }
func (r *adminTestUserRepo) SetIsSeller(ctx context.Context, userID uuid.UUID) error      { return nil }
func (r *adminTestUserRepo) SetIsBuyer(ctx context.Context, userID uuid.UUID) error       { return nil }
func (r *adminTestUserRepo) SetIsRider(ctx context.Context, userID uuid.UUID) error       { return nil }
func (r *adminTestUserRepo) Suspend(ctx context.Context, userID uuid.UUID, reason string) error {
	return nil
}
func (r *adminTestUserRepo) Unsuspend(ctx context.Context, userID uuid.UUID) error    { return nil }
func (r *adminTestUserRepo) VerifySeller(ctx context.Context, userID uuid.UUID) error { return nil }
func (r *adminTestUserRepo) List(ctx context.Context, filters interface{}, limit, offset int) ([]*domain.User, error) {
	return nil, nil
}
func (r *adminTestUserRepo) GetStatistics(ctx context.Context) (*repository.UserStatistics, error) {
	return nil, nil
}
func (r *adminTestUserRepo) SoftDelete(ctx context.Context, userID uuid.UUID, reason string) error {
	return nil
}
func (r *adminTestUserRepo) GetAllActiveUserIDs(ctx context.Context) ([]uuid.UUID, error) {
	return nil, nil
}
func (r *adminTestUserRepo) GetActiveUserIDsByRole(ctx context.Context, role string) ([]uuid.UUID, error) {
	return nil, nil
}

func TestRequireAdminRejectsNonAdminSupportStaff(t *testing.T) {
	gin.SetMode(gin.TestMode)

	userID := uuid.New()
	mw := NewAdminMiddleware(&adminTestUserRepo{
		user: &domain.User{
			ID:            userID,
			PhoneNumber:   "+2348222222222",
			PhoneVerified: true,
			IsAdmin:       false,
		},
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	c.Set("user_id", userID)

	mw.RequireAdmin()(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", recorder.Code)
	}
}
