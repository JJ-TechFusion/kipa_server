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

type RiderHandler struct {
	riderService service.RiderService
}

func NewRiderHandler(riderService service.RiderService) *RiderHandler {
	return &RiderHandler{riderService: riderService}
}

func (h *RiderHandler) RegisterRoutes(router *gin.RouterGroup, authMiddleware *middleware.AuthMiddleware) {
	rider := router.Group("/rider")
	rider.Use(authMiddleware.RequireAuth())
	{
		// Profile management
		rider.GET("/profile", h.GetProfile)
		rider.POST("/profile", h.CreateProfile)
		rider.PATCH("/profile", h.UpdateProfile)

		// Online status
		rider.POST("/go-online", h.GoOnline)
		rider.POST("/go-offline", h.GoOffline)
		rider.PATCH("/location", h.UpdateLocation)

		// Job management
		rider.GET("/available-jobs", h.GetAvailableJobs)
		rider.POST("/jobs/:id/accept", h.AcceptJob)
		rider.GET("/jobs/active", h.GetActiveJob)
		rider.POST("/jobs/:id/pickup", h.ConfirmPickup)
		rider.POST("/jobs/:id/deliver", h.ConfirmDelivery)
		rider.GET("/jobs/history", h.GetJobHistory)

		// Earnings
		rider.GET("/earnings", h.GetEarnings)
	}
}

// GetProfile gets the rider's profile
// GET /rider/profile
func (h *RiderHandler) GetProfile(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	profile, err := h.riderService.GetProfile(c.Request.Context(), userID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, riderProfileToResponse(profile))
}

// CreateProfile creates a new rider profile
// POST /rider/profile
func (h *RiderHandler) CreateProfile(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req service.CreateRiderProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	profile, err := h.riderService.CreateProfile(c.Request.Context(), userID, &req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusCreated, riderProfileToResponse(profile))
}

// UpdateProfile updates the rider's profile
// PATCH /rider/profile
func (h *RiderHandler) UpdateProfile(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req service.UpdateRiderProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	profile, err := h.riderService.UpdateProfile(c.Request.Context(), userID, &req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, riderProfileToResponse(profile))
}

// GoOnline sets the rider as online
// POST /rider/go-online
func (h *RiderHandler) GoOnline(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	if err := h.riderService.GoOnline(c.Request.Context(), userID); err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{"message": "You are now online and available for deliveries"})
}

// GoOffline sets the rider as offline
// POST /rider/go-offline
func (h *RiderHandler) GoOffline(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	if err := h.riderService.GoOffline(c.Request.Context(), userID); err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{"message": "You are now offline"})
}

// UpdateLocation updates the rider's current location
// PATCH /rider/location
func (h *RiderHandler) UpdateLocation(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req UpdateLocationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	var heading, speed float64
	if req.Heading != nil {
		heading = *req.Heading
	}
	if req.Speed != nil {
		speed = *req.Speed
	}

	if err := h.riderService.UpdateLocation(c.Request.Context(), userID, req.Latitude, req.Longitude, heading, speed); err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{"message": "Location updated"})
}

// GetAvailableJobs gets jobs available to the rider
// GET /rider/available-jobs
func (h *RiderHandler) GetAvailableJobs(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	jobs, err := h.riderService.GetAvailableJobs(c.Request.Context(), userID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	responses := make([]DeliveryJobResponse, len(jobs))
	for i, job := range jobs {
		responses[i] = deliveryJobToResponse(job)
	}

	util.Success(c, http.StatusOK, gin.H{
		"jobs":  responses,
		"count": len(responses),
	})
}

// AcceptJob accepts a delivery job
// POST /rider/jobs/:id/accept
func (h *RiderHandler) AcceptJob(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	jobID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid job ID")
		return
	}

	job, err := h.riderService.AcceptJob(c.Request.Context(), userID, jobID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"message":     "Job accepted successfully",
		"job":         deliveryJobToResponse(job),
		"pickup_code": job.PickupCode,
	})
}

// GetActiveJob gets the rider's current active job
// GET /rider/jobs/active
func (h *RiderHandler) GetActiveJob(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	job, err := h.riderService.GetActiveJob(c.Request.Context(), userID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	if job == nil {
		util.Success(c, http.StatusOK, gin.H{"job": nil, "message": "No active job"})
		return
	}

	util.Success(c, http.StatusOK, deliveryJobToResponse(job))
}

// ConfirmPickup confirms pickup with verification code
// POST /rider/jobs/:id/pickup
func (h *RiderHandler) ConfirmPickup(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	jobID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid job ID")
		return
	}

	var req VerificationCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	if err := h.riderService.ConfirmPickup(c.Request.Context(), userID, jobID, req.Code); err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{"message": "Pickup confirmed. Item collected successfully."})
}

