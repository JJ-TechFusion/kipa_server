package handler

import (
	"encoding/json"
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
		rider.GET("/profile", h.GetProfile)
		rider.POST("/profile", h.CreateProfile)
		rider.PATCH("/profile", h.UpdateProfile)
		rider.POST("/go-online", h.GoOnline)
		rider.POST("/go-offline", h.GoOffline)
		rider.PATCH("/location", h.UpdateLocation)
		rider.GET("/available-jobs", h.GetAvailableJobs)
		rider.GET("/available-jobs/:id", h.GetAvailableJobByID)
		rider.POST("/jobs/:id/accept", h.AcceptJob)
		rider.GET("/jobs/active", h.GetActiveJob)
		rider.POST("/jobs/:id/pickup", h.ConfirmPickup)
		rider.POST("/jobs/:id/deliver", h.ConfirmDelivery)
		rider.GET("/jobs/history", h.GetJobHistory)
		rider.GET("/earnings", h.GetEarnings)
		rider.POST("/jobs/:id/mark-seller-unavailable", h.MarkSellerUnavailable)
		rider.POST("/jobs/:id/mark-buyer-unavailable", h.MarkBuyerUnavailable)
		rider.POST("/jobs/:id/complete-forced-return", h.CompleteForcedReturn)
	}
}
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
func (h *RiderHandler) GetAvailableJobs(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	availableJobs, err := h.riderService.GetAvailableJobs(c.Request.Context(), userID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	responses := make([]DeliveryJobResponse, len(availableJobs))
	for i, aj := range availableJobs {
		responses[i] = deliveryJobToResponse(aj.Job)
		if aj.Errand != nil {
			responses[i].ErrandDetails = errandToDetailsResponse(aj.Errand)
		}
	}

	util.Success(c, http.StatusOK, gin.H{
		"jobs":  responses,
		"count": len(responses),
	})
}
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

	job, errand, err := h.riderService.AcceptJob(c.Request.Context(), userID, jobID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	jobResp := deliveryJobToResponse(job)
	if errand != nil {
		jobResp.ErrandDetails = errandToDetailsResponse(errand)
	}

	util.Success(c, http.StatusOK, gin.H{
		"message":     "Job accepted successfully",
		"job":         jobResp,
		"pickup_code": job.PickupCode,
	})
}
func (h *RiderHandler) GetActiveJob(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	details, err := h.riderService.GetActiveJob(c.Request.Context(), userID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	if details == nil {
		util.Success(c, http.StatusOK, gin.H{"job": nil, "message": "No active job"})
		return
	}

	job := details.Job
	pr := details.PaymentRequest

	resp := DeliveryJobDetailResponse{
		DeliveryJobResponse: deliveryJobToResponse(job),
		PickupLat:           job.PickupLat,
		PickupLng:           job.PickupLng,
		DropoffLat:          job.DropoffLat,
		DropoffLng:          job.DropoffLng,
	}

	if pr != nil {
		var images []string
		if len(pr.ItemImages) > 0 {
			_ = json.Unmarshal(pr.ItemImages, &images)
		}
		resp.PackageDetails = &PackageDetailsResponse{
			ItemName:        pr.ItemName,
			ItemDescription: pr.ItemDescription,
			ItemPrice:       pr.ItemPrice,
			ItemImages:      images,
			IsPaid:          pr.Status.IsPaid(),
			OrderStatus:     string(pr.Status),
		}
	}

	if details.Errand != nil {
		resp.ErrandDetails = errandToDetailsResponse(details.Errand)
	}

	if details.Buyer != nil {
		resp.Buyer = &ContactInfoResponse{
			Name:  details.Buyer.Name,
			Phone: details.Buyer.Phone,
		}
	}

	if details.Seller != nil {
		resp.Seller = &ContactInfoResponse{
			Name:  details.Seller.Name,
			Phone: details.Seller.Phone,
		}
	}

	util.Success(c, http.StatusOK, resp)
}
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
func (h *RiderHandler) MarkSellerUnavailable(c *gin.Context) {
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

	if err := h.riderService.MarkSellerUnavailable(c.Request.Context(), userID, jobID); err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{"message": "Seller marked as unavailable. Job cancelled and buyer will be refunded."})
}
func (h *RiderHandler) MarkBuyerUnavailable(c *gin.Context) {
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

	forcedReturnInfo, err := h.riderService.MarkBuyerUnavailable(c.Request.Context(), userID, jobID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"message":              "Buyer marked as unavailable. Please return item to seller.",
		"forced_return_job_id": forcedReturnInfo.ForcedReturnJobID,
		"return_code":          forcedReturnInfo.ReturnCode,
		"rebook_deadline":      forcedReturnInfo.RebookDeadline,
	})
}
func (h *RiderHandler) CompleteForcedReturn(c *gin.Context) {
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

	if err := h.riderService.CompleteForcedReturn(c.Request.Context(), userID, jobID, req.Code); err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{"message": "Return completed. Payment credited to your wallet."})
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
	case errors.Is(err, domain.ErrPickupCodeExpired):
		return http.StatusBadRequest, "CODE_EXPIRED", "Pickup verification code has expired"
	case errors.Is(err, domain.ErrDropoffCodeExpired):
		return http.StatusBadRequest, "CODE_EXPIRED", "Delivery verification code has expired"
	case errors.Is(err, domain.ErrPickupCodeLocked):
		return http.StatusForbidden, "CODE_LOCKED", "Too many failed pickup code attempts. Contact support."
	case errors.Is(err, domain.ErrDropoffCodeLocked):
		return http.StatusForbidden, "CODE_LOCKED", "Too many failed delivery code attempts. Contact support."
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
	VehicleBrand    *string   `json:"vehicle_brand,omitempty"`
	VehicleColor    *string   `json:"vehicle_color,omitempty"`
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
		VehicleBrand:    p.VehicleBrand,
		VehicleColor:    p.VehicleColor,
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

