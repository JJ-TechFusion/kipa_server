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
	prService                service.PaymentRequestService
	orderService             service.OrderService
	escrowService            service.EscrowService
	userRepo                 repository.UserRepository
	deliveryJobRepo          repository.DeliveryJobRepository
	disputeRepo              repository.DisputeRepository
	disputeResolutionService service.DisputeResolutionService
	driverService            service.DriverService
}

func NewBuyerHandler(
	prService service.PaymentRequestService,
	orderService service.OrderService,
	escrowService service.EscrowService,
	userRepo repository.UserRepository,
	deliveryJobRepo repository.DeliveryJobRepository,
	disputeRepo repository.DisputeRepository,
	disputeResolutionService service.DisputeResolutionService,
	driverService service.DriverService,
) *BuyerHandler {
	return &BuyerHandler{
		prService:                prService,
		orderService:             orderService,
		escrowService:            escrowService,
		userRepo:                 userRepo,
		deliveryJobRepo:          deliveryJobRepo,
		disputeRepo:              disputeRepo,
		disputeResolutionService: disputeResolutionService,
		driverService:            driverService,
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
		purchases.GET("/:id/return", h.GetReturnStatus)
		purchases.POST("/:id/initiate-return", h.InitiateReturn)
		purchases.POST("/:id/ready-for-return", h.ReadyForReturn)
		purchases.GET("/:id/rebook-status", h.GetRebookStatus)
		purchases.POST("/:id/rebook", h.RebookDelivery)
		purchases.GET("/driver-deliveries", h.ListBuyerDriverDeliveries)
		purchases.GET("/driver-deliveries/:delivery_id", h.GetBuyerDriverDelivery)
		purchases.GET("/driver-deliveries/:delivery_id/timeline", h.GetDriverDeliveryTimeline)
		purchases.GET("/driver-deliveries/:delivery_id/dropoff-code", h.GetDropoffCode)
		purchases.POST("/driver-deliveries/:delivery_id/confirm", h.ConfirmDriverDelivery)
		purchases.POST("/driver-deliveries/:delivery_id/dispute", h.DisputeDriverDelivery)
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
	seller, err := h.userRepo.GetByID(c.Request.Context(), pr.SellerID)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, "SELLER_NOT_FOUND", "Failed to retrieve seller information")
		return
	}

	util.Success(c, http.StatusOK, paymentPageToResponse(pr, seller))
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
			"order_id":           resp.OrderID,
			"payment_request_id": resp.PaymentRequestID,
			"authorization_url":  resp.AuthorizationURL,
			"access_code":        resp.AccessCode,
			"reference":          resp.Reference,
			"payment_method":     "card",
		})

	case "wallet":
		resp, err := h.orderService.ProcessWalletPayment(c.Request.Context(), userID, code)
		if err != nil {
			h.handleError(c, err)
			return
		}
		pr, _ := h.prService.GetByID(c.Request.Context(), resp.PaymentRequestID)

		response := gin.H{
			"order_id":           resp.OrderID,
			"payment_request_id": resp.PaymentRequestID,
			"escrow_id":          resp.EscrowID,
			"status":             resp.Status,
			"payment_method":     "wallet",
			"message":            "Payment completed successfully",
		}

		if pr != nil {
			response["delivery_type"] = string(pr.DeliveryType)
			response["pickup_address"] = pr.PickupAddress
			response["dropoff_address"] = pr.DropoffAddress
			if resp.LogisticsDeliveryID != nil {
				response["logistics_delivery_id"] = resp.LogisticsDeliveryID
			}
			if pr.DeliveryJobID != nil {
				response["delivery_job_id"] = pr.DeliveryJobID
			}
		}

		util.Success(c, http.StatusOK, response)

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
	pr, _ := h.prService.GetByID(c.Request.Context(), resp.PaymentRequestID)

	response := gin.H{
		"order_id":           resp.OrderID,
		"payment_request_id": resp.PaymentRequestID,
		"payment_status":     resp.PaymentStatus,
		"escrow_id":          resp.EscrowID,
		"status":             resp.Status,
		"message":            "Payment verified successfully",
	}

	if pr != nil {
		response["delivery_type"] = string(pr.DeliveryType)
		response["pickup_address"] = pr.PickupAddress
		response["dropoff_address"] = pr.DropoffAddress
		if resp.LogisticsDeliveryID != nil {
			response["logistics_delivery_id"] = resp.LogisticsDeliveryID
		}
		if pr.DeliveryJobID != nil {
			response["delivery_job_id"] = pr.DeliveryJobID
		}
	}

	util.Success(c, http.StatusOK, response)
}
func (h *BuyerHandler) ListPurchases(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}
	statusFilter := c.Query("status")

	limit := 50
	offset := 0

	orders, err := h.orderService.GetOrdersByBuyerID(c.Request.Context(), userID, limit, offset)
	if err != nil {
		h.handleError(c, err)
		return
	}
	statusCounts := map[string]int{
		"all":         0,
		"processing":  0,
		"in_delivery": 0,
		"completed":   0,
		"disputed":    0,
	}

	responses := make([]PurchaseResponse, 0)
	for _, order := range orders {
		if order.PaymentStatus != domain.OrderPaymentVerified {
			continue
		}

		pr, err := h.prService.GetByID(c.Request.Context(), order.PaymentRequestID)
		if err != nil {
			continue
		}
		statusCounts["all"]++
		switch order.Status {
		case domain.OrderStatusPaid, domain.OrderStatusProcessing, domain.OrderStatusReady:
			statusCounts["processing"]++
		case domain.OrderStatusInDelivery, domain.OrderStatusDelivered:
			statusCounts["in_delivery"]++
		case domain.OrderStatusCompleted:
			statusCounts["completed"]++
		case domain.OrderStatusDisputed:
			statusCounts["disputed"]++
		}
		if statusFilter != "" && statusFilter != "all" {
			switch statusFilter {
			case "processing":
				if order.Status != domain.OrderStatusPaid && order.Status != domain.OrderStatusProcessing && order.Status != domain.OrderStatusReady {
					continue
				}
			case "in_delivery":
				if order.Status != domain.OrderStatusInDelivery && order.Status != domain.OrderStatusDelivered {
					continue
				}
			case "completed":
				if order.Status != domain.OrderStatusCompleted {
					continue
				}
			case "disputed":
				if order.Status != domain.OrderStatusDisputed {
					continue
				}
			}
		}

		var job *domain.DeliveryJob
		if pr.DeliveryType == domain.DeliveryTypeIntraState && pr.DeliveryJobID != nil {
			job, _ = h.deliveryJobRepo.GetByID(c.Request.Context(), *pr.DeliveryJobID)
		}

		responses = append(responses, orderToPurchaseResponse(order, pr, job))
	}

	util.Success(c, http.StatusOK, gin.H{
		"purchases":     responses,
		"count":         len(responses),
		"status_counts": statusCounts,
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
	var returnJob *domain.DeliveryJob
	var forcedReturnJob *domain.DeliveryJob
	if pr.DeliveryType == domain.DeliveryTypeIntraState {
		if pr.DeliveryJobID != nil {
			job, _ = h.deliveryJobRepo.GetByID(c.Request.Context(), *pr.DeliveryJobID)
		}
		if pr.ReturnDeliveryJobID != nil {
			returnJob, _ = h.deliveryJobRepo.GetByID(c.Request.Context(), *pr.ReturnDeliveryJobID)
		}
		if pr.ForcedReturnJobID != nil {
			forcedReturnJob, _ = h.deliveryJobRepo.GetByID(c.Request.Context(), *pr.ForcedReturnJobID)
		}
	}

	resp := orderToPurchaseResponseWithReturn(order, pr, job, returnJob)
	timelineReturnJob := returnJob
	if forcedReturnJob != nil && returnJob == nil {
		timelineReturnJob = forcedReturnJob
	}
	timeline := BuildTimelineWithReturn(TimelineViewBuyer, pr, order, job, timelineReturnJob)
	resp.Timeline = &timeline

	util.Success(c, http.StatusOK, resp)
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
	dispute := domain.NewDispute(*order.EscrowID, userID, req.Reason, req.Evidence)
	if err := h.disputeRepo.Create(c.Request.Context(), dispute); err != nil {
		_ = err
	}

	if err := h.orderService.UpdateStatus(c.Request.Context(), order.ID, domain.OrderStatusDisputed); err != nil {
		h.handleError(c, err)
		return
	}
	pr, err := h.prService.GetByID(c.Request.Context(), order.PaymentRequestID)
	if err == nil && !pr.IsReusable {
		_ = h.prService.UpdateStatus(c.Request.Context(), order.PaymentRequestID, domain.PRStatusDisputed)
	}

	util.Success(c, http.StatusOK, gin.H{
		"message":    "Dispute opened. Our team will review your case.",
		"dispute_id": dispute.ID,
	})
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
	ProcessingTimeHours  *int     `json:"processing_time_hours,omitempty"`
	DeliveryType         string   `json:"delivery_type"`
	EstimatedDeliveryFee *float64 `json:"estimated_delivery_fee,omitempty"`
	BuyerServiceFee      *float64 `json:"buyer_service_fee,omitempty"`
	EstimatedTotal       *float64 `json:"estimated_total,omitempty"`
	SellerID             uuid.UUID `json:"seller_id"`
	SellerName           string   `json:"seller_name"`
	SellerEmail          *string  `json:"seller_email,omitempty"`
	SellerPhone          string   `json:"seller_phone"`
}

func paymentPageToResponse(pr *domain.PaymentRequest, seller *domain.User) PaymentPageResponse {
	return PaymentPageResponse{
		ItemName:             pr.ItemName,
		ItemDescription:      pr.ItemDescription,
		ItemPrice:            pr.ItemPrice,
		ProcessingTimeHours:  pr.ProcessingTimeHours,
		DeliveryType:         string(pr.DeliveryType),
		EstimatedDeliveryFee: pr.EstimatedDeliveryFee,
		BuyerServiceFee:      pr.BuyerServiceFee,
		EstimatedTotal:       pr.EstimatedTotal,
		SellerID:             pr.SellerID,
		SellerName:           seller.FullName(),
		SellerEmail:          seller.Email,
		SellerPhone:          seller.PhoneNumber,
	}
}

type PurchaseResponse struct {
	ID                   uuid.UUID  `json:"id"`
	PaymentRequestID     uuid.UUID  `json:"payment_request_id"`
	ItemName             string     `json:"item_name"`
	ItemDescription      *string    `json:"item_description,omitempty"`
	ItemPrice            float64    `json:"item_price"`
	ProcessingTimeHours  *int       `json:"processing_time_hours,omitempty"`
	DeliveryType         string     `json:"delivery_type"`
	PickupAddress        *string    `json:"pickup_address,omitempty"`
	DropoffAddress       *string    `json:"dropoff_address,omitempty"`
	EstimatedDeliveryFee float64    `json:"estimated_delivery_fee"`
	BuyerServiceFee      float64    `json:"buyer_service_fee"`
	TotalAmount          float64    `json:"total_amount"`
	PaymentMethod        string     `json:"payment_method"`
	Status               string     `json:"status"`
	PRStatus             string     `json:"pr_status"` // Payment request status for debugging
	SellerID             uuid.UUID  `json:"seller_id"`
	EscrowID             *uuid.UUID `json:"escrow_id,omitempty"`
	DeliveryJobID        *uuid.UUID `json:"delivery_job_id,omitempty"`
	ReturnJobID          *uuid.UUID `json:"return_job_id,omitempty"`
	PaidAt               *string    `json:"paid_at,omitempty"`
	CreatedAt            string     `json:"created_at"`

	Delivery *PurchaseDeliveryInfo `json:"delivery,omitempty"`
	Timeline *TimelineResponse     `json:"timeline,omitempty"`
}

type PurchaseDeliveryInfo struct {
	JobID         *uuid.UUID `json:"job_id,omitempty"`
	ReturnJobID   *uuid.UUID `json:"return_job_id,omitempty"`
	Status        string     `json:"status"`
	Phase         string     `json:"phase"` // "delivery" or "return"
	PickupCode    *string    `json:"pickup_code,omitempty"`  // For buyer in return phase
	DropoffCode   *string    `json:"dropoff_code,omitempty"` // For buyer in delivery phase
	RiderAssigned bool       `json:"rider_assigned"`
	PickedUpAt    *string    `json:"picked_up_at,omitempty"`
	DeliveredAt   *string    `json:"delivered_at,omitempty"`
	ReturnStatus      *string `json:"return_status,omitempty"`
	ReturnRiderAssigned bool  `json:"return_rider_assigned,omitempty"`
	ReturnPickedUpAt  *string `json:"return_picked_up_at,omitempty"`
	ReturnDeliveredAt *string `json:"return_delivered_at,omitempty"`
}

func orderToPurchaseResponse(order *domain.Order, pr *domain.PaymentRequest, job *domain.DeliveryJob) PurchaseResponse {
	return orderToPurchaseResponseWithReturn(order, pr, job, nil)
}

func orderToPurchaseResponseWithReturn(order *domain.Order, pr *domain.PaymentRequest, job *domain.DeliveryJob, returnJob *domain.DeliveryJob) PurchaseResponse {
	resp := PurchaseResponse{
		ID:                   order.ID,
		PaymentRequestID:     order.PaymentRequestID,
		ItemName:             pr.ItemName,
		ItemDescription:      pr.ItemDescription,
		ItemPrice:            order.ItemAmount,
		ProcessingTimeHours:  pr.ProcessingTimeHours,
		DeliveryType:         string(pr.DeliveryType),
		PickupAddress:        pr.PickupAddress,
		DropoffAddress:       pr.DropoffAddress,
		EstimatedDeliveryFee: order.DeliveryFeeEstimate,
		BuyerServiceFee:      order.BuyerServiceFee,
		TotalAmount:          order.TotalAmount,
		PaymentMethod:        string(order.PaymentMethod),
		Status:               string(order.Status),
		PRStatus:             string(pr.Status),
		SellerID:             pr.SellerID,
		EscrowID:             order.EscrowID,
		DeliveryJobID:        pr.DeliveryJobID,
		ReturnJobID:          pr.ReturnDeliveryJobID,
		CreatedAt:            order.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	if order.PaidAt != nil {
		paidAt := order.PaidAt.Format("2006-01-02T15:04:05Z07:00")
		resp.PaidAt = &paidAt
	}
	isReturnPhase := pr.Status.IsReturnFlow()

	if job != nil {
		deliveryInfo := &PurchaseDeliveryInfo{
			JobID:         &job.ID,
			Status:        string(job.Status),
			RiderAssigned: job.RiderID != nil,
			Phase:         "delivery",
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
		if isReturnPhase && returnJob != nil {
			deliveryInfo.Phase = "return"
			deliveryInfo.ReturnJobID = &returnJob.ID
			returnStatus := string(returnJob.Status)
			deliveryInfo.ReturnStatus = &returnStatus
			deliveryInfo.ReturnRiderAssigned = returnJob.RiderID != nil
			if returnJob.RiderID != nil && returnJob.PickupCode != nil {
				deliveryInfo.PickupCode = returnJob.PickupCode
			}

			if returnJob.PickedUpAt != nil {
				t := returnJob.PickedUpAt.Format("2006-01-02T15:04:05Z07:00")
				deliveryInfo.ReturnPickedUpAt = &t
			}
			if returnJob.DeliveredAt != nil {
				t := returnJob.DeliveredAt.Format("2006-01-02T15:04:05Z07:00")
				deliveryInfo.ReturnDeliveredAt = &t
			}
		} else if isReturnPhase && returnJob == nil {
			deliveryInfo.Phase = "return"
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
	if pr.DeliveryType == domain.DeliveryTypeIntraState && pr.DeliveryJobID != nil {
		job, _ = h.deliveryJobRepo.GetByID(c.Request.Context(), *pr.DeliveryJobID)
	}

	timeline := BuildTimeline(TimelineViewBuyer, pr, order, job)

	util.Success(c, http.StatusOK, timeline)
}
func (h *BuyerHandler) GetReturnStatus(c *gin.Context) {
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
	if !pr.Status.IsReturnFlow() && pr.Status != domain.PRStatusReturnRequired {
		util.Error(c, http.StatusNotFound, "NO_RETURN", "No return required for this purchase")
		return
	}
	canConfirmReady := pr.Status == domain.PRStatusReturnRequired

	response := BuyerReturnStatusResponse{
		PaymentRequestID: pr.ID,
		Status:           string(pr.Status),
		DisputeOutcome:   pr.DisputeOutcome,
		CanInitiate:      pr.Status == domain.PRStatusReturnRequired && pr.ReturnDeliveryJobID == nil,
		CanConfirmReady:  canConfirmReady,
	}
	if pr.ReturnDeliveryJobID != nil {
		returnJob, err := h.deliveryJobRepo.GetByID(c.Request.Context(), *pr.ReturnDeliveryJobID)
		if err == nil {
			response.ReturnJob = &BuyerReturnJobInfo{
				ID:             returnJob.ID,
				Status:         string(returnJob.Status),
				PickupAddress:  returnJob.PickupAddress,
				DropoffAddress: returnJob.DropoffAddress,
				RiderAssigned:  returnJob.RiderID != nil,
				PickupCode:     returnJob.PickupCode,
				CreatedAt:      returnJob.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			}
			if returnJob.AcceptedAt != nil {
				t := returnJob.AcceptedAt.Format("2006-01-02T15:04:05Z07:00")
				response.ReturnJob.AcceptedAt = &t
			}
			if returnJob.PickedUpAt != nil {
				t := returnJob.PickedUpAt.Format("2006-01-02T15:04:05Z07:00")
				response.ReturnJob.PickedUpAt = &t
			}
			if returnJob.DeliveredAt != nil {
				t := returnJob.DeliveredAt.Format("2006-01-02T15:04:05Z07:00")
				response.ReturnJob.DeliveredAt = &t
			}
		}
	}

	util.Success(c, http.StatusOK, response)
}
func (h *BuyerHandler) InitiateReturn(c *gin.Context) {
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
	if pr.Status != domain.PRStatusReturnRequired {
		util.Error(c, http.StatusBadRequest, "INVALID_STATUS", "Return is not required for this purchase")
		return
	}
	if pr.ReturnDeliveryJobID != nil {
		util.Error(c, http.StatusBadRequest, "ALREADY_INITIATED", "Return has already been initiated")
		return
	}
	returnJob, err := h.disputeResolutionService.CreateReturnDeliveryJob(c.Request.Context(), pr.ID)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, "RETURN_FAILED", err.Error())
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"message":       "Return initiated successfully. A rider will be dispatched to pick up the item.",
		"return_job_id": returnJob.ID,
		"status":        string(returnJob.Status),
		"pickup_address": returnJob.PickupAddress,
		"dropoff_address": returnJob.DropoffAddress,
	})
}
func (h *BuyerHandler) ReadyForReturn(c *gin.Context) {
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
	if pr.Status != domain.PRStatusReturnRequired {
		util.Error(c, http.StatusBadRequest, "INVALID_STATUS", "Purchase is not awaiting return confirmation")
		return
	}
	returnJob, err := h.disputeResolutionService.ConfirmReadyForReturn(c.Request.Context(), pr.ID, userID)
	if err != nil {
		util.Error(c, http.StatusBadRequest, "RETURN_FAILED", err.Error())
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"message":         "Return search started. A rider will be assigned shortly.",
		"return_job_id":   returnJob.ID,
		"status":          string(returnJob.Status),
		"pickup_address":  returnJob.PickupAddress,
		"pickup_lat":      returnJob.PickupLat,
		"pickup_lng":      returnJob.PickupLng,
		"dropoff_address": returnJob.DropoffAddress,
		"dropoff_lat":     returnJob.DropoffLat,
		"dropoff_lng":     returnJob.DropoffLng,
		"delivery_fee":    returnJob.DeliveryFee,
		"pickup_code":     returnJob.PickupCode, // Note: Generated when rider accepts
	})
}
type BuyerReturnStatusResponse struct {
	PaymentRequestID uuid.UUID           `json:"payment_request_id"`
	Status           string              `json:"status"`
	DisputeOutcome   *string             `json:"dispute_outcome,omitempty"`
	CanInitiate      bool                `json:"can_initiate"`
	CanConfirmReady  bool                `json:"can_confirm_ready"`
	ReturnJob        *BuyerReturnJobInfo `json:"return_job,omitempty"`
}

type BuyerReturnJobInfo struct {
	ID             uuid.UUID `json:"id"`
	Status         string    `json:"status"`
	PickupAddress  string    `json:"pickup_address"`
	DropoffAddress string    `json:"dropoff_address"`
	RiderAssigned  bool      `json:"rider_assigned"`
	PickupCode     *string   `json:"pickup_code,omitempty"`
	CreatedAt      string    `json:"created_at"`
	AcceptedAt     *string   `json:"accepted_at,omitempty"`
	PickedUpAt     *string   `json:"picked_up_at,omitempty"`
	DeliveredAt    *string   `json:"delivered_at,omitempty"`
}
func (h *BuyerHandler) GetRebookStatus(c *gin.Context) {
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

	status, err := h.prService.GetRebookStatus(c.Request.Context(), order.PaymentRequestID, userID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, status)
}
func (h *BuyerHandler) RebookDelivery(c *gin.Context) {
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

	job, err := h.prService.RebookDelivery(c.Request.Context(), order.PaymentRequestID, userID, &service.RebookDeliveryRequest{})
	if err != nil {
		util.Error(c, http.StatusBadRequest, "REBOOK_FAILED", err.Error())
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"message":         "Delivery rebooked successfully. Rider will continue to your location.",
		"job_id":          job.ID,
		"status":          string(job.Status),
		"dropoff_address": job.DropoffAddress,
	})
}

