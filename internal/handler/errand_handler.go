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
type ErrandHandler struct {
	errandService service.ErrandService
}

func NewErrandHandler(errandService service.ErrandService) *ErrandHandler {
	return &ErrandHandler{
		errandService: errandService,
	}
}
func (h *ErrandHandler) RegisterRoutes(rg *gin.RouterGroup, authMiddleware *middleware.AuthMiddleware) {
	errands := rg.Group("/errands")
	errands.Use(authMiddleware.RequireAuth())
	{
		errands.POST("", h.Create)
		errands.GET("", h.List)
		errands.GET("/active", h.GetActive)
		errands.GET("/:id", h.GetByID)
		errands.GET("/:id/timeline", h.GetTimeline)
		errands.POST("/:id/confirm", h.Confirm)
		errands.POST("/:id/complete", h.Complete)
		errands.DELETE("/:id", h.Cancel)
	}
}
func (h *ErrandHandler) Create(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req service.CreateErrandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	errand, err := h.errandService.Create(c.Request.Context(), userID, &req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	feeBreakdown := domain.NewRiderFeeBreakdown(errand.DeliveryFee)

	util.Success(c, http.StatusCreated, ErrandResponse{
		Errand:       errand,
		FeeBreakdown: feeBreakdown,
		Message:      "Errand created. Confirm to start searching for a rider.",
	})
}
func (h *ErrandHandler) List(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	limit := 20
	offset := 0

	errands, err := h.errandService.List(c.Request.Context(), userID, limit, offset)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"errands": errands,
		"count":   len(errands),
	})
}
func (h *ErrandHandler) GetActive(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	errands, err := h.errandService.GetActive(c.Request.Context(), userID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"errands": errands,
		"count":   len(errands),
	})
}
func (h *ErrandHandler) GetByID(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	errandID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid errand ID")
		return
	}

	result, err := h.errandService.GetWithDeliveryDetails(c.Request.Context(), userID, errandID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	feeBreakdown := domain.NewRiderFeeBreakdown(result.Errand.DeliveryFee)
	timeline := BuildErrandTimeline(result.Errand)

	util.Success(c, http.StatusOK, ErrandWithDetailsResponse{
		Errand:       result.Errand,
		DeliveryJob:  result.DeliveryJob,
		FeeBreakdown: feeBreakdown,
		Timeline:     &timeline,
	})
}
func (h *ErrandHandler) GetTimeline(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	errandID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid errand ID")
		return
	}

	errand, err := h.errandService.GetByID(c.Request.Context(), userID, errandID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	timeline := BuildErrandTimeline(errand)
	util.Success(c, http.StatusOK, timeline)
}

func (h *ErrandHandler) Confirm(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	errandID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid errand ID")
		return
	}

	errand, err := h.errandService.Confirm(c.Request.Context(), userID, errandID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"errand":      errand,
		"dropoff_code": errand.DropoffCode,
		"message":     "Searching for a rider. You will be notified when one accepts.",
	})
}
func (h *ErrandHandler) Cancel(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	errandID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid errand ID")
		return
	}

	if err := h.errandService.Cancel(c.Request.Context(), userID, errandID); err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"message": "Errand cancelled successfully",
	})
}
func (h *ErrandHandler) Complete(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	errandID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid errand ID")
		return
	}

	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	errand, err := h.errandService.CompleteErrand(c.Request.Context(), userID, errandID, req.Code)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"errand":  errand,
		"message": "Delivery confirmed successfully. Thank you!",
	})
}

func (h *ErrandHandler) handleError(c *gin.Context, err error) {
	status, code, message := mapErrandErrorToResponse(err)
	util.Error(c, status, code, message)
}

func mapErrandErrorToResponse(err error) (int, string, string) {
	switch {
	case errors.Is(err, domain.ErrErrandNotFound):
		return http.StatusNotFound, "ERRAND_NOT_FOUND", "Errand not found"
	case errors.Is(err, domain.ErrErrandNotConfirmable):
		return http.StatusBadRequest, "NOT_CONFIRMABLE", "Errand cannot be confirmed in its current state"
	case errors.Is(err, domain.ErrErrandNotCancellable):
		return http.StatusBadRequest, "NOT_CANCELLABLE", "Errand cannot be cancelled in its current state"
	case errors.Is(err, domain.ErrUnauthorized):
		return http.StatusForbidden, domain.CodeUnauthorized, "You don't have permission for this action"
	case errors.Is(err, domain.ErrInvalidVehicleType):
		return http.StatusBadRequest, "INVALID_VEHICLE_TYPE", "Invalid vehicle type: must be 'motorcycle' or 'car'"
	case errors.Is(err, domain.ErrZoneNotFound):
		return http.StatusBadRequest, "OUTSIDE_COVERAGE", "Pickup or dropoff location is outside our delivery coverage"
	default:
		return http.StatusInternalServerError, domain.CodeInternalError, "An unexpected error occurred"
	}
}

type ErrandResponse struct {
	Errand       *domain.Errand            `json:"errand"`
	FeeBreakdown *domain.RiderFeeBreakdown `json:"fee_breakdown,omitempty"`
	Timeline     *ErrandTimelineResponse   `json:"timeline,omitempty"`
	Message      string                    `json:"message,omitempty"`
}

type ErrandWithDetailsResponse struct {
	Errand       *domain.Errand              `json:"errand"`
	DeliveryJob  *service.ErrandDeliveryInfo `json:"delivery_job,omitempty"`
	FeeBreakdown *domain.RiderFeeBreakdown   `json:"fee_breakdown,omitempty"`
	Timeline     *ErrandTimelineResponse     `json:"timeline,omitempty"`
	Message      string                      `json:"message,omitempty"`
}
