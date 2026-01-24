package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"kipa/internal/domain"
	"kipa/internal/handler/middleware"
	"kipa/internal/service"
	"kipa/internal/util"
)

type DriverHandler struct {
	driverService service.DriverService
}

func NewDriverHandler(driverService service.DriverService) *DriverHandler {
	return &DriverHandler{driverService: driverService}
}

func (h *DriverHandler) RegisterRoutes(router *gin.RouterGroup, authMiddleware *middleware.AuthMiddleware) {
	driver := router.Group("/driver")
	driver.Use(authMiddleware.RequireAuth())
	{
		// Profile management
		driver.GET("/profile", h.GetProfile)
		driver.POST("/profile", h.CreateProfile)
		driver.PATCH("/profile", h.UpdateProfile)

		// Online/Offline
		driver.POST("/go-online", h.GoOnline)
		driver.POST("/go-offline", h.GoOffline)

		// Available deliveries
		driver.GET("/available-deliveries", h.GetAvailableDeliveries)
		driver.POST("/deliveries/:id/accept", h.AcceptDelivery)

		// Current trip
		driver.GET("/trip", h.GetCurrentTrip)
		driver.GET("/trip/deliveries", h.GetTripDeliveries)

		// Manual status updates (no GPS tracking)
		driver.POST("/deliveries/:id/at-pickup", h.MarkAtPickup)
		driver.POST("/deliveries/:id/pickup", h.VerifyPickupCode)
		driver.POST("/trip/depart", h.MarkDeparted)
		driver.POST("/trip/arrive", h.MarkArrived)
		driver.POST("/deliveries/:id/dropoff", h.VerifyDropoffCode)
		driver.POST("/deliveries/:id/buyer-unreachable", h.MarkBuyerUnreachable)

		// History
		driver.GET("/history", h.GetTripHistory)
		driver.GET("/deliveries/history", h.GetDeliveryHistory)
		driver.GET("/earnings", h.GetEarnings)
	}
}

// ============================================
// PROFILE MANAGEMENT
// ============================================

func (h *DriverHandler) GetProfile(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	profile, err := h.driverService.GetProfile(c.Request.Context(), userID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, driverProfileToResponse(profile))
}

func (h *DriverHandler) CreateProfile(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req service.CreateDriverProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	profile, err := h.driverService.CreateProfile(c.Request.Context(), userID, &req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusCreated, driverProfileToResponse(profile))
}

func (h *DriverHandler) UpdateProfile(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req service.UpdateDriverProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	profile, err := h.driverService.UpdateProfile(c.Request.Context(), userID, &req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, driverProfileToResponse(profile))
}

// ============================================
// ONLINE/OFFLINE
// ============================================

type GoOnlineRequest struct {
	OriginCity      string `json:"origin_city" binding:"required"`
	DestinationCity string `json:"destination_city" binding:"required"`
}

func (h *DriverHandler) GoOnline(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req GoOnlineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	trip, err := h.driverService.GoOnline(c.Request.Context(), userID, req.OriginCity, req.DestinationCity)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, driverTripToResponse(trip))
}

func (h *DriverHandler) GoOffline(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	if err := h.driverService.GoOffline(c.Request.Context(), userID); err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{"message": "You are now offline"})
}

// ============================================
// AVAILABLE DELIVERIES
// ============================================

func (h *DriverHandler) GetAvailableDeliveries(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	deliveries, err := h.driverService.GetAvailableDeliveries(c.Request.Context(), userID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	responses := make([]DriverDeliveryResponse, len(deliveries))
	for i, d := range deliveries {
		responses[i] = driverDeliveryToResponse(d)
	}

	util.Success(c, http.StatusOK, responses)
}

func (h *DriverHandler) AcceptDelivery(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	deliveryID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, domain.CodeInvalidInput, "Invalid delivery ID")
		return
	}

	delivery, err := h.driverService.AcceptDelivery(c.Request.Context(), userID, deliveryID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, driverDeliveryToResponse(delivery))
}

// ============================================
// TRIP MANAGEMENT
// ============================================

func (h *DriverHandler) GetCurrentTrip(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	trip, err := h.driverService.GetCurrentTrip(c.Request.Context(), userID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, driverTripToResponse(trip))
}

func (h *DriverHandler) GetTripDeliveries(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	deliveries, err := h.driverService.GetTripDeliveries(c.Request.Context(), userID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	responses := make([]DriverDeliveryResponse, len(deliveries))
	for i, d := range deliveries {
		responses[i] = driverDeliveryToResponse(d)
	}

	util.Success(c, http.StatusOK, responses)
}

// ============================================
// MANUAL STATUS UPDATES
// ============================================

func (h *DriverHandler) MarkAtPickup(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	deliveryID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, domain.CodeInvalidInput, "Invalid delivery ID")
		return
	}

	if err := h.driverService.MarkAtPickup(c.Request.Context(), userID, deliveryID); err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{"message": "Marked as arrived at pickup location"})
}

