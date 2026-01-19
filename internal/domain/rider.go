package domain

import (
	"time"

	"github.com/google/uuid"
)

// RiderStatus represents the verification status of a rider
type RiderStatus string

const (
	RiderStatusPendingVerification RiderStatus = "pending_verification"
	RiderStatusVerified            RiderStatus = "verified"
	RiderStatusSuspended           RiderStatus = "suspended"
	RiderStatusDeactivated         RiderStatus = "deactivated"
)

// RiderProfile represents a user's rider profile
type RiderProfile struct {
	ID     uuid.UUID `db:"id" json:"id"`
	UserID uuid.UUID `db:"user_id" json:"user_id"`

	// Verification documents
	LicenseNumber       *string `db:"license_number" json:"license_number,omitempty"`
	NINNumber           *string `db:"nin_number" json:"nin_number,omitempty"`
	LicensePhotoURL     *string `db:"license_photo_url" json:"license_photo_url,omitempty"`
	NINPhotoURL         *string `db:"nin_photo_url" json:"nin_photo_url,omitempty"`
	FaceVerificationURL *string `db:"face_verification_url" json:"face_verification_url,omitempty"`

	// Vehicle info
	VehicleType     *string `db:"vehicle_type" json:"vehicle_type,omitempty"`
	VehiclePlate    *string `db:"vehicle_plate" json:"vehicle_plate,omitempty"`
	VehiclePhotoURL *string `db:"vehicle_photo_url" json:"vehicle_photo_url,omitempty"`

	// Status and availability
	Status             RiderStatus `db:"status" json:"status"`
	IsOnline           bool        `db:"is_online" json:"is_online"`
	CurrentLocation    []byte      `db:"current_location" json:"-"` // PostGIS GEOGRAPHY
	LastLocationUpdate *time.Time  `db:"last_location_update" json:"last_location_update,omitempty"`

	// Performance metrics
	Rating          float64 `db:"rating" json:"rating"`
	TotalDeliveries int     `db:"total_deliveries" json:"total_deliveries"`
	TotalEarnings   float64 `db:"total_earnings" json:"total_earnings"`

	// Commission tracking (direct payment model)
	UnpaidCommission     float64    `db:"unpaid_commission" json:"unpaid_commission"`
	UnpaidDeliveryCount  int        `db:"unpaid_delivery_count" json:"unpaid_delivery_count"`
	LastCommissionPaidAt *time.Time `db:"last_commission_paid_at" json:"last_commission_paid_at,omitempty"`
	IsCommissionBlocked  bool       `db:"is_commission_blocked" json:"is_commission_blocked"`

	// Privacy
	LocationSharingEnabled bool `db:"location_sharing_enabled" json:"location_sharing_enabled"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// IsAvailable returns true if the rider can accept new jobs
func (r *RiderProfile) IsAvailable() bool {
	return r.Status == RiderStatusVerified && r.IsOnline
}

// CanGoOnline returns true if the rider can go online
func (r *RiderProfile) CanGoOnline() bool {
	return r.Status == RiderStatusVerified
}

// GoOnline sets the rider as online
func (r *RiderProfile) GoOnline() error {
	if !r.CanGoOnline() {
		return ErrRiderNotVerified
	}
	r.IsOnline = true
	return nil
}

// GoOffline sets the rider as offline
func (r *RiderProfile) GoOffline() {
	r.IsOnline = false
}

// UpdateRating updates the rider's rating based on a new review
func (r *RiderProfile) UpdateRating(newRating float64) {
	if r.TotalDeliveries == 0 {
		r.Rating = newRating
		return
	}
	// Calculate weighted average
	totalScore := r.Rating * float64(r.TotalDeliveries)
	totalScore += newRating
	r.TotalDeliveries++
	r.Rating = totalScore / float64(r.TotalDeliveries)
}

// IncrementDeliveries increments the total deliveries count
func (r *RiderProfile) IncrementDeliveries() {
	r.TotalDeliveries++
}

// AddEarnings adds to the rider's total earnings
func (r *RiderProfile) AddEarnings(amount float64) {
	r.TotalEarnings += amount
}

// CanAcceptJobs checks if rider can accept new delivery jobs
// Returns false if blocked for unpaid commission
func (r *RiderProfile) CanAcceptJobs() bool {
	return r.Status == RiderStatusVerified &&
		r.IsOnline &&
		!r.IsCommissionBlocked
}

// ShouldBlockForCommission checks if rider should be blocked
// Block happens AFTER 5 unpaid rides (grace period is rides 1-5)
func (r *RiderProfile) ShouldBlockForCommission() bool {
	return r.UnpaidDeliveryCount >= MaxUnpaidRidesBeforeBlock
}

// RidesUntilBlocked returns how many more unpaid rides until blocked
func (r *RiderProfile) RidesUntilBlocked() int {
	remaining := MaxUnpaidRidesBeforeBlock - r.UnpaidDeliveryCount
	if remaining < 0 {
		return 0
	}
	return remaining
}
