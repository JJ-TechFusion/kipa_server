package domain

import (
	"time"

	"github.com/google/uuid"
)

// LogisticsDeliveryStatus represents the state of an inter-state logistics delivery
type LogisticsDeliveryStatus string

const (
	LDStatusAwaitingShipment      LogisticsDeliveryStatus = "awaiting_shipment"
	LDStatusShipped               LogisticsDeliveryStatus = "shipped"
	LDStatusDeliveryClaimed       LogisticsDeliveryStatus = "delivery_claimed"
	LDStatusConfirmed             LogisticsDeliveryStatus = "confirmed"
	LDStatusPendingAdminApproval  LogisticsDeliveryStatus = "pending_admin_approval"
	LDStatusFundsReleased         LogisticsDeliveryStatus = "funds_released"
	LDStatusDisputed              LogisticsDeliveryStatus = "disputed"
	LDStatusReturnRequired        LogisticsDeliveryStatus = "return_required"
	LDStatusReturnShipped         LogisticsDeliveryStatus = "return_shipped"

	LDStatusReturnDamageDisputed  LogisticsDeliveryStatus = "return_damage_disputed" // Seller claims damage
	LDStatusPendingRefundApproval LogisticsDeliveryStatus = "pending_refund_approval" // Awaiting admin to approve refund after good return
	LDStatusReturnConfirmed       LogisticsDeliveryStatus = "return_confirmed"
	LDStatusRefunded              LogisticsDeliveryStatus = "refunded"
	LDStatusReleasedByAdmin       LogisticsDeliveryStatus = "released_by_admin"
	LDStatusCancelled             LogisticsDeliveryStatus = "cancelled"
)

// LogisticsCarrier represents known logistics companies
type LogisticsCarrier string

const (
	CarrierGIG        LogisticsCarrier = "gig_logistics"
	CarrierKwik       LogisticsCarrier = "kwik_delivery"
	CarrierABC        LogisticsCarrier = "abc_transport"
	CarrierPeaceMass  LogisticsCarrier = "peace_mass"
	CarrierGodIsGood  LogisticsCarrier = "god_is_good"
	CarrierDHL        LogisticsCarrier = "dhl"
	CarrierFedEx      LogisticsCarrier = "fedex"
	CarrierOther      LogisticsCarrier = "other"
)

func (c LogisticsCarrier) IsValid() bool {
	switch c {
	case CarrierGIG, CarrierKwik, CarrierABC, CarrierPeaceMass, CarrierGodIsGood, CarrierDHL, CarrierFedEx, CarrierOther:
		return true
	}
	return false
}

func (c LogisticsCarrier) DisplayName() string {
	names := map[LogisticsCarrier]string{
		CarrierGIG:       "GIG Logistics",
		CarrierKwik:      "Kwik Delivery",
		CarrierABC:       "ABC Transport",
		CarrierPeaceMass: "Peace Mass Transit",
		CarrierGodIsGood: "God Is Good Motors",
		CarrierDHL:       "DHL",
		CarrierFedEx:     "FedEx",
		CarrierOther:     "Other",
	}
	if name, ok := names[c]; ok {
		return name
	}
	return string(c)
}

