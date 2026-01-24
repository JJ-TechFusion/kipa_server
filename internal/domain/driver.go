package domain

import (
	"time"

	"github.com/google/uuid"
)

// DriverStatus represents the operational status of a driver
type DriverStatus string

const (
	DriverStatusOffline    DriverStatus = "offline"
	DriverStatusIdle       DriverStatus = "idle"
	DriverStatusLoading    DriverStatus = "loading"
	DriverStatusInTransit  DriverStatus = "in_transit"
	DriverStatusDelivering DriverStatus = "delivering"
)

func (s DriverStatus) IsValid() bool {
	switch s {
	case DriverStatusOffline, DriverStatusIdle, DriverStatusLoading,
		DriverStatusInTransit, DriverStatusDelivering:
		return true
	}
	return false
}

func (s DriverStatus) CanAcceptDeliveries() bool {
	return s == DriverStatusIdle || s == DriverStatusLoading
}

// DriverVehicleType represents the type of vehicle a driver operates
type DriverVehicleType string

const (
	DriverVehicleCar DriverVehicleType = "car"
	DriverVehicleVan DriverVehicleType = "van"
	DriverVehicleBus DriverVehicleType = "bus"
)

func (v DriverVehicleType) IsValid() bool {
	switch v {
	case DriverVehicleCar, DriverVehicleVan, DriverVehicleBus:
		return true
	}
	return false
}

