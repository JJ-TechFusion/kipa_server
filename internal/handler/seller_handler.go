package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"kipa/internal/domain"
	"kipa/internal/handler/middleware"
	"kipa/internal/repository"
	"kipa/internal/service"
	"kipa/internal/util"
)

type SellerHandler struct {
	orderService             service.OrderService
	prService                service.PaymentRequestService
	escrowService            service.EscrowService
	deliveryJobRepo          repository.DeliveryJobRepository
	disputeResolutionService service.DisputeResolutionService
	driverService            service.DriverService
}

func NewSellerHandler(
	orderService service.OrderService,
	prService service.PaymentRequestService,
	escrowService service.EscrowService,
	deliveryJobRepo repository.DeliveryJobRepository,
	disputeResolutionService service.DisputeResolutionService,
	driverService service.DriverService,
) *SellerHandler {
	return &SellerHandler{
		orderService:             orderService,
		prService:                prService,
		escrowService:            escrowService,
		deliveryJobRepo:          deliveryJobRepo,
		disputeResolutionService: disputeResolutionService,
		driverService:            driverService,
	}
}

func (h *SellerHandler) RegisterRoutes(router *gin.RouterGroup, authMiddleware *middleware.AuthMiddleware) {
	seller := router.Group("/seller")
	seller.Use(authMiddleware.RequireAuth())
	{
		seller.GET("/sales", h.ListSales)
		seller.GET("/sales/:id", h.GetSale)
		seller.GET("/sales/:id/timeline", h.GetSaleTimeline)
		seller.GET("/earnings", h.GetEarnings)

		// Return flow endpoints
		seller.GET("/sales/:id/return", h.GetReturnStatus)
		seller.POST("/sales/:id/confirm-return", h.ConfirmReturn)

		// Driver delivery endpoints (inter-state)
		seller.GET("/driver-deliveries", h.ListDriverDeliveries)
		seller.GET("/driver-deliveries/:id", h.GetDriverDelivery)
		seller.GET("/driver-deliveries/:id/timeline", h.GetDriverDeliveryTimeline)
		seller.GET("/driver-deliveries/:id/pickup-code", h.GetPickupCode)
		seller.POST("/driver-deliveries/:id/cancel", h.CancelDriverDelivery)

		// Driver offer management (for inter-state deliveries)
		seller.GET("/driver-deliveries/:id/offers", h.GetDeliveryOffers)
		seller.POST("/driver-deliveries/:id/offers/:oid/accept", h.AcceptDriverOffer)
		seller.POST("/driver-deliveries/:id/offers/:oid/reject", h.RejectDriverOffer)
		seller.POST("/driver-deliveries/:id/offers/:oid/counter", h.CounterDriverOffer)
	}
}
func (h *SellerHandler) ListSales(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	limit := 20
	offset := 0

	orders, err := h.orderService.GetOrdersBySellerID(c.Request.Context(), userID, limit, offset)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to fetch sales")
		return
	}
	responses := make([]SaleResponse, 0, len(orders))
	for _, order := range orders {
		pr, err := h.prService.GetByID(c.Request.Context(), order.PaymentRequestID)
		if err != nil {
			continue
		}
		var escrowStatus string
		if order.EscrowID != nil {
			escrow, err := h.escrowService.GetByID(c.Request.Context(), *order.EscrowID)
			if err == nil {
				escrowStatus = string(escrow.Status)
			}
		}

		responses = append(responses, orderToSaleResponse(order, pr, escrowStatus))
	}

	util.Success(c, http.StatusOK, gin.H{
		"sales": responses,
		"count": len(responses),
	})
}
func (h *SellerHandler) GetSale(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid sale ID")
		return
	}

	order, err := h.orderService.GetOrderByID(c.Request.Context(), id)
	if err != nil {
		util.Error(c, http.StatusNotFound, "NOT_FOUND", "Sale not found")
		return
	}
	pr, err := h.prService.GetByID(c.Request.Context(), order.PaymentRequestID)
	if err != nil {
		util.Error(c, http.StatusNotFound, "NOT_FOUND", "Payment request not found")
		return
	}
	if pr.SellerID != userID {
		util.Error(c, http.StatusForbidden, domain.CodeUnauthorized, "You don't have access to this sale")
		return
	}
	var escrowStatus string
	var escrowDetails *EscrowDetails
	if order.EscrowID != nil {
		escrow, err := h.escrowService.GetByID(c.Request.Context(), *order.EscrowID)
		if err == nil {
			escrowStatus = string(escrow.Status)
			escrowDetails = &EscrowDetails{
				ID:            escrow.ID,
				Status:        string(escrow.Status),
				ItemAmount:    escrow.ItemAmount,
				TotalLocked:   escrow.TotalLocked,
				SellerPayout:  escrow.SellerPayout,
				FundedAt:      formatTimePtr(escrow.FundedAt),
				ReleasedAt:    formatTimePtr(escrow.ReleasedAt),
			}
		}
	}

	// Get delivery job for timeline (intra-state)
	var job *domain.DeliveryJob
	if pr.DeliveryType == domain.DeliveryTypeIntraState {
		job, _ = h.deliveryJobRepo.GetByPaymentRequestID(c.Request.Context(), order.PaymentRequestID)
	}

	var deliveryInfo *SaleDeliveryInfo
	if job != nil {
		deliveryInfo = &SaleDeliveryInfo{
			JobID:         &job.ID,
			Status:        string(job.Status),
			RiderAssigned: job.RiderID != nil,
		}
		if job.RiderID != nil && job.PickupCode != nil {
			deliveryInfo.PickupCode = job.PickupCode
		}
		if job.PickedUpAt != nil {
			t := job.PickedUpAt.Format("2006-01-02T15:04:05Z07:00")
			deliveryInfo.PickedUpAt = &t
		}
		if job.DeliveredAt != nil {
			t := job.DeliveredAt.Format("2006-01-02T15:04:05Z07:00")
			deliveryInfo.DeliveredAt = &t
		}
	}

	timeline := BuildTimeline(TimelineViewSeller, pr, order, job)
	response := SaleDetailResponse{
		SaleResponse: orderToSaleResponse(order, pr, escrowStatus),
		Escrow:       escrowDetails,
		Delivery:     deliveryInfo,
		Timeline:     &timeline,
	}

	util.Success(c, http.StatusOK, response)
}
func (h *SellerHandler) GetEarnings(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}
	orders, err := h.orderService.GetOrdersBySellerID(c.Request.Context(), userID, 1000, 0)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to fetch orders")
		return
	}

	var pendingAmount float64
	var releasedAmount float64
	var pendingCount int
	var releasedCount int

	for _, order := range orders {
		if order.EscrowID == nil {
			continue
		}

		escrow, err := h.escrowService.GetByID(c.Request.Context(), *order.EscrowID)
		if err != nil {
			continue
		}

		switch escrow.Status {
		case domain.EscrowStatusFunded, domain.EscrowStatusDeliveryInProgress, domain.EscrowStatusDelivered, domain.EscrowStatusConfirmationWindow:
			pendingAmount += escrow.ItemAmount // Seller gets item amount minus platform fee
			pendingCount++
		case domain.EscrowStatusReleased:
			if escrow.SellerPayout != nil {
				releasedAmount += *escrow.SellerPayout
			} else {
				releasedAmount += escrow.ItemAmount
			}
			releasedCount++
		}
	}

	util.Success(c, http.StatusOK, gin.H{
		"pending": gin.H{
			"amount": pendingAmount,
			"count":  pendingCount,
		},
		"released": gin.H{
			"amount": releasedAmount,
			"count":  releasedCount,
		},
		"total_sales": len(orders),
	})
}