func (h *BuyerHandler) ListBuyerDriverDeliveries(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	limit := 20
	offset := 0

	deliveries, err := h.driverService.GetDeliveriesByBuyer(c.Request.Context(), userID, limit, offset)
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

func (h *BuyerHandler) GetBuyerDriverDelivery(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	id, err := uuid.Parse(c.Param("delivery_id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid delivery ID")
		return
	}

	delivery, err := h.driverService.GetDeliveryForBuyer(c.Request.Context(), userID, id)
	if err != nil {
		if err == domain.ErrUnauthorized {
			util.Error(c, http.StatusForbidden, domain.CodeUnauthorized, "You don't have access to this delivery")
			return
		}
		util.Error(c, http.StatusNotFound, "NOT_FOUND", "Delivery not found")
		return
	}
	pr, _ := h.prService.GetByID(c.Request.Context(), delivery.PaymentRequestID)

	resp := driverDeliveryToResponse(delivery)
	if pr != nil {
		timeline := BuildDriverDeliveryTimeline(TimelineViewBuyer, delivery, pr)
		resp.Timeline = &timeline
	}

	util.Success(c, http.StatusOK, resp)
}

func (h *BuyerHandler) GetDropoffCode(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	id, err := uuid.Parse(c.Param("delivery_id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid delivery ID")
		return
	}

	code, err := h.driverService.GetDropoffCodeForBuyer(c.Request.Context(), userID, id)
	if err != nil {
		if err == domain.ErrUnauthorized {
			util.Error(c, http.StatusForbidden, domain.CodeUnauthorized, "You don't have access to this delivery")
			return
		}
		util.Error(c, http.StatusBadRequest, "CODE_ERROR", err.Error())
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"dropoff_code": code,
		"message":      "Give this code to the driver to confirm delivery",
	})
}

