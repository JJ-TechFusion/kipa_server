package domain

import (
	"time"

	"github.com/google/uuid"
)
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

	// Return delivery statuses (for return jobs after dispute)
	DJStatusReturnDelivered       DeliveryJobStatus = "return_delivered"        // Rider delivered return to seller
	DJStatusReturnConfirmed       DeliveryJobStatus = "return_confirmed"        // Seller confirmed good condition
	DJStatusReturnDamageDisputed  DeliveryJobStatus = "return_damage_disputed"  // Seller claims damage
	DJStatusReturnRefunded        DeliveryJobStatus = "return_refunded"         // Buyer refunded after return

	// Unavailability statuses
	DJStatusSellerUnavailable DeliveryJobStatus = "seller_unavailable" // Seller not available at pickup
	DJStatusBuyerUnavailable  DeliveryJobStatus = "buyer_unavailable"  // Buyer not available at dropoff
	DJStatusForcedReturn      DeliveryJobStatus = "forced_return"      // Forced return in progress
	DJStatusForcedReturnDone  DeliveryJobStatus = "forced_return_done" // Forced return completed
)

// ReturnCondition represents the condition of returned items
type ReturnCondition string

const (
	ReturnConditionGood       ReturnCondition = "good"
	ReturnConditionDamaged    ReturnCondition = "damaged"
	ReturnConditionIncomplete ReturnCondition = "incomplete"
)
type RiderOfferResponse string

const (
	RiderOfferPending  RiderOfferResponse = "pending"
	RiderOfferAccepted RiderOfferResponse = "accepted"
	RiderOfferDeclined RiderOfferResponse = "declined"
	RiderOfferExpired  RiderOfferResponse = "expired"
)
type VehicleType string

