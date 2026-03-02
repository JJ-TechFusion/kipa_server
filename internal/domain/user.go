package domain

import (
	"time"

	"github.com/google/uuid"
)
type User struct {
	ID              uuid.UUID `db:"id" json:"id"`
	PhoneNumber     string    `db:"phone_number" json:"phone_number"`
	PhoneVerified   bool      `db:"phone_verified" json:"phone_verified"`
	Email           *string   `db:"email" json:"email,omitempty"`
	FirstName       *string   `db:"first_name" json:"first_name,omitempty"`
	LastName        *string   `db:"last_name" json:"last_name,omitempty"`
	ProfilePhotoURL *string   `db:"profile_photo_url" json:"profile_photo_url,omitempty"`
	IsSeller bool `db:"is_seller" json:"is_seller"`
	IsBuyer  bool `db:"is_buyer" json:"is_buyer"`
	IsRider  bool `db:"is_rider" json:"is_rider"`
	IsAdmin  bool `db:"is_admin" json:"is_admin"`
	SellerVerified   bool       `db:"seller_verified" json:"seller_verified"`
	SellerVerifiedAt *time.Time `db:"seller_verified_at" json:"seller_verified_at,omitempty"`
	IsSuspended      bool       `db:"is_suspended" json:"is_suspended"`
	SuspensionReason *string    `db:"suspension_reason" json:"suspension_reason,omitempty"`
	LastLoginAt      *time.Time `db:"last_login_at" json:"last_login_at,omitempty"`

	// Flutterwave virtual account preference
	FlwVirtualAccountDeclinedAt *time.Time `db:"flw_virtual_account_declined_at" json:"flw_virtual_account_declined_at,omitempty"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}
func (u *User) FullName() string {
	if u.FirstName == nil && u.LastName == nil {
		return ""
	}
	first := ""
	last := ""
	if u.FirstName != nil {
		first = *u.FirstName
	}
	if u.LastName != nil {
		last = *u.LastName
	}
	if first == "" {
		return last
	}
	if last == "" {
		return first
	}
	return first + " " + last
}
func (u *User) CanSell() bool {
	return u.IsSeller && u.SellerVerified && !u.IsSuspended
}
func (u *User) CanBuy() bool {
	return !u.IsSuspended
}
func (u *User) CanRide() bool {
	return u.IsRider && !u.IsSuspended
}
func (u *User) SetAsSeller() {
	u.IsSeller = true
}
func (u *User) VerifySeller() {
	u.SellerVerified = true
	now := time.Now()
	u.SellerVerifiedAt = &now
}
func (u *User) SetAsBuyer() {
	u.IsBuyer = true
}
func (u *User) SetAsRider() {
	u.IsRider = true
}

func (u *User) CanAccessAdmin() bool {
	return u.IsAdmin && !u.IsSuspended
}

type OTPPurpose string

const (
	OTPPurposeLogin  OTPPurpose = "login"
	OTPPurposeSignup OTPPurpose = "signup"
	OTPPurposeVerify OTPPurpose = "verify"
)
type OTPVerification struct {
	ID             uuid.UUID  `db:"id"`
	PhoneNumber    string     `db:"phone_number"`
	VerificationID uuid.UUID  `db:"verification_id"`
	CodeHash       string     `db:"code_hash"`
	Purpose        OTPPurpose `db:"purpose"`
	ExpiresAt      time.Time  `db:"expires_at"`
	Attempts       int        `db:"attempts"`
	VerifiedAt     *time.Time `db:"verified_at"`
	CreatedAt      time.Time  `db:"created_at"`
}
func (o *OTPVerification) IsExpired() bool {
	return time.Now().After(o.ExpiresAt)
}
func (o *OTPVerification) IsVerified() bool {
	return o.VerifiedAt != nil
}
func (o *OTPVerification) IncrementAttempts() {
	o.Attempts++
}
type IdempotencyKey struct {
	ID           uuid.UUID `db:"id"`
	Key          string    `db:"key"`
	Operation    string    `db:"operation"`
	RequestHash  string    `db:"request_hash"`
	ResponseData []byte    `db:"response_data"` // JSONB
	CreatedAt    time.Time `db:"created_at"`
}
type Session struct {
	UserID       uuid.UUID `json:"user_id"`
	DeviceID     string    `json:"device_id"`
	RefreshToken string    `json:"refresh_token"`
	UserAgent    string    `json:"user_agent"`
	IPAddress    string    `json:"ip_address"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}
func (s *Session) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}