type VerifyCodeRequest struct {
	Code string `json:"code" binding:"required,len=6"`
}

func (h *DriverHandler) VerifyPickupCode(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	deliveryID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, domain.CodeInvalidInput, "Invalid delivery ID")
		return
	}

	var req VerifyCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	if err := h.driverService.VerifyPickupCode(c.Request.Context(), userID, deliveryID, req.Code); err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{"message": "Pickup verified. Item collected."})
}

func (h *DriverHandler) MarkDeparted(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	if err := h.driverService.MarkDeparted(c.Request.Context(), userID); err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{"message": "Trip departed. Have a safe journey!"})
}

func (h *DriverHandler) MarkArrived(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	if err := h.driverService.MarkArrived(c.Request.Context(), userID); err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{"message": "Arrived at destination. Ready for deliveries."})
}

func (h *DriverHandler) VerifyDropoffCode(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	deliveryID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, domain.CodeInvalidInput, "Invalid delivery ID")
		return
	}

	var req VerifyCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	if err := h.driverService.VerifyDropoffCode(c.Request.Context(), userID, deliveryID, req.Code); err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{"message": "Delivery completed. Awaiting buyer confirmation."})
}

func (h *DriverHandler) MarkBuyerUnreachable(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	deliveryID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, domain.CodeInvalidInput, "Invalid delivery ID")
		return
	}

	if err := h.driverService.MarkBuyerUnreachable(c.Request.Context(), userID, deliveryID); err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{"message": "Buyer marked as unreachable. They have 24 hours to respond."})
}

// ============================================
// HISTORY
// ============================================

func (h *DriverHandler) GetTripHistory(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	limit := driverParseIntParam(c, "limit", 20)
	offset := driverParseIntParam(c, "offset", 0)

	trips, err := h.driverService.GetTripHistory(c.Request.Context(), userID, limit, offset)
	if err != nil {
		h.handleError(c, err)
		return
	}

	responses := make([]DriverTripResponse, len(trips))
	for i, t := range trips {
		responses[i] = driverTripToResponse(t)
	}

	util.Success(c, http.StatusOK, responses)
}

func (h *DriverHandler) GetDeliveryHistory(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	limit := driverParseIntParam(c, "limit", 20)
	offset := driverParseIntParam(c, "offset", 0)

	deliveries, err := h.driverService.GetDeliveryHistory(c.Request.Context(), userID, limit, offset)
	if err != nil {
		h.handleError(c, err)
		return
	}

	responses := make([]DriverDeliveryResponse, len(deliveries))
	for i, d := range deliveries {
		responses[i] = driverDeliveryToResponse(d)
	}

	util.Success(c, http.StatusOK, responses)
}

func (h *DriverHandler) GetEarnings(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	earnings, err := h.driverService.GetEarnings(c.Request.Context(), userID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, earnings)
}

// ============================================
// ERROR HANDLING
// ============================================

