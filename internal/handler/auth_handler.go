package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"kipa/internal/domain"
	"kipa/internal/dto"
	"kipa/internal/handler/middleware"
	"kipa/internal/service"
	"kipa/internal/util"
)

type AuthHandler struct {
	authService service.AuthService
}

func NewAuthHandler(authService service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

func (h *AuthHandler) RegisterRoutes(router *gin.RouterGroup, authMiddleware *middleware.AuthMiddleware) {
	auth := router.Group("/auth")
	{
		auth.POST("/otp/send", h.SendOTP)
		auth.POST("/otp/verify", h.VerifyOTP)
		auth.POST("/token/refresh", h.RefreshToken)
		auth.POST("/logout", authMiddleware.RequireAuth(), h.Logout)
		auth.GET("/me", authMiddleware.RequireAuth(), h.Me)
		auth.PATCH("/profile", authMiddleware.RequireAuth(), h.UpdateProfile)
	}
}

func (h *AuthHandler) SendOTP(c *gin.Context) {
	var req dto.SendOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	response, err := h.authService.SendOTP(c.Request.Context(), &req, getClientIP(c), getUserAgent(c))
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, response)
}

func (h *AuthHandler) VerifyOTP(c *gin.Context) {
	var req dto.VerifyOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	response, err := h.authService.VerifyOTP(c.Request.Context(), &req, getClientIP(c), getUserAgent(c))
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, response)
}

func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req dto.RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	response, err := h.authService.RefreshToken(c.Request.Context(), &req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, response)
}

func (h *AuthHandler) Logout(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req dto.LogoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	response, err := h.authService.Logout(c.Request.Context(), userID, &req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, response)
}

func (h *AuthHandler) Me(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	response, err := h.authService.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, response)
}
func (h *AuthHandler) UpdateProfile(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req dto.UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	response, err := h.authService.UpdateProfile(c.Request.Context(), userID, &req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, response)
}

func (h *AuthHandler) handleError(c *gin.Context, err error) {
	status, code, message := mapErrorToResponse(err)
	util.Error(c, status, code, message)
}

func mapErrorToResponse(err error) (int, string, string) {
	switch {
	case errors.Is(err, domain.ErrInvalidOTP):
		return http.StatusUnauthorized, domain.CodeInvalidOTP, "Invalid OTP code"
	case errors.Is(err, domain.ErrOTPExpired):
		return http.StatusUnauthorized, domain.CodeOTPExpired, "OTP has expired"
	case errors.Is(err, domain.ErrMaxOTPAttemptsExceeded):
		return http.StatusUnauthorized, domain.CodeOTPMaxAttempts, "Maximum verification attempts exceeded. Please request a new OTP."
	case errors.Is(err, domain.ErrOTPRateLimitExceeded):
		return http.StatusTooManyRequests, domain.CodeOTPRateLimit, "Too many OTP requests. Please try again later."
	case errors.Is(err, domain.ErrOTPAlreadyVerified):
		return http.StatusBadRequest, domain.CodeInvalidOTP, "OTP has already been used"
	case errors.Is(err, domain.ErrInvalidToken):
		return http.StatusUnauthorized, domain.CodeInvalidToken, "Invalid token"
	case errors.Is(err, domain.ErrTokenExpired):
		return http.StatusUnauthorized, domain.CodeTokenExpired, "Token has expired"
	case errors.Is(err, domain.ErrInvalidRefreshToken):
		return http.StatusUnauthorized, domain.CodeInvalidToken, "Invalid refresh token"
	case errors.Is(err, domain.ErrSessionNotFound), errors.Is(err, domain.ErrSessionRevoked):
		return http.StatusUnauthorized, domain.CodeSessionRevoked, "Session has been revoked"
	case errors.Is(err, domain.ErrUserNotFound):
		return http.StatusNotFound, domain.CodeUserNotFound, "User not found"
	case errors.Is(err, domain.ErrUserSuspended):
		return http.StatusForbidden, domain.CodeUserSuspended, "Account is suspended"
	case errors.Is(err, domain.ErrIdempotencyKeyExists):
		return http.StatusConflict, "DUPLICATE_REQUEST", "This request has already been processed"
	case errors.Is(err, domain.ErrSMSDeliveryFailed):
		return http.StatusServiceUnavailable, domain.CodeSMSFailed, "Failed to send OTP. Please try again."
	default:
		return http.StatusInternalServerError, domain.CodeInternalError, "An unexpected error occurred"
	}
}

func getClientIP(c *gin.Context) string {
	ip := c.GetHeader("X-Forwarded-For")
	if ip == "" {
		ip = c.GetHeader("X-Real-IP")
	}
	if ip == "" {
		ip = c.ClientIP()
	}
	return ip
}

func getUserAgent(c *gin.Context) string {
	return c.GetHeader("User-Agent")
}
