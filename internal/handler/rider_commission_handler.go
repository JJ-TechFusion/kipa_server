package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"kipa/internal/domain"
	"kipa/internal/handler/middleware"
	"kipa/internal/service"
	"kipa/internal/util"
)

// RiderCommissionHandler handles rider commission API endpoints
type RiderCommissionHandler struct {
	commissionService service.RiderCommissionService
}

func NewRiderCommissionHandler(commissionService service.RiderCommissionService) *RiderCommissionHandler {
	return &RiderCommissionHandler{
		commissionService: commissionService,
	}
}

// RegisterRoutes registers commission routes
func (h *RiderCommissionHandler) RegisterRoutes(rg *gin.RouterGroup, authMiddleware *middleware.AuthMiddleware) {
	commission := rg.Group("/rider/commission")
	commission.Use(authMiddleware.RequireAuth())
	{
		commission.GET("", h.GetCommissionSummary)
		commission.GET("/pending", h.GetPendingCommissions)
		commission.POST("/pay", h.PayCommission)
	}
}

// GetCommissionSummary returns the rider's commission summary
// GET /rider/commission
func (h *RiderCommissionHandler) GetCommissionSummary(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	summary, err := h.commissionService.GetSummary(c.Request.Context(), userID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, summary)
}

// GetPendingCommissions returns the rider's pending commission records
// GET /rider/commission/pending
func (h *RiderCommissionHandler) GetPendingCommissions(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	commissions, err := h.commissionService.GetPendingCommissions(c.Request.Context(), userID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	// Calculate total
	var total float64
	for _, comm := range commissions {
		total += comm.CommissionAmount
	}

	util.Success(c, http.StatusOK, gin.H{
		"commissions": commissions,
		"total":       total,
		"count":       len(commissions),
	})
}

// PayCommission pays all pending commissions from rider's wallet
// POST /rider/commission/pay
func (h *RiderCommissionHandler) PayCommission(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	result, err := h.commissionService.PayAllFromWallet(c.Request.Context(), userID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, result)
}

func (h *RiderCommissionHandler) handleError(c *gin.Context, err error) {
	status, code, message := mapCommissionErrorToResponse(err)
	util.Error(c, status, code, message)
}

func mapCommissionErrorToResponse(err error) (int, string, string) {
	switch {
	case errors.Is(err, domain.ErrRiderNotFound):
		return http.StatusNotFound, "RIDER_NOT_FOUND", "Rider profile not found"
	case errors.Is(err, domain.ErrNoCommissionOwed):
		return http.StatusBadRequest, "NO_COMMISSION_OWED", "No outstanding commission to pay"
	case errors.Is(err, domain.ErrInsufficientBalance):
		return http.StatusBadRequest, "INSUFFICIENT_BALANCE", "Insufficient wallet balance to pay commission"
	case errors.Is(err, domain.ErrPartialCommissionPaymentNotAllowed):
		return http.StatusBadRequest, "PARTIAL_PAYMENT_NOT_ALLOWED", "Partial payment not allowed - must pay full outstanding commission"
	default:
		return http.StatusInternalServerError, domain.CodeInternalError, "An unexpected error occurred"
	}
}
