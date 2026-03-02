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

type TransactionHandler struct {
	prService       service.PaymentRequestService
	orderService    service.OrderService
	userRepo        repository.UserRepository
	deliveryJobRepo repository.DeliveryJobRepository
}

func NewTransactionHandler(
	prService service.PaymentRequestService,
	orderService service.OrderService,
	userRepo repository.UserRepository,
	deliveryJobRepo repository.DeliveryJobRepository,
) *TransactionHandler {
	return &TransactionHandler{
		prService:       prService,
		orderService:    orderService,
		userRepo:        userRepo,
		deliveryJobRepo: deliveryJobRepo,
	}
}

func (h *TransactionHandler) RegisterRoutes(router *gin.RouterGroup, authMiddleware *middleware.AuthMiddleware) {
	tx := router.Group("/transactions")
	tx.Use(authMiddleware.RequireAuth())
	{
		tx.GET("", h.ListTransactions)
		tx.GET("/active-deliveries", h.GetActiveDeliveries)
		tx.GET("/:id", h.GetTransactionStatus)
	}
}

// ListTransactions returns all transactions for the user (as buyer or seller)
func (h *TransactionHandler) ListTransactions(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	// Get status filter from query param (optional)
	statusFilter := c.Query("status") // "active", "completed", "all" (default)

	var transactions []TransactionListItem

	// Get payment requests where user is seller (that have been paid)
	sellerPRs, _ := h.prService.GetBySellerID(c.Request.Context(), userID, 100, 0)
	for _, pr := range sellerPRs {
		// Skip drafts and link_active (not yet paid)
		if pr.Status == domain.PRStatusDraft || pr.Status == domain.PRStatusLinkActive {
			continue
		}

		// Apply status filter
		if !h.matchesStatusFilter(pr.Status, statusFilter) {
			continue
		}

		item := h.buildTransactionListItem(c, pr, "seller")
		transactions = append(transactions, item)
	}

	// Get payment requests where user is buyer
	buyerPRs, _ := h.prService.GetByBuyerID(c.Request.Context(), userID, 100, 0)
	for _, pr := range buyerPRs {
		// Apply status filter
		if !h.matchesStatusFilter(pr.Status, statusFilter) {
			continue
		}

		item := h.buildTransactionListItem(c, pr, "buyer")
		transactions = append(transactions, item)
	}

	// Sort by created_at descending (most recent first)
	for i := 0; i < len(transactions)-1; i++ {
		for j := i + 1; j < len(transactions); j++ {
			if transactions[i].CreatedAt < transactions[j].CreatedAt {
				transactions[i], transactions[j] = transactions[j], transactions[i]
			}
		}
	}

	util.Success(c, http.StatusOK, gin.H{
		"transactions": transactions,
		"count":        len(transactions),
	})
}

func (h *TransactionHandler) matchesStatusFilter(status domain.PaymentRequestStatus, filter string) bool {
	if filter == "" || filter == "all" {
		return true
	}

	isActive := status == domain.PRStatusPaidAwaitingFulfillment ||
		status == domain.PRStatusReadyForPickup ||
		status == domain.PRStatusSearchingRider ||
		status == domain.PRStatusRiderAssigned ||
		status == domain.PRStatusInDelivery ||
		status == domain.PRStatusDelivered ||
		status == domain.PRStatusConfirmationWindow ||
		status == domain.PRStatusDisputed ||
		status == domain.PRStatusPendingRebook ||
		status.IsReturnFlow()

	isCompleted := status == domain.PRStatusCompleted ||
		status == domain.PRStatusRefunded ||
		status == domain.PRStatusCancelled ||
		status == domain.PRStatusRebookExpired

	switch filter {
	case "active":
		return isActive
	case "completed":
		return isCompleted
	default:
		return true
	}
}