type SaleResponse struct {
	ID               uuid.UUID  `json:"id"`
	PaymentRequestID uuid.UUID  `json:"payment_request_id"`
	ItemName         string     `json:"item_name"`
	ItemPrice        float64    `json:"item_price"`
	TotalAmount      float64    `json:"total_amount"`
	PaymentMethod    string     `json:"payment_method"`
	PaymentStatus    string     `json:"payment_status"`
	OrderStatus      string     `json:"order_status"`
	EscrowStatus     string     `json:"escrow_status,omitempty"`
	BuyerID          uuid.UUID  `json:"buyer_id"`
	CreatedAt        string     `json:"created_at"`
	PaidAt           *string    `json:"paid_at,omitempty"`
}

type EscrowDetails struct {
	ID            uuid.UUID `json:"id"`
	Status        string    `json:"status"`
	ItemAmount    float64   `json:"item_amount"`
	TotalLocked   float64   `json:"total_locked"`
	SellerPayout  *float64  `json:"seller_payout,omitempty"`
	FundedAt      *string   `json:"funded_at,omitempty"`
	ReleasedAt    *string   `json:"released_at,omitempty"`
}

type SaleDeliveryInfo struct {
	JobID         *uuid.UUID `json:"job_id,omitempty"`
	Status        string     `json:"status"`
	PickupCode    *string    `json:"pickup_code,omitempty"`
	RiderAssigned bool       `json:"rider_assigned"`
	PickedUpAt    *string    `json:"picked_up_at,omitempty"`
	DeliveredAt   *string    `json:"delivered_at,omitempty"`
}