// ConfirmDelivery confirms delivery with verification code
// POST /rider/jobs/:id/deliver
func (h *RiderHandler) ConfirmDelivery(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	jobID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid job ID")
		return
	}

	var req VerificationCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	if err := h.riderService.ConfirmDelivery(c.Request.Context(), userID, jobID, req.Code); err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{"message": "Delivery confirmed. Payment will be credited to your wallet."})
}

// GetJobHistory gets the rider's completed jobs
// GET /rider/jobs/history
func (h *RiderHandler) GetJobHistory(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	limit := 20
	offset := 0

	jobs, err := h.riderService.GetJobHistory(c.Request.Context(), userID, limit, offset)
	if err != nil {
		h.handleError(c, err)
		return
	}

	responses := make([]DeliveryJobResponse, len(jobs))
	for i, job := range jobs {
		responses[i] = deliveryJobToResponse(job)
	}

	util.Success(c, http.StatusOK, gin.H{
		"jobs":  responses,
		"count": len(responses),
	})
}

// GetEarnings gets the rider's earnings summary
// GET /rider/earnings
func (h *RiderHandler) GetEarnings(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	earnings, err := h.riderService.GetEarnings(c.Request.Context(), userID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, earnings)
}

func (h *RiderHandler) handleError(c *gin.Context, err error) {
	status, code, message := mapRiderErrorToResponse(err)
	util.Error(c, status, code, message)
}

func mapRiderErrorToResponse(err error) (int, string, string) {
	switch {
	case errors.Is(err, domain.ErrRiderProfileNotFound):
		return http.StatusNotFound, "PROFILE_NOT_FOUND", "Rider profile not found"
	case errors.Is(err, domain.ErrRiderNotVerified):
		return http.StatusForbidden, "NOT_VERIFIED", "Your rider profile is not yet verified"
	case errors.Is(err, domain.ErrRiderAlreadyOnline):
		return http.StatusBadRequest, "ALREADY_ONLINE", "You are already online"
	case errors.Is(err, domain.ErrRiderNotOnline):
		return http.StatusBadRequest, "NOT_ONLINE", "You must be online to perform this action"
	case errors.Is(err, domain.ErrRiderBlockedForCommission):
		return http.StatusForbidden, "COMMISSION_BLOCKED", "You have unpaid commission. Please pay your outstanding commission to continue accepting jobs."
	case errors.Is(err, domain.ErrJobAlreadyAccepted):
		return http.StatusConflict, "JOB_ALREADY_ACCEPTED", "This job has already been accepted by another rider"
	case errors.Is(err, domain.ErrInvalidPickupCode):
		return http.StatusBadRequest, "INVALID_CODE", "Invalid pickup verification code"
	case errors.Is(err, domain.ErrInvalidDropoffCode):
		return http.StatusBadRequest, "INVALID_CODE", "Invalid delivery verification code"
	case errors.Is(err, domain.ErrDeliveryJobNotFound):
		return http.StatusNotFound, "JOB_NOT_FOUND", "Delivery job not found"
	case errors.Is(err, domain.ErrInvalidStatusTransition):
		return http.StatusBadRequest, "INVALID_STATUS", "Cannot perform this action in the current status"
	case errors.Is(err, domain.ErrUnauthorized):
		return http.StatusForbidden, domain.CodeUnauthorized, "You don't have permission for this action"
	default:
		return http.StatusInternalServerError, domain.CodeInternalError, "An unexpected error occurred"
	}
}

// Request/Response types

type UpdateLocationRequest struct {
	Latitude  float64  `json:"latitude" binding:"required"`
	Longitude float64  `json:"longitude" binding:"required"`
	Heading   *float64 `json:"heading"` // Optional
	Speed     *float64 `json:"speed"`   // Optional
}

type VerificationCodeRequest struct {
	Code string `json:"code" binding:"required"`
}

type RiderProfileResponse struct {
	ID              uuid.UUID `json:"id"`
	UserID          uuid.UUID `json:"user_id"`
	Status          string    `json:"status"`
	IsOnline        bool      `json:"is_online"`
	VehicleType     *string   `json:"vehicle_type,omitempty"`
	VehiclePlate    *string   `json:"vehicle_plate,omitempty"`
	Rating          float64   `json:"rating"`
	TotalDeliveries int       `json:"total_deliveries"`
	TotalEarnings   float64   `json:"total_earnings"`
	CreatedAt       string    `json:"created_at"`
}

