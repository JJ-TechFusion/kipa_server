package domain

import (
	"time"

	"github.com/google/uuid"
)

// DeliveryJobStatus represents the lifecycle of a delivery job
type DeliveryJobStatus string

const (
	DJStatusCreated       DeliveryJobStatus = "created"
	DJStatusSearching     DeliveryJobStatus = "searching"
	DJStatusAccepted      DeliveryJobStatus = "accepted"
	DJStatusEnRoutePickup DeliveryJobStatus = "en_route_pickup"
	DJStatusPickedUp      DeliveryJobStatus = "picked_up"
	DJStatusInTransit     DeliveryJobStatus = "in_transit"
	DJStatusDelivered     DeliveryJobStatus = "delivered"
	DJStatusPaidOut       DeliveryJobStatus = "paid_out"
	DJStatusTimeout       DeliveryJobStatus = "timeout"
	DJStatusCancelled     DeliveryJobStatus = "cancelled"
	DJStatusFailed        DeliveryJobStatus = "failed"
)

// RiderOfferResponse represents a rider's response to a job offer
type RiderOfferResponse string

const (
	RiderOfferPending  RiderOfferResponse = "pending"
	RiderOfferAccepted RiderOfferResponse = "accepted"
	RiderOfferDeclined RiderOfferResponse = "declined"
	RiderOfferExpired  RiderOfferResponse = "expired"
)

// VehicleType represents the type of vehicle for delivery
type VehicleType string

const (
	VehicleTypeMotorcycle VehicleType = "motorcycle"
	VehicleTypeCar        VehicleType = "car"
)

// IsValid checks if the vehicle type is valid
func (v VehicleType) IsValid() bool {
	return v == VehicleTypeMotorcycle || v == VehicleTypeCar
}

// DeliveryJob represents an intra-state delivery assignment
type DeliveryJob struct {
	ID               uuid.UUID  `db:"id" json:"id"`
	PaymentRequestID uuid.UUID  `db:"payment_request_id" json:"payment_request_id"`
	RiderID          *uuid.UUID `db:"rider_id" json:"rider_id,omitempty"`

	// Source tracking (payment_request or errand)
	SourceType *string    `db:"source_type" json:"source_type,omitempty"`
	ErrandID   *uuid.UUID `db:"errand_id" json:"errand_id,omitempty"`

	// Locations (copied from payment request for immutability)
	PickupAddress   string  `db:"pickup_address" json:"pickup_address"`
	PickupLat       float64 `db:"-" json:"pickup_lat,omitempty"` // Transient for creation
	PickupLng       float64 `db:"-" json:"pickup_lng,omitempty"` // Transient for creation
	PickupLocation  []byte  `db:"pickup_location" json:"-"`      // PostGIS GEOGRAPHY
	DropoffAddress  string  `db:"dropoff_address" json:"dropoff_address"`
	DropoffLat      float64 `db:"-" json:"dropoff_lat,omitempty"` // Transient for creation
	DropoffLng      float64 `db:"-" json:"dropoff_lng,omitempty"` // Transient for creation
	DropoffLocation []byte  `db:"dropoff_location" json:"-"`      // PostGIS GEOGRAPHY
	DistanceKM      *float64 `db:"distance_km" json:"distance_km,omitempty"`

	// Required vehicle type (selected by seller)
	RequiredVehicleType VehicleType `db:"required_vehicle_type" json:"required_vehicle_type"`

	// Fees (locked when rider accepts)
	DeliveryFee         float64  `db:"delivery_fee" json:"delivery_fee"`
	RiderPayout         *float64 `db:"rider_payout" json:"rider_payout,omitempty"`
	PlatformDeliveryFee *float64 `db:"platform_delivery_fee" json:"platform_delivery_fee,omitempty"`

	// Verification codes
	PickupCode  *string `db:"pickup_code" json:"pickup_code,omitempty"`
	DropoffCode *string `db:"dropoff_code" json:"dropoff_code,omitempty"`

	// Search tracking
	SearchRadiusKM      int        `db:"search_radius_km" json:"search_radius_km"`
	SearchStartedAt     *time.Time `db:"search_started_at" json:"search_started_at,omitempty"`
	SearchTimeoutAt     *time.Time `db:"search_timeout_at" json:"search_timeout_at,omitempty"`
	LastRadiusExpandAt  *time.Time `db:"last_radius_expand_at" json:"last_radius_expand_at,omitempty"`

	// Status
	Status DeliveryJobStatus `db:"status" json:"status"`

	// ETA tracking
	CurrentETAMinutes *int       `db:"current_eta_minutes" json:"current_eta_minutes,omitempty"`
	ETAUpdatedAt      *time.Time `db:"eta_updated_at" json:"eta_updated_at,omitempty"`

	// Timestamps
	AcceptedAt  *time.Time `db:"accepted_at" json:"accepted_at,omitempty"`
	PickedUpAt  *time.Time `db:"picked_up_at" json:"picked_up_at,omitempty"`
	DeliveredAt *time.Time `db:"delivered_at" json:"delivered_at,omitempty"`
	CreatedAt   time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at" json:"updated_at"`
}

// RiderJobOffer tracks which riders were offered a job
type RiderJobOffer struct {
	ID            uuid.UUID          `db:"id" json:"id"`
	DeliveryJobID uuid.UUID          `db:"delivery_job_id" json:"delivery_job_id"`
	RiderID       uuid.UUID          `db:"rider_id" json:"rider_id"`
	DistanceKM    *float64           `db:"distance_km" json:"distance_km,omitempty"`
	OfferedAt     time.Time          `db:"offered_at" json:"offered_at"`
	Response      RiderOfferResponse `db:"response" json:"response"`
	RespondedAt   *time.Time         `db:"responded_at" json:"responded_at,omitempty"`
}

// CanTransitionTo checks if a status transition is valid
func (s DeliveryJobStatus) CanTransitionTo(next DeliveryJobStatus) bool {
	transitions := map[DeliveryJobStatus][]DeliveryJobStatus{
		DJStatusCreated:       {DJStatusSearching, DJStatusCancelled},
		DJStatusSearching:     {DJStatusAccepted, DJStatusTimeout},
		DJStatusAccepted:      {DJStatusEnRoutePickup, DJStatusCancelled},
		DJStatusEnRoutePickup: {DJStatusPickedUp, DJStatusCancelled, DJStatusFailed},
		DJStatusPickedUp:      {DJStatusInTransit},
		DJStatusInTransit:     {DJStatusDelivered, DJStatusFailed},
		DJStatusDelivered:     {DJStatusPaidOut},
		DJStatusTimeout:       {DJStatusSearching}, // Can retry search
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

// IsTerminal returns true if the status is a final state
func (s DeliveryJobStatus) IsTerminal() bool {
	return s == DJStatusPaidOut || s == DJStatusCancelled || s == DJStatusFailed
}

// IsActive returns true if the job is in an active state
func (s DeliveryJobStatus) IsActive() bool {
	activeStatuses := []DeliveryJobStatus{
		DJStatusSearching,
		DJStatusAccepted,
		DJStatusEnRoutePickup,
		DJStatusPickedUp,
		DJStatusInTransit,
	}
	for _, as := range activeStatuses {
		if s == as {
			return true
		}
	}
	return false
}

// HasRider returns true if a rider has been assigned
func (j *DeliveryJob) HasRider() bool {
	return j.RiderID != nil
}

// CanBeAccepted returns true if the job can still be accepted
func (j *DeliveryJob) CanBeAccepted() bool {
	return j.Status == DJStatusSearching && j.RiderID == nil
}