type SaleDetailResponse struct {
	SaleResponse
	Escrow   *EscrowDetails    `json:"escrow,omitempty"`
	Delivery *SaleDeliveryInfo `json:"delivery,omitempty"`
	Timeline *TimelineResponse `json:"timeline,omitempty"`
}

func orderToSaleResponse(order *domain.Order, pr *domain.PaymentRequest, escrowStatus string) SaleResponse {
	resp := SaleResponse{
		ID:               order.ID,
		PaymentRequestID: order.PaymentRequestID,
		ItemName:         pr.ItemName,
		ItemPrice:        order.ItemAmount,
		TotalAmount:      order.TotalAmount,
		PaymentMethod:    string(order.PaymentMethod),
		PaymentStatus:    string(order.PaymentStatus),
		OrderStatus:      string(order.Status),
		EscrowStatus:     escrowStatus,
		BuyerID:          order.BuyerID,
		CreatedAt:        order.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	if order.PaidAt != nil {
		paidAt := order.PaidAt.Format("2006-01-02T15:04:05Z07:00")
		resp.PaidAt = &paidAt
	}

	return resp
}

func formatTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	formatted := t.Format("2006-01-02T15:04:05Z07:00")
	return &formatted
}
func (h *SellerHandler) GetSaleTimeline(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid sale ID")
		return
	}

	order, err := h.orderService.GetOrderByID(c.Request.Context(), id)
	if err != nil {
		util.Error(c, http.StatusNotFound, "NOT_FOUND", "Sale not found")
		return
	}

	pr, err := h.prService.GetByID(c.Request.Context(), order.PaymentRequestID)
	if err != nil {
		util.Error(c, http.StatusNotFound, "NOT_FOUND", "Payment request not found")
		return
	}

	if pr.SellerID != userID {
		util.Error(c, http.StatusForbidden, domain.CodeUnauthorized, "You don't have access to this sale")
		return
	}
	var job *domain.DeliveryJob
	if pr.DeliveryType == domain.DeliveryTypeIntraState {
		job, _ = h.deliveryJobRepo.GetByPaymentRequestID(c.Request.Context(), order.PaymentRequestID)
	}

	timeline := BuildTimeline(TimelineViewSeller, pr, order, job)

	util.Success(c, http.StatusOK, timeline)
}

// GetReturnStatus returns the return job status for a sale
func (h *SellerHandler) GetReturnStatus(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid sale ID")
		return
	}

	order, err := h.orderService.GetOrderByID(c.Request.Context(), id)
	if err != nil {
		util.Error(c, http.StatusNotFound, "NOT_FOUND", "Sale not found")
		return
	}

	pr, err := h.prService.GetByID(c.Request.Context(), order.PaymentRequestID)
	if err != nil {
		util.Error(c, http.StatusNotFound, "NOT_FOUND", "Payment request not found")
		return
	}

	if pr.SellerID != userID {
		util.Error(c, http.StatusForbidden, domain.CodeUnauthorized, "You don't have access to this sale")
		return
	}

	// Check if there's a return job
	if pr.ReturnDeliveryJobID == nil {
		util.Error(c, http.StatusNotFound, "NO_RETURN", "No return in progress for this sale")
		return
	}

	returnJob, err := h.deliveryJobRepo.GetByID(c.Request.Context(), *pr.ReturnDeliveryJobID)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to fetch return job")
		return
	}

	util.Success(c, http.StatusOK, ReturnStatusResponse{
		ReturnJobID:       returnJob.ID,
		Status:            string(returnJob.Status),
		IsReturn:          returnJob.IsReturn,
		ReturnCondition:   returnJob.ReturnCondition,
		DamageClaimReason: returnJob.DamageClaimReason,
		ReturnConfirmedAt: formatTimePtr(returnJob.ReturnConfirmedAt),
		PickupAddress:     returnJob.PickupAddress,
		DropoffAddress:    returnJob.DropoffAddress,
		RiderAssigned:     returnJob.RiderID != nil,
		CreatedAt:         returnJob.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		CanConfirm:        returnJob.Status == domain.DJStatusReturnDelivered,
	})
}