func (h *TransactionHandler) buildTransactionListItem(c *gin.Context, pr *domain.PaymentRequest, role string) TransactionListItem {
	item := TransactionListItem{
		PaymentRequestID: pr.ID,
		Role:             role,
		ItemName:         pr.ItemName,
		ItemPrice:        pr.ItemPrice,
		Status:           string(pr.Status),
		StatusText:       h.getStatusText(pr.Status, role),
		DeliveryType:     string(pr.DeliveryType),
		CreatedAt:        pr.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	// Add total amount
	if pr.EstimatedTotal != nil {
		item.TotalAmount = pr.EstimatedTotal
	}

	// Add paid timestamp
	if pr.PaidAt != nil {
		paidAt := pr.PaidAt.Format("2006-01-02T15:04:05Z07:00")
		item.PaidAt = &paidAt
	}

	// Get counterparty info
	if role == "seller" && pr.BuyerID != nil {
		buyer, err := h.userRepo.GetByID(c.Request.Context(), *pr.BuyerID)
		if err == nil {
			item.CounterpartyName = buyer.FullName()
			item.CounterpartyRole = "Buyer"
		}
	} else if role == "buyer" {
		seller, err := h.userRepo.GetByID(c.Request.Context(), pr.SellerID)
		if err == nil {
			item.CounterpartyName = seller.FullName()
			item.CounterpartyRole = "Seller"
		}
	}

	// For intra-state: get delivery job info if available
	if pr.DeliveryJobID != nil {
		item.DeliveryJobID = pr.DeliveryJobID
		job, err := h.deliveryJobRepo.GetByID(c.Request.Context(), *pr.DeliveryJobID)
		if err == nil {
			item.PickupAddress = &job.PickupAddress
			item.PickupLat = &job.PickupLat
			item.PickupLng = &job.PickupLng
			item.DropoffAddress = &job.DropoffAddress
			item.DropoffLat = &job.DropoffLat
			item.DropoffLng = &job.DropoffLng
		}
	}

	// For inter-state: include logistics delivery ID and addresses from payment request
	if pr.LogisticsDeliveryID != nil {
		item.LogisticsDeliveryID = pr.LogisticsDeliveryID
	}

	// Include addresses and coordinates from payment request if not already set
	// (for inter-state or before delivery job created)
	if item.PickupAddress == nil && pr.PickupAddress != nil {
		item.PickupAddress = pr.PickupAddress
	}
	if item.DropoffAddress == nil && pr.DropoffAddress != nil {
		item.DropoffAddress = pr.DropoffAddress
	}
	if item.PickupLat == nil && pr.PickupLat != 0 {
		item.PickupLat = &pr.PickupLat
	}
	if item.PickupLng == nil && pr.PickupLng != 0 {
		item.PickupLng = &pr.PickupLng
	}
	if item.DropoffLat == nil && pr.DropoffLat != 0 {
		item.DropoffLat = &pr.DropoffLat
	}
	if item.DropoffLng == nil && pr.DropoffLng != 0 {
		item.DropoffLng = &pr.DropoffLng
	}

	return item
}

func (h *TransactionHandler) getStatusText(status domain.PaymentRequestStatus, role string) string {
	switch status {
	case domain.PRStatusPaidAwaitingFulfillment:
		if role == "seller" {
			return "Awaiting your fulfillment"
		}
		return "Awaiting seller fulfillment"
	case domain.PRStatusReadyForPickup:
		return "Ready for pickup"
	case domain.PRStatusSearchingRider:
		return "Finding rider"
	case domain.PRStatusRiderAssigned:
		return "Rider assigned"
	case domain.PRStatusInDelivery:
		return "In delivery"
	case domain.PRStatusDelivered:
		return "Delivered"
	case domain.PRStatusConfirmationWindow:
		return "Awaiting confirmation"
	case domain.PRStatusCompleted:
		return "Completed"
	case domain.PRStatusDisputed:
		return "Disputed"
	case domain.PRStatusRefunded:
		return "Refunded"
	case domain.PRStatusCancelled:
		return "Cancelled"
	case domain.PRStatusPendingRebook:
		return "Pending rebook"
	case domain.PRStatusReturnRequired:
		return "Return required"
	case domain.PRStatusReturnSearchingRider:
		return "Finding return rider"
	case domain.PRStatusReturnInProgress:
		return "Return in progress"
	case domain.PRStatusReturnDelivered:
		return "Return delivered"
	case domain.PRStatusReturnConfirmed:
		return "Return confirmed"
	default:
		return string(status)
	}
}

// GetTransactionStatus returns transaction details based on viewer role (buyer or seller)
func (h *TransactionHandler) GetTransactionStatus(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	prID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_ID", "Invalid transaction ID")
		return
	}

	pr, err := h.prService.GetByID(c.Request.Context(), prID)
	if err != nil {
		if errors.Is(err, domain.ErrPaymentRequestNotFound) {
			util.Error(c, http.StatusNotFound, "NOT_FOUND", "Transaction not found")
			return
		}
		util.Error(c, http.StatusInternalServerError, domain.CodeInternalError, "Failed to get transaction")
		return
	}

	// Determine viewer role
	var viewerRole string
	if pr.SellerID == userID {
		viewerRole = "seller"
	} else if pr.BuyerID != nil && *pr.BuyerID == userID {
		viewerRole = "buyer"
	} else {
		util.Error(c, http.StatusForbidden, domain.CodeUnauthorized, "You don't have access to this transaction")
		return
	}

	// Build response
	resp := TransactionStatusResponse{
		ID:                  pr.ID,
		Status:              string(pr.Status),
		ViewerRole:          viewerRole,
		ItemName:            pr.ItemName,
		ItemDescription:     pr.ItemDescription,
		ItemPrice:           pr.ItemPrice,
		DeliveryType:        string(pr.DeliveryType),
		ProcessingTimeHours: pr.ProcessingTimeHours,
		DeliveryJobID:       pr.DeliveryJobID,
		CreatedAt:           pr.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	// Fee info based on viewer role
	if viewerRole == "seller" {
		resp.FeeInfo = &TransactionFeeInfo{
			EstimatedDeliveryFee: pr.EstimatedDeliveryFee,
			PlatformFee:          pr.SellerPlatformFee,
		}
		// Calculate what seller receives
		if pr.SellerPlatformFee != nil {
			youReceive := pr.ItemPrice - *pr.SellerPlatformFee
			resp.FeeInfo.YouReceive = &youReceive
		} else {
			resp.FeeInfo.YouReceive = &pr.ItemPrice
		}
		// Show buyer total for seller's reference
		resp.FeeInfo.BuyerPaysTotal = pr.EstimatedTotal
	} else {
		// Buyer view
		resp.FeeInfo = &TransactionFeeInfo{
			EstimatedDeliveryFee: pr.EstimatedDeliveryFee,
			ServiceFee:           pr.BuyerServiceFee,
			BuyerPaysTotal:       pr.EstimatedTotal,
		}
	}

	// Get seller info
	seller, err := h.userRepo.GetByID(c.Request.Context(), pr.SellerID)
	if err == nil {
		resp.Seller = &TransactionPartyInfo{
			ID:          seller.ID,
			Name:        seller.FullName(),
			PhoneNumber: seller.PhoneNumber,
		}
	}

	// Get buyer info (if payment made)
	if pr.BuyerID != nil {
		buyer, err := h.userRepo.GetByID(c.Request.Context(), *pr.BuyerID)
		if err == nil {
			resp.Buyer = &TransactionPartyInfo{
				ID:          buyer.ID,
				Name:        buyer.FullName(),
				PhoneNumber: buyer.PhoneNumber,
			}
		}
	}

	// Payment info (if paid)
	if pr.PaidAt != nil {
		paidAt := pr.PaidAt.Format("2006-01-02T15:04:05Z07:00")
		resp.Payment = &TransactionPaymentInfo{
			IsPaid: true,
			PaidAt: &paidAt,
		}

		// Get order for payment details
		if pr.BuyerID != nil {
			orders, err := h.orderService.GetOrdersByBuyerID(c.Request.Context(), *pr.BuyerID, 10, 0)
			if err == nil {
				for _, order := range orders {
					if order.PaymentRequestID == pr.ID && order.PaymentStatus == domain.OrderPaymentVerified {
						resp.Payment.Amount = order.TotalAmount
						resp.Payment.PaymentMethod = string(order.PaymentMethod)
						break
					}
				}
			}
		}
	} else {
		resp.Payment = &TransactionPaymentInfo{
			IsPaid: false,
		}
	}

	// Delivery info
	if pr.DeliveryJobID != nil {
		job, err := h.deliveryJobRepo.GetByID(c.Request.Context(), *pr.DeliveryJobID)
		if err == nil {
			resp.Delivery = &TransactionDeliveryInfo{
				JobID:          job.ID,
				Status:         string(job.Status),
				RiderAssigned:  job.RiderID != nil,
				PickupAddress:  job.PickupAddress,
				PickupLat:      job.PickupLat,
				PickupLng:      job.PickupLng,
				DropoffAddress: job.DropoffAddress,
				DropoffLat:     job.DropoffLat,
				DropoffLng:     job.DropoffLng,
			}
			if job.AcceptedAt != nil {
				t := job.AcceptedAt.Format("2006-01-02T15:04:05Z07:00")
				resp.Delivery.AcceptedAt = &t
			}
			if job.PickedUpAt != nil {
				t := job.PickedUpAt.Format("2006-01-02T15:04:05Z07:00")
				resp.Delivery.PickedUpAt = &t
			}
			if job.DeliveredAt != nil {
				t := job.DeliveredAt.Format("2006-01-02T15:04:05Z07:00")
				resp.Delivery.DeliveredAt = &t
			}
		}
	}

	// Build timeline
	var order *domain.Order
	if pr.BuyerID != nil {
		orders, _ := h.orderService.GetOrdersByBuyerID(c.Request.Context(), *pr.BuyerID, 10, 0)
		for _, o := range orders {
			if o.PaymentRequestID == pr.ID {
				order = o
				break
			}
		}
	}

	var job *domain.DeliveryJob
	if pr.DeliveryJobID != nil {
		job, _ = h.deliveryJobRepo.GetByID(c.Request.Context(), *pr.DeliveryJobID)
	}

	// Only build timeline if order exists (buyer has paid)
	// Otherwise show awaiting payment timeline
	if order != nil {
		var timelineView TimelineViewType
		if viewerRole == "seller" {
			timelineView = TimelineViewSeller
		} else {
			timelineView = TimelineViewBuyer
		}
		timeline := BuildTimeline(timelineView, pr, order, job)
		resp.Timeline = &timeline
	} else {
		// Pre-payment timeline for seller
		resp.Timeline = &TimelineResponse{
			Steps: []TimelineStep{
				{
					Step:        "link_created",
					Title:       "Payment Link Created",
					Description: "You created a payment request",
					Status:      "completed",
					Timestamp:   pr.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
				},
				{
					Step:        "awaiting_payment",
					Title:       "Awaiting Payment",
					Description: "Waiting for buyer to make payment",
					Status:      "current",
				},
			},
			CurrentStep: "awaiting_payment",
		}
	}

	util.Success(c, http.StatusOK, resp)
}

// Response types

type TransactionStatusResponse struct {
	ID                  uuid.UUID                `json:"id"`
	Status              string                   `json:"status"`
	ViewerRole          string                   `json:"viewer_role"` // "buyer" or "seller"
	ItemName            string                   `json:"item_name"`
	ItemDescription     *string                  `json:"item_description,omitempty"`
	ItemPrice           float64                  `json:"item_price"`
	DeliveryType        string                   `json:"delivery_type"`
	ProcessingTimeHours *int                     `json:"processing_time_hours,omitempty"`
	DeliveryJobID       *uuid.UUID               `json:"delivery_job_id,omitempty"`
	CreatedAt           string                   `json:"created_at"`
	FeeInfo             *TransactionFeeInfo      `json:"fee_info"`
	Payment             *TransactionPaymentInfo  `json:"payment"`
	Seller              *TransactionPartyInfo    `json:"seller"`
	Buyer               *TransactionPartyInfo    `json:"buyer,omitempty"`
	Delivery            *TransactionDeliveryInfo `json:"delivery,omitempty"`
	Timeline            *TimelineResponse        `json:"timeline"`
}

type TransactionFeeInfo struct {
	EstimatedDeliveryFee *float64 `json:"estimated_delivery_fee,omitempty"`
	ServiceFee           *float64 `json:"service_fee,omitempty"`   // Buyer's service fee (buyer view)
	PlatformFee          *float64 `json:"platform_fee,omitempty"`  // Seller's platform fee (seller view)
	BuyerPaysTotal       *float64 `json:"buyer_pays_total,omitempty"`
	YouReceive           *float64 `json:"you_receive,omitempty"`   // Seller view only
}

type TransactionPaymentInfo struct {
	IsPaid        bool    `json:"is_paid"`
	Amount        float64 `json:"amount,omitempty"`
	PaymentMethod string  `json:"payment_method,omitempty"`
	PaidAt        *string `json:"paid_at,omitempty"`
}

type TransactionPartyInfo struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	PhoneNumber string    `json:"phone_number"`
}

