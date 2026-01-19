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

type PaymentRequestHandler struct {
	prService service.PaymentRequestService
}

func NewPaymentRequestHandler(prService service.PaymentRequestService) *PaymentRequestHandler {
	return &PaymentRequestHandler{prService: prService}
}

func (h *PaymentRequestHandler) RegisterRoutes(router *gin.RouterGroup, authMiddleware *middleware.AuthMiddleware) {
	pr := router.Group("/payment-requests")
	pr.Use(authMiddleware.RequireAuth())
	{
		pr.POST("", h.CreateDraft)
		pr.GET("", h.ListMyRequests)
		pr.GET("/history", h.ListHistory)
		pr.GET("/:id", h.GetByID)
		pr.PATCH("/:id", h.Edit)
		pr.PATCH("/:id/fulfillment", h.ConfigureFulfillment)
		pr.POST("/:id/ready-for-pickup", h.MarkReadyForPickup)
		pr.DELETE("/:id", h.Cancel)
	}
}
func (h *PaymentRequestHandler) CreateDraft(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var req service.CreateDraftRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	pr, err := h.prService.CreateDraft(c.Request.Context(), userID, &req)
	if err != nil {
		c.Error(err)
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusCreated, paymentRequestToResponse(pr))
}
func (h *PaymentRequestHandler) ListMyRequests(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	limit := 20
	offset := 0

	requests, err := h.prService.GetBySellerID(c.Request.Context(), userID, limit, offset)
	if err != nil {
		h.handleError(c, err)
		return
	}

	responses := make([]PaymentRequestResponse, len(requests))
	for i, pr := range requests {
		responses[i] = paymentRequestToResponse(pr)
	}

	util.Success(c, http.StatusOK, gin.H{
		"payment_requests": responses,
		"count":            len(responses),
	})
}
func (h *PaymentRequestHandler) GetByID(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid payment request ID")
		return
	}

	pr, err := h.prService.GetByID(c.Request.Context(), id)
	if err != nil {
		h.handleError(c, err)
		return
	}
	if pr.SellerID != userID {
		util.Error(c, http.StatusForbidden, domain.CodeUnauthorized, "You don't have access to this payment request")
		return
	}

	util.Success(c, http.StatusOK, paymentRequestToResponse(pr))
}
func (h *PaymentRequestHandler) ConfigureFulfillment(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid payment request ID")
		return
	}

	var req service.ConfigureFulfillmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	pr, err := h.prService.ConfigureFulfillment(c.Request.Context(), id, userID, &req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{
		"payment_request": paymentRequestToResponse(pr),
		"payment_url":     "https://kipa.ng/pay/" + *pr.PaymentCode,
	})
}
type MarkReadyForPickupRequest struct {
	VehicleType string `json:"vehicle_type" binding:"required,oneof=motorcycle car"`
}
func (h *PaymentRequestHandler) MarkReadyForPickup(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid payment request ID")
		return
	}
	pr, err := h.prService.MarkReadyForPickup(c.Request.Context(), id, userID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, paymentRequestToResponse(pr))
}
func (h *PaymentRequestHandler) Cancel(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid payment request ID")
		return
	}

	if err := h.prService.Cancel(c.Request.Context(), id, userID); err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, gin.H{"message": "Payment request cancelled"})
}

func (h *PaymentRequestHandler) Edit(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid payment request ID")
		return
	}

	var req service.EditPaymentRequestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.ValidationError(c, err)
		return
	}

	pr, err := h.prService.Edit(c.Request.Context(), id, userID, &req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	util.Success(c, http.StatusOK, paymentRequestToResponse(pr))
}

func (h *PaymentRequestHandler) ListHistory(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	limit := 20
	offset := 0

	requests, err := h.prService.GetHistoryBySellerID(c.Request.Context(), userID, limit, offset)
	if err != nil {
		h.handleError(c, err)
		return
	}

	responses := make([]PaymentRequestResponse, len(requests))
	for i, pr := range requests {
		responses[i] = paymentRequestToResponse(pr)
	}

	util.Success(c, http.StatusOK, gin.H{
		"payment_requests": responses,
		"count":            len(responses),
	})
}

func (h *PaymentRequestHandler) handleError(c *gin.Context, err error) {
	status, code, message := mapPaymentRequestErrorToResponse(err)
	util.Error(c, status, code, message)
}

func mapPaymentRequestErrorToResponse(err error) (int, string, string) {
	switch {
	case errors.Is(err, domain.ErrPaymentRequestNotFound):
		return http.StatusNotFound, "PAYMENT_REQUEST_NOT_FOUND", "Payment request not found"
	case errors.Is(err, domain.ErrUnauthorized):
		return http.StatusForbidden, domain.CodeUnauthorized, "You don't have permission to access this resource"
	case errors.Is(err, domain.ErrInvalidStatusTransition):
		return http.StatusBadRequest, "INVALID_STATUS", "Cannot perform this action in the current status"
	case errors.Is(err, domain.ErrCannotCancel):
		return http.StatusBadRequest, "CANNOT_CANCEL", "Cannot cancel a payment request that has been paid"
	case errors.Is(err, domain.ErrZoneNotFound):
		return http.StatusBadRequest, "ZONE_NOT_FOUND", "Delivery zone not found for the specified location"
	case errors.Is(err, domain.ErrPaymentRequestNotEditable):
		return http.StatusBadRequest, "NOT_EDITABLE", "Payment request cannot be edited in its current status"
	default:
		return http.StatusInternalServerError, domain.CodeInternalError, "An unexpected error occurred"
	}
}
type PaymentRequestResponse struct {
	ID                   uuid.UUID `json:"id"`
	SellerID             uuid.UUID `json:"seller_id"`
	ItemName             string    `json:"item_name"`
	ItemDescription      *string   `json:"item_description,omitempty"`
	ItemPrice            float64   `json:"item_price"`
	DeliveryType         string    `json:"delivery_type"`
	PickupAddress        *string   `json:"pickup_address,omitempty"`
	DropoffAddress       *string   `json:"dropoff_address,omitempty"`
	EstimatedDeliveryFee *float64  `json:"estimated_delivery_fee,omitempty"`
	BuyerServiceFee      *float64  `json:"buyer_service_fee,omitempty"`
	EstimatedTotal       *float64  `json:"estimated_total,omitempty"`
	PaymentCode          *string   `json:"payment_code,omitempty"`
	Status               string    `json:"status"`
	BuyerID              *uuid.UUID `json:"buyer_id,omitempty"`
	CreatedAt            string    `json:"created_at"`
}

func paymentRequestToResponse(pr *domain.PaymentRequest) PaymentRequestResponse {
	return PaymentRequestResponse{
		ID:                   pr.ID,
		SellerID:             pr.SellerID,
		ItemName:             pr.ItemName,
		ItemDescription:      pr.ItemDescription,
		ItemPrice:            pr.ItemPrice,
		DeliveryType:         string(pr.DeliveryType),
		PickupAddress:        pr.PickupAddress,
		DropoffAddress:       pr.DropoffAddress,
		EstimatedDeliveryFee: pr.EstimatedDeliveryFee,
		BuyerServiceFee:      pr.BuyerServiceFee,
		EstimatedTotal:       pr.EstimatedTotal,
		PaymentCode:          pr.PaymentCode,
		Status:               string(pr.Status),
		BuyerID:              pr.BuyerID,
		CreatedAt:            pr.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