// ConfirmReturn handles seller confirming return receipt
func (h *SellerHandler) ConfirmReturn(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid sale ID")
		return
	}

	var req ConfirmReturnRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	order, err := h.orderService.GetOrderByID(c.Request.Context(), id)
	if err != nil {
		util.Error(c, http.StatusNotFound, "NOT_FOUND", "Sale not found")
		return
	}

	pr, err := h.prService.GetByID(c.Request.Context(), order.PaymentRequestID)
	if err != nil {
		util.Error(c, http.StatusNotFound, "NOT_FOUND", "Payment request not found")
		return
	}

	if pr.SellerID != userID {
		util.Error(c, http.StatusForbidden, domain.CodeUnauthorized, "You don't have access to this sale")
		return
	}

	if pr.ReturnDeliveryJobID == nil {
		util.Error(c, http.StatusBadRequest, "NO_RETURN", "No return in progress for this sale")
		return
	}

	// Build service request
	serviceReq := &service.ConfirmReturnRequest{
		Condition:          req.Condition,
		Notes:              req.Notes,
		DamageReason:       req.DamageReason,
		DamageEvidenceURLs: req.DamageEvidenceURLs,
	}

	if err := h.disputeResolutionService.ConfirmReturnReceived(c.Request.Context(), *pr.ReturnDeliveryJobID, userID, serviceReq); err != nil {
		util.Error(c, http.StatusBadRequest, "CONFIRM_FAILED", err.Error())
		return
	}

	message := "Return confirmed. Buyer will be refunded."
	if req.Condition == "damaged" || req.Condition == "incomplete" {
		message = "Damage claim submitted. Admin will review and decide on the refund amount."
	}

	util.Success(c, http.StatusOK, gin.H{"message": message})
}

type ReturnStatusResponse struct {
	ReturnJobID       uuid.UUID `json:"return_job_id"`
	Status            string    `json:"status"`
	IsReturn          bool      `json:"is_return"`
	ReturnCondition   *string   `json:"return_condition,omitempty"`
	DamageClaimReason *string   `json:"damage_claim_reason,omitempty"`
	ReturnConfirmedAt *string   `json:"return_confirmed_at,omitempty"`
	PickupAddress     string    `json:"pickup_address"`
	DropoffAddress    string    `json:"dropoff_address"`
	RiderAssigned     bool      `json:"rider_assigned"`
	CreatedAt         string    `json:"created_at"`
	CanConfirm        bool      `json:"can_confirm"`
}

type ConfirmReturnRequest struct {
	Condition          string   `json:"condition" binding:"required,oneof=good damaged incomplete"`
	Notes              *string  `json:"notes"`
	DamageReason       *string  `json:"damage_reason"`
	DamageEvidenceURLs []string `json:"damage_evidence_urls"`
}

// ============================================
// Driver Delivery Endpoints (Inter-state)
// ============================================

func (h *SellerHandler) ListDriverDeliveries(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	limit := 20
	offset := 0

	deliveries, err := h.driverService.GetDeliveriesBySeller(c.Request.Context(), userID, limit, offset)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to fetch driver deliveries")
		return
	}

	responses := make([]DriverDeliveryResponse, 0, len(deliveries))
	for _, d := range deliveries {
		responses = append(responses, driverDeliveryToResponse(d))
	}

	util.Success(c, http.StatusOK, gin.H{
		"deliveries": responses,
		"count":      len(responses),
	})
}