func (h *BuyerHandler) ConfirmDriverDelivery(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	id, err := uuid.Parse(c.Param("delivery_id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid delivery ID")
		return
	}

	if err := h.driverService.ConfirmDelivery(c.Request.Context(), userID, id); err != nil {
		if err == domain.ErrUnauthorized {
			util.Error(c, http.StatusForbidden, domain.CodeUnauthorized, "You don't have access to this delivery")
			return
		}
		util.Error(c, http.StatusBadRequest, "CONFIRM_ERROR", err.Error())
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"message": "Delivery confirmed. Payment released to seller.",
	})
}

func (h *BuyerHandler) DisputeDriverDelivery(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	id, err := uuid.Parse(c.Param("delivery_id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid delivery ID")
		return
	}

	var req DriverDeliveryDisputeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	if err := h.driverService.DisputeDelivery(c.Request.Context(), userID, id, req.Reason); err != nil {
		if err == domain.ErrUnauthorized {
			util.Error(c, http.StatusForbidden, domain.CodeUnauthorized, "You don't have access to this delivery")
			return
		}
		util.Error(c, http.StatusBadRequest, "DISPUTE_ERROR", err.Error())
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"message": "Dispute opened. Our team will review your case.",
	})
}

type DriverDeliveryDisputeRequest struct {
	Reason string `json:"reason" binding:"required"`
}

func (h *BuyerHandler) GetDriverDeliveryTimeline(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	id, err := uuid.Parse(c.Param("delivery_id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid delivery ID")
		return
	}

	delivery, err := h.driverService.GetDeliveryForBuyer(c.Request.Context(), userID, id)
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

	timeline := BuildDriverDeliveryTimeline(TimelineViewBuyer, delivery, pr)
	util.Success(c, http.StatusOK, timeline)
}
