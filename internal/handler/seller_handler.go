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
	orderService    service.OrderService
	prService       service.PaymentRequestService
	escrowService   service.EscrowService
	deliveryJobRepo repository.DeliveryJobRepository
}

func NewSellerHandler(
	orderService service.OrderService,
	prService service.PaymentRequestService,
	escrowService service.EscrowService,
	deliveryJobRepo repository.DeliveryJobRepository,
) *SellerHandler {
	return &SellerHandler{
		orderService:    orderService,
		prService:       prService,
		escrowService:   escrowService,
		deliveryJobRepo: deliveryJobRepo,
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
	}
}

// ListSales lists all sales (orders) for the authenticated seller
// GET /seller/sales
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

	// Enrich with payment request data
	responses := make([]SaleResponse, 0, len(orders))
	for _, order := range orders {
		pr, err := h.prService.GetByID(c.Request.Context(), order.PaymentRequestID)
		if err != nil {
			continue
		}

		// Get escrow if exists
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

// GetSale gets a specific sale (order) for the seller
// GET /seller/sales/:id
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

	// Get payment request to verify ownership
	pr, err := h.prService.GetByID(c.Request.Context(), order.PaymentRequestID)
	if err != nil {
		util.Error(c, http.StatusNotFound, "NOT_FOUND", "Payment request not found")
		return
	}

	// Verify ownership - the payment request must belong to this seller
	if pr.SellerID != userID {
		util.Error(c, http.StatusForbidden, domain.CodeUnauthorized, "You don't have access to this sale")
		return
	}

	// Get escrow details if exists
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

	response := SaleDetailResponse{
		SaleResponse: orderToSaleResponse(order, pr, escrowStatus),
		Escrow:       escrowDetails,
	}

	util.Success(c, http.StatusOK, response)
}

// GetEarnings returns earnings summary for the seller
// GET /seller/earnings
func (h *SellerHandler) GetEarnings(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	// Get all orders for this seller
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
			// Money still in escrow - pending
			pendingAmount += escrow.ItemAmount // Seller gets item amount minus platform fee
			pendingCount++
		case domain.EscrowStatusReleased:
			// Money released to seller
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

// Response types

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

type SaleDetailResponse struct {
	SaleResponse
	Escrow *EscrowDetails `json:"escrow,omitempty"`
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

// GetSaleTimeline returns a transaction timeline for the seller
// GET /seller/sales/:id/timeline
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

	// Get delivery job if exists
	var job *domain.DeliveryJob
	if pr.DeliveryType == domain.DeliveryTypeIntraState {
		job, _ = h.deliveryJobRepo.GetByPaymentRequestID(c.Request.Context(), order.PaymentRequestID)
	}

	timeline := BuildTimeline(TimelineViewSeller, pr, order, job)

	util.Success(c, http.StatusOK, timeline)
}