func (h *SellerHandler) GetDriverDelivery(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid delivery ID")
		return
	}

	delivery, err := h.driverService.GetDeliveryForSeller(c.Request.Context(), userID, id)
	if err != nil {
		if err == domain.ErrUnauthorized {
			util.Error(c, http.StatusForbidden, domain.CodeUnauthorized, "You don't have access to this delivery")
			return
		}
		util.Error(c, http.StatusNotFound, "NOT_FOUND", "Delivery not found")
		return
	}

	// Get payment request for timeline
	pr, _ := h.prService.GetByID(c.Request.Context(), delivery.PaymentRequestID)

	resp := driverDeliveryToResponse(delivery)
	if pr != nil {
		timeline := BuildDriverDeliveryTimeline(TimelineViewSeller, delivery, pr)
		resp.Timeline = &timeline
	}

	util.Success(c, http.StatusOK, resp)
}

func (h *SellerHandler) GetPickupCode(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid delivery ID")
		return
	}

	code, err := h.driverService.GetPickupCodeForSeller(c.Request.Context(), userID, id)
	if err != nil {
		if err == domain.ErrUnauthorized {
			util.Error(c, http.StatusForbidden, domain.CodeUnauthorized, "You don't have access to this delivery")
			return
		}
		util.Error(c, http.StatusBadRequest, "CODE_ERROR", err.Error())
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"pickup_code": code,
		"message":     "Give this code to the driver to confirm handover",
	})
}

func (h *SellerHandler) CancelDriverDelivery(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid delivery ID")
		return
	}

	if err := h.driverService.CancelDelivery(c.Request.Context(), userID, id); err != nil {
		if err == domain.ErrUnauthorized {
			util.Error(c, http.StatusForbidden, domain.CodeUnauthorized, "You don't have access to this delivery")
			return
		}
		util.Error(c, http.StatusBadRequest, "CANCEL_ERROR", err.Error())
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"message": "Delivery cancelled successfully. Buyer will be refunded.",
	})
}

func (h *SellerHandler) GetDriverDeliveryTimeline(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid delivery ID")
		return
	}

	delivery, err := h.driverService.GetDeliveryForSeller(c.Request.Context(), userID, id)
	if err != nil {
		if err == domain.ErrUnauthorized {
			util.Error(c, http.StatusForbidden, domain.CodeUnauthorized, "You don't have access to this delivery")
			return
		}
		util.Error(c, http.StatusNotFound, "NOT_FOUND", "Delivery not found")
		return
	}

	pr, err := h.prService.GetByID(c.Request.Context(), delivery.PaymentRequestID)
	if err != nil {
		util.Error(c, http.StatusNotFound, "NOT_FOUND", "Payment request not found")
		return
	}

	timeline := BuildDriverDeliveryTimeline(TimelineViewSeller, delivery, pr)
	util.Success(c, http.StatusOK, timeline)
}

// ============================================
// Driver Offer Management (Inter-state)
// ============================================

func (h *SellerHandler) GetDeliveryOffers(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	deliveryID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid delivery ID")
		return
	}

	offers, err := h.driverService.GetOffersForDelivery(c.Request.Context(), userID, deliveryID)
	if err != nil {
		h.handleOfferError(c, err)
		return
	}

	responses := make([]SellerDriverOfferResponse, 0, len(offers))
	for _, o := range offers {
		responses = append(responses, driverOfferToSellerResponse(o))
	}

	util.Success(c, http.StatusOK, gin.H{
		"offers": responses,
		"count":  len(responses),
	})
}

func (h *SellerHandler) AcceptDriverOffer(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	offerID, err := uuid.Parse(c.Param("oid"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid offer ID")
		return
	}

	delivery, err := h.driverService.AcceptOffer(c.Request.Context(), userID, offerID)
	if err != nil {
		h.handleOfferError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"message":  "Offer accepted. Driver has been assigned.",
		"delivery": driverDeliveryToResponse(delivery),
	})
}

type RejectOfferRequest struct {
	Note string `json:"note"`
}

