package handler

import (
	"errors"
	"net/http"

	"kipa/internal/domain"
	"kipa/internal/dto"
	"kipa/internal/handler/middleware"
	"kipa/internal/service"
	"kipa/internal/util"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type LogisticsHandler struct {
	logisticsService service.LogisticsDeliveryService
}

func NewLogisticsHandler(logisticsService service.LogisticsDeliveryService) *LogisticsHandler {
	return &LogisticsHandler{logisticsService: logisticsService}
}

func (h *LogisticsHandler) RegisterRoutes(router *gin.RouterGroup, authMiddleware *middleware.AuthMiddleware, adminMiddleware *middleware.AdminMiddleware) {
	logistics := router.Group("/logistics")
	logistics.Use(authMiddleware.RequireAuth())
	{
		// Queries
		logistics.GET("/:id", h.GetDelivery)
		logistics.GET("/:id/details", h.GetDeliveryDetails)
		logistics.GET("/:id/timeline", h.GetTimeline)
		logistics.GET("/seller", h.ListSellerDeliveries)
		logistics.GET("/buyer", h.ListBuyerDeliveries)

		// Seller actions
		logistics.POST("/:id/ship", h.MarkShipped)
		logistics.PATCH("/:id/tracking", h.UpdateTracking)
		logistics.POST("/:id/claim-delivery", h.ClaimDelivery)
		logistics.POST("/:id/confirm-return", h.ConfirmReturnReceived)

		// Buyer actions
		logistics.POST("/:id/confirm", h.ConfirmDelivery)
		logistics.POST("/:id/cancel", h.CancelDelivery)
		logistics.POST("/:id/return-shipped", h.MarkReturnShipped)

		// Dispute actions (both buyer and seller)
		logistics.POST("/:id/dispute", h.OpenDispute)

		// Evidence upload
		logistics.POST("/:id/evidence", h.UploadEvidence)
	}

	// Admin routes
	admin := router.Group("/admin/logistics")
	admin.Use(authMiddleware.RequireAuth(), adminMiddleware.RequireAdmin())
	{
		admin.GET("/pending-approval", h.ListPendingAdminApproval)
		admin.GET("/damage-claims", h.ListDamageClaims)
		admin.GET("/pending-refunds", h.ListPendingRefundApprovals)
		admin.POST("/:id/resolve-dispute", h.ResolveDispute)
		admin.POST("/:id/approve-release", h.ApproveRelease)
		admin.POST("/:id/resolve-damage-claim", h.ResolveDamageClaim)
		admin.POST("/:id/approve-refund", h.ApproveRefund)
	}
}

// ============================================
// QUERIES
// ============================================

func (h *LogisticsHandler) GetDelivery(c *gin.Context) {
	deliveryID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid delivery ID")
		return
	}

	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	delivery, err := h.logisticsService.GetByID(c.Request.Context(), deliveryID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	// Check authorization
	if delivery.SellerID != userID && delivery.BuyerID != userID {
		util.Error(c, http.StatusForbidden, domain.CodeUnauthorized, "Not authorized to view this delivery")
		return
	}

	// Get timeline
	timeline, _ := h.logisticsService.GetTimeline(c.Request.Context(), deliveryID)

	util.Success(c, http.StatusOK, gin.H{
		"delivery": delivery,
		"timeline": timeline,
	})
}

func (h *LogisticsHandler) GetDeliveryDetails(c *gin.Context) {
	deliveryID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid delivery ID")
		return
	}

	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	details, err := h.logisticsService.GetDetails(c.Request.Context(), deliveryID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	// Check authorization
	if details.Delivery.SellerID != userID && details.Delivery.BuyerID != userID {
		util.Error(c, http.StatusForbidden, domain.CodeUnauthorized, "Not authorized to view this delivery")
		return
	}

	// Get timeline
	timeline, _ := h.logisticsService.GetTimeline(c.Request.Context(), deliveryID)

	util.Success(c, http.StatusOK, gin.H{
		"delivery":        details.Delivery,
		"payment_request": details.PaymentRequest,
		"escrow":          details.Escrow,
		"seller":          details.Seller,
		"buyer":           details.Buyer,
		"events":          details.Events,
		"evidence":        details.Evidence,
		"dispute":         details.Dispute,
		"timeline":        timeline,
	})
}

func (h *LogisticsHandler) ListSellerDeliveries(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req dto.ListLogisticsDeliveriesRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	var status *domain.LogisticsDeliveryStatus
	if req.Status != nil {
		s := domain.LogisticsDeliveryStatus(*req.Status)
		status = &s
	}

	limit := req.Limit
	if limit == 0 {
		limit = 20
	}

	response, err := h.logisticsService.ListForSeller(c.Request.Context(), userID, status, limit, req.Offset)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, response)
}