func riderProfileToResponse(p *domain.RiderProfile) RiderProfileResponse {
	return RiderProfileResponse{
		ID:              p.ID,
		UserID:          p.UserID,
		Status:          string(p.Status),
		IsOnline:        p.IsOnline,
		VehicleType:     p.VehicleType,
		VehiclePlate:    p.VehiclePlate,
		Rating:          p.Rating,
		TotalDeliveries: p.TotalDeliveries,
		TotalEarnings:   p.TotalEarnings,
		CreatedAt:       p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

type JobOfferResponse struct {
	ID            uuid.UUID `json:"id"`
	DeliveryJobID uuid.UUID `json:"delivery_job_id"`
	DistanceKM    *float64  `json:"distance_km,omitempty"`
	OfferedAt     string    `json:"offered_at"`
}

func jobOfferToResponse(o *domain.RiderJobOffer) JobOfferResponse {
	return JobOfferResponse{
		ID:            o.ID,
		DeliveryJobID: o.DeliveryJobID,
		DistanceKM:    o.DistanceKM,
		OfferedAt:     o.OfferedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

type DeliveryJobResponse struct {
	ID                  uuid.UUID                  `json:"id"`
	Status              string                     `json:"status"`
	PickupAddress       string                     `json:"pickup_address"`
	DropoffAddress      string                     `json:"dropoff_address"`
	DistanceKM          *float64                   `json:"distance_km,omitempty"`
	DeliveryFee         float64                    `json:"delivery_fee"`
	RequiredVehicleType string                     `json:"required_vehicle_type"`
	RiderPayout         *float64                   `json:"rider_payout,omitempty"`
	FeeBreakdown        *FeeBreakdownResponse      `json:"fee_breakdown,omitempty"`
	PickupCode          *string                    `json:"pickup_code,omitempty"`
	DropoffCode         *string                    `json:"dropoff_code,omitempty"`
	AcceptedAt          *string                    `json:"accepted_at,omitempty"`
	PickedUpAt          *string                    `json:"picked_up_at,omitempty"`
	DeliveredAt         *string                    `json:"delivered_at,omitempty"`
	CreatedAt           string                     `json:"created_at"`
}

// FeeBreakdownResponse shows the rider what they'll earn from a delivery
type FeeBreakdownResponse struct {
	DeliveryFee        float64 `json:"delivery_fee"`        // Full fee buyer pays: ₦2,500
	PlatformCommission float64 `json:"platform_commission"` // 15% to platform: ₦375
	YourTake           float64 `json:"your_take"`           // What rider keeps: ₦2,125
	CommissionRate     float64 `json:"commission_rate"`     // 0.15
}

func deliveryJobToResponse(j *domain.DeliveryJob) DeliveryJobResponse {
	resp := DeliveryJobResponse{
		ID:                  j.ID,
		Status:              string(j.Status),
		PickupAddress:       j.PickupAddress,
		DropoffAddress:      j.DropoffAddress,
		DistanceKM:          j.DistanceKM,
		DeliveryFee:         j.DeliveryFee,
		RequiredVehicleType: string(j.RequiredVehicleType),
		RiderPayout:         j.RiderPayout,
		PickupCode:          j.PickupCode,
		DropoffCode:         j.DropoffCode,
		CreatedAt:           j.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	// Add fee breakdown for rider transparency
	if j.DeliveryFee > 0 {
		breakdown := domain.NewRiderFeeBreakdown(j.DeliveryFee)
		resp.FeeBreakdown = &FeeBreakdownResponse{
			DeliveryFee:        breakdown.DeliveryFee,
			PlatformCommission: breakdown.PlatformCommission,
			YourTake:           breakdown.RiderTake,
			CommissionRate:     breakdown.CommissionRate,
		}
	}

	if j.AcceptedAt != nil {
		acceptedAt := j.AcceptedAt.Format("2006-01-02T15:04:05Z07:00")
		resp.AcceptedAt = &acceptedAt
	}
	if j.PickedUpAt != nil {
		pickedUpAt := j.PickedUpAt.Format("2006-01-02T15:04:05Z07:00")
		resp.PickedUpAt = &pickedUpAt
	}
	if j.DeliveredAt != nil {
		deliveredAt := j.DeliveredAt.Format("2006-01-02T15:04:05Z07:00")
		resp.DeliveredAt = &deliveredAt
	}

	return resp
}