// LogisticsDelivery represents an inter-state delivery using third-party logistics
type LogisticsDelivery struct {
	ID               uuid.UUID `db:"id" json:"id"`
	PaymentRequestID uuid.UUID `db:"payment_request_id" json:"payment_request_id"`
	EscrowID         uuid.UUID `db:"escrow_id" json:"escrow_id"`
	SellerID         uuid.UUID `db:"seller_id" json:"seller_id"`
	BuyerID          uuid.UUID `db:"buyer_id" json:"buyer_id"`

	// Logistics info
	Carrier             *LogisticsCarrier `db:"carrier" json:"carrier,omitempty"`
	CarrierOtherName    *string          `db:"carrier_other_name" json:"carrier_other_name,omitempty"`
	TrackingNumber      *string          `db:"tracking_number" json:"tracking_number,omitempty"`
	TrackingURL         *string          `db:"tracking_url" json:"tracking_url,omitempty"`
	ShipmentReceiptURL  *string          `db:"shipment_receipt_url" json:"shipment_receipt_url,omitempty"`
	EstimatedDeliveryAt *time.Time       `db:"estimated_delivery_at" json:"estimated_delivery_at,omitempty"`

	// Addresses (copied from payment request for record)
	PickupAddress  string `db:"pickup_address" json:"pickup_address"`
	PickupState    string `db:"pickup_state" json:"pickup_state"`
	DropoffAddress string `db:"dropoff_address" json:"dropoff_address"`
	DropoffState   string `db:"dropoff_state" json:"dropoff_state"`

	// Status
	Status LogisticsDeliveryStatus `db:"status" json:"status"`

	// Deadlines
	ShipDeadline              time.Time  `db:"ship_deadline" json:"ship_deadline"`
	ConfirmationDeadline      *time.Time `db:"confirmation_deadline" json:"confirmation_deadline,omitempty"`
	DisputeAutoEscalationAt   *time.Time `db:"dispute_auto_escalation_at" json:"dispute_auto_escalation_at,omitempty"`

	// High value flag
	RequiresAdminApproval bool `db:"requires_admin_approval" json:"requires_admin_approval"`

	// Return flow
	ReturnTrackingNumber  *string  `db:"return_tracking_number" json:"return_tracking_number,omitempty"`
	ReturnCarrier         *string  `db:"return_carrier" json:"return_carrier,omitempty"`
	ReturnCostBuyerShare  *float64 `db:"return_cost_buyer_share" json:"return_cost_buyer_share,omitempty"`
	ReturnCostSellerShare *float64 `db:"return_cost_seller_share" json:"return_cost_seller_share,omitempty"`

	// Return condition and damage claim
	ReturnCondition       *string    `db:"return_condition" json:"return_condition,omitempty"`
	DamageClaimReason     *string    `db:"damage_claim_reason" json:"damage_claim_reason,omitempty"`
	DamageClaimEvidenceURLs []string `db:"-" json:"damage_claim_evidence_urls,omitempty"` // Loaded from evidence table
	ReturnReceivedAt      *time.Time `db:"return_received_at" json:"return_received_at,omitempty"`

	// Dispute reference
	DisputeID         *uuid.UUID `db:"dispute_id" json:"dispute_id,omitempty"`
	DisputeOutcome    *string    `db:"dispute_outcome" json:"dispute_outcome,omitempty"` // buyer_wins, seller_wins, partial_refund

	// Reminder tracking
	LastReminderSentAt *time.Time `db:"last_reminder_sent_at" json:"last_reminder_sent_at,omitempty"`
	ReminderCount      int        `db:"reminder_count" json:"reminder_count"`

	// Timestamps
	ShippedAt            *time.Time `db:"shipped_at" json:"shipped_at,omitempty"`
	DeliveryClaimedAt    *time.Time `db:"delivery_claimed_at" json:"delivery_claimed_at,omitempty"`
	ConfirmedAt          *time.Time `db:"confirmed_at" json:"confirmed_at,omitempty"`
	ReleasedAt           *time.Time `db:"released_at" json:"released_at,omitempty"`
	CancelledAt          *time.Time `db:"cancelled_at" json:"cancelled_at,omitempty"`
	CreatedAt            time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt            time.Time  `db:"updated_at" json:"updated_at"`
}

// CanTransitionTo validates state transitions
func (s LogisticsDeliveryStatus) CanTransitionTo(next LogisticsDeliveryStatus) bool {
	transitions := map[LogisticsDeliveryStatus][]LogisticsDeliveryStatus{
		LDStatusAwaitingShipment: {
			LDStatusShipped,
			LDStatusCancelled,
		},
		LDStatusShipped: {
			LDStatusDeliveryClaimed,
			// Dispute only allowed after seller claims delivery
		},
		LDStatusDeliveryClaimed: {
			LDStatusConfirmed,
			LDStatusDisputed,
		},
		LDStatusConfirmed: {
			LDStatusFundsReleased,
			LDStatusPendingAdminApproval,
		},
		LDStatusPendingAdminApproval: {
			LDStatusFundsReleased,
			LDStatusDisputed,
		},
		LDStatusDisputed: {
			LDStatusFundsReleased,       // Seller wins
			LDStatusRefunded,            // Direct refund (rare)
			LDStatusReleasedByAdmin,     // Admin releases to seller
			LDStatusReturnRequired,      // Buyer wins → must return first
		},
		LDStatusReturnRequired: {
			LDStatusReturnShipped,
			LDStatusRefunded, // Admin can refund if return not possible
		},
	LDStatusReturnShipped: {
			LDStatusReturnConfirmed,      // Seller confirms good condition
			LDStatusReturnDamageDisputed, // Seller claims damage
		},
		LDStatusReturnConfirmed: {
			LDStatusPendingRefundApproval, // Awaiting admin approval
			LDStatusRefunded,              // Auto-refund for low value
		},
		LDStatusReturnDamageDisputed: {
			LDStatusRefunded,        // Admin decides full refund anyway
			LDStatusReturnConfirmed, // Admin decides partial refund
		},
		LDStatusPendingRefundApproval: {
			LDStatusRefunded, // Admin approves refund
		},
	}

	allowed, exists := transitions[s]
	if !exists {
		return false
	}

	for _, t := range allowed {
		if t == next {
			return true
		}
	}
	return false
}

// IsTerminal checks if the status is a final state
func (s LogisticsDeliveryStatus) IsTerminal() bool {
	switch s {
	case LDStatusFundsReleased, LDStatusRefunded, LDStatusReleasedByAdmin, LDStatusCancelled:
		return true
	}
	return false
}

// CanBeCancelled checks if the delivery can be cancelled
func (ld *LogisticsDelivery) CanBeCancelled() bool {
	return ld.Status == LDStatusAwaitingShipment
}

