package dto

import (
	"time"

	"kipa/internal/domain"

	"github.com/google/uuid"
)

type MarkShippedRequest struct {
	Carrier             string  `json:"carrier" binding:"required"`
	CarrierOtherName    *string `json:"carrier_other_name" binding:"omitempty,max=255"`
	TrackingNumber      string  `json:"tracking_number" binding:"required,min=3,max=255"`
	TrackingURL         *string `json:"tracking_url" binding:"omitempty,url"`
	ShipmentReceiptURL  string  `json:"shipment_receipt_url" binding:"required,url"`
	EstimatedDeliveryAt *string `json:"estimated_delivery_at" binding:"omitempty"` // RFC3339 format
}

type ClaimDeliveryRequest struct {
	DeliveryProofURL *string `json:"delivery_proof_url" binding:"omitempty,url"`
	Notes            *string `json:"notes" binding:"omitempty,max=1000"`
}

type ConfirmDeliveryRequest struct {
	Rating *int    `json:"rating" binding:"omitempty,min=1,max=5"`
	Notes  *string `json:"notes" binding:"omitempty,max=1000"`
}

type OpenDisputeRequest struct {
	Reason      string   `json:"reason" binding:"required,min=10,max=2000"`
	EvidenceURLs []string `json:"evidence_urls" binding:"omitempty,max=10,dive,url"`
}

type UpdateTrackingRequest struct {
	TrackingNumber      *string `json:"tracking_number" binding:"omitempty,min=3,max=255"`
	TrackingURL         *string `json:"tracking_url" binding:"omitempty,url"`
	EstimatedDeliveryAt *string `json:"estimated_delivery_at" binding:"omitempty"`
}

type UploadEvidenceRequest struct {
	EvidenceType string  `json:"evidence_type" binding:"required,oneof=shipment_receipt tracking_screenshot delivery_photo package_photo damage_photo unboxing_video communication_screenshot other"`
	FileURL      string  `json:"file_url" binding:"required,url"`
	Description  *string `json:"description" binding:"omitempty,max=500"`
}

type AdminResolveDisputeRequest struct {
	Outcome         string   `json:"outcome" binding:"required,oneof=buyer_wins seller_wins partial_refund return_required"`
	ResolutionNotes string   `json:"resolution_notes" binding:"required,min=10,max=2000"`
	RefundAmount    *float64 `json:"refund_amount" binding:"omitempty,min=0"` // For partial refund
	ReturnCostBuyerShare  *float64 `json:"return_cost_buyer_share" binding:"omitempty,min=0,max=100"`
	ReturnCostSellerShare *float64 `json:"return_cost_seller_share" binding:"omitempty,min=0,max=100"`
}

type AdminApproveReleaseRequest struct {
	Approved bool    `json:"approved" binding:"required"`
	Notes    *string `json:"notes" binding:"omitempty,max=1000"`
}

type MarkReturnShippedRequest struct {
	ReturnCarrier        string `json:"return_carrier" binding:"required,max=255"`
	ReturnTrackingNumber string `json:"return_tracking_number" binding:"required,min=3,max=255"`
}

type ClaimReturnDeliveryRequest struct {
	DeliveryProofURL *string `json:"delivery_proof_url" binding:"omitempty,url"`
	Notes            *string `json:"notes" binding:"omitempty,max=1000"`
}

type ConfirmReturnReceivedRequest struct {
	Condition         string   `json:"condition" binding:"required,oneof=good damaged incomplete"`
	Notes             *string  `json:"notes" binding:"omitempty,max=1000"`
	DamageReason      *string  `json:"damage_reason" binding:"omitempty,max=2000"`      // Required if condition is damaged/incomplete
	DamageEvidenceURLs []string `json:"damage_evidence_urls" binding:"omitempty,max=10"` // Evidence photos/videos
}

type AdminResolveDamageClaimRequest struct {
	Decision        string   `json:"decision" binding:"required,oneof=full_refund partial_refund release_to_seller"`
	RefundAmount    *float64 `json:"refund_amount" binding:"omitempty,min=0"` // For partial refund
	ResolutionNotes string   `json:"resolution_notes" binding:"required,min=10,max=2000"`
}

