package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/domain"
	"kipa/internal/service"
)

type NotificationHandler struct {
	notificationService service.NotificationService
	logger              *zap.Logger
}

func NewNotificationHandler(
	notificationService service.NotificationService,
	logger *zap.Logger,
) *NotificationHandler {
	return &NotificationHandler{
		notificationService: notificationService,
		logger:              logger,
	}
}

func (h *NotificationHandler) RegisterRoutes(router *gin.RouterGroup) {
	router.POST("/device-token", h.RegisterToken)
	router.DELETE("/device-token", h.RemoveToken)
}

type RegisterTokenRequest struct {
	Token    string `json:"token" binding:"required"`
	Platform string `json:"platform" binding:"required,oneof=android ios web"`
}

func (h *NotificationHandler) RegisterToken(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req RegisterTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	platform := domain.DevicePlatform(req.Platform)

	if err := h.notificationService.RegisterToken(c.Request.Context(), userID.(uuid.UUID), req.Token, platform); err != nil {
		h.logger.Error("failed to register device token", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to register token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "token registered successfully"})
}

type RemoveTokenRequest struct {
	Token string `json:"token" binding:"required"`
}


func (h *NotificationHandler) RemoveToken(c *gin.Context) {
	var req RemoveTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.notificationService.RemoveToken(c.Request.Context(), req.Token); err != nil {
		h.logger.Error("failed to remove device token", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "token removed successfully"})
}
