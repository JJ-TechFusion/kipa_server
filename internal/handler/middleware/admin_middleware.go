package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"kipa/internal/repository"
)

type AdminMiddleware struct {
	userRepo repository.UserRepository
}

func NewAdminMiddleware(userRepo repository.UserRepository) *AdminMiddleware {
	return &AdminMiddleware{
		userRepo: userRepo,
	}
}

func (m *AdminMiddleware) RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		userIDValue, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "unauthorized",
				"message": "authentication required",
			})
			c.Abort()
			return
		}

		userID, ok := userIDValue.(uuid.UUID)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "unauthorized",
				"message": "invalid user ID",
			})
			c.Abort()
			return
		}


		user, err := m.userRepo.GetByID(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "unauthorized",
				"message": "user not found",
			})
			c.Abort()
			return
		}

		if !user.CanAccessAdmin() {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "forbidden",
				"message": "admin access required",
			})
			c.Abort()
			return
		}

		// User is admin, continue
		c.Next()
	}
}