// CanBeShipped checks if seller can mark as shipped
func (ld *LogisticsDelivery) CanBeShipped() bool {
	return ld.Status == LDStatusAwaitingShipment
}

// CanClaimDelivery checks if seller can claim delivery
func (ld *LogisticsDelivery) CanClaimDelivery() bool {
	return ld.Status == LDStatusShipped
}

// CanConfirmDelivery checks if buyer can confirm
func (ld *LogisticsDelivery) CanConfirmDelivery() bool {
	return ld.Status == LDStatusDeliveryClaimed
}

// CanOpenDispute checks if buyer can open a dispute
// Buyer can only dispute after seller has claimed delivery
func (ld *LogisticsDelivery) CanOpenDispute() bool {
	return ld.Status == LDStatusDeliveryClaimed
}

// CanSellerOpenDispute checks if seller can open dispute (buyer unresponsive)
func (ld *LogisticsDelivery) CanSellerOpenDispute() bool {
	if ld.Status != LDStatusDeliveryClaimed {
		return false
	}
	if ld.DeliveryClaimedAt == nil {
		return false
	}
	// Seller can open dispute after 7 days of buyer non-response
	return time.Since(*ld.DeliveryClaimedAt) >= 7*24*time.Hour
}

// CanSellerConfirmReturn checks if seller can confirm return received
func (ld *LogisticsDelivery) CanSellerConfirmReturn() bool {
	return ld.Status == LDStatusReturnShipped
}

// IsInReturnFlow checks if delivery is in return flow
func (ld *LogisticsDelivery) IsInReturnFlow() bool {
	switch ld.Status {
	case LDStatusReturnRequired, LDStatusReturnShipped,
		LDStatusReturnDamageDisputed, LDStatusPendingRefundApproval, LDStatusReturnConfirmed:
		return true
	}
	return false
}

// HasDamageClaim checks if there's a pending damage claim
func (ld *LogisticsDelivery) HasDamageClaim() bool {
	return ld.Status == LDStatusReturnDamageDisputed
}

// IsShipDeadlineExceeded checks if ship deadline has passed
func (ld *LogisticsDelivery) IsShipDeadlineExceeded() bool {
	return ld.Status == LDStatusAwaitingShipment && time.Now().After(ld.ShipDeadline)
}

// ShouldAutoEscalate checks if this should be auto-escalated to admin
func (ld *LogisticsDelivery) ShouldAutoEscalate() bool {
	if ld.Status != LDStatusDeliveryClaimed {
		return false
	}
	if ld.DisputeAutoEscalationAt == nil {
		return false
	}
	return time.Now().After(*ld.DisputeAutoEscalationAt)
}

// LogisticsDeliveryEvent represents an audit log entry for a logistics delivery
type LogisticsDeliveryEvent struct {
	ID                   uuid.UUID                `db:"id" json:"id"`
	LogisticsDeliveryID  uuid.UUID                `db:"logistics_delivery_id" json:"logistics_delivery_id"`
	FromStatus           *LogisticsDeliveryStatus `db:"from_status" json:"from_status,omitempty"`
	ToStatus             LogisticsDeliveryStatus  `db:"to_status" json:"to_status"`
	ActorID              *uuid.UUID               `db:"actor_id" json:"actor_id,omitempty"`
	ActorType            string                   `db:"actor_type" json:"actor_type"` // buyer, seller, admin, system
	Reason               *string                  `db:"reason" json:"reason,omitempty"`
	Metadata             []byte                   `db:"metadata" json:"-"` // JSONB
	CreatedAt            time.Time                `db:"created_at" json:"created_at"`
}

// LogisticsEvidence represents uploaded evidence for a logistics delivery
type LogisticsEvidence struct {
	ID                  uuid.UUID  `db:"id" json:"id"`
	LogisticsDeliveryID uuid.UUID  `db:"logistics_delivery_id" json:"logistics_delivery_id"`
	UploadedBy          uuid.UUID  `db:"uploaded_by" json:"uploaded_by"`
	UploaderType        string     `db:"uploader_type" json:"uploader_type"` // buyer, seller
	EvidenceType        string     `db:"evidence_type" json:"evidence_type"` // shipment_receipt, tracking_screenshot, delivery_photo, damage_photo, unboxing_video
	FileURL             string     `db:"file_url" json:"file_url"`
	Description         *string    `db:"description" json:"description,omitempty"`
	UploadedAt          time.Time  `db:"uploaded_at" json:"uploaded_at"`
}

// LogisticsDeliveryDetails includes related entities for display
type LogisticsDeliveryDetails struct {
	Delivery       *LogisticsDelivery   `json:"delivery"`
	PaymentRequest *PaymentRequest      `json:"payment_request"`
	Escrow         *Escrow              `json:"escrow"`
	Seller         *User                `json:"seller"`
	Buyer          *User                `json:"buyer"`
	Events         []*LogisticsDeliveryEvent `json:"events,omitempty"`
	Evidence       []*LogisticsEvidence `json:"evidence,omitempty"`
	Dispute        *Dispute             `json:"dispute,omitempty"`
}
