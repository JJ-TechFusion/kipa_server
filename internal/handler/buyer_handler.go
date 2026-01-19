package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"kipa/internal/domain"
	"kipa/internal/handler/middleware"
	"kipa/internal/repository"
	"kipa/internal/service"
	"kipa/internal/util"
)

type BuyerHandler struct {
	prService       service.PaymentRequestService
	orderService    service.OrderService
	escrowService   service.EscrowService
	userRepo        repository.UserRepository
	deliveryJobRepo repository.DeliveryJobRepository
}

func NewBuyerHandler(prService service.PaymentRequestService, orderService service.OrderService, escrowService service.EscrowService, userRepo repository.UserRepository, deliveryJobRepo repository.DeliveryJobRepository) *BuyerHandler {
	return &BuyerHandler{
		prService:       prService,
		orderService:    orderService,
		escrowService:   escrowService,
		userRepo:        userRepo,
		deliveryJobRepo: deliveryJobRepo,
	}
}

func (h *BuyerHandler) RegisterRoutes(router *gin.RouterGroup, authMiddleware *middleware.AuthMiddleware) {
	router.GET("/pay/:code", h.GetPaymentPage)
	router.GET("/pay/:code/verify", h.VerifyPayment)
	pay := router.Group("/pay")
	pay.Use(authMiddleware.RequireAuth())
	{
		pay.POST("/:code/initialize", h.InitializePayment)
	}

	purchases := router.Group("/purchases")
	purchases.Use(authMiddleware.RequireAuth())
	{
		purchases.GET("", h.ListPurchases)
		purchases.GET("/:id", h.GetPurchase)
		purchases.GET("/:id/timeline", h.GetPurchaseTimeline)
		purchases.POST("/:id/confirm", h.ConfirmDelivery)
		purchases.POST("/:id/dispute", h.DisputePurchase)
	}
	orders := router.Group("/orders")
	orders.Use(authMiddleware.RequireAuth())
	{
		orders.GET("", h.ListOrders)
		orders.GET("/:id", h.GetOrder)
	}
}
func (h *BuyerHandler) GetPaymentPage(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		util.Error(c, http.StatusBadRequest, "INVALID_CODE", "Payment code is required")
		return
	}

	pr, err := h.prService.GetByPaymentCode(c.Request.Context(), code)
	if err != nil {
		h.handleError(c, err)
		return
	}
	if pr.Status == domain.PRStatusExpired {
		util.Error(c, http.StatusGone, "LINK_EXPIRED", "This payment link has expired")
		return
	}
	if pr.Status.IsPaid() {
		util.Error(c, http.StatusBadRequest, "ALREADY_PAID", "This payment request has already been paid")
		return
	}
	if pr.Status == domain.PRStatusCancelled {
		util.Error(c, http.StatusGone, "CANCELLED", "This payment request has been cancelled")
		return
	}

	util.Success(c, http.StatusOK, paymentPageToResponse(pr))
}
func (h *BuyerHandler) InitializePayment(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	code := c.Param("code")
	if code == "" {
		util.Error(c, http.StatusBadRequest, "INVALID_CODE", "Payment code is required")
		return
	}

	var req InitializePaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	switch req.PaymentMethod {
	case "card":
		user, err := h.userRepo.GetByID(c.Request.Context(), userID)
		if err != nil {
			util.Error(c, http.StatusInternalServerError, "USER_NOT_FOUND", "Failed to retrieve user profile")
			return
		}
		email := req.Email
		if email == "" && user.Email != nil {
			email = *user.Email
		}
		if email == "" {
			util.Error(c, http.StatusBadRequest, "EMAIL_REQUIRED", "Please update your profile with an email address to make card payments")
			return
		}
		initReq := &service.InitializePaymentRequest{
			CallbackURL: req.CallbackURL,
			Email:       email,
		}
		resp, err := h.orderService.InitializePayment(c.Request.Context(), userID, code, initReq)
		if err != nil {
			h.handleError(c, err)
			return
		}

		util.Success(c, http.StatusOK, gin.H{
			"order_id":          resp.OrderID,
			"authorization_url": resp.AuthorizationURL,
			"access_code":       resp.AccessCode,
			"reference":         resp.Reference,
			"payment_method":    "card",
		})

	case "wallet":
		resp, err := h.orderService.ProcessWalletPayment(c.Request.Context(), userID, code)
		if err != nil {
			h.handleError(c, err)
			return
		}

		util.Success(c, http.StatusOK, gin.H{
			"order_id":       resp.OrderID,
			"escrow_id":      resp.EscrowID,
			"status":         resp.Status,
			"payment_method": "wallet",
			"message":        "Payment completed successfully",
		})

	default:
		util.Error(c, http.StatusBadRequest, "INVALID_PAYMENT_METHOD", "Payment method must be 'card' or 'wallet'")
	}
}
func (h *BuyerHandler) VerifyPayment(c *gin.Context) {
	reference := c.Query("reference")
	if reference == "" {
		util.Error(c, http.StatusBadRequest, "INVALID_REFERENCE", "Payment reference is required")
		return
	}

	resp, err := h.orderService.VerifyPayment(c.Request.Context(), reference)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"order_id":       resp.OrderID,
		"payment_status": resp.PaymentStatus,
		"escrow_id":      resp.EscrowID,
		"status":         resp.Status,
		"message":        "Payment verified successfully",
	})
}
func (h *BuyerHandler) ListPurchases(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	limit := 20
	offset := 0

	orders, err := h.orderService.GetOrdersByBuyerID(c.Request.Context(), userID, limit, offset)
	if err != nil {
		h.handleError(c, err)
		return
	}
	responses := make([]PurchaseResponse, 0)
	for _, order := range orders {
		if order.PaymentStatus != domain.OrderPaymentVerified {
			continue
		}

		pr, err := h.prService.GetByID(c.Request.Context(), order.PaymentRequestID)
		if err != nil {
			continue // Skip if can't fetch payment request
		}
		var job *domain.DeliveryJob
		if pr.DeliveryType == domain.DeliveryTypeIntraState {
			job, _ = h.deliveryJobRepo.GetByPaymentRequestID(c.Request.Context(), order.PaymentRequestID)
		}

		responses = append(responses, orderToPurchaseResponse(order, pr, job))
	}

	util.Success(c, http.StatusOK, gin.H{
		"purchases": responses,
		"count":     len(responses),
	})
}
func (h *BuyerHandler) GetPurchase(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid purchase ID")
		return
	}

	order, err := h.orderService.GetOrderByID(c.Request.Context(), id)
	if err != nil {
		h.handleError(c, err)
		return
	}
	if order.BuyerID != userID {
		util.Error(c, http.StatusForbidden, domain.CodeUnauthorized, "You don't have access to this purchase")
		return
	}
	pr, err := h.prService.GetByID(c.Request.Context(), order.PaymentRequestID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	var job *domain.DeliveryJob
	if pr.DeliveryType == domain.DeliveryTypeIntraState {
		job, _ = h.deliveryJobRepo.GetByPaymentRequestID(c.Request.Context(), order.PaymentRequestID)
	}

	util.Success(c, http.StatusOK, orderToPurchaseResponse(order, pr, job))
}
func (h *BuyerHandler) ConfirmDelivery(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid purchase ID")
		return
	}

	order, err := h.orderService.GetOrderByID(c.Request.Context(), id)
	if err != nil {
		h.handleError(c, err)
		return
	}
	if order.BuyerID != userID {
		util.Error(c, http.StatusForbidden, domain.CodeUnauthorized, "You don't have access to this purchase")
		return
	}
	if order.EscrowID == nil {
		util.Error(c, http.StatusBadRequest, "NO_ESCROW", "No escrow found for this purchase")
		return
	}
	if err := h.escrowService.Release(c.Request.Context(), *order.EscrowID); err != nil {
		h.handleError(c, err)
		return
	}
	if err := h.orderService.UpdateStatus(c.Request.Context(), order.ID, domain.OrderStatusCompleted); err != nil {
		h.handleError(c, err)
		return
	}
	pr, err := h.prService.GetByID(c.Request.Context(), order.PaymentRequestID)
	if err == nil && !pr.IsReusable {
		_ = h.prService.MarkCompleted(c.Request.Context(), order.PaymentRequestID)
	}

	util.Success(c, http.StatusOK, gin.H{"message": "Delivery confirmed. Payment released to seller."})
}
func (h *BuyerHandler) DisputePurchase(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid purchase ID")
		return
	}

	var req DisputeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	order, err := h.orderService.GetOrderByID(c.Request.Context(), id)
	if err != nil {
		h.handleError(c, err)
		return
	}
	if order.BuyerID != userID {
		util.Error(c, http.StatusForbidden, domain.CodeUnauthorized, "You don't have access to this purchase")
		return
	}
	if order.EscrowID == nil {
		util.Error(c, http.StatusBadRequest, "NO_ESCROW", "No escrow found for this purchase")
		return
	}
	if err := h.escrowService.Dispute(c.Request.Context(), *order.EscrowID, req.Reason); err != nil {
		h.handleError(c, err)
		return
	}
	if err := h.orderService.UpdateStatus(c.Request.Context(), order.ID, domain.OrderStatusDisputed); err != nil {
		h.handleError(c, err)
		return
	}
	pr, err := h.prService.GetByID(c.Request.Context(), order.PaymentRequestID)
	if err == nil && !pr.IsReusable {
		_ = h.prService.UpdateStatus(c.Request.Context(), order.PaymentRequestID, domain.PRStatusDisputed)
	}

	util.Success(c, http.StatusOK, gin.H{"message": "Dispute opened. Our team will review your case."})
}
func (h *BuyerHandler) ListOrders(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	limit := 20
	offset := 0

	orders, err := h.orderService.GetOrdersByBuyerID(c.Request.Context(), userID, limit, offset)
	if err != nil {
		h.handleError(c, err)
		return
	}

	responses := make([]OrderResponse, len(orders))
	for i, order := range orders {
		responses[i] = orderToResponse(order)
	}

	util.Success(c, http.StatusOK, gin.H{
		"orders": responses,
		"count":  len(responses),
	})
}
func (h *BuyerHandler) GetOrder(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid order ID")
		return
	}

	order, err := h.orderService.GetOrderByID(c.Request.Context(), id)
	if err != nil {
		h.handleError(c, err)
		return
	}

	
	if order.BuyerID != userID {
		util.Error(c, http.StatusForbidden, domain.CodeUnauthorized, "You don't have access to this order")
		return
	}

	util.Success(c, http.StatusOK, orderToResponse(order))
}

