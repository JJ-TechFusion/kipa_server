package domain

import (
	"time"

	"github.com/google/uuid"
)
type DriverOfferStatus string

const (
	DriverOfferStatusPending         DriverOfferStatus = "pending"
	DriverOfferStatusAccepted        DriverOfferStatus = "accepted"
	DriverOfferStatusRejected        DriverOfferStatus = "rejected"
	DriverOfferStatusCountered       DriverOfferStatus = "countered"
	DriverOfferStatusCounterAccepted DriverOfferStatus = "counter_accepted"
	DriverOfferStatusCounterRejected DriverOfferStatus = "counter_rejected"
	DriverOfferStatusExpired         DriverOfferStatus = "expired"
	DriverOfferStatusWithdrawn       DriverOfferStatus = "withdrawn"
)

func (s DriverOfferStatus) IsValid() bool {
	switch s {
	case DriverOfferStatusPending, DriverOfferStatusAccepted, DriverOfferStatusRejected,
		DriverOfferStatusCountered, DriverOfferStatusCounterAccepted, DriverOfferStatusCounterRejected,
		DriverOfferStatusExpired, DriverOfferStatusWithdrawn:
		return true
	}
	return false
}

func (s DriverOfferStatus) IsTerminal() bool {
	switch s {
	case DriverOfferStatusAccepted, DriverOfferStatusRejected,
		DriverOfferStatusCounterAccepted, DriverOfferStatusCounterRejected,
		DriverOfferStatusExpired, DriverOfferStatusWithdrawn:
		return true
	}
	return false
}
type DriverOffer struct {
	ID         uuid.UUID `db:"id" json:"id"`
	DeliveryID uuid.UUID `db:"delivery_id" json:"delivery_id"`
	DriverID   uuid.UUID `db:"driver_id" json:"driver_id"`
	OfferedAmount float64  `db:"offered_amount" json:"offered_amount"`
	CounterAmount *float64 `db:"counter_amount" json:"counter_amount,omitempty"`
	AgreedAmount  *float64 `db:"agreed_amount" json:"agreed_amount,omitempty"`
	Status DriverOfferStatus `db:"status" json:"status"`
	ExpiresAt        time.Time  `db:"expires_at" json:"expires_at"`
	CounterExpiresAt *time.Time `db:"counter_expires_at" json:"counter_expires_at,omitempty"`
	CounteredAt *time.Time `db:"countered_at" json:"countered_at,omitempty"`
	RespondedAt *time.Time `db:"responded_at" json:"responded_at,omitempty"`
	DriverNote *string `db:"driver_note" json:"driver_note,omitempty"`
	SellerNote *string `db:"seller_note" json:"seller_note,omitempty"`
	OfferCount int `db:"offer_count" json:"offer_count"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

const (
	OfferExpiryHours        = 24
	CounterOfferExpiryHours = 24
	MaxOfferAttempts        = 3
)
func (o *DriverOffer) IsExpired() bool {
	now := time.Now()

	if o.Status == DriverOfferStatusPending {
		return now.After(o.ExpiresAt)
	}

	if o.Status == DriverOfferStatusCountered && o.CounterExpiresAt != nil {
		return now.After(*o.CounterExpiresAt)
	}

	return false
}
func (o *DriverOffer) CanBeAccepted() bool {
	return o.Status == DriverOfferStatusPending && !o.IsExpired()
}
func (o *DriverOffer) CanBeRejected() bool {
	return o.Status == DriverOfferStatusPending && !o.IsExpired()
}
func (o *DriverOffer) CanBeCountered() bool {
	return o.Status == DriverOfferStatusPending && !o.IsExpired()
}
func (o *DriverOffer) CanBeWithdrawn() bool {
	return o.Status == DriverOfferStatusPending && !o.IsExpired()
}
func (o *DriverOffer) CanAcceptCounter() bool {
	if o.Status != DriverOfferStatusCountered {
		return false
	}
	if o.CounterExpiresAt != nil && time.Now().After(*o.CounterExpiresAt) {
		return false
	}
	return true
}
func (o *DriverOffer) CanRejectCounter() bool {
	return o.CanAcceptCounter()
}
func (o *DriverOffer) CanDriverReOffer() bool {
	if o.OfferCount >= MaxOfferAttempts {
		return false
	}
	return o.Status == DriverOfferStatusRejected ||
		o.Status == DriverOfferStatusExpired ||
		o.Status == DriverOfferStatusCounterRejected
}