type AdminApproveRefundRequest struct {
	Notes *string `json:"notes" binding:"omitempty,max=1000"`
}
type CancelDeliveryRequest struct {
	Reason string `json:"reason" binding:"required,min=5,max=500"`
}

type ListLogisticsDeliveriesRequest struct {
	Status *string `form:"status" binding:"omitempty"`
	Limit  int     `form:"limit" binding:"omitempty,min=1,max=100"`
	Offset int     `form:"offset" binding:"omitempty,min=0"`
}

type LogisticsDeliveryResponse struct {
	ID               uuid.UUID `json:"id"`
	PaymentRequestID uuid.UUID `json:"payment_request_id"`
	EscrowID         uuid.UUID `json:"escrow_id"`

	Carrier             string     `json:"carrier"`
	CarrierDisplayName  string     `json:"carrier_display_name"`
	CarrierOtherName    *string    `json:"carrier_other_name,omitempty"`
	TrackingNumber      *string    `json:"tracking_number,omitempty"`
	TrackingURL         *string    `json:"tracking_url,omitempty"`
	ShipmentReceiptURL  *string    `json:"shipment_receipt_url,omitempty"`
	EstimatedDeliveryAt *time.Time `json:"estimated_delivery_at,omitempty"`

	PickupAddress  string `json:"pickup_address"`
	PickupState    string `json:"pickup_state"`
	DropoffAddress string `json:"dropoff_address"`
	DropoffState   string `json:"dropoff_state"`

	Status        string `json:"status"`
	StatusDisplay string `json:"status_display"`

	ShipDeadline            time.Time  `json:"ship_deadline"`
	ConfirmationDeadline    *time.Time `json:"confirmation_deadline,omitempty"`
	IsShipDeadlineExceeded  bool       `json:"is_ship_deadline_exceeded"`

	RequiresAdminApproval bool `json:"requires_admin_approval"`

	ReturnTrackingNumber  *string  `json:"return_tracking_number,omitempty"`
	ReturnCarrier         *string  `json:"return_carrier,omitempty"`
	ReturnCostBuyerShare  *float64 `json:"return_cost_buyer_share,omitempty"`
	ReturnCostSellerShare *float64 `json:"return_cost_seller_share,omitempty"`

	ReturnCondition        *string    `json:"return_condition,omitempty"`
	DamageClaimReason      *string    `json:"damage_claim_reason,omitempty"`
	DamageClaimEvidenceURLs []string  `json:"damage_claim_evidence_urls,omitempty"`
	ReturnReceivedAt       *time.Time `json:"return_received_at,omitempty"`
	DisputeOutcome         *string    `json:"dispute_outcome,omitempty"`

	ItemName  string  `json:"item_name"`
	ItemPrice float64 `json:"item_price"`

	EscrowAmount float64 `json:"escrow_amount"`
	EscrowStatus string  `json:"escrow_status"`

	Seller ParticipantInfo `json:"seller"`
	Buyer  ParticipantInfo `json:"buyer"`

	AvailableActions []string `json:"available_actions"`

	ShippedAt         *time.Time `json:"shipped_at,omitempty"`
	DeliveryClaimedAt *time.Time `json:"delivery_claimed_at,omitempty"`
	ConfirmedAt       *time.Time `json:"confirmed_at,omitempty"`
	ReleasedAt        *time.Time `json:"released_at,omitempty"`
	CancelledAt       *time.Time `json:"cancelled_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type ParticipantInfo struct {
	ID          uuid.UUID `json:"id"`
	PhoneNumber string    `json:"phone_number"`
	FirstName   *string   `json:"first_name,omitempty"`
	LastName    *string   `json:"last_name,omitempty"`
}


type LogisticsDeliveryEventResponse struct {
	ID         uuid.UUID  `json:"id"`
	FromStatus *string    `json:"from_status,omitempty"`
	ToStatus   string     `json:"to_status"`
	ActorID    *uuid.UUID `json:"actor_id,omitempty"`
	ActorType  string     `json:"actor_type"`
	Reason     *string    `json:"reason,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type LogisticsEvidenceResponse struct {
	ID           uuid.UUID `json:"id"`
	UploadedBy   uuid.UUID `json:"uploaded_by"`
	UploaderType string    `json:"uploader_type"`
	EvidenceType string    `json:"evidence_type"`
	FileURL      string    `json:"file_url"`
	Description  *string   `json:"description,omitempty"`
	UploadedAt   time.Time `json:"uploaded_at"`
}