func (h *DriverHandler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrDriverProfileNotFound):
		util.Error(c, http.StatusNotFound, "DRIVER_PROFILE_NOT_FOUND", "Driver profile not found. Please create one first.")
	case errors.Is(err, domain.ErrDriverProfileExists):
		util.Error(c, http.StatusConflict, "DRIVER_PROFILE_EXISTS", "Driver profile already exists")
	case errors.Is(err, domain.ErrDriverNotVerified):
		util.Error(c, http.StatusForbidden, "DRIVER_NOT_VERIFIED", "Your driver profile is pending verification")
	case errors.Is(err, domain.ErrDriverAlreadyOnline):
		util.Error(c, http.StatusConflict, "DRIVER_ALREADY_ONLINE", "You are already online")
	case errors.Is(err, domain.ErrDriverHasActiveTrip):
		util.Error(c, http.StatusConflict, "DRIVER_HAS_ACTIVE_TRIP", "Cannot go offline with active deliveries")
	case errors.Is(err, domain.ErrDriverNotAccepting):
		util.Error(c, http.StatusBadRequest, "DRIVER_NOT_ACCEPTING", "You are not accepting deliveries. Go online first.")
	case errors.Is(err, domain.ErrNoActiveTrip):
		util.Error(c, http.StatusNotFound, "NO_ACTIVE_TRIP", "No active trip found")
	case errors.Is(err, domain.ErrDriverCapacityExceeded):
		util.Error(c, http.StatusBadRequest, "CAPACITY_EXCEEDED", "This delivery exceeds your carrying capacity")
	case errors.Is(err, domain.ErrDeliveryAlreadyAssigned):
		util.Error(c, http.StatusConflict, "DELIVERY_ALREADY_ASSIGNED", "This delivery has already been assigned")
	case errors.Is(err, domain.ErrRouteMismatch):
		util.Error(c, http.StatusBadRequest, "ROUTE_MISMATCH", "Delivery route does not match your trip")
	case errors.Is(err, domain.ErrInvalidPickupCode):
		util.Error(c, http.StatusBadRequest, "INVALID_PICKUP_CODE", "Invalid pickup code")
	case errors.Is(err, domain.ErrInvalidDropoffCode):
		util.Error(c, http.StatusBadRequest, "INVALID_DROPOFF_CODE", "Invalid dropoff code")
	case errors.Is(err, domain.ErrPickupCodeLocked):
		util.Error(c, http.StatusForbidden, "PICKUP_CODE_LOCKED", "Too many failed attempts. Contact support.")
	case errors.Is(err, domain.ErrDropoffCodeLocked):
		util.Error(c, http.StatusForbidden, "DROPOFF_CODE_LOCKED", "Too many failed attempts. Contact support.")
	case errors.Is(err, domain.ErrInvalidStatusTransition):
		util.Error(c, http.StatusBadRequest, "INVALID_STATUS", "This action is not allowed in the current state")
	case errors.Is(err, domain.ErrUnauthorized):
		util.Error(c, http.StatusForbidden, domain.CodeUnauthorized, "You don't have permission for this action")
	case errors.Is(err, domain.ErrDriverDeliveryNotFound):
		util.Error(c, http.StatusNotFound, "DELIVERY_NOT_FOUND", "Delivery not found")
	default:
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "An error occurred")
	}
}

// ============================================
// RESPONSE TYPES
// ============================================

type DriverProfileResponse struct {
	ID                  string  `json:"id"`
	UserID              string  `json:"user_id"`
	Status              string  `json:"status"`
	VehicleType         string  `json:"vehicle_type"`
	LicensePlate        string  `json:"license_plate"`
	IsVerified          bool    `json:"is_verified"`
	MaxCarryingValue    float64 `json:"max_carrying_value"`
	CurrentCarryValue   float64 `json:"current_carry_value"`
	MaxPackageCount     int     `json:"max_package_count"`
	CurrentPackageCount int     `json:"current_package_count"`
	TotalDeliveries     int     `json:"total_deliveries"`
	TotalEarnings       float64 `json:"total_earnings"`
	Rating              float64 `json:"rating"`
	CurrentTripID       *string `json:"current_trip_id,omitempty"`
	OriginCity          *string `json:"origin_city,omitempty"`
	DestinationCity     *string `json:"destination_city,omitempty"`
	CreatedAt           string  `json:"created_at"`
}

type DriverTripResponse struct {
	ID              string   `json:"id"`
	DriverID        string   `json:"driver_id"`
	OriginCity      string   `json:"origin_city"`
	DestinationCity string   `json:"destination_city"`
	Status          string   `json:"status"`
	TotalPackages   int      `json:"total_packages"`
	TotalValue      float64  `json:"total_value"`
	TotalDriverFee  float64  `json:"total_driver_fee"`
	AcceptingUntil  *string  `json:"accepting_until,omitempty"`
	DepartedAt      *string  `json:"departed_at,omitempty"`
	ArrivedAt       *string  `json:"arrived_at,omitempty"`
	CompletedAt     *string  `json:"completed_at,omitempty"`
	CreatedAt       string   `json:"created_at"`
}

type DriverDeliveryResponse struct {
	ID               string   `json:"id"`
	PaymentRequestID string   `json:"payment_request_id"`
	TripID           *string  `json:"trip_id,omitempty"`
	DriverID         *string  `json:"driver_id,omitempty"`
	OriginCity       string   `json:"origin_city"`
	DestinationCity  string   `json:"destination_city"`
	PickupAddress    string   `json:"pickup_address"`
	DropoffAddress   string   `json:"dropoff_address"`
	ItemValue        float64  `json:"item_value"`
	DriverFee        float64  `json:"driver_fee"`
	Status           string   `json:"status"`
	PickupAttempts   int      `json:"pickup_attempts"`
	DropoffAttempts  int      `json:"dropoff_attempts"`
	DriverAssignedAt *string  `json:"driver_assigned_at,omitempty"`
	AtPickupAt       *string  `json:"at_pickup_at,omitempty"`
	PickedUpAt       *string  `json:"picked_up_at,omitempty"`
	DepartedAt       *string  `json:"departed_at,omitempty"`
	ArrivedAt        *string  `json:"arrived_at,omitempty"`
	DeliveredAt      *string  `json:"delivered_at,omitempty"`
	CompletedAt      *string  `json:"completed_at,omitempty"`
	CreatedAt        string   `json:"created_at"`
	Timeline         *DriverDeliveryTimelineResponse `json:"timeline,omitempty"`
}

