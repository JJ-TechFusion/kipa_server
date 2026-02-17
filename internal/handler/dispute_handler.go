package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"kipa/internal/domain"
	"kipa/internal/handler/middleware"
	"kipa/internal/service"
	"kipa/internal/util"
)

type DisputeHandler struct {
	disputeService service.DisputeService
}

func NewDisputeHandler(disputeService service.DisputeService) *DisputeHandler {
	return &DisputeHandler{
		disputeService: disputeService,
	}
}

func (h *DisputeHandler) RegisterRoutes(rg *gin.RouterGroup, authMiddleware *middleware.AuthMiddleware) {
	disputes := rg.Group("/disputes")
	disputes.Use(authMiddleware.RequireAuth())
	{
		disputes.GET("", h.List)
		disputes.GET("/:id", h.GetByID)
	}
}

func (h *DisputeHandler) List(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	limit := 20
	offset := 0

	disputes, err := h.disputeService.ListForUser(c.Request.Context(), userID, limit, offset)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to retrieve disputes")
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"disputes": disputes,
		"count":    len(disputes),
	})
}

func (h *DisputeHandler) GetByID(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	disputeID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid dispute ID")
		return
	}

	dispute, err := h.disputeService.GetByIDForUser(c.Request.Context(), userID, disputeID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, dispute)
}

func (h *DisputeHandler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrDisputeNotFound):
		util.Error(c, http.StatusNotFound, "DISPUTE_NOT_FOUND", "Dispute not found")
	case errors.Is(err, domain.ErrUnauthorized):
		util.Error(c, http.StatusForbidden, domain.CodeUnauthorized, "You don't have permission to view this dispute")
	default:
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "An unexpected error occurred")
	}
}
