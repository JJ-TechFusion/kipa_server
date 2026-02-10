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
		tx.GET("/active-deliveries", h.GetActiveDeliveries)
		tx.GET("/:id", h.GetTransactionStatus)
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
				DropoffAddress: job.DropoffAddress,
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
	DropoffAddress string    `json:"dropoff_address"`
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