func (h *LogisticsHandler) ListBuyerDeliveries(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req dto.ListLogisticsDeliveriesRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	var status *domain.LogisticsDeliveryStatus
	if req.Status != nil {
		s := domain.LogisticsDeliveryStatus(*req.Status)
		status = &s
	}

	limit := req.Limit
	if limit == 0 {
		limit = 20
	}

	response, err := h.logisticsService.ListForBuyer(c.Request.Context(), userID, status, limit, req.Offset)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, response)
}

// ============================================
// SELLER ACTIONS
// ============================================

func (h *LogisticsHandler) MarkShipped(c *gin.Context) {
	deliveryID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid delivery ID")
		return
	}

	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req dto.MarkShippedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	if err := h.logisticsService.MarkShipped(c.Request.Context(), deliveryID, userID, &req); err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{"message": "Item marked as shipped"})
}

func (h *LogisticsHandler) UpdateTracking(c *gin.Context) {
	deliveryID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid delivery ID")
		return
	}

	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req dto.UpdateTrackingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	if err := h.logisticsService.UpdateTracking(c.Request.Context(), deliveryID, userID, &req); err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{"message": "Tracking info updated"})
}

func (h *LogisticsHandler) ClaimDelivery(c *gin.Context) {
	deliveryID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid delivery ID")
		return
	}

	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req dto.ClaimDeliveryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	if err := h.logisticsService.ClaimDelivery(c.Request.Context(), deliveryID, userID, &req); err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{"message": "Delivery claimed - buyer has been notified to confirm"})
}

func (h *LogisticsHandler) ConfirmReturnReceived(c *gin.Context) {
	deliveryID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid delivery ID")
		return
	}

	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req dto.ConfirmReturnReceivedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	if err := h.logisticsService.ConfirmReturnReceived(c.Request.Context(), deliveryID, userID, &req); err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{"message": "Return confirmed - refund will be processed"})
}

// ============================================
// BUYER ACTIONS
// ============================================

func (h *LogisticsHandler) ConfirmDelivery(c *gin.Context) {
	deliveryID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid delivery ID")
		return
	}

	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req dto.ConfirmDeliveryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	if err := h.logisticsService.ConfirmDelivery(c.Request.Context(), deliveryID, userID, &req); err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{"message": "Delivery confirmed - payment released to seller"})
}

func (h *LogisticsHandler) CancelDelivery(c *gin.Context) {
	deliveryID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid delivery ID")
		return
	}

	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req dto.CancelDeliveryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	if err := h.logisticsService.CancelDelivery(c.Request.Context(), deliveryID, userID, &req); err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{"message": "Order cancelled - refund will be processed"})
}

func (h *LogisticsHandler) MarkReturnShipped(c *gin.Context) {
	deliveryID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid delivery ID")
		return
	}

	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req dto.MarkReturnShippedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	if err := h.logisticsService.MarkReturnShipped(c.Request.Context(), deliveryID, userID, &req); err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{"message": "Return shipped - seller has been notified"})
}

// ============================================
// DISPUTE ACTIONS
// ============================================

