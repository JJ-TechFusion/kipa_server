package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/domain"
	"kipa/internal/dto"
	"kipa/internal/handler/middleware"
	"kipa/internal/service"
)

type SupportChatHandler struct {
	supportChatService service.SupportChatService
	logger             *zap.Logger
}

func NewSupportChatHandler(supportChatService service.SupportChatService, logger *zap.Logger) *SupportChatHandler {
	return &SupportChatHandler{
		supportChatService: supportChatService,
		logger:             logger,
	}
}


func (h *SupportChatHandler) GetOrCreateConversation(c *gin.Context) {
	userID, exists := middleware.GetUserID(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	context, err := h.supportChatService.GetOrCreateConversation(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("failed to get or create support conversation", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get support conversation"})
		return
	}

	c.JSON(http.StatusOK, context)
}

func (h *SupportChatHandler) SendMessage(c *gin.Context) {
	userID, exists := middleware.GetUserID(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req dto.SendSupportMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	message, err := h.supportChatService.SendMessage(c.Request.Context(), userID, &req)
	if err != nil {
		h.logger.Error("failed to send support message", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send message"})
		return
	}

	c.JSON(http.StatusCreated, message)
}

func (h *SupportChatHandler) GetHistory(c *gin.Context) {
	userID, exists := middleware.GetUserID(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	limit := 50
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	var beforeID *uuid.UUID
	if beforeIDStr := c.Query("before_id"); beforeIDStr != "" {
		if id, err := uuid.Parse(beforeIDStr); err == nil {
			beforeID = &id
		}
	}

	messages, err := h.supportChatService.GetHistory(c.Request.Context(), userID, limit, beforeID)
	if err != nil {
		h.logger.Error("failed to get support chat history", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve chat history"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"messages": messages})
}

func (h *SupportChatHandler) MarkAsRead(c *gin.Context) {
	userID, exists := middleware.GetUserID(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	if err := h.supportChatService.MarkAsRead(c.Request.Context(), userID); err != nil {
		h.logger.Error("failed to mark support messages as read", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to mark messages as read"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *SupportChatHandler) ListUserConversations(c *gin.Context) {
	userID, exists := middleware.GetUserID(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	conversations, err := h.supportChatService.ListUserConversations(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("failed to list user conversations", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list conversations"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"conversations": conversations})
}

func (h *SupportChatHandler) GetUserConversation(c *gin.Context) {
	userID, exists := middleware.GetUserID(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	conversationIDStr := c.Param("id")
	conversationID, err := uuid.Parse(conversationIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid conversation ID"})
		return
	}

	context, err := h.supportChatService.GetUserConversation(c.Request.Context(), userID, conversationID)
	if err != nil {
		h.logger.Error("failed to get user conversation", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "Conversation not found"})
		return
	}

	c.JSON(http.StatusOK, context)
}

func (h *SupportChatHandler) SendMessageToConversation(c *gin.Context) {
	userID, exists := middleware.GetUserID(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	conversationIDStr := c.Param("id")
	conversationID, err := uuid.Parse(conversationIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid conversation ID"})
		return
	}

	var req dto.SendSupportMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	message, err := h.supportChatService.SendMessageToConversation(c.Request.Context(), userID, conversationID, &req)
	if err != nil {
		h.logger.Error("failed to send message to conversation", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send message"})
		return
	}

	c.JSON(http.StatusCreated, message)
}

func (h *SupportChatHandler) GetConversationMessages(c *gin.Context) {
	userID, exists := middleware.GetUserID(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	conversationIDStr := c.Param("id")
	conversationID, err := uuid.Parse(conversationIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid conversation ID"})
		return
	}

	limit := 50
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	var beforeID *uuid.UUID
	if beforeIDStr := c.Query("before_id"); beforeIDStr != "" {
		if id, err := uuid.Parse(beforeIDStr); err == nil {
			beforeID = &id
		}
	}

	messages, err := h.supportChatService.GetConversationHistory(c.Request.Context(), userID, conversationID, limit, beforeID)
	if err != nil {
		h.logger.Error("failed to get conversation messages", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve messages"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"messages": messages})
}

func (h *SupportChatHandler) MarkConversationAsRead(c *gin.Context) {
	userID, exists := middleware.GetUserID(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	conversationIDStr := c.Param("id")
	conversationID, err := uuid.Parse(conversationIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid conversation ID"})
		return
	}

	if err := h.supportChatService.MarkConversationAsRead(c.Request.Context(), userID, conversationID); err != nil {
		h.logger.Error("failed to mark conversation as read", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to mark messages as read"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *SupportChatHandler) ListConversations(c *gin.Context) {
	limit := 50
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	offset := 0
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if parsedOffset, err := strconv.Atoi(offsetStr); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	var status *domain.SupportConversationStatus
	if statusStr := c.Query("status"); statusStr != "" {
		s := domain.SupportConversationStatus(statusStr)
		status = &s
	}

	conversations, total, err := h.supportChatService.ListConversations(c.Request.Context(), status, limit, offset)
	if err != nil {
		h.logger.Error("failed to list support conversations", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list conversations"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"conversations": conversations,
		"total":         total,
	})
}

func (h *SupportChatHandler) GetConversationDetails(c *gin.Context) {
	conversationIDStr := c.Param("id")
	conversationID, err := uuid.Parse(conversationIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid conversation ID"})
		return
	}

	details, err := h.supportChatService.GetConversationDetails(c.Request.Context(), conversationID)
	if err != nil {
		h.logger.Error("failed to get support conversation details", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get conversation details"})
		return
	}

	c.JSON(http.StatusOK, details)
}

func (h *SupportChatHandler) AdminSendMessage(c *gin.Context) {
	adminID, exists := middleware.GetUserID(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	conversationIDStr := c.Param("id")
	conversationID, err := uuid.Parse(conversationIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid conversation ID"})
		return
	}

	var req dto.SendSupportMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	message, err := h.supportChatService.AdminSendMessage(c.Request.Context(), adminID, conversationID, &req)
	if err != nil {
		h.logger.Error("failed to send admin support message", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send message"})
		return
	}

	c.JSON(http.StatusCreated, message)
}

func (h *SupportChatHandler) AdminGetHistory(c *gin.Context) {
	conversationIDStr := c.Param("id")
	conversationID, err := uuid.Parse(conversationIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid conversation ID"})
		return
	}

	limit := 50
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	var beforeID *uuid.UUID
	if beforeIDStr := c.Query("before_id"); beforeIDStr != "" {
		if id, err := uuid.Parse(beforeIDStr); err == nil {
			beforeID = &id
		}
	}

	messages, err := h.supportChatService.AdminGetHistory(c.Request.Context(), conversationID, limit, beforeID)
	if err != nil {
		h.logger.Error("failed to get admin support chat history", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve chat history"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"messages": messages})
}

func (h *SupportChatHandler) AdminMarkAsRead(c *gin.Context) {
	conversationIDStr := c.Param("id")
	conversationID, err := uuid.Parse(conversationIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid conversation ID"})
		return
	}

	if err := h.supportChatService.AdminMarkAsRead(c.Request.Context(), conversationID); err != nil {
		h.logger.Error("failed to mark admin support messages as read", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to mark messages as read"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *SupportChatHandler) ResolveConversation(c *gin.Context) {
	adminID, exists := middleware.GetUserID(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	conversationIDStr := c.Param("id")
	conversationID, err := uuid.Parse(conversationIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid conversation ID"})
		return
	}

	if err := h.supportChatService.ResolveConversation(c.Request.Context(), adminID, conversationID); err != nil {
		h.logger.Error("failed to resolve support conversation", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to resolve conversation"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "conversation resolved successfully"})
}

func (h *SupportChatHandler) AdminSendMessageToUser(c *gin.Context) {
	adminID, exists := middleware.GetUserID(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	userIDStr := c.Param("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	var req dto.SendSupportMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	message, err := h.supportChatService.AdminSendMessageToUser(c.Request.Context(), adminID, userID, &req)
	if err != nil {
		h.logger.Error("failed to send message to user", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send message"})
		return
	}

	c.JSON(http.StatusCreated, message)
}

func (h *SupportChatHandler) RegisterRoutes(r *gin.RouterGroup, authMiddleware *middleware.AuthMiddleware, adminMiddleware *middleware.AdminMiddleware) {
	support := r.Group("/support")
	support.Use(authMiddleware.RequireAuth())
	{
		// General support chat (creates/gets chat without dispute)
		support.GET("", h.GetOrCreateConversation)
		support.POST("/messages", h.SendMessage)
		support.GET("/messages", h.GetHistory)
		support.POST("/messages/read", h.MarkAsRead)

		// Multi-conversation endpoints (including dispute chats)
		support.GET("/conversations", h.ListUserConversations)
		support.GET("/conversations/:id", h.GetUserConversation)
		support.POST("/conversations/:id/messages", h.SendMessageToConversation)
		support.GET("/conversations/:id/messages", h.GetConversationMessages)
		support.POST("/conversations/:id/read", h.MarkConversationAsRead)
	}

	// Admin support routes (still uses admin middleware for backwards compatibility)
	adminSupport := r.Group("/admin/support")
	adminSupport.Use(authMiddleware.RequireAuth())
	adminSupport.Use(adminMiddleware.RequireAdmin())
	{
		adminSupport.GET("/conversations", h.ListConversations)
		adminSupport.GET("/conversations/:id", h.GetConversationDetails)
		adminSupport.POST("/conversations/:id/messages", h.AdminSendMessage)
		adminSupport.GET("/conversations/:id/messages", h.AdminGetHistory)
		adminSupport.POST("/conversations/:id/read", h.AdminMarkAsRead)
		adminSupport.POST("/conversations/:id/resolve", h.ResolveConversation)
		adminSupport.POST("/users/:user_id/messages", h.AdminSendMessageToUser)
	}
}

// RegisterRoutesWithSupport registers routes with support staff middleware (preferred).
// Support staff can access these endpoints based on their configured permissions.
func (h *SupportChatHandler) RegisterRoutesWithSupport(r *gin.RouterGroup, authMiddleware *middleware.AuthMiddleware, supportMiddleware *middleware.SupportMiddleware) {
	support := r.Group("/support")
	support.Use(authMiddleware.RequireAuth())
	{
		// General support chat (creates/gets chat without dispute)
		support.GET("", h.GetOrCreateConversation)
		support.POST("/messages", h.SendMessage)
		support.GET("/messages", h.GetHistory)
		support.POST("/messages/read", h.MarkAsRead)

		// Multi-conversation endpoints (including dispute chats)
		support.GET("/conversations", h.ListUserConversations)
		support.GET("/conversations/:id", h.GetUserConversation)
		support.POST("/conversations/:id/messages", h.SendMessageToConversation)
		support.GET("/conversations/:id/messages", h.GetConversationMessages)
		support.POST("/conversations/:id/read", h.MarkConversationAsRead)
	}

	// Admin/Support staff routes - accessible by admin or support staff with permissions
	adminSupport := r.Group("/admin/support")
	adminSupport.Use(authMiddleware.RequireAuth())
	adminSupport.Use(supportMiddleware.RequireSupport())
	{
		// List/view conversations - all support staff can access
		adminSupport.GET("/conversations", h.ListConversations)
		adminSupport.GET("/conversations/:id/messages", h.AdminGetHistory)

		// View dispute details - requires view_disputes permission
		adminSupport.GET("/conversations/:id",
			supportMiddleware.RequirePermission(domain.PermViewDisputes),
			h.GetConversationDetails)

		// Reply to chats - requires reply_chats permission
		adminSupport.POST("/conversations/:id/messages",
			supportMiddleware.RequirePermission(domain.PermReplyChats),
			h.AdminSendMessage)

		// Mark as read - requires reply_chats permission
		adminSupport.POST("/conversations/:id/read",
			supportMiddleware.RequirePermission(domain.PermReplyChats),
			h.AdminMarkAsRead)

		// Resolve conversation - requires resolve_chats permission
		adminSupport.POST("/conversations/:id/resolve",
			supportMiddleware.RequirePermission(domain.PermResolveChats),
			h.ResolveConversation)

		// Message any user - requires message_users permission
		adminSupport.POST("/users/:user_id/messages",
			supportMiddleware.RequirePermission(domain.PermMessageUsers),
			h.AdminSendMessageToUser)
	}
}