func (h *BuyerHandler) handleError(c *gin.Context, err error) {
	status, code, message := mapBuyerErrorToResponse(err)
	util.Error(c, status, code, message)
}

func mapBuyerErrorToResponse(err error) (int, string, string) {
	switch {
	case errors.Is(err, domain.ErrPaymentRequestNotFound):
		return http.StatusNotFound, "NOT_FOUND", "Payment request not found"
	case errors.Is(err, domain.ErrPaymentRequestExpired):
		return http.StatusGone, "EXPIRED", "This payment link has expired"
	case errors.Is(err, domain.ErrInvalidStatusTransition):
		return http.StatusBadRequest, "INVALID_STATUS", "Cannot perform this action in the current status"
	case errors.Is(err, domain.ErrCannotDispute):
		return http.StatusBadRequest, "CANNOT_DISPUTE", "Cannot dispute at this stage"
	case errors.Is(err, domain.ErrEscrowNotFound):
		return http.StatusNotFound, "ESCROW_NOT_FOUND", "Escrow not found"
	case errors.Is(err, domain.ErrOrderNotFound):
		return http.StatusNotFound, "ORDER_NOT_FOUND", "Order not found"
	case errors.Is(err, domain.ErrOrderAlreadyPaid):
		return http.StatusBadRequest, "ALREADY_PAID", "Order already paid"
	case errors.Is(err, domain.ErrOrderPaymentFailed):
		return http.StatusBadRequest, "PAYMENT_FAILED", "Payment failed"
	case errors.Is(err, domain.ErrMaxUsesExceeded):
		return http.StatusBadRequest, "MAX_USES_REACHED", "This payment link has reached its maximum number of uses"
	case errors.Is(err, domain.ErrLinkNotReusable):
		return http.StatusBadRequest, "LINK_USED", "This payment link has already been used"
	case errors.Is(err, domain.ErrPaymentVerificationFailed):
		return http.StatusBadRequest, "VERIFICATION_FAILED", "Payment verification failed"
	case errors.Is(err, domain.ErrPaymentNotSuccessful):
		return http.StatusBadRequest, "PAYMENT_NOT_SUCCESSFUL", "Payment was not successful"
	case errors.Is(err, domain.ErrPaymentAmountMismatch):
		return http.StatusBadRequest, "AMOUNT_MISMATCH", "Payment amount does not match expected amount"
	case errors.Is(err, domain.ErrInsufficientBalance):
		return http.StatusBadRequest, "INSUFFICIENT_BALANCE", "Insufficient wallet balance"
	default:
		return http.StatusInternalServerError, domain.CodeInternalError, "An unexpected error occurred"
	}
}



