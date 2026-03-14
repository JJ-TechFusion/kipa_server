package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"kipa/internal/domain"
	"kipa/internal/repository"
)

type SupportMiddleware struct {
	staffRepo repository.SupportStaffRepository
	userRepo  repository.UserRepository
}

func NewSupportMiddleware(staffRepo repository.SupportStaffRepository, userRepo repository.UserRepository) *SupportMiddleware {
	return &SupportMiddleware{
		staffRepo: staffRepo,
		userRepo:  userRepo,
	}
}

// RequireSupport requires the user to be either an admin or active support staff.
// Sets "is_support_staff" and optionally "support_staff" in context.
func (m *SupportMiddleware) RequireSupport() gin.HandlerFunc {
	return func(c *gin.Context) {
		userIDValue, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "authentication required",
			})
			c.Abort()
			return
		}

		userID, ok := userIDValue.(uuid.UUID)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "invalid user ID",
			})
			c.Abort()
			return
		}

		user, err := m.userRepo.GetByID(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "user not found",
			})
			c.Abort()
			return
		}

		// Admin always passes
		if user.CanAccessAdmin() {
			c.Set("is_admin", true)
			c.Set("is_support_staff", false)
			c.Next()
			return
		}

		// Check if user is active support staff
		staff, err := m.staffRepo.GetByUserID(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "forbidden",
				"message": "support access required",
			})
			c.Abort()
			return
		}

		if !staff.IsActive {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "forbidden",
				"message": "support staff access has been deactivated",
			})
			c.Abort()
			return
		}

		c.Set("is_admin", false)
		c.Set("is_support_staff", true)
		c.Set("support_staff", staff)
		c.Next()
	}
}

// RequirePermission requires the user to have a specific support permission.
// Admin always passes. Support staff must have the specific permission.
func (m *SupportMiddleware) RequirePermission(perm domain.SupportPermission) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Admin bypass
		if isAdmin, exists := c.Get("is_admin"); exists && isAdmin.(bool) {
			c.Next()
			return
		}

		// Check support staff permission
		staffValue, exists := c.Get("support_staff")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "forbidden",
				"message": "support staff access required",
			})
			c.Abort()
			return
		}

		staff, ok := staffValue.(*domain.SupportStaff)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "internal_error",
				"message": "invalid support staff context",
			})
			c.Abort()
			return
		}

		if !staff.HasPermission(perm) {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "forbidden",
				"message": "insufficient permission: " + string(perm),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// IsSupportOrAdmin returns true if the current user is either admin or support staff.
func IsSupportOrAdmin(c *gin.Context) bool {
	if isAdmin, exists := c.Get("is_admin"); exists && isAdmin.(bool) {
		return true
	}
	if isSupport, exists := c.Get("is_support_staff"); exists && isSupport.(bool) {
		return true
	}
	return false
}

// GetSupportStaff returns the support staff from context, if present.
func GetSupportStaff(c *gin.Context) (*domain.SupportStaff, bool) {
	staffValue, exists := c.Get("support_staff")
	if !exists {
		return nil, false
	}
	staff, ok := staffValue.(*domain.SupportStaff)
	return staff, ok
}
