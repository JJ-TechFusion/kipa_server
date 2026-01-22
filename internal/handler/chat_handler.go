package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/handler/middleware"
	"kipa/internal/service"
)

type ChatHandler struct {
	chatService service.ChatService
	logger      *zap.Logger
}

func NewChatHandler(chatService service.ChatService, logger *zap.Logger) *ChatHandler {
	return &ChatHandler{
		chatService: chatService,
		logger:      logger,
	}
}

func (h *ChatHandler) GetHistory(c *gin.Context) {
	jobIDStr := c.Param("job_id")
	jobID, err := uuid.Parse(jobIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid job ID"})
		return
	}


	userID, exists := middleware.GetUserID(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	if _, err := h.chatService.ValidateParticipant(c, userID, jobID); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "You are not a participant in this job"})
		return
	}

	limitStr := c.Query("limit")
	limit := 50
	if limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err == nil {
			limit = parsedLimit
		}
	}

	var beforeID *uuid.UUID
	beforeIDStr := c.Query("before_id")
	if beforeIDStr != "" {
		id, err := uuid.Parse(beforeIDStr)
		if err == nil {
			beforeID = &id
		}
	}

	messages, err := h.chatService.GetHistory(c, jobID, limit, beforeID)
	if err != nil {
		h.logger.Error("failed to get chat history", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve chat history"})
		return
	}

	c.JSON(http.StatusOK, messages)
}

func (h *ChatHandler) MarkAsRead(c *gin.Context) {
	jobIDStr := c.Param("job_id")
	jobID, err := uuid.Parse(jobIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid job ID"})
		return
	}

	userID, exists := middleware.GetUserID(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	if err := h.chatService.MarkAsRead(c, jobID, userID); err != nil {
		h.logger.Error("failed to mark messages as read", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to mark messages as read"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *ChatHandler) GetUnreadCount(c *gin.Context) {
    jobIDStr := c.Param("job_id")
    jobID, err := uuid.Parse(jobIDStr)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid job ID"})
        return
    }

    userID, exists := middleware.GetUserID(c)
    if !exists {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    count, err := h.chatService.GetUnreadCount(c, jobID, userID)
    if err != nil {
        h.logger.Error("failed to get unread count", zap.Error(err))
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get unread count"})
        return
    }

    c.JSON(http.StatusOK, gin.H{"count": count})
}

func (h *ChatHandler) RegisterRoutes(r *gin.RouterGroup, authMiddleware *middleware.AuthMiddleware) {
	chat := r.Group("/chat")
	chat.Use(authMiddleware.RequireAuth())
	{
		chat.GET("/:job_id/history", h.GetHistory)
		chat.POST("/:job_id/read", h.MarkAsRead)
		chat.GET("/:job_id/unread", h.GetUnreadCount)
	}
}