type InitializePaymentRequest struct {
	PaymentMethod string `json:"payment_method" binding:"required,oneof=card wallet"` 
	CallbackURL   string `json:"callback_url"`                                        
	Email         string `json:"email"`                                               
}

type DisputeRequest struct {
	Reason   string   `json:"reason" binding:"required"`
	Evidence []string `json:"evidence"`
}

type PaymentPageResponse struct {
	ItemName             string   `json:"item_name"`
	ItemDescription      *string  `json:"item_description,omitempty"`
	ItemPrice            float64  `json:"item_price"`
	DeliveryType         string   `json:"delivery_type"`
	EstimatedDeliveryFee *float64 `json:"estimated_delivery_fee,omitempty"`
	BuyerServiceFee      *float64 `json:"buyer_service_fee,omitempty"`
	EstimatedTotal       *float64 `json:"estimated_total,omitempty"`
	SellerID             uuid.UUID `json:"seller_id"`
}

func paymentPageToResponse(pr *domain.PaymentRequest) PaymentPageResponse {
	return PaymentPageResponse{
		ItemName:             pr.ItemName,
		ItemDescription:      pr.ItemDescription,
		ItemPrice:            pr.ItemPrice,
		DeliveryType:         string(pr.DeliveryType),
		EstimatedDeliveryFee: pr.EstimatedDeliveryFee,
		BuyerServiceFee:      pr.BuyerServiceFee,
		EstimatedTotal:       pr.EstimatedTotal,
		SellerID:             pr.SellerID,
	}
}