// DriverProfile represents an inter-state driver's profile
type DriverProfile struct {
	ID     uuid.UUID `db:"id" json:"id"`
	UserID uuid.UUID `db:"user_id" json:"user_id"`

	// Status
	Status DriverStatus `db:"status" json:"status"`

	// Capacity limits
	MaxCarryingValue    float64 `db:"max_carrying_value" json:"max_carrying_value"`
	CurrentCarryValue   float64 `db:"current_carry_value" json:"current_carry_value"`
	MaxPackageCount     int     `db:"max_package_count" json:"max_package_count"`
	CurrentPackageCount int     `db:"current_package_count" json:"current_package_count"`

	// Vehicle info
	VehicleType  DriverVehicleType `db:"vehicle_type" json:"vehicle_type"`
	LicensePlate string            `db:"license_plate" json:"license_plate"`
	IsVerified   bool              `db:"is_verified" json:"is_verified"`

	// Stats
	TotalDeliveries int     `db:"total_deliveries" json:"total_deliveries"`
	TotalEarnings   float64 `db:"total_earnings" json:"total_earnings"`
	Rating          float64 `db:"rating" json:"rating"`

	// Current trip
	CurrentTripID   *uuid.UUID `db:"current_trip_id" json:"current_trip_id,omitempty"`
	OriginCity      *string    `db:"origin_city" json:"origin_city,omitempty"`
	DestinationCity *string    `db:"destination_city" json:"destination_city,omitempty"`

	// Verification documents
	NINNumber        *string `db:"nin_number" json:"nin_number,omitempty"`
	NINPhotoURL      *string `db:"nin_photo_url" json:"nin_photo_url,omitempty"`
	LicenseNumber    *string `db:"license_number" json:"license_number,omitempty"`
	LicensePhotoURL  *string `db:"license_photo_url" json:"license_photo_url,omitempty"`
	VehiclePhotoURL  *string `db:"vehicle_photo_url" json:"vehicle_photo_url,omitempty"`
	FacePhotoURL     *string `db:"face_photo_url" json:"face_photo_url,omitempty"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

func (d *DriverProfile) CanGoOnline() bool {
	return d.IsVerified && d.Status == DriverStatusOffline
}

func (d *DriverProfile) GoOnline() error {
	if !d.CanGoOnline() {
		return ErrDriverNotVerified
	}
	d.Status = DriverStatusIdle
	return nil
}

func (d *DriverProfile) GoOffline() error {
	if d.Status == DriverStatusInTransit || d.Status == DriverStatusDelivering {
		return ErrDriverHasActiveTrip
	}
	d.Status = DriverStatusOffline
	d.CurrentTripID = nil
	d.OriginCity = nil
	d.DestinationCity = nil
	return nil
}

func (d *DriverProfile) CanAcceptDelivery(itemValue float64) bool {
	if !d.Status.CanAcceptDeliveries() {
		return false
	}
	if d.CurrentCarryValue+itemValue > d.MaxCarryingValue {
		return false
	}
	if d.CurrentPackageCount >= d.MaxPackageCount {
		return false
	}
	return true
}

func (d *DriverProfile) AddCapacity(itemValue float64) {
	d.CurrentCarryValue += itemValue
	d.CurrentPackageCount++
}

func (d *DriverProfile) ReleaseCapacity(itemValue float64) {
	d.CurrentCarryValue -= itemValue
	if d.CurrentCarryValue < 0 {
		d.CurrentCarryValue = 0
	}
	d.CurrentPackageCount--
	if d.CurrentPackageCount < 0 {
		d.CurrentPackageCount = 0
	}
}

func (d *DriverProfile) ResetCapacity() {
	d.CurrentCarryValue = 0
	d.CurrentPackageCount = 0
}

func (d *DriverProfile) AvailableCapacity() float64 {
	return d.MaxCarryingValue - d.CurrentCarryValue
}

func (d *DriverProfile) AvailableSlots() int {
	return d.MaxPackageCount - d.CurrentPackageCount
}

// DriverTripStatus represents the status of a driver trip
type DriverTripStatus string

const (
	DriverTripStatusAccepting  DriverTripStatus = "accepting"
	DriverTripStatusLoading    DriverTripStatus = "loading"
	DriverTripStatusInTransit  DriverTripStatus = "in_transit"
	DriverTripStatusArrived    DriverTripStatus = "arrived"
	DriverTripStatusDelivering DriverTripStatus = "delivering"
	DriverTripStatusCompleted  DriverTripStatus = "completed"
	DriverTripStatusCancelled  DriverTripStatus = "cancelled"
)

func (s DriverTripStatus) IsValid() bool {
	switch s {
	case DriverTripStatusAccepting, DriverTripStatusLoading, DriverTripStatusInTransit,
		DriverTripStatusArrived, DriverTripStatusDelivering, DriverTripStatusCompleted,
		DriverTripStatusCancelled:
		return true
	}
	return false
}

func (s DriverTripStatus) CanAcceptDeliveries() bool {
	return s == DriverTripStatusAccepting || s == DriverTripStatusLoading
}

func (s DriverTripStatus) IsTerminal() bool {
	return s == DriverTripStatusCompleted || s == DriverTripStatusCancelled
}

// DriverTrip groups multiple deliveries for a single driver route
type DriverTrip struct {
	ID       uuid.UUID `db:"id" json:"id"`
	DriverID uuid.UUID `db:"driver_id" json:"driver_id"`

	// Route
	OriginCity      string `db:"origin_city" json:"origin_city"`
	DestinationCity string `db:"destination_city" json:"destination_city"`

	// Status
	Status DriverTripStatus `db:"status" json:"status"`

	// Aggregates
	TotalPackages  int     `db:"total_packages" json:"total_packages"`
	TotalValue     float64 `db:"total_value" json:"total_value"`
	TotalDriverFee float64 `db:"total_driver_fee" json:"total_driver_fee"`

	// Timeline
	AcceptingUntil *time.Time `db:"accepting_until" json:"accepting_until,omitempty"`
	DepartedAt     *time.Time `db:"departed_at" json:"departed_at,omitempty"`
	ArrivedAt      *time.Time `db:"arrived_at" json:"arrived_at,omitempty"`
	CompletedAt    *time.Time `db:"completed_at" json:"completed_at,omitempty"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

func (t *DriverTrip) CanAcceptDeliveries() bool {
	return t.Status.CanAcceptDeliveries()
}

func (t *DriverTrip) CanDepart() bool {
	return t.Status == DriverTripStatusLoading && t.TotalPackages > 0
}

func (t *DriverTrip) CanMarkArrived() bool {
	return t.Status == DriverTripStatusInTransit
}

func (t *DriverTrip) AddDelivery(itemValue, driverFee float64) {
	t.TotalPackages++
	t.TotalValue += itemValue
	t.TotalDriverFee += driverFee
}

func (t *DriverTrip) RemoveDelivery(itemValue, driverFee float64) {
	t.TotalPackages--
	if t.TotalPackages < 0 {
		t.TotalPackages = 0
	}
	t.TotalValue -= itemValue
	if t.TotalValue < 0 {
		t.TotalValue = 0
	}
	t.TotalDriverFee -= driverFee
	if t.TotalDriverFee < 0 {
		t.TotalDriverFee = 0
	}
}

// DriverDeliveryStatus represents the status of an individual driver delivery
type DriverDeliveryStatus string

const (
	DDStatusAwaitingDriver      DriverDeliveryStatus = "awaiting_driver"
	DDStatusDriverAssigned      DriverDeliveryStatus = "driver_assigned"
	DDStatusAwaitingPickup      DriverDeliveryStatus = "awaiting_pickup"
	DDStatusPickedUp            DriverDeliveryStatus = "picked_up"
	DDStatusInTransit           DriverDeliveryStatus = "in_transit"
	DDStatusArrived             DriverDeliveryStatus = "arrived"
	DDStatusConfirmationWindow  DriverDeliveryStatus = "confirmation_window"
	DDStatusCompleted           DriverDeliveryStatus = "completed"
	DDStatusDriverPaid          DriverDeliveryStatus = "driver_paid"
	DDStatusDisputed            DriverDeliveryStatus = "disputed"
	DDStatusRefunded            DriverDeliveryStatus = "refunded"
	DDStatusCancelled           DriverDeliveryStatus = "cancelled"
	DDStatusExpired             DriverDeliveryStatus = "expired"
	DDStatusBuyerUnreachable    DriverDeliveryStatus = "buyer_unreachable"
	DDStatusReturnToSeller      DriverDeliveryStatus = "return_to_seller"
	DDStatusReturnedToSeller    DriverDeliveryStatus = "returned_to_seller"
	DDStatusPickupCodeLockout   DriverDeliveryStatus = "pickup_code_lockout"
	DDStatusDropoffCodeLockout  DriverDeliveryStatus = "dropoff_code_lockout"
)

func (s DriverDeliveryStatus) IsValid() bool {
	switch s {
	case DDStatusAwaitingDriver, DDStatusDriverAssigned, DDStatusAwaitingPickup,
		DDStatusPickedUp, DDStatusInTransit, DDStatusArrived, DDStatusConfirmationWindow,
		DDStatusCompleted, DDStatusDriverPaid, DDStatusDisputed, DDStatusRefunded,
		DDStatusCancelled, DDStatusExpired, DDStatusBuyerUnreachable, DDStatusReturnToSeller,
		DDStatusReturnedToSeller, DDStatusPickupCodeLockout, DDStatusDropoffCodeLockout:
		return true
	}
	return false
}

func (s DriverDeliveryStatus) IsTerminal() bool {
	switch s {
	case DDStatusDriverPaid, DDStatusRefunded, DDStatusCancelled, DDStatusExpired,
		DDStatusReturnedToSeller:
		return true
	}
	return false
}

func (s DriverDeliveryStatus) CanTransitionTo(next DriverDeliveryStatus) bool {
	transitions := map[DriverDeliveryStatus][]DriverDeliveryStatus{
		DDStatusAwaitingDriver:     {DDStatusDriverAssigned, DDStatusExpired, DDStatusCancelled},
		DDStatusDriverAssigned:     {DDStatusAwaitingPickup, DDStatusCancelled},
		DDStatusAwaitingPickup:     {DDStatusPickedUp, DDStatusPickupCodeLockout, DDStatusCancelled},
		DDStatusPickedUp:           {DDStatusInTransit},
		DDStatusInTransit:          {DDStatusArrived},
		DDStatusArrived:            {DDStatusConfirmationWindow, DDStatusDropoffCodeLockout, DDStatusBuyerUnreachable},
		DDStatusConfirmationWindow: {DDStatusCompleted, DDStatusDisputed},
		DDStatusCompleted:          {DDStatusDriverPaid},
		DDStatusDisputed:           {DDStatusRefunded, DDStatusCompleted}, // Admin resolves
		DDStatusBuyerUnreachable:   {DDStatusReturnToSeller, DDStatusConfirmationWindow},
		DDStatusReturnToSeller:     {DDStatusReturnedToSeller},
		DDStatusReturnedToSeller:   {DDStatusRefunded},
		DDStatusPickupCodeLockout:  {DDStatusAwaitingPickup, DDStatusCancelled}, // Admin resolves
		DDStatusDropoffCodeLockout: {DDStatusArrived, DDStatusBuyerUnreachable}, // Admin resolves
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

// DriverDelivery represents a single package being delivered by a driver
type DriverDelivery struct {
	ID               uuid.UUID  `db:"id" json:"id"`
	PaymentRequestID uuid.UUID  `db:"payment_request_id" json:"payment_request_id"`
	TripID           *uuid.UUID `db:"trip_id" json:"trip_id,omitempty"`
	DriverID         *uuid.UUID `db:"driver_id" json:"driver_id,omitempty"`

	// Route
	OriginCity      string `db:"origin_city" json:"origin_city"`
	DestinationCity string `db:"destination_city" json:"destination_city"`
	PickupAddress   string `db:"pickup_address" json:"pickup_address"`
	DropoffAddress  string `db:"dropoff_address" json:"dropoff_address"`

	// Value
	ItemValue float64 `db:"item_value" json:"item_value"`
	DriverFee float64 `db:"driver_fee" json:"driver_fee"`

	// Codes
	PickupCode            *string    `db:"pickup_code" json:"pickup_code,omitempty"`
	PickupCodeGeneratedAt *time.Time `db:"pickup_code_generated_at" json:"pickup_code_generated_at,omitempty"`
	PickupAttempts        int        `db:"pickup_attempts" json:"pickup_attempts"`
	DropoffCode           *string    `db:"dropoff_code" json:"dropoff_code,omitempty"`
	DropoffCodeGeneratedAt *time.Time `db:"dropoff_code_generated_at" json:"dropoff_code_generated_at,omitempty"`
	DropoffAttempts       int        `db:"dropoff_attempts" json:"dropoff_attempts"`

	// Status
	Status DriverDeliveryStatus `db:"status" json:"status"`

	// Timeline (manual updates, no GPS)
	DriverAssignedAt *time.Time `db:"driver_assigned_at" json:"driver_assigned_at,omitempty"`
	AtPickupAt       *time.Time `db:"at_pickup_at" json:"at_pickup_at,omitempty"`
	PickedUpAt       *time.Time `db:"picked_up_at" json:"picked_up_at,omitempty"`
	DepartedAt       *time.Time `db:"departed_at" json:"departed_at,omitempty"`
	ArrivedAt        *time.Time `db:"arrived_at" json:"arrived_at,omitempty"`
	DeliveredAt      *time.Time `db:"delivered_at" json:"delivered_at,omitempty"`
	CompletedAt      *time.Time `db:"completed_at" json:"completed_at,omitempty"`

	// Buyer unreachable
	BuyerUnreachableAt  *time.Time `db:"buyer_unreachable_at" json:"buyer_unreachable_at,omitempty"`
	UnreachableDeadline *time.Time `db:"unreachable_deadline" json:"unreachable_deadline,omitempty"`

	// Escrow
	EscrowID *uuid.UUID `db:"escrow_id" json:"escrow_id,omitempty"`

	// Seller/Buyer references (from payment request)
	SellerID uuid.UUID  `db:"seller_id" json:"seller_id"`
	BuyerID  *uuid.UUID `db:"buyer_id" json:"buyer_id,omitempty"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

const (
	MaxPickupAttempts  = 3
	MaxDropoffAttempts = 3
	CodeLength         = 6
	CodeExpiryHours    = 24
	// AutoConfirmDays is for auto-confirm if buyer doesn't respond (not a constraint on manual confirm)
	AutoConfirmDays = 7
	BuyerUnreachableWindowHours = 24
	BuyerUnreachablePenaltyPercent = 10.0
)

func (d *DriverDelivery) CanVerifyPickupCode() bool {
	return d.Status == DDStatusAwaitingPickup && d.PickupAttempts < MaxPickupAttempts
}

func (d *DriverDelivery) CanVerifyDropoffCode() bool {
	return d.Status == DDStatusArrived && d.DropoffAttempts < MaxDropoffAttempts
}

func (d *DriverDelivery) IncrementPickupAttempts() bool {
	d.PickupAttempts++
	return d.PickupAttempts >= MaxPickupAttempts
}

func (d *DriverDelivery) IncrementDropoffAttempts() bool {
	d.DropoffAttempts++
	return d.DropoffAttempts >= MaxDropoffAttempts
}

func (d *DriverDelivery) IsPickupCodeLocked() bool {
	return d.PickupAttempts >= MaxPickupAttempts
}

func (d *DriverDelivery) IsDropoffCodeLocked() bool {
	return d.DropoffAttempts >= MaxDropoffAttempts
}

func (d *DriverDelivery) HasDriver() bool {
	return d.DriverID != nil
}

func (d *DriverDelivery) CanBeCancelled() bool {
	// Can only cancel before item is picked up
	switch d.Status {
	case DDStatusAwaitingDriver, DDStatusDriverAssigned, DDStatusAwaitingPickup:
		return true
	}
	return false
}

// CanBeCancelledWithTrip checks if delivery can be cancelled considering trip status
func (d *DriverDelivery) CanBeCancelledWithTrip(tripStatus DriverTripStatus) bool {
	if !d.CanBeCancelled() {
		return false
	}
	// If trip assigned, can only cancel before trip departs (in_transit)
	if d.TripID != nil {
		return tripStatus == DriverTripStatusAccepting || tripStatus == DriverTripStatusLoading
	}
	return true
}

func (d *DriverDelivery) CalculatePenalty() float64 {
	return d.ItemValue * (BuyerUnreachablePenaltyPercent / 100.0)
}

// DriverDeliveryActorType represents who performed an action
type DriverDeliveryActorType string

const (
	DDActorDriver DriverDeliveryActorType = "driver"
	DDActorBuyer  DriverDeliveryActorType = "buyer"
	DDActorSeller DriverDeliveryActorType = "seller"
	DDActorSystem DriverDeliveryActorType = "system"
	DDActorAdmin  DriverDeliveryActorType = "admin"
)

// DriverDeliveryEvent tracks status changes for audit
type DriverDeliveryEvent struct {
	ID         uuid.UUID               `db:"id" json:"id"`
	DeliveryID uuid.UUID               `db:"delivery_id" json:"delivery_id"`
	FromStatus *DriverDeliveryStatus   `db:"from_status" json:"from_status,omitempty"`
	ToStatus   DriverDeliveryStatus    `db:"to_status" json:"to_status"`
	ActorType  DriverDeliveryActorType `db:"actor_type" json:"actor_type"`
	ActorID    *uuid.UUID              `db:"actor_id" json:"actor_id,omitempty"`
	Reason     *string                 `db:"reason" json:"reason,omitempty"`
	Metadata   []byte                  `db:"metadata" json:"metadata,omitempty"`
	CreatedAt  time.Time               `db:"created_at" json:"created_at"`
}