func driverProfileToResponse(p *domain.DriverProfile) DriverProfileResponse {
	resp := DriverProfileResponse{
		ID:                  p.ID.String(),
		UserID:              p.UserID.String(),
		Status:              string(p.Status),
		VehicleType:         string(p.VehicleType),
		LicensePlate:        p.LicensePlate,
		IsVerified:          p.IsVerified,
		MaxCarryingValue:    p.MaxCarryingValue,
		CurrentCarryValue:   p.CurrentCarryValue,
		MaxPackageCount:     p.MaxPackageCount,
		CurrentPackageCount: p.CurrentPackageCount,
		TotalDeliveries:     p.TotalDeliveries,
		TotalEarnings:       p.TotalEarnings,
		Rating:              p.Rating,
		CreatedAt:           p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	if p.CurrentTripID != nil {
		id := p.CurrentTripID.String()
		resp.CurrentTripID = &id
	}
	if p.OriginCity != nil {
		resp.OriginCity = p.OriginCity
	}
	if p.DestinationCity != nil {
		resp.DestinationCity = p.DestinationCity
	}

	return resp
}

func driverTripToResponse(t *domain.DriverTrip) DriverTripResponse {
	resp := DriverTripResponse{
		ID:              t.ID.String(),
		DriverID:        t.DriverID.String(),
		OriginCity:      t.OriginCity,
		DestinationCity: t.DestinationCity,
		Status:          string(t.Status),
		TotalPackages:   t.TotalPackages,
		TotalValue:      t.TotalValue,
		TotalDriverFee:  t.TotalDriverFee,
		CreatedAt:       t.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	if t.AcceptingUntil != nil {
		s := t.AcceptingUntil.Format("2006-01-02T15:04:05Z07:00")
		resp.AcceptingUntil = &s
	}
	if t.DepartedAt != nil {
		s := t.DepartedAt.Format("2006-01-02T15:04:05Z07:00")
		resp.DepartedAt = &s
	}
	if t.ArrivedAt != nil {
		s := t.ArrivedAt.Format("2006-01-02T15:04:05Z07:00")
		resp.ArrivedAt = &s
	}
	if t.CompletedAt != nil {
		s := t.CompletedAt.Format("2006-01-02T15:04:05Z07:00")
		resp.CompletedAt = &s
	}

	return resp
}

func driverDeliveryToResponse(d *domain.DriverDelivery) DriverDeliveryResponse {
	resp := DriverDeliveryResponse{
		ID:               d.ID.String(),
		PaymentRequestID: d.PaymentRequestID.String(),
		OriginCity:       d.OriginCity,
		DestinationCity:  d.DestinationCity,
		PickupAddress:    d.PickupAddress,
		DropoffAddress:   d.DropoffAddress,
		ItemValue:        d.ItemValue,
		DriverFee:        d.DriverFee,
		Status:           string(d.Status),
		PickupAttempts:   d.PickupAttempts,
		DropoffAttempts:  d.DropoffAttempts,
		CreatedAt:        d.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	if d.TripID != nil {
		id := d.TripID.String()
		resp.TripID = &id
	}
	if d.DriverID != nil {
		id := d.DriverID.String()
		resp.DriverID = &id
	}
	if d.DriverAssignedAt != nil {
		s := d.DriverAssignedAt.Format("2006-01-02T15:04:05Z07:00")
		resp.DriverAssignedAt = &s
	}
	if d.AtPickupAt != nil {
		s := d.AtPickupAt.Format("2006-01-02T15:04:05Z07:00")
		resp.AtPickupAt = &s
	}
	if d.PickedUpAt != nil {
		s := d.PickedUpAt.Format("2006-01-02T15:04:05Z07:00")
		resp.PickedUpAt = &s
	}
	if d.DepartedAt != nil {
		s := d.DepartedAt.Format("2006-01-02T15:04:05Z07:00")
		resp.DepartedAt = &s
	}
	if d.ArrivedAt != nil {
		s := d.ArrivedAt.Format("2006-01-02T15:04:05Z07:00")
		resp.ArrivedAt = &s
	}
	if d.DeliveredAt != nil {
		s := d.DeliveredAt.Format("2006-01-02T15:04:05Z07:00")
		resp.DeliveredAt = &s
	}
	if d.CompletedAt != nil {
		s := d.CompletedAt.Format("2006-01-02T15:04:05Z07:00")
		resp.CompletedAt = &s
	}

	return resp
}

func driverParseIntParam(c *gin.Context, key string, defaultValue int) int {
	if v := c.Query(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultValue
}