type PurchaseResponse struct {
	ID                   uuid.UUID  `json:"id"`
	PaymentRequestID     uuid.UUID  `json:"payment_request_id"`                  
	ItemName             string     `json:"item_name"`
	ItemDescription      *string    `json:"item_description,omitempty"`
	ItemPrice            float64    `json:"item_price"`
	DeliveryType         string     `json:"delivery_type"`
	EstimatedDeliveryFee float64    `json:"estimated_delivery_fee"`
	BuyerServiceFee      float64    `json:"buyer_service_fee"`
	TotalAmount          float64    `json:"total_amount"`
	PaymentMethod        string     `json:"payment_method"`
	Status               string     `json:"status"`
	SellerID             uuid.UUID  `json:"seller_id"`
	EscrowID             *uuid.UUID `json:"escrow_id,omitempty"`
	PaidAt               *string    `json:"paid_at,omitempty"`
	CreatedAt            string     `json:"created_at"`

	Delivery *PurchaseDeliveryInfo `json:"delivery,omitempty"`
}

type PurchaseDeliveryInfo struct {
	Status       string  `json:"status"`                  
	DropoffCode  *string `json:"dropoff_code,omitempty"` 
	RiderAssigned bool   `json:"rider_assigned"`          
	PickedUpAt   *string `json:"picked_up_at,omitempty"`
	DeliveredAt  *string `json:"delivered_at,omitempty"`
}