const (
	VehicleTypeMotorcycle VehicleType = "motorcycle"
	VehicleTypeCar        VehicleType = "car"
)
func (v VehicleType) IsValid() bool {
	return v == VehicleTypeMotorcycle || v == VehicleTypeCar
}
type DeliveryJob struct {
	ID               uuid.UUID  `db:"id" json:"id"`
	PaymentRequestID uuid.UUID  `db:"payment_request_id" json:"payment_request_id"`
	RiderID          *uuid.UUID `db:"rider_id" json:"rider_id,omitempty"`
	SourceType *string    `db:"source_type" json:"source_type,omitempty"`
	ErrandID   *uuid.UUID `db:"errand_id" json:"errand_id,omitempty"`

	// Return delivery fields (for return jobs after dispute)
	IsReturn          bool       `db:"is_return" json:"is_return"`
	OriginalJobID     *uuid.UUID `db:"original_job_id" json:"original_job_id,omitempty"`
	ReturnCondition   *string    `db:"return_condition" json:"return_condition,omitempty"`
	DamageClaimReason *string    `db:"damage_claim_reason" json:"damage_claim_reason,omitempty"`
	ReturnConfirmedAt *time.Time `db:"return_confirmed_at" json:"return_confirmed_at,omitempty"`
	DisputeID         *uuid.UUID `db:"dispute_id" json:"dispute_id,omitempty"`

	// Forced return fields (for buyer unavailable scenario)
	IsForcedReturn       bool       `db:"is_forced_return" json:"is_forced_return"`
	UnavailableMarkedAt  *time.Time `db:"unavailable_marked_at" json:"unavailable_marked_at,omitempty"`

	PickupAddress   string  `db:"pickup_address" json:"pickup_address"`
	PickupLat       float64 `db:"-" json:"pickup_lat,omitempty"` // Transient for creation
	PickupLng       float64 `db:"-" json:"pickup_lng,omitempty"` // Transient for creation
	PickupLocation  []byte  `db:"pickup_location" json:"-"`      // PostGIS GEOGRAPHY
	DropoffAddress  string  `db:"dropoff_address" json:"dropoff_address"`
	DropoffLat      float64 `db:"-" json:"dropoff_lat,omitempty"` // Transient for creation
	DropoffLng      float64 `db:"-" json:"dropoff_lng,omitempty"` // Transient for creation
	DropoffLocation []byte  `db:"dropoff_location" json:"-"`      // PostGIS GEOGRAPHY
	DistanceKM      *float64 `db:"distance_km" json:"distance_km,omitempty"`
	RequiredVehicleType VehicleType `db:"required_vehicle_type" json:"required_vehicle_type"`
	DeliveryFee         float64  `db:"delivery_fee" json:"delivery_fee"`
	RiderPayout         *float64 `db:"rider_payout" json:"rider_payout,omitempty"`
	PlatformDeliveryFee *float64 `db:"platform_delivery_fee" json:"platform_delivery_fee,omitempty"`
	PickupCode  *string `db:"pickup_code" json:"pickup_code,omitempty"`
	DropoffCode *string `db:"dropoff_code" json:"dropoff_code,omitempty"`
	SearchRadiusKM      int        `db:"search_radius_km" json:"search_radius_km"`
	SearchStartedAt     *time.Time `db:"search_started_at" json:"search_started_at,omitempty"`
	SearchTimeoutAt     *time.Time `db:"search_timeout_at" json:"search_timeout_at,omitempty"`
	LastRadiusExpandAt  *time.Time `db:"last_radius_expand_at" json:"last_radius_expand_at,omitempty"`
	Status DeliveryJobStatus `db:"status" json:"status"`
	CurrentETAMinutes *int       `db:"current_eta_minutes" json:"current_eta_minutes,omitempty"`
	ETAUpdatedAt      *time.Time `db:"eta_updated_at" json:"eta_updated_at,omitempty"`
	AcceptedAt  *time.Time `db:"accepted_at" json:"accepted_at,omitempty"`
	PickedUpAt  *time.Time `db:"picked_up_at" json:"picked_up_at,omitempty"`
	DeliveredAt *time.Time `db:"delivered_at" json:"delivered_at,omitempty"`
	CreatedAt   time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at" json:"updated_at"`
}
type RiderJobOffer struct {
	ID            uuid.UUID          `db:"id" json:"id"`
	DeliveryJobID uuid.UUID          `db:"delivery_job_id" json:"delivery_job_id"`
	RiderID       uuid.UUID          `db:"rider_id" json:"rider_id"`
	DistanceKM    *float64           `db:"distance_km" json:"distance_km,omitempty"`
	OfferedAt     time.Time          `db:"offered_at" json:"offered_at"`
	Response      RiderOfferResponse `db:"response" json:"response"`
	RespondedAt   *time.Time         `db:"responded_at" json:"responded_at,omitempty"`
}
func (s DeliveryJobStatus) CanTransitionTo(next DeliveryJobStatus) bool {
	transitions := map[DeliveryJobStatus][]DeliveryJobStatus{
		DJStatusCreated:       {DJStatusSearching, DJStatusCancelled},
		DJStatusSearching:     {DJStatusAccepted, DJStatusTimeout},
		DJStatusAccepted:      {DJStatusEnRoutePickup, DJStatusCancelled},
		DJStatusEnRoutePickup: {DJStatusPickedUp, DJStatusCancelled, DJStatusFailed, DJStatusSellerUnavailable},
		DJStatusPickedUp:      {DJStatusInTransit},
		DJStatusInTransit:     {DJStatusDelivered, DJStatusFailed, DJStatusBuyerUnavailable},
		DJStatusDelivered:     {DJStatusPaidOut, DJStatusReturnDelivered}, // Return jobs use ReturnDelivered
		DJStatusTimeout:       {DJStatusSearching}, // Can retry search

		// Return job flow (after dispute buyer_wins)
		DJStatusReturnDelivered:      {DJStatusReturnConfirmed, DJStatusReturnDamageDisputed},
		DJStatusReturnConfirmed:      {DJStatusReturnRefunded},
		DJStatusReturnDamageDisputed: {DJStatusReturnRefunded, DJStatusReturnConfirmed}, // Admin decides

		// Unavailability flow
		DJStatusSellerUnavailable: {}, // Terminal - job cancelled, buyer refunded
		DJStatusBuyerUnavailable:  {DJStatusForcedReturn},
		DJStatusForcedReturn:      {DJStatusForcedReturnDone},
		DJStatusForcedReturnDone:  {}, // Terminal - waiting for buyer rebook or refund
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
func (s DeliveryJobStatus) IsTerminal() bool {
	switch s {
	case DJStatusPaidOut, DJStatusCancelled, DJStatusFailed, DJStatusReturnRefunded,
		DJStatusSellerUnavailable, DJStatusForcedReturnDone:
		return true
	}
	return false
}

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

func (s DeliveryJobStatus) IsReturnStatus() bool {
	switch s {
	case DJStatusReturnDelivered, DJStatusReturnConfirmed, DJStatusReturnDamageDisputed, DJStatusReturnRefunded:
		return true
	}
	return false
}

func (j *DeliveryJob) HasRider() bool {
	return j.RiderID != nil
}

func (j *DeliveryJob) CanBeAccepted() bool {
	return j.Status == DJStatusSearching && j.RiderID == nil
}

func (j *DeliveryJob) IsReturnJob() bool {
	return j.IsReturn
}

func (j *DeliveryJob) CanSellerConfirmReturn() bool {
	return j.IsReturn && j.Status == DJStatusReturnDelivered
}

func (j *DeliveryJob) HasDamageClaim() bool {
	return j.Status == DJStatusReturnDamageDisputed
}

func (j *DeliveryJob) IsForcedReturnJob() bool {
	return j.IsForcedReturn
}

func (j *DeliveryJob) IsUnavailabilityStatus() bool {
	switch j.Status {
	case DJStatusSellerUnavailable, DJStatusBuyerUnavailable, DJStatusForcedReturn, DJStatusForcedReturnDone:
		return true
	}
	return false
}

func (j *DeliveryJob) CanMarkSellerUnavailable() bool {
	return j.Status == DJStatusEnRoutePickup || j.Status == DJStatusAccepted
}

func (j *DeliveryJob) CanMarkBuyerUnavailable() bool {
	return j.Status == DJStatusInTransit || j.Status == DJStatusPickedUp
}