func (h *SellerHandler) RejectDriverOffer(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	offerID, err := uuid.Parse(c.Param("oid"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid offer ID")
		return
	}

	var req RejectOfferRequest
	c.ShouldBindJSON(&req) // Optional note

	if err := h.driverService.RejectOffer(c.Request.Context(), userID, offerID, req.Note); err != nil {
		h.handleOfferError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{"message": "Offer rejected"})
}

type CounterOfferRequest struct {
	Amount float64 `json:"amount" binding:"required,gt=0"`
	Note   string  `json:"note"`
}

func (h *SellerHandler) CounterDriverOffer(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	offerID, err := uuid.Parse(c.Param("oid"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid offer ID")
		return
	}

	var req CounterOfferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	offer, err := h.driverService.CounterOffer(c.Request.Context(), userID, offerID, &service.CounterOfferRequest{
		Amount: req.Amount,
		Note:   req.Note,
	})
	if err != nil {
		h.handleOfferError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"message": "Counter offer sent to driver",
		"offer":   driverOfferToSellerResponse(offer),
	})
}

func (h *SellerHandler) handleOfferError(c *gin.Context, err error) {
	switch {
	case err == domain.ErrOfferNotFound:
		util.Error(c, http.StatusNotFound, "OFFER_NOT_FOUND", "Offer not found")
	case err == domain.ErrOfferExpired:
		util.Error(c, http.StatusBadRequest, "OFFER_EXPIRED", "This offer has expired")
	case err == domain.ErrDeliveryNotAcceptingOffers:
		util.Error(c, http.StatusBadRequest, "NOT_ACCEPTING_OFFERS", "This delivery is not accepting offers")
	case err == domain.ErrOfferCannotBeAccepted:
		util.Error(c, http.StatusBadRequest, "CANNOT_ACCEPT", "This offer cannot be accepted in its current state")
	case err == domain.ErrOfferCannotBeRejected:
		util.Error(c, http.StatusBadRequest, "CANNOT_REJECT", "This offer cannot be rejected in its current state")
	case err == domain.ErrOfferCannotBeCountered:
		util.Error(c, http.StatusBadRequest, "CANNOT_COUNTER", "This offer cannot be countered in its current state")
	case err == domain.ErrUnauthorized:
		util.Error(c, http.StatusForbidden, domain.CodeUnauthorized, "You don't have access to this delivery")
	case err == domain.ErrDriverDeliveryNotFound:
		util.Error(c, http.StatusNotFound, "DELIVERY_NOT_FOUND", "Delivery not found")
	default:
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "An error occurred")
	}
}

// ============================================
// Offer Response Types
// ============================================

type SellerDriverOfferResponse struct {
	ID               string   `json:"id"`
	DeliveryID       string   `json:"delivery_id"`
	DriverID         string   `json:"driver_id"`
	OfferedAmount    float64  `json:"offered_amount"`
	CounterAmount    *float64 `json:"counter_amount,omitempty"`
	AgreedAmount     *float64 `json:"agreed_amount,omitempty"`
	Status           string   `json:"status"`
	ExpiresAt        string   `json:"expires_at"`
	CounterExpiresAt *string  `json:"counter_expires_at,omitempty"`
	CounteredAt      *string  `json:"countered_at,omitempty"`
	RespondedAt      *string  `json:"responded_at,omitempty"`
	DriverNote       *string  `json:"driver_note,omitempty"`
	SellerNote       *string  `json:"seller_note,omitempty"`
	OfferCount       int      `json:"offer_count"`
	CreatedAt        string   `json:"created_at"`
}

func driverOfferToSellerResponse(o *domain.DriverOffer) SellerDriverOfferResponse {
	resp := SellerDriverOfferResponse{
		ID:            o.ID.String(),
		DeliveryID:    o.DeliveryID.String(),
		DriverID:      o.DriverID.String(),
		OfferedAmount: o.OfferedAmount,
		Status:        string(o.Status),
		ExpiresAt:     o.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
		OfferCount:    o.OfferCount,
		CreatedAt:     o.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		DriverNote:    o.DriverNote,
		SellerNote:    o.SellerNote,
	}

	if o.CounterAmount != nil {
		resp.CounterAmount = o.CounterAmount
	}
	if o.AgreedAmount != nil {
		resp.AgreedAmount = o.AgreedAmount
	}
	if o.CounterExpiresAt != nil {
		s := o.CounterExpiresAt.Format("2006-01-02T15:04:05Z07:00")
		resp.CounterExpiresAt = &s
	}
	if o.CounteredAt != nil {
		s := o.CounteredAt.Format("2006-01-02T15:04:05Z07:00")
		resp.CounteredAt = &s
	}
	if o.RespondedAt != nil {
		s := o.RespondedAt.Format("2006-01-02T15:04:05Z07:00")
		resp.RespondedAt = &s
	}

	return resp
}
