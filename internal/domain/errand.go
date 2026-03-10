package domain

import (
	"time"

	"github.com/google/uuid"
)
type ErrandStatus string

const (
	ErrandStatusDraft               ErrandStatus = "draft"
	ErrandStatusSearching           ErrandStatus = "searching"
	ErrandStatusAccepted            ErrandStatus = "accepted"
	ErrandStatusPickedUp            ErrandStatus = "picked_up"
	ErrandStatusInTransit           ErrandStatus = "in_transit"
	ErrandStatusDelivered           ErrandStatus = "delivered"
	ErrandStatusCompleted           ErrandStatus = "completed"
	ErrandStatusCancelled           ErrandStatus = "cancelled"
	ErrandStatusReceiverUnavailable ErrandStatus = "receiver_unavailable" // Receiver unavailable, settled outside app
)
type Errand struct {
	ID     uuid.UUID `db:"id" json:"id"`
	UserID uuid.UUID `db:"user_id" json:"user_id"`
	PickupAddress      string  `db:"pickup_address" json:"pickup_address"`
	PickupLat          float64 `db:"pickup_lat" json:"pickup_lat"`
	PickupLng          float64 `db:"pickup_lng" json:"pickup_lng"`
	PickupContactName  *string `db:"pickup_contact_name" json:"pickup_contact_name,omitempty"`
	PickupContactPhone *string `db:"pickup_contact_phone" json:"pickup_contact_phone,omitempty"`
	PickupInstructions *string `db:"pickup_instructions" json:"pickup_instructions,omitempty"`
	DropoffAddress      string  `db:"dropoff_address" json:"dropoff_address"`
	DropoffLat          float64 `db:"dropoff_lat" json:"dropoff_lat"`
	DropoffLng          float64 `db:"dropoff_lng" json:"dropoff_lng"`
	DropoffContactName  *string `db:"dropoff_contact_name" json:"dropoff_contact_name,omitempty"`
	DropoffContactPhone *string `db:"dropoff_contact_phone" json:"dropoff_contact_phone,omitempty"`
	PackageDescription *string  `db:"package_description" json:"package_description,omitempty"`
	PackageWeightKg    *float64 `db:"package_weight_kg" json:"package_weight_kg,omitempty"`
	PackagePhotoURL    *string  `db:"package_photo_url" json:"package_photo_url,omitempty"`
	VehicleType VehicleType `db:"vehicle_type" json:"vehicle_type"`
	DistanceKM  *float64    `db:"distance_km" json:"distance_km,omitempty"`
	DeliveryFee float64     `db:"delivery_fee" json:"delivery_fee"`
	Status      ErrandStatus `db:"status" json:"status"`
	DropoffCode *string      `db:"dropoff_code" json:"dropoff_code,omitempty"`
	RiderID       *uuid.UUID `db:"rider_id" json:"rider_id,omitempty"`
	DeliveryJobID *uuid.UUID `db:"delivery_job_id" json:"delivery_job_id,omitempty"`
	ConfirmedAt *time.Time `db:"confirmed_at" json:"confirmed_at,omitempty"`
	AcceptedAt  *time.Time `db:"accepted_at" json:"accepted_at,omitempty"`
	PickedUpAt  *time.Time `db:"picked_up_at" json:"picked_up_at,omitempty"`
	DeliveredAt *time.Time `db:"delivered_at" json:"delivered_at,omitempty"`
	CompletedAt *time.Time `db:"completed_at" json:"completed_at,omitempty"`
	CancelledAt *time.Time `db:"cancelled_at" json:"cancelled_at,omitempty"`
	CreatedAt   time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at" json:"updated_at"`
}
func (e *Errand) CanConfirm() bool {
	return e.Status == ErrandStatusDraft
}
func (e *Errand) CanCancel() bool {
	return e.Status == ErrandStatusDraft ||
		e.Status == ErrandStatusSearching ||
		e.Status == ErrandStatusAccepted
}
func (e *Errand) IsActive() bool {
	return e.Status == ErrandStatusSearching ||
		e.Status == ErrandStatusAccepted ||
		e.Status == ErrandStatusPickedUp ||
		e.Status == ErrandStatusInTransit
}
func (e *Errand) CanComplete() bool {
	return e.Status == ErrandStatusDelivered
}
const (
	DeliverySourcePaymentRequest = "payment_request"
	DeliverySourceErrand         = "errand"
)
