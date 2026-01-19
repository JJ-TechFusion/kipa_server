package domain

import (
	"time"

	"github.com/google/uuid"
)
type EscrowStatus string

const (
	EscrowStatusCreated            EscrowStatus = "created"
	EscrowStatusFunded             EscrowStatus = "funded"
	EscrowStatusDeliveryInProgress EscrowStatus = "delivery_in_progress"
	EscrowStatusDelivered          EscrowStatus = "delivered"
	EscrowStatusConfirmationWindow EscrowStatus = "confirmation_window"
	EscrowStatusReleased           EscrowStatus = "released"
	EscrowStatusDisputed           EscrowStatus = "disputed"
	EscrowStatusRefunded           EscrowStatus = "refunded"
	EscrowStatusPartialRefund      EscrowStatus = "partial_refund"
)
type Escrow struct {
	ID               uuid.UUID  `db:"id" json:"id"`
	PaymentRequestID uuid.UUID  `db:"payment_request_id" json:"payment_request_id"`
	OrderID          *uuid.UUID `db:"order_id" json:"order_id,omitempty"`
	BuyerID          uuid.UUID  `db:"buyer_id" json:"buyer_id"`
	SellerID         uuid.UUID  `db:"seller_id" json:"seller_id"`
	ItemAmount          float64 `db:"item_amount" json:"item_amount"`
	DeliveryFeeEstimate float64 `db:"delivery_fee_estimate" json:"delivery_fee_estimate"`
	BuyerFee            float64 `db:"buyer_fee" json:"buyer_fee"`
	TotalLocked         float64 `db:"total_locked" json:"total_locked"`
	ActualDeliveryFee     *float64 `db:"actual_delivery_fee" json:"actual_delivery_fee,omitempty"`
	DeliveryFeeAdjustment *float64 `db:"delivery_fee_adjustment" json:"delivery_fee_adjustment,omitempty"`
	SellerPayout    *float64 `db:"seller_payout" json:"seller_payout,omitempty"`
	RiderPayout     *float64 `db:"rider_payout" json:"rider_payout,omitempty"`
	PlatformRevenue *float64 `db:"platform_revenue" json:"platform_revenue,omitempty"`
	PaymentReference *string `db:"payment_reference" json:"payment_reference,omitempty"`
	PaymentProvider  *string `db:"payment_provider" json:"payment_provider,omitempty"`
	Status EscrowStatus `db:"status" json:"status"`
	FundedAt                 *time.Time `db:"funded_at" json:"funded_at,omitempty"`
	DeliveryConfirmedAt      *time.Time `db:"delivery_confirmed_at" json:"delivery_confirmed_at,omitempty"`
	ConfirmationWindowEndsAt *time.Time `db:"confirmation_window_ends_at" json:"confirmation_window_ends_at,omitempty"`
	ReleasedAt               *time.Time `db:"released_at" json:"released_at,omitempty"`
	CreatedAt                time.Time  `db:"created_at" json:"created_at"`
}
func (s EscrowStatus) CanTransitionTo(next EscrowStatus) bool {
	transitions := map[EscrowStatus][]EscrowStatus{
		EscrowStatusCreated:            {EscrowStatusFunded},
		EscrowStatusFunded:             {EscrowStatusDeliveryInProgress, EscrowStatusDelivered, EscrowStatusDisputed, EscrowStatusRefunded},
		EscrowStatusDeliveryInProgress: {EscrowStatusDelivered, EscrowStatusDisputed},
		EscrowStatusDelivered:          {EscrowStatusConfirmationWindow},
		EscrowStatusConfirmationWindow: {EscrowStatusReleased, EscrowStatusDisputed},
		EscrowStatusDisputed:           {EscrowStatusReleased, EscrowStatusRefunded, EscrowStatusPartialRefund},
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
func (s EscrowStatus) IsTerminal() bool {
	return s == EscrowStatusReleased || s == EscrowStatusRefunded || s == EscrowStatusPartialRefund
}
func (s EscrowStatus) IsFunded() bool {
	fundedStatuses := []EscrowStatus{
		EscrowStatusFunded,
		EscrowStatusDeliveryInProgress,
		EscrowStatusDelivered,
		EscrowStatusConfirmationWindow,
		EscrowStatusReleased,
		EscrowStatusDisputed,
	}
	for _, fs := range fundedStatuses {
		if s == fs {
			return true
		}
	}
	return false
}
func (e *Escrow) CanBeReleased() bool {
	return e.Status == EscrowStatusConfirmationWindow
}
func (e *Escrow) CanBeDisputed() bool {
	disputeableStatuses := []EscrowStatus{
		EscrowStatusFunded,
		EscrowStatusDeliveryInProgress,
		EscrowStatusDelivered,
		EscrowStatusConfirmationWindow,
	}
	for _, ds := range disputeableStatuses {
		if e.Status == ds {
			return true
		}
	}
	return false
}
func (e *Escrow) CalculatePayouts(sellerFeePercent, riderFeePercent float64) {
	sellerFee := e.ItemAmount * sellerFeePercent
	sellerPayout := e.ItemAmount - sellerFee
	e.SellerPayout = &sellerPayout
	deliveryFee := e.DeliveryFeeEstimate
	if e.ActualDeliveryFee != nil {
		deliveryFee = *e.ActualDeliveryFee
	}
	platformDeliveryFee := deliveryFee * riderFeePercent
	riderPayout := deliveryFee - platformDeliveryFee
	e.RiderPayout = &riderPayout
	platformRevenue := e.BuyerFee + sellerFee + platformDeliveryFee
	e.PlatformRevenue = &platformRevenue
}