type TransactionDeliveryInfo struct {
	JobID          uuid.UUID `json:"job_id"`
	Status         string    `json:"status"`
	RiderAssigned  bool      `json:"rider_assigned"`
	PickupAddress  string    `json:"pickup_address"`
	PickupLat      float64   `json:"pickup_lat"`
	PickupLng      float64   `json:"pickup_lng"`
	DropoffAddress string    `json:"dropoff_address"`
	DropoffLat     float64   `json:"dropoff_lat"`
	DropoffLng     float64   `json:"dropoff_lng"`
	AcceptedAt     *string   `json:"accepted_at,omitempty"`
	PickedUpAt     *string   `json:"picked_up_at,omitempty"`
	DeliveredAt    *string   `json:"delivered_at,omitempty"`
}

// GetActiveDeliveries returns active deliveries for the authenticated user (as buyer or seller)
func (h *TransactionHandler) GetActiveDeliveries(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		util.Error(c, http.StatusUnauthorized, domain.CodeUnauthorized, "User not authenticated")
		return
	}

	var activeDeliveries []ActiveDeliveryResponse

	// Get payment requests where user is seller
	sellerPRs, _ := h.prService.GetBySellerID(c.Request.Context(), userID, 50, 0)
	for _, pr := range sellerPRs {
		if pr.DeliveryJobID == nil {
			continue
		}
		job, err := h.deliveryJobRepo.GetByID(c.Request.Context(), *pr.DeliveryJobID)
		if err != nil || !job.Status.IsActive() {
			continue
		}

		delivery := h.buildActiveDeliveryResponse(c, pr, job, "seller")
		activeDeliveries = append(activeDeliveries, delivery)
	}

	// Get payment requests where user is buyer
	buyerPRs, _ := h.prService.GetByBuyerID(c.Request.Context(), userID, 50, 0)
	for _, pr := range buyerPRs {
		if pr.DeliveryJobID == nil {
			continue
		}
		job, err := h.deliveryJobRepo.GetByID(c.Request.Context(), *pr.DeliveryJobID)
		if err != nil || !job.Status.IsActive() {
			continue
		}

		delivery := h.buildActiveDeliveryResponse(c, pr, job, "buyer")
		activeDeliveries = append(activeDeliveries, delivery)
	}

	util.Success(c, http.StatusOK, gin.H{
		"active_deliveries": activeDeliveries,
		"count":             len(activeDeliveries),
	})
}