func orderToPurchaseResponse(order *domain.Order, pr *domain.PaymentRequest, job *domain.DeliveryJob) PurchaseResponse {
	resp := PurchaseResponse{
		ID:                   order.ID,
		PaymentRequestID:     order.PaymentRequestID,
		ItemName:             pr.ItemName,
		ItemDescription:      pr.ItemDescription,
		ItemPrice:            order.ItemAmount,
		DeliveryType:         string(pr.DeliveryType),
		EstimatedDeliveryFee: order.DeliveryFeeEstimate,
		BuyerServiceFee:      order.BuyerServiceFee,
		TotalAmount:          order.TotalAmount,
		PaymentMethod:        string(order.PaymentMethod),
		Status:               string(order.Status),
		SellerID:             pr.SellerID,
		EscrowID:             order.EscrowID,
		CreatedAt:            order.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	if order.PaidAt != nil {
		paidAt := order.PaidAt.Format("2006-01-02T15:04:05Z07:00")
		resp.PaidAt = &paidAt
	}
	if job != nil {
		deliveryInfo := &PurchaseDeliveryInfo{
			Status:        string(job.Status),
			RiderAssigned: job.RiderID != nil,
		}
		if job.RiderID != nil && job.DropoffCode != nil {
			deliveryInfo.DropoffCode = job.DropoffCode
		}

		if job.PickedUpAt != nil {
			t := job.PickedUpAt.Format("2006-01-02T15:04:05Z07:00")
			deliveryInfo.PickedUpAt = &t
		}
		if job.DeliveredAt != nil {
			t := job.DeliveredAt.Format("2006-01-02T15:04:05Z07:00")
			deliveryInfo.DeliveredAt = &t
		}

		resp.Delivery = deliveryInfo
	}

	return resp
}

type OrderResponse struct {
	ID                  uuid.UUID  `json:"id"`
	PaymentRequestID    uuid.UUID  `json:"payment_request_id"`
	EscrowID            *uuid.UUID `json:"escrow_id,omitempty"`
	PaymentMethod       string     `json:"payment_method"`
	PaymentStatus       string     `json:"payment_status"`
	ItemAmount          float64    `json:"item_amount"`
	DeliveryFeeEstimate float64    `json:"delivery_fee_estimate"`
	BuyerServiceFee     float64    `json:"buyer_service_fee"`
	TotalAmount         float64    `json:"total_amount"`
	Status              string     `json:"status"`
	CreatedAt           string     `json:"created_at"`
	PaidAt              *string    `json:"paid_at,omitempty"`
}

func orderToResponse(order *domain.Order) OrderResponse {
	resp := OrderResponse{
		ID:                  order.ID,
		PaymentRequestID:    order.PaymentRequestID,
		EscrowID:            order.EscrowID,
		PaymentMethod:       string(order.PaymentMethod),
		PaymentStatus:       string(order.PaymentStatus),
		ItemAmount:          order.ItemAmount,
		DeliveryFeeEstimate: order.DeliveryFeeEstimate,
		BuyerServiceFee:     order.BuyerServiceFee,
		TotalAmount:         order.TotalAmount,
		Status:              string(order.Status),
		CreatedAt:           order.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	if order.PaidAt != nil {
		paidAt := order.PaidAt.Format("2006-01-02T15:04:05Z07:00")
		resp.PaidAt = &paidAt
	}

	return resp
}
func (h *BuyerHandler) GetPurchaseTimeline(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid purchase ID")
		return
	}

	order, err := h.orderService.GetOrderByID(c.Request.Context(), id)
	if err != nil {
		h.handleError(c, err)
		return
	}

	if order.BuyerID != userID {
		util.Error(c, http.StatusForbidden, domain.CodeUnauthorized, "You don't have access to this purchase")
		return
	}

	pr, err := h.prService.GetByID(c.Request.Context(), order.PaymentRequestID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	var job *domain.DeliveryJob
	if pr.DeliveryType == domain.DeliveryTypeIntraState {
		job, _ = h.deliveryJobRepo.GetByPaymentRequestID(c.Request.Context(), order.PaymentRequestID)
	}

	timeline := BuildTimeline(TimelineViewBuyer, pr, order, job)

	util.Success(c, http.StatusOK, timeline)
}