type LogisticsDeliveryDetailsResponse struct {
	Delivery LogisticsDeliveryResponse        `json:"delivery"`
	Events   []LogisticsDeliveryEventResponse `json:"events"`
	Evidence []LogisticsEvidenceResponse      `json:"evidence"`
	Dispute  *DisputeResponse                 `json:"dispute,omitempty"`
	Timeline *DeliveryTimelineResponse        `json:"timeline,omitempty"`
}


type DisputeResponse struct {
	ID              uuid.UUID  `json:"id"`
	Status          string     `json:"status"`
	Outcome         string     `json:"outcome"`
	Reason          string     `json:"reason"`
	ResolutionNotes *string    `json:"resolution_notes,omitempty"`
	ResolvedBy      *uuid.UUID `json:"resolved_by,omitempty"`
	ResolvedAt      *time.Time `json:"resolved_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

type LogisticsDeliveryListResponse struct {
	Deliveries []LogisticsDeliveryResponse `json:"deliveries"`
	Total      int                         `json:"total"`
	Limit      int                         `json:"limit"`
	Offset     int                         `json:"offset"`
}
func GetStatusDisplay(status domain.LogisticsDeliveryStatus) string {
	displays := map[domain.LogisticsDeliveryStatus]string{
		domain.LDStatusAwaitingShipment:      "Awaiting Shipment",
		domain.LDStatusShipped:               "Shipped",
		domain.LDStatusDeliveryClaimed:       "Delivery Claimed",
		domain.LDStatusConfirmed:             "Confirmed",
		domain.LDStatusPendingAdminApproval:  "Pending Admin Approval",
		domain.LDStatusFundsReleased:         "Funds Released",
		domain.LDStatusDisputed:              "Disputed",
		domain.LDStatusReturnRequired:        "Return Required",
		domain.LDStatusReturnShipped:          "Return Shipped",
		domain.LDStatusReturnDeliveryClaimed:  "Return Delivery Claimed",
		domain.LDStatusReturnDamageDisputed:   "Return Damage Disputed",
		domain.LDStatusPendingRefundApproval: "Pending Refund Approval",
		domain.LDStatusReturnConfirmed:       "Return Confirmed",
		domain.LDStatusRefunded:              "Refunded",
		domain.LDStatusReleasedByAdmin:       "Released by Admin",
		domain.LDStatusCancelled:             "Cancelled",
	}
	if display, ok := displays[status]; ok {
		return display
	}
	return string(status)
}
func GetAvailableActions(delivery *domain.LogisticsDelivery, userID uuid.UUID) []string {
	var actions []string

	isSeller := delivery.SellerID == userID
	isBuyer := delivery.BuyerID == userID

	switch delivery.Status {
	case domain.LDStatusAwaitingShipment:
		if isSeller {
			actions = append(actions, "mark_shipped")
		}
		if isBuyer && delivery.IsShipDeadlineExceeded() {
			actions = append(actions, "cancel", "request_refund")
		}
	case domain.LDStatusShipped:
		if isSeller {
			if delivery.CanClaimDelivery() {
				actions = append(actions, "claim_delivery")
			}
			actions = append(actions, "update_tracking")
		}
	case domain.LDStatusDeliveryClaimed:
		if isBuyer {
			actions = append(actions, "confirm_delivery", "open_dispute")
		}
		if isSeller && delivery.CanSellerOpenDispute() {
			actions = append(actions, "open_dispute")
		}
	case domain.LDStatusReturnRequired:
		if isBuyer {
			actions = append(actions, "mark_return_shipped")
		}
	case domain.LDStatusReturnShipped:
		if isSeller {
			actions = append(actions, "confirm_return_received")
		}
	case domain.LDStatusReturnConfirmed, domain.LDStatusPendingRefundApproval:
	case domain.LDStatusReturnDamageDisputed:
	}
	if !delivery.Status.IsTerminal() {
		actions = append(actions, "upload_evidence")
	}

	return actions
}
func ToLogisticsDeliveryResponse(
	delivery *domain.LogisticsDelivery,
	pr *domain.PaymentRequest,
	escrow *domain.Escrow,
	seller *domain.User,
	buyer *domain.User,
	viewerID uuid.UUID,
) LogisticsDeliveryResponse {
	resp := LogisticsDeliveryResponse{
		ID:               delivery.ID,
		PaymentRequestID: delivery.PaymentRequestID,
		EscrowID:         delivery.EscrowID,


		Carrier: func() string {
			if delivery.Carrier != nil {
				return string(*delivery.Carrier)
			}
			return ""
		}(),
		CarrierDisplayName: func() string {
			if delivery.Carrier != nil {
				return delivery.Carrier.DisplayName()
			}
			return ""
		}(),
		CarrierOtherName:    delivery.CarrierOtherName,
		TrackingNumber:      delivery.TrackingNumber,
		TrackingURL:         delivery.TrackingURL,
		ShipmentReceiptURL:  delivery.ShipmentReceiptURL,
		EstimatedDeliveryAt: delivery.EstimatedDeliveryAt,

		PickupAddress:  delivery.PickupAddress,
		PickupState:    delivery.PickupState,
		DropoffAddress: delivery.DropoffAddress,
		DropoffState:   delivery.DropoffState,

		Status:        string(delivery.Status),
		StatusDisplay: GetStatusDisplay(delivery.Status),

		ShipDeadline:            delivery.ShipDeadline,
		ConfirmationDeadline:    delivery.ConfirmationDeadline,
		IsShipDeadlineExceeded:  delivery.IsShipDeadlineExceeded(),

		RequiresAdminApproval: delivery.RequiresAdminApproval,

		ReturnTrackingNumber:  delivery.ReturnTrackingNumber,
		ReturnCarrier:         delivery.ReturnCarrier,
		ReturnCostBuyerShare:  delivery.ReturnCostBuyerShare,
		ReturnCostSellerShare: delivery.ReturnCostSellerShare,

		ReturnCondition:         delivery.ReturnCondition,
		DamageClaimReason:       delivery.DamageClaimReason,
		DamageClaimEvidenceURLs: delivery.DamageClaimEvidenceURLs,
		ReturnReceivedAt:        delivery.ReturnReceivedAt,
		DisputeOutcome:          delivery.DisputeOutcome,

		ShippedAt:         delivery.ShippedAt,
		DeliveryClaimedAt: delivery.DeliveryClaimedAt,
		ConfirmedAt:       delivery.ConfirmedAt,
		ReleasedAt:        delivery.ReleasedAt,
		CancelledAt:       delivery.CancelledAt,
		CreatedAt:         delivery.CreatedAt,
		UpdatedAt:         delivery.UpdatedAt,

		AvailableActions: GetAvailableActions(delivery, viewerID),
	}

	if pr != nil {
		resp.ItemName = pr.ItemName
		resp.ItemPrice = pr.ItemPrice
	}

	if escrow != nil {
		resp.EscrowAmount = escrow.TotalLocked
		resp.EscrowStatus = string(escrow.Status)
	}

	if seller != nil {
		resp.Seller = ParticipantInfo{
			ID:          seller.ID,
			PhoneNumber: seller.PhoneNumber,
			FirstName:   seller.FirstName,
			LastName:    seller.LastName,
		}
	}

	if buyer != nil {
		resp.Buyer = ParticipantInfo{
			ID:          buyer.ID,
			PhoneNumber: buyer.PhoneNumber,
			FirstName:   buyer.FirstName,
			LastName:    buyer.LastName,
		}
	}

	return resp
}
type TimelineStep struct {
	Status      string     `json:"status"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Timestamp   *time.Time `json:"timestamp,omitempty"`
	ActorType   string     `json:"actor_type,omitempty"` // buyer, seller, admin, system
	IsCompleted bool       `json:"is_completed"`
	IsCurrent   bool       `json:"is_current"`
	IsDispute   bool       `json:"is_dispute"` // True if this is a dispute-related step
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}
type DeliveryTimelineResponse struct {
	DeliveryID    uuid.UUID      `json:"delivery_id"`
	CurrentStatus string         `json:"current_status"`
	Steps         []TimelineStep `json:"steps"`
	HasDispute    bool           `json:"has_dispute"`
	DisputeSteps  []TimelineStep `json:"dispute_steps,omitempty"` // Separate list for dispute flow
}
func BuildTimeline(delivery *domain.LogisticsDelivery, events []*domain.LogisticsDeliveryEvent) *DeliveryTimelineResponse {
	timeline := &DeliveryTimelineResponse{
		DeliveryID:    delivery.ID,
		CurrentStatus: string(delivery.Status),
		Steps:         make([]TimelineStep, 0),
		HasDispute:    delivery.DisputeID != nil,
	}
	standardSteps := []struct {
		Status      domain.LogisticsDeliveryStatus
		Title       string
		Description string
	}{
		{domain.LDStatusAwaitingShipment, "Order Placed", "Waiting for seller to ship"},
		{domain.LDStatusShipped, "Shipped", "Item shipped via logistics carrier"},
		{domain.LDStatusDeliveryClaimed, "Delivery Claimed", "Seller claims item delivered"},
		{domain.LDStatusConfirmed, "Delivery Confirmed", "Buyer confirmed receipt"},
		{domain.LDStatusFundsReleased, "Payment Released", "Funds released to seller"},
	}
	eventByStatus := make(map[domain.LogisticsDeliveryStatus]*domain.LogisticsDeliveryEvent)
	for _, e := range events {
		eventByStatus[e.ToStatus] = e
	}
	for i, step := range standardSteps {
		event, hasEvent := eventByStatus[step.Status]
		isCompleted := hasEvent
		if step.Status == domain.LDStatusAwaitingShipment && len(events) > 0 {
			isCompleted = true
		}

		ts := TimelineStep{
			Status:      string(step.Status),
			Title:       step.Title,
			Description: step.Description,
			IsCompleted: isCompleted,
			IsCurrent:   step.Status == delivery.Status,
			IsDispute:   false,
		}

		if hasEvent {
			ts.Timestamp = &event.CreatedAt
			ts.ActorType = event.ActorType
		}
		if delivery.DisputeID != nil && i > 1 && !isCompleted && !ts.IsCurrent {
			continue
		}

		timeline.Steps = append(timeline.Steps, ts)
	}
	if delivery.DisputeID != nil {
		timeline.DisputeSteps = buildDisputeTimeline(delivery, events)
	}

	return timeline
}
func buildDisputeTimeline(delivery *domain.LogisticsDelivery, events []*domain.LogisticsDeliveryEvent) []TimelineStep {
	steps := make([]TimelineStep, 0)
	eventByStatus := make(map[domain.LogisticsDeliveryStatus]*domain.LogisticsDeliveryEvent)
	for _, e := range events {
		eventByStatus[e.ToStatus] = e
	}
	if event, ok := eventByStatus[domain.LDStatusDisputed]; ok {
		reason := ""
		if event.Reason != nil {
			reason = *event.Reason
		}
		steps = append(steps, TimelineStep{
			Status:      "disputed",
			Title:       "Dispute Opened",
			Description: reason,
			Timestamp:   &event.CreatedAt,
			ActorType:   event.ActorType,
			IsCompleted: true,
			IsDispute:   true,
		})
	}
	buyerWon := delivery.DisputeOutcome != nil &&
		(*delivery.DisputeOutcome == "buyer_favor" || *delivery.DisputeOutcome == "buyer_wins")
	isInReturnFlow := delivery.IsInReturnFlow()
	if buyerWon || isInReturnFlow {
		_, hasReturnShipped := eventByStatus[domain.LDStatusReturnShipped]
		if event, ok := eventByStatus[domain.LDStatusReturnRequired]; ok {
			isCompleted := hasReturnShipped || delivery.Status != domain.LDStatusReturnRequired
			steps = append(steps, TimelineStep{
				Status:      "return_required",
				Title:       "Return Required",
				Description: "Buyer must return item before refund",
				Timestamp:   &event.CreatedAt,
				ActorType:   "admin",
				IsCompleted: isCompleted,
				IsCurrent:   delivery.Status == domain.LDStatusReturnRequired,
				IsDispute:   true,
			})
		}
		_, hasReturnConfirmed := eventByStatus[domain.LDStatusReturnConfirmed]
		if event, ok := eventByStatus[domain.LDStatusReturnShipped]; ok {
			isCompleted := hasReturnConfirmed || delivery.Status == domain.LDStatusReturnDamageDisputed ||
				delivery.Status == domain.LDStatusRefunded
			steps = append(steps, TimelineStep{
				Status:      "return_shipped",
				Title:       "Return Shipped",
				Description: "Buyer shipped return",
				Timestamp:   &event.CreatedAt,
				ActorType:   "buyer",
				IsCompleted: isCompleted,
				IsCurrent:   delivery.Status == domain.LDStatusReturnShipped,
				IsDispute:   true,
				Metadata: map[string]interface{}{
					"carrier":         delivery.ReturnCarrier,
					"tracking_number": delivery.ReturnTrackingNumber,
				},
			})
		}
		if event, ok := eventByStatus[domain.LDStatusReturnDeliveryClaimed]; ok {
			isCompleted := hasReturnConfirmed || delivery.Status == domain.LDStatusReturnDamageDisputed ||
				delivery.Status == domain.LDStatusRefunded
			steps = append(steps, TimelineStep{
				Status:      "return_delivery_claimed",
				Title:       "Return Delivery Claimed",
				Description: "Buyer claims return delivered to seller",
				Timestamp:   &event.CreatedAt,
				ActorType:   "buyer",
				IsCompleted: isCompleted,
				IsCurrent:   delivery.Status == domain.LDStatusReturnDeliveryClaimed,
				IsDispute:   true,
				Metadata: map[string]interface{}{
					"confirmation_deadline": delivery.ReturnConfirmationDeadline,
				},
			})
		}
		if delivery.Status == domain.LDStatusReturnDamageDisputed {
			steps = append(steps, TimelineStep{
				Status:      "return_damage_disputed",
				Title:       "Damage Claimed",
				Description: "Seller claims return is damaged",
				Timestamp:   delivery.ReturnReceivedAt,
				ActorType:   "seller",
				IsCurrent:   true,
				IsDispute:   true,
				Metadata: map[string]interface{}{
					"damage_reason": delivery.DamageClaimReason,
				},
			})
		} else if event, ok := eventByStatus[domain.LDStatusReturnConfirmed]; ok {
			steps = append(steps, TimelineStep{
				Status:      "return_confirmed",
				Title:       "Return Received",
				Description: "Seller confirmed return in good condition",
				Timestamp:   &event.CreatedAt,
				ActorType:   "seller",
				IsCompleted: delivery.Status == domain.LDStatusRefunded || delivery.Status == domain.LDStatusPendingRefundApproval,
				IsCurrent:   delivery.Status == domain.LDStatusReturnConfirmed,
				IsDispute:   true,
			})
		}
		if event, ok := eventByStatus[domain.LDStatusRefunded]; ok {
			steps = append(steps, TimelineStep{
				Status:      "refunded",
				Title:       "Refund Processed",
				Description: "Buyer refunded",
				Timestamp:   &event.CreatedAt,
				ActorType:   "system",
				IsCompleted: true,
				IsDispute:   true,
			})
		}
	} else if delivery.DisputeOutcome != nil &&
		(*delivery.DisputeOutcome == "seller_favor" || *delivery.DisputeOutcome == "seller_wins") {
		if event, ok := eventByStatus[domain.LDStatusReleasedByAdmin]; ok {
			steps = append(steps, TimelineStep{
				Status:      "released_by_admin",
				Title:       "Dispute Resolved - Seller Wins",
				Description: "Admin released funds to seller",
				Timestamp:   &event.CreatedAt,
				ActorType:   "admin",
				IsCompleted: true,
				IsDispute:   true,
			})
		}
	}
	if len(steps) > 0 && !delivery.Status.IsTerminal() {
		for i := range steps {
			if !steps[i].IsCompleted {
				steps[i].IsCurrent = true
				break
			}
		}
	}

	return steps
}