func (h *TransactionHandler) buildActiveDeliveryResponse(
	c *gin.Context,
	pr *domain.PaymentRequest,
	job *domain.DeliveryJob,
	role string,
) ActiveDeliveryResponse {
	resp := ActiveDeliveryResponse{
		PaymentRequestID: pr.ID,
		DeliveryJobID:    job.ID,
		ViewerRole:       role,
		ItemName:         pr.ItemName,
		ItemDescription:  pr.ItemDescription,
		ItemPrice:        pr.ItemPrice,
		DeliveryStatus:   string(job.Status),
		RiderAssigned:    job.RiderID != nil,
		CreatedAt:        job.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	// Determine rider status text
	switch job.Status {
	case domain.DJStatusSearching:
		resp.RiderStatusText = "Searching for rider"
	case domain.DJStatusAccepted:
		resp.RiderStatusText = "Rider assigned"
	case domain.DJStatusEnRoutePickup:
		resp.RiderStatusText = "Rider en route to pickup"
	case domain.DJStatusPickedUp:
		resp.RiderStatusText = "Item picked up"
	case domain.DJStatusInTransit:
		resp.RiderStatusText = "In transit"
	default:
		resp.RiderStatusText = string(job.Status)
	}

	// Get counterparty info
	if role == "seller" && pr.BuyerID != nil {
		buyer, err := h.userRepo.GetByID(c.Request.Context(), *pr.BuyerID)
		if err == nil {
			resp.Counterparty = &ActiveDeliveryPartyInfo{
				Role:        "Buyer",
				Name:        buyer.FullName(),
				PhoneNumber: buyer.PhoneNumber,
				ImageURL:    buyer.ProfilePhotoURL,
			}
		}
	} else if role == "buyer" {
		seller, err := h.userRepo.GetByID(c.Request.Context(), pr.SellerID)
		if err == nil {
			resp.Counterparty = &ActiveDeliveryPartyInfo{
				Role:        "Seller",
				Name:        seller.FullName(),
				PhoneNumber: seller.PhoneNumber,
				ImageURL:    seller.ProfilePhotoURL,
			}
		}
	}

	// Add timestamps
	if job.AcceptedAt != nil {
		t := job.AcceptedAt.Format("2006-01-02T15:04:05Z07:00")
		resp.AcceptedAt = &t
	}
	if job.PickedUpAt != nil {
		t := job.PickedUpAt.Format("2006-01-02T15:04:05Z07:00")
		resp.PickedUpAt = &t
	}

	return resp
}

type ActiveDeliveryResponse struct {
	PaymentRequestID uuid.UUID `json:"payment_request_id"`
	DeliveryJobID    uuid.UUID `json:"delivery_job_id"`
	ViewerRole       string    `json:"viewer_role"` // "buyer" or "seller"
	ItemName         string    `json:"item_name"`
	ItemDescription  *string   `json:"item_description,omitempty"`
	ItemPrice        float64   `json:"item_price"`
	DeliveryStatus   string    `json:"delivery_status"`
	RiderStatusText  string    `json:"rider_status_text"`
	RiderAssigned    bool      `json:"rider_assigned"`
	Counterparty     *ActiveDeliveryPartyInfo `json:"counterparty"`
	CreatedAt        string    `json:"created_at"`
	AcceptedAt       *string   `json:"accepted_at,omitempty"`
	PickedUpAt       *string   `json:"picked_up_at,omitempty"`
}

type ActiveDeliveryPartyInfo struct {
	Role        string  `json:"role"` // "Buyer" or "Seller"
	Name        string  `json:"name"`
	PhoneNumber string  `json:"phone_number"`
	ImageURL    *string `json:"image_url,omitempty"`
}

// TransactionListItem represents a transaction in the list view
type TransactionListItem struct {
	PaymentRequestID    uuid.UUID  `json:"payment_request_id"` // Use for intra-state rider search
	Role                string     `json:"role"`               // "buyer" or "seller"
	ItemName            string     `json:"item_name"`
	ItemPrice           float64    `json:"item_price"`
	TotalAmount         *float64   `json:"total_amount,omitempty"`
	Status              string     `json:"status"`
	StatusText          string     `json:"status_text"` // Human readable status
	DeliveryType        string     `json:"delivery_type"`
	CounterpartyName    string     `json:"counterparty_name,omitempty"`
	CounterpartyRole    string     `json:"counterparty_role,omitempty"` // "Buyer" or "Seller"
	CreatedAt           string     `json:"created_at"`
	PaidAt              *string    `json:"paid_at,omitempty"`
	DeliveryJobID       *uuid.UUID `json:"delivery_job_id,omitempty"`       // For intra-state
	LogisticsDeliveryID *uuid.UUID `json:"logistics_delivery_id,omitempty"` // For inter-state mark shipped
	PickupAddress       *string    `json:"pickup_address,omitempty"`
	PickupLat           *float64   `json:"pickup_lat,omitempty"`
	PickupLng           *float64   `json:"pickup_lng,omitempty"`
	DropoffAddress      *string    `json:"dropoff_address,omitempty"`
	DropoffLat          *float64   `json:"dropoff_lat,omitempty"`
	DropoffLng          *float64   `json:"dropoff_lng,omitempty"`
}
