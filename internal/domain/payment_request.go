package domain

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)
type DeliveryType string

const (
	DeliveryTypeIntraState DeliveryType = "intra_state"
	DeliveryTypeInterState DeliveryType = "inter_state"
	DeliveryTypePickup     DeliveryType = "pickup"
)
type PaymentRequestStatus string

const (
	PRStatusDraft                   PaymentRequestStatus = "draft"
	PRStatusLinkActive              PaymentRequestStatus = "link_active"
	PRStatusPaidAwaitingFulfillment PaymentRequestStatus = "paid_awaiting_fulfillment"
	PRStatusReadyForPickup          PaymentRequestStatus = "ready_for_pickup"
	PRStatusSearchingRider          PaymentRequestStatus = "searching_rider"
	PRStatusRiderAssigned           PaymentRequestStatus = "rider_assigned"
	PRStatusInDelivery              PaymentRequestStatus = "in_delivery"
	PRStatusDelivered               PaymentRequestStatus = "delivered"
	PRStatusConfirmationWindow      PaymentRequestStatus = "confirmation_window"
	PRStatusCompleted               PaymentRequestStatus = "completed"
	PRStatusNoRiderFound            PaymentRequestStatus = "no_rider_found"
	PRStatusExpired                 PaymentRequestStatus = "expired"
	PRStatusCancelled               PaymentRequestStatus = "cancelled"
	PRStatusDisputed                PaymentRequestStatus = "disputed"
	PRStatusRefunded                PaymentRequestStatus = "refunded"
)
type PaymentRequest struct {
	ID       uuid.UUID `db:"id" json:"id"`
	SellerID uuid.UUID `db:"seller_id" json:"seller_id"`
	ItemName            string  `db:"item_name" json:"item_name"`
	ItemDescription     *string `db:"item_description" json:"item_description,omitempty"`
	ItemPrice           float64 `db:"item_price" json:"item_price"`
	ItemImages          []byte  `db:"item_images" json:"-"` // JSONB stored as []byte
	ProcessingTimeHours *int    `db:"processing_time_hours" json:"processing_time_hours,omitempty"`
	DeliveryType    DeliveryType `db:"delivery_type" json:"delivery_type"`
	VehicleType     *VehicleType `db:"vehicle_type" json:"vehicle_type,omitempty"`
	PickupAddress   *string      `db:"pickup_address" json:"pickup_address,omitempty"`
	PickupLocation  []byte       `db:"pickup_location" json:"-"` // PostGIS GEOGRAPHY
	DropoffAddress  *string      `db:"dropoff_address" json:"dropoff_address,omitempty"`
	DropoffLocation []byte       `db:"dropoff_location" json:"-"` // PostGIS GEOGRAPHY
	EstimatedDeliveryFee *float64 `db:"estimated_delivery_fee" json:"estimated_delivery_fee,omitempty"`
	BuyerServiceFee      *float64 `db:"buyer_service_fee" json:"buyer_service_fee,omitempty"`
	SellerPlatformFee    *float64 `db:"seller_platform_fee" json:"seller_platform_fee,omitempty"`
	EstimatedTotal       *float64 `db:"estimated_total" json:"estimated_total,omitempty"`
	ActualDeliveryFee *float64 `db:"actual_delivery_fee" json:"actual_delivery_fee,omitempty"`
	ActualTotal       *float64 `db:"actual_total" json:"actual_total,omitempty"`
	PaymentCode   *string    `db:"payment_code" json:"payment_code,omitempty"`
	LinkExpiresAt *time.Time `db:"link_expires_at" json:"link_expires_at,omitempty"`
	IsReusable   bool `db:"is_reusable" json:"is_reusable"`           // If true, link can be used by multiple buyers
	MaxUses      *int `db:"max_uses" json:"max_uses,omitempty"`       // Max number of times link can be used (null = unlimited if reusable)
	CurrentUses  int  `db:"current_uses" json:"current_uses"`         // Current number of successful payments
	LogisticsCarrier *string `db:"logistics_carrier" json:"logistics_carrier,omitempty"`
	TrackingNumber   *string `db:"tracking_number" json:"tracking_number,omitempty"`
	TrackingURL      *string `db:"tracking_url" json:"tracking_url,omitempty"`
	Status PaymentRequestStatus `db:"status" json:"status"`
	EscrowID      *uuid.UUID `db:"escrow_id" json:"escrow_id,omitempty"`
	DeliveryJobID *uuid.UUID `db:"delivery_job_id" json:"delivery_job_id,omitempty"`
	BuyerID       *uuid.UUID `db:"buyer_id" json:"buyer_id,omitempty"`
	LinkGeneratedAt  *time.Time `db:"link_generated_at" json:"link_generated_at,omitempty"`
	PaidAt           *time.Time `db:"paid_at" json:"paid_at,omitempty"`
	ReadyForPickupAt *time.Time `db:"ready_for_pickup_at" json:"ready_for_pickup_at,omitempty"`
	DeliveredAt      *time.Time `db:"delivered_at" json:"delivered_at,omitempty"`
	CompletedAt      *time.Time `db:"completed_at" json:"completed_at,omitempty"`
	CreatedAt        time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt        time.Time  `db:"updated_at" json:"updated_at"`
	LastReminderSentAt *time.Time `db:"last_reminder_sent_at" json:"last_reminder_sent_at,omitempty"`
	ReminderCount      int        `db:"reminder_count" json:"reminder_count"`
}
type PaymentRequestEvent struct {
	ID               uuid.UUID             `db:"id" json:"id"`
	PaymentRequestID uuid.UUID             `db:"payment_request_id" json:"payment_request_id"`
	FromStatus       *PaymentRequestStatus `db:"from_status" json:"from_status,omitempty"`
	ToStatus         PaymentRequestStatus  `db:"to_status" json:"to_status"`
	ActorID          *uuid.UUID            `db:"actor_id" json:"actor_id,omitempty"`
	ActorType        *string               `db:"actor_type" json:"actor_type,omitempty"`
	Reason           *string               `db:"reason" json:"reason,omitempty"`
	Metadata         []byte                `db:"metadata" json:"-"` // JSONB
	CreatedAt        time.Time             `db:"created_at" json:"created_at"`
}
func (s PaymentRequestStatus) CanTransitionTo(next PaymentRequestStatus) bool {
	transitions := map[PaymentRequestStatus][]PaymentRequestStatus{
		PRStatusDraft: {PRStatusLinkActive, PRStatusCancelled},
		PRStatusLinkActive: {PRStatusPaidAwaitingFulfillment, PRStatusExpired, PRStatusCancelled},
		PRStatusPaidAwaitingFulfillment: {PRStatusReadyForPickup, PRStatusSearchingRider, PRStatusDisputed, PRStatusRefunded},
		PRStatusReadyForPickup: {PRStatusSearchingRider, PRStatusInDelivery, PRStatusDelivered, PRStatusDisputed},
		PRStatusSearchingRider: {PRStatusRiderAssigned, PRStatusNoRiderFound},
		PRStatusRiderAssigned: {PRStatusInDelivery, PRStatusCancelled},
		PRStatusNoRiderFound: {PRStatusSearchingRider, PRStatusRefunded}, // Can retry or refund
		PRStatusInDelivery: {PRStatusDelivered, PRStatusDisputed},
		PRStatusDelivered: {PRStatusConfirmationWindow},
		PRStatusConfirmationWindow: {PRStatusCompleted, PRStatusDisputed},
		PRStatusDisputed: {PRStatusCompleted, PRStatusRefunded},
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
func (s PaymentRequestStatus) IsTerminal() bool {
	return s == PRStatusCompleted || s == PRStatusRefunded || s == PRStatusExpired || s == PRStatusCancelled
}
func (s PaymentRequestStatus) IsPaid() bool {
	paidStatuses := []PaymentRequestStatus{
		PRStatusPaidAwaitingFulfillment,
		PRStatusReadyForPickup,
		PRStatusSearchingRider,
		PRStatusRiderAssigned,
		PRStatusInDelivery,
		PRStatusDelivered,
		PRStatusConfirmationWindow,
		PRStatusCompleted,
		PRStatusDisputed,
	}
	for _, ps := range paidStatuses {
		if s == ps {
			return true
		}
	}
	return false
}
type NullPaymentRequestStatus struct {
	Status PaymentRequestStatus
	Valid  bool
}

func (n *NullPaymentRequestStatus) Scan(value interface{}) error {
	if value == nil {
		n.Status, n.Valid = "", false
		return nil
	}
	n.Valid = true
	return (*sql.NullString)(&struct {
		String string
		Valid  bool
	}{string(n.Status), n.Valid}).Scan(value)
}