func (h *LogisticsHandler) OpenDispute(c *gin.Context) {
	deliveryID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid delivery ID")
		return
	}

	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req dto.OpenDisputeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	// Determine opener type by checking delivery
	delivery, err := h.logisticsService.GetByID(c.Request.Context(), deliveryID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	var openerType string
	if delivery.BuyerID == userID {
		openerType = "buyer"
	} else if delivery.SellerID == userID {
		openerType = "seller"
	} else {
		util.Error(c, http.StatusForbidden, domain.CodeUnauthorized, "Not authorized to dispute this delivery")
		return
	}

	if err := h.logisticsService.OpenDispute(c.Request.Context(), deliveryID, userID, openerType, &req); err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{"message": "Dispute opened - admin will review"})
}

// ============================================
// EVIDENCE
// ============================================

func (h *LogisticsHandler) UploadEvidence(c *gin.Context) {
	deliveryID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid delivery ID")
		return
	}

	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req dto.UploadEvidenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	// Determine uploader type
	delivery, err := h.logisticsService.GetByID(c.Request.Context(), deliveryID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	var uploaderType string
	if delivery.BuyerID == userID {
		uploaderType = "buyer"
	} else if delivery.SellerID == userID {
		uploaderType = "seller"
	} else {
		util.Error(c, http.StatusForbidden, domain.CodeUnauthorized, "Not authorized to upload evidence for this delivery")
		return
	}

	if err := h.logisticsService.UploadEvidence(c.Request.Context(), deliveryID, userID, uploaderType, &req); err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{"message": "Evidence uploaded"})
}

// ============================================
// ADMIN ACTIONS
// ============================================

func (h *LogisticsHandler) ListPendingAdminApproval(c *gin.Context) {
	var req dto.ListLogisticsDeliveriesRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	limit := req.Limit
	if limit == 0 {
		limit = 20
	}

	response, err := h.logisticsService.ListPendingAdminApproval(c.Request.Context(), limit, req.Offset)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, response)
}

func (h *LogisticsHandler) ResolveDispute(c *gin.Context) {
	deliveryID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid delivery ID")
		return
	}

	adminID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req dto.AdminResolveDisputeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	if err := h.logisticsService.ResolveDispute(c.Request.Context(), deliveryID, adminID, &req); err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{"message": "Dispute resolved"})
}

func (h *LogisticsHandler) ApproveRelease(c *gin.Context) {
	deliveryID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid delivery ID")
		return
	}

	adminID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req dto.AdminApproveReleaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	if err := h.logisticsService.ApproveRelease(c.Request.Context(), deliveryID, adminID, &req); err != nil {
		h.handleError(c, err)
		return
	}

	if req.Approved {
		util.Success(c, http.StatusOK, gin.H{"message": "High-value release approved"})
	} else {
		util.Success(c, http.StatusOK, gin.H{"message": "Release flagged for investigation"})
	}
}

func (h *LogisticsHandler) ResolveDamageClaim(c *gin.Context) {
	deliveryID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid delivery ID")
		return
	}

	adminID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req dto.AdminResolveDamageClaimRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	if err := h.logisticsService.ResolveDamageClaim(c.Request.Context(), deliveryID, adminID, &req); err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{"message": "Damage claim resolved"})
}

func (h *LogisticsHandler) ApproveRefund(c *gin.Context) {
	deliveryID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid delivery ID")
		return
	}

	adminID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req dto.AdminApproveRefundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	if err := h.logisticsService.ApproveRefund(c.Request.Context(), deliveryID, adminID, &req); err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{"message": "Refund approved"})
}

func (h *LogisticsHandler) ListDamageClaims(c *gin.Context) {
	var req dto.ListLogisticsDeliveriesRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	limit := req.Limit
	if limit == 0 {
		limit = 20
	}

	response, err := h.logisticsService.ListDamageClaims(c.Request.Context(), limit, req.Offset)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, response)
}

