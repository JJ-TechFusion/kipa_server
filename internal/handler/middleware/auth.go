package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"kipa/internal/domain"
	"kipa/internal/service"
	"kipa/internal/util"
)

const (
	AuthorizationHeader = "Authorization"
	BearerSchema        = "Bearer "
	UserIDKey           = "user_id"
	DeviceIDKey         = "device_id"
	ClaimsKey           = "claims"
)

type AuthMiddleware struct {
	authService service.AuthService
}

func NewAuthMiddleware(authService service.AuthService) *AuthMiddleware {
	return &AuthMiddleware{authService: authService}
}

func (m *AuthMiddleware) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := extractToken(c)
		if err != nil {
			util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "Missing or invalid authorization header")
			c.Abort()
			return
		}

		claims, err := m.authService.ValidateAccessToken(c.Request.Context(), token)
		if err != nil {
			status := http.StatusUnauthorized
			code := domain.CodeInvalidToken
			message := "Invalid token"

			if err == domain.ErrTokenExpired {
				code = domain.CodeTokenExpired
				message = "Token has expired"
			}

			util.Error(c, status, code, message)
			c.Abort()
			return
		}

		c.Set(UserIDKey, claims.UserID)
		c.Set(DeviceIDKey, claims.DeviceID)
		c.Set(ClaimsKey, claims)
		c.Next()
	}
}

func (m *AuthMiddleware) OptionalAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := extractToken(c)
		if err != nil {
			c.Next()
			return
		}

		claims, err := m.authService.ValidateAccessToken(c.Request.Context(), token)
		if err != nil {
			c.Next()
			return
		}

		c.Set(UserIDKey, claims.UserID)
		c.Set(DeviceIDKey, claims.DeviceID)
		c.Set(ClaimsKey, claims)
		c.Next()
	}
}

func extractToken(c *gin.Context) (string, error) {
	header := c.GetHeader(AuthorizationHeader)
	if header == "" {
		return "", domain.ErrInvalidToken
	}

	if !strings.HasPrefix(header, BearerSchema) {
		return "", domain.ErrInvalidToken
	}

	return strings.TrimPrefix(header, BearerSchema), nil
}

func GetUserID(c *gin.Context) (uuid.UUID, bool) {
	value, exists := c.Get(UserIDKey)
	if !exists {
		return uuid.Nil, false
	}
	userID, ok := value.(uuid.UUID)
	return userID, ok
}

func GetDeviceID(c *gin.Context) (string, bool) {
	value, exists := c.Get(DeviceIDKey)
	if !exists {
		return "", false
	}
	deviceID, ok := value.(string)
	return deviceID, ok
}

func GetClaims(c *gin.Context) (*util.AccessTokenClaims, bool) {
	value, exists := c.Get(ClaimsKey)
	if !exists {
		return nil, false
	}
	claims, ok := value.(*util.AccessTokenClaims)
	return claims, ok
}
