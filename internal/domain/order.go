package domain

import (
	"time"

	"github.com/google/uuid"
)
type OrderPaymentStatus string

const (
	OrderPaymentPending     OrderPaymentStatus = "pending"
	OrderPaymentInitialized OrderPaymentStatus = "initialized"
	OrderPaymentVerified    OrderPaymentStatus = "verified"
	OrderPaymentFailed      OrderPaymentStatus = "failed"
)
type OrderStatus string

const (
	OrderStatusPending    OrderStatus = "pending"
	OrderStatusPaid       OrderStatus = "paid"
	OrderStatusProcessing OrderStatus = "processing"
	OrderStatusReady      OrderStatus = "ready"
	OrderStatusInDelivery OrderStatus = "in_delivery"
	OrderStatusDelivered  OrderStatus = "delivered"
	OrderStatusCompleted  OrderStatus = "completed"
	OrderStatusCancelled  OrderStatus = "cancelled"
	OrderStatusDisputed   OrderStatus = "disputed"
)
type PaymentMethod string

const (
	PaymentMethodCard   PaymentMethod = "card"
	PaymentMethodWallet PaymentMethod = "wallet"
)
type Order struct {
	ID               uuid.UUID `db:"id" json:"id"`
	PaymentRequestID uuid.UUID `db:"payment_request_id" json:"payment_request_id"`
	BuyerID          uuid.UUID `db:"buyer_id" json:"buyer_id"`
	EscrowID         *uuid.UUID `db:"escrow_id" json:"escrow_id,omitempty"`
	PaymentMethod       PaymentMethod      `db:"payment_method" json:"payment_method"`
	PaystackReference   *string            `db:"paystack_reference" json:"paystack_reference,omitempty"`
	PaystackAccessCode  *string            `db:"paystack_access_code" json:"paystack_access_code,omitempty"`
	PaymentStatus       OrderPaymentStatus `db:"payment_status" json:"payment_status"`
	ItemAmount          float64 `db:"item_amount" json:"item_amount"`
	DeliveryFeeEstimate float64 `db:"delivery_fee_estimate" json:"delivery_fee_estimate"`
	BuyerServiceFee     float64 `db:"buyer_service_fee" json:"buyer_service_fee"`
	TotalAmount         float64 `db:"total_amount" json:"total_amount"`
	Status OrderStatus `db:"status" json:"status"`
	CreatedAt time.Time  `db:"created_at" json:"created_at"`
	PaidAt    *time.Time `db:"paid_at" json:"paid_at,omitempty"`
	UpdatedAt time.Time  `db:"updated_at" json:"updated_at"`
}
func (s OrderStatus) CanTransitionTo(next OrderStatus) bool {
	transitions := map[OrderStatus][]OrderStatus{
		OrderStatusPending:    {OrderStatusPaid, OrderStatusCancelled},
		OrderStatusPaid:       {OrderStatusProcessing, OrderStatusReady, OrderStatusDisputed, OrderStatusCancelled},
		OrderStatusProcessing: {OrderStatusReady, OrderStatusDisputed, OrderStatusCancelled},
		OrderStatusReady:      {OrderStatusInDelivery, OrderStatusCompleted, OrderStatusDisputed}, // Completed for pickup type
		OrderStatusInDelivery: {OrderStatusDelivered, OrderStatusDisputed},
		OrderStatusDelivered:  {OrderStatusCompleted, OrderStatusDisputed},
		OrderStatusDisputed:   {OrderStatusCompleted, OrderStatusCancelled},
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
func (s OrderStatus) IsTerminal() bool {
	return s == OrderStatusCompleted || s == OrderStatusCancelled
}
func (o *Order) IsPaid() bool {
	return o.PaymentStatus == OrderPaymentVerified && o.Status != OrderStatusPending
}
func (o *Order) CanBeVerified() bool {
	return o.PaymentStatus == OrderPaymentInitialized
}
func GeneratePaystackReference(orderID uuid.UUID) string {
	return "kipa_ord_" + orderID.String()[:8]
}