type DeliveryJobResponse struct {
	ID                  uuid.UUID              `json:"id"`
	DeliveryType        string                 `json:"delivery_type"`
	Status              string                 `json:"status"`
	PickupAddress       string                 `json:"pickup_address"`
	DropoffAddress      string                 `json:"dropoff_address"`
	DistanceKM          *float64               `json:"distance_km,omitempty"`
	EstimatedMinutes    *int                   `json:"estimated_minutes,omitempty"`
	DeliveryFee         float64                `json:"delivery_fee"`
	RequiredVehicleType string                 `json:"required_vehicle_type"`
	RiderPayout         *float64               `json:"rider_payout,omitempty"`
	FeeBreakdown        *FeeBreakdownResponse  `json:"fee_breakdown,omitempty"`
	PickupCode          *string                `json:"pickup_code,omitempty"`
	DropoffCode         *string                `json:"dropoff_code,omitempty"`
	AcceptedAt          *string                `json:"accepted_at,omitempty"`
	PickedUpAt          *string                `json:"picked_up_at,omitempty"`
	DeliveredAt         *string                `json:"delivered_at,omitempty"`
	CreatedAt           string                 `json:"created_at"`
	ErrandDetails       *ErrandDetailsResponse `json:"errand_details,omitempty"`
}
type FeeBreakdownResponse struct {
	DeliveryFee        float64 `json:"delivery_fee"`        // Full fee buyer pays: ₦2,500
	PlatformCommission float64 `json:"platform_commission"` // 15% to platform: ₦375
	YourTake           float64 `json:"your_take"`           // What rider keeps: ₦2,125
	CommissionRate     float64 `json:"commission_rate"`     // 0.15
}

