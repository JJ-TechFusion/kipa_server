package dto

import (
	"time"

	"github.com/google/uuid"
)

// Request DTOs

type SendOTPRequest struct {
	PhoneNumber string `json:"phone_number" binding:"required,e164"`
	DeviceID    string `json:"device_id" binding:"required,min=16,max=64"`
}

type VerifyOTPRequest struct {
	PhoneNumber    string `json:"phone_number" binding:"required,e164"`
	VerificationID string `json:"verification_id" binding:"required,uuid"`
	OTPCode        string `json:"otp_code" binding:"required,len=6,numeric"`
	DeviceID       string `json:"device_id" binding:"required,min=16,max=64"`
	IdempotencyKey string `json:"idempotency_key" binding:"required,min=32,max=64"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
	DeviceID     string `json:"device_id" binding:"required"`
}

type LogoutRequest struct {
	DeviceID   string `json:"device_id" binding:"required"`
	AllDevices bool   `json:"all_devices"`
}

type UpdateProfileRequest struct {
	FirstName       *string `json:"first_name" binding:"omitempty,min=1,max=100"`
	LastName        *string `json:"last_name" binding:"omitempty,min=1,max=100"`
	Email           *string `json:"email" binding:"omitempty,email"`
	ProfilePhotoURL *string `json:"profile_photo_url" binding:"omitempty,url"`
}

// Response DTOs

type SendOTPResponse struct {
	VerificationID string    `json:"verification_id"`
	ExpiresAt      time.Time `json:"expires_at"`
	RetryAfter     int       `json:"retry_after_seconds"`
}

type VerifyOTPResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	ExpiresIn    int          `json:"expires_in"`
	TokenType    string       `json:"token_type"`
	User         UserResponse `json:"user"`
	IsNewUser    bool         `json:"is_new_user"`
}

type RefreshTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

type UserResponse struct {
	ID              uuid.UUID `json:"id"`
	PhoneNumber     string    `json:"phone_number"`
	PhoneVerified   bool      `json:"phone_verified"`
	Email           *string   `json:"email,omitempty"`
	FirstName       *string   `json:"first_name,omitempty"`
	LastName        *string   `json:"last_name,omitempty"`
	IsSeller        bool      `json:"is_seller"`
	IsBuyer         bool      `json:"is_buyer"`
	IsRider         bool      `json:"is_rider"`
	SellerVerified  bool      `json:"seller_verified"`
	ProfilePhotoURL *string   `json:"profile_photo_url,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type LogoutResponse struct {
	Message string `json:"message"`
}

// Internal DTOs (service layer)

type TokenPair struct {
	AccessToken           string
	RefreshToken          string
	AccessTokenExpiresAt  time.Time
	RefreshTokenExpiresAt time.Time
}

type OTPGenerationResult struct {
	VerificationID uuid.UUID
	PlainOTP       string
	HashedOTP      string
	ExpiresAt      time.Time
}

type SMSDeliveryResult struct {
	Success    bool
	MessageID  string
	Provider   string
	FailReason string
}