func (h *LogisticsHandler) ListPendingRefundApprovals(c *gin.Context) {
	var req dto.ListLogisticsDeliveriesRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	limit := req.Limit
	if limit == 0 {
		limit = 20
	}

	response, err := h.logisticsService.ListPendingRefundApprovals(c.Request.Context(), limit, req.Offset)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, response)
}

func (h *LogisticsHandler) GetTimeline(c *gin.Context) {
	deliveryID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid delivery ID")
		return
	}

	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	// Check authorization first
	delivery, err := h.logisticsService.GetByID(c.Request.Context(), deliveryID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	if delivery.SellerID != userID && delivery.BuyerID != userID {
		util.Error(c, http.StatusForbidden, domain.CodeUnauthorized, "Not authorized to view this delivery timeline")
		return
	}

	timeline, err := h.logisticsService.GetTimeline(c.Request.Context(), deliveryID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, timeline)
}

// ============================================
// ERROR HANDLING
// ============================================

func (h *LogisticsHandler) handleError(c *gin.Context, err error) {
	status, code, message := mapLogisticsErrorToResponse(err)
	util.Error(c, status, code, message)
}

func mapLogisticsErrorToResponse(err error) (int, string, string) {
	switch {
	case errors.Is(err, domain.ErrLogisticsDeliveryNotFound):
		return http.StatusNotFound, "DELIVERY_NOT_FOUND", "Logistics delivery not found"
	case errors.Is(err, domain.ErrUnauthorized):
		return http.StatusForbidden, domain.CodeUnauthorized, "Not authorized for this action"
	case errors.Is(err, domain.ErrInvalidCarrier):
		return http.StatusBadRequest, "INVALID_CARRIER", "Invalid logistics carrier"
	case errors.Is(err, domain.ErrTrackingNumberRequired):
		return http.StatusBadRequest, "TRACKING_REQUIRED", "Tracking number is required"
	case errors.Is(err, domain.ErrShipmentReceiptRequired):
		return http.StatusBadRequest, "RECEIPT_REQUIRED", "Shipment receipt is required"
	case errors.Is(err, domain.ErrCannotShipYet):
		return http.StatusBadRequest, "CANNOT_SHIP", "Cannot mark as shipped in current state"
	case errors.Is(err, domain.ErrCannotClaimDelivery):
		return http.StatusBadRequest, "CANNOT_CLAIM", "Cannot claim delivery in current state"
	case errors.Is(err, domain.ErrCannotClaimDeliveryTooEarly):
		return http.StatusBadRequest, "CLAIM_TOO_EARLY", "Cannot claim delivery within 24 hours of shipping"
	case errors.Is(err, domain.ErrCannotConfirmDelivery):
		return http.StatusBadRequest, "CANNOT_CONFIRM", "Cannot confirm delivery in current state"
	case errors.Is(err, domain.ErrCannotCancelAfterShipment):
		return http.StatusBadRequest, "CANNOT_CANCEL", "Cannot cancel after item has been shipped"
	case errors.Is(err, domain.ErrShipDeadlineExceeded):
		return http.StatusBadRequest, "DEADLINE_NOT_EXCEEDED", "Ship deadline has not been exceeded yet"
	case errors.Is(err, domain.ErrCannotOpenDisputeInState):
		return http.StatusBadRequest, "CANNOT_DISPUTE", "Cannot open dispute in current state"
	case errors.Is(err, domain.ErrSellerCannotDisputeYet):
		return http.StatusBadRequest, "DISPUTE_TOO_EARLY", "Seller can only open dispute after 7 days of buyer non-response"
	case errors.Is(err, domain.ErrDisputeAlreadyOpen):
		return http.StatusConflict, "DISPUTE_EXISTS", "A dispute is already open for this delivery"
	case errors.Is(err, domain.ErrInvalidStatusTransition):
		return http.StatusBadRequest, "INVALID_STATUS", "Invalid status transition"
	default:
		return http.StatusInternalServerError, domain.CodeInternalError, "An unexpected error occurred"
	}
}