func deliveryJobToResponse(j *domain.DeliveryJob) DeliveryJobResponse {
	deliveryType := "intrastate"
	if j.SourceType != nil && *j.SourceType == domain.DeliverySourceErrand {
		deliveryType = "errand"
	}

	resp := DeliveryJobResponse{
		ID:                  j.ID,
		DeliveryType:        deliveryType,
		Status:              string(j.Status),
		PickupAddress:       j.PickupAddress,
		DropoffAddress:      j.DropoffAddress,
		DistanceKM:          j.DistanceKM,
		EstimatedMinutes:    j.CurrentETAMinutes,
		DeliveryFee:         j.DeliveryFee,
		RequiredVehicleType: string(j.RequiredVehicleType),
		RiderPayout:         j.RiderPayout,
		PickupCode:          j.PickupCode,
		DropoffCode:         j.DropoffCode,
		CreatedAt:           j.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
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

type DeliveryJobDetailResponse struct {
	DeliveryJobResponse
	PickupLat      float64                 `json:"pickup_lat"`
	PickupLng      float64                 `json:"pickup_lng"`
	DropoffLat     float64                 `json:"dropoff_lat"`
	DropoffLng     float64                 `json:"dropoff_lng"`
	PackageDetails *PackageDetailsResponse `json:"package_details,omitempty"`
	Buyer          *ContactInfoResponse    `json:"buyer,omitempty"`
	Seller         *ContactInfoResponse    `json:"seller,omitempty"`
}

type ContactInfoResponse struct {
	Name  string `json:"name"`
	Phone string `json:"phone"`
}

type PackageDetailsResponse struct {
	ItemName        string   `json:"item_name"`
	ItemDescription *string  `json:"item_description,omitempty"`
	ItemPrice       float64  `json:"item_price"`
	ItemImages      []string `json:"item_images,omitempty"`
	IsPaid          bool     `json:"is_paid"`
	OrderStatus     string   `json:"order_status"`
}

type ErrandDetailsResponse struct {
	PickupInstructions  *string `json:"pickup_instructions,omitempty"`
	PackageDescription  *string `json:"package_description,omitempty"`
	PickupContactName   *string `json:"pickup_contact_name,omitempty"`
	PickupContactPhone  *string `json:"pickup_contact_phone,omitempty"`
	DropoffContactName  *string `json:"dropoff_contact_name,omitempty"`
	DropoffContactPhone *string `json:"dropoff_contact_phone,omitempty"`
}

func (h *RiderHandler) GetAvailableJobByID(c *gin.Context) {
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

	job, pr, errand, err := h.riderService.GetAvailableJobByID(c.Request.Context(), userID, jobID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	resp := DeliveryJobDetailResponse{
		DeliveryJobResponse: deliveryJobToResponse(job),
		PickupLat:           job.PickupLat,
		PickupLng:           job.PickupLng,
		DropoffLat:          job.DropoffLat,
		DropoffLng:          job.DropoffLng,
	}

	if pr != nil {
		var images []string
		if len(pr.ItemImages) > 0 {
			_ = json.Unmarshal(pr.ItemImages, &images)
		}
		resp.PackageDetails = &PackageDetailsResponse{
			ItemName:        pr.ItemName,
			ItemDescription: pr.ItemDescription,
			ItemPrice:       pr.ItemPrice,
			ItemImages:      images,
			IsPaid:          pr.Status.IsPaid(),
			OrderStatus:     string(pr.Status),
		}
	}

	if errand != nil {
		resp.ErrandDetails = errandToDetailsResponse(errand)
	}

	util.Success(c, http.StatusOK, resp)
}

func errandToDetailsResponse(e *domain.Errand) *ErrandDetailsResponse {
	return &ErrandDetailsResponse{
		PickupInstructions:  e.PickupInstructions,
		PackageDescription:  e.PackageDescription,
		PickupContactName:   e.PickupContactName,
		PickupContactPhone:  e.PickupContactPhone,
		DropoffContactName:  e.DropoffContactName,
		DropoffContactPhone: e.DropoffContactPhone,
	}
}
