package domain

import (
	"time"

	"github.com/google/uuid"
)

// BankAccount represents a user's saved bank account for withdrawals
type BankAccount struct {
	ID              uuid.UUID  `db:"id" json:"id"`
	UserID          uuid.UUID  `db:"user_id" json:"user_id"`
	BankCode        string     `db:"bank_code" json:"bank_code"`
	BankName        string     `db:"bank_name" json:"bank_name"`
	AccountNumber   string     `db:"account_number" json:"account_number"`
	AccountName     string     `db:"account_name" json:"account_name"`
	RecipientCode   string     `db:"recipient_code" json:"recipient_code"` // Paystack recipient code
	IsDefault       bool       `db:"is_default" json:"is_default"`
	IsVerified      bool       `db:"is_verified" json:"is_verified"`
	CreatedAt       time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time  `db:"updated_at" json:"updated_at"`
}

// VirtualAccount represents a dedicated virtual account for wallet funding
type VirtualAccount struct {
	ID               uuid.UUID  `db:"id" json:"id"`
	UserID           uuid.UUID  `db:"user_id" json:"user_id"`
	BankName         string     `db:"bank_name" json:"bank_name"`
	BankSlug         string     `db:"bank_slug" json:"bank_slug"`
	AccountNumber    string     `db:"account_number" json:"account_number"`
	AccountName      string     `db:"account_name" json:"account_name"`
	CustomerCode     string     `db:"customer_code" json:"customer_code"`     // Paystack customer code
	DedicatedAcctID  int64      `db:"dedicated_acct_id" json:"dedicated_acct_id"` // Paystack DVA ID
	IsActive         bool       `db:"is_active" json:"is_active"`
	CreatedAt        time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt        time.Time  `db:"updated_at" json:"updated_at"`
}

// WithdrawalStatus represents the status of a withdrawal request
type WithdrawalStatus string

const (
	WithdrawalStatusPending    WithdrawalStatus = "pending"
	WithdrawalStatusProcessing WithdrawalStatus = "processing"
	WithdrawalStatusSuccess    WithdrawalStatus = "success"
	WithdrawalStatusFailed     WithdrawalStatus = "failed"
	WithdrawalStatusReversed   WithdrawalStatus = "reversed"
)

// Withdrawal represents a withdrawal request from wallet to bank
type Withdrawal struct {
	ID              uuid.UUID        `db:"id" json:"id"`
	UserID          uuid.UUID        `db:"user_id" json:"user_id"`
	WalletID        uuid.UUID        `db:"wallet_id" json:"wallet_id"`
	BankAccountID   uuid.UUID        `db:"bank_account_id" json:"bank_account_id"`
	Amount          float64          `db:"amount" json:"amount"`
	Fee             float64          `db:"fee" json:"fee"`               // Platform fee for withdrawal
	NetAmount       float64          `db:"net_amount" json:"net_amount"` // Amount - Fee
	Status          WithdrawalStatus `db:"status" json:"status"`
	TransferCode    *string          `db:"transfer_code" json:"transfer_code,omitempty"`   // Paystack transfer code
	TransferRef     string           `db:"transfer_ref" json:"transfer_ref"`               // Our reference
	FailureReason   *string          `db:"failure_reason" json:"failure_reason,omitempty"`
	ProcessedAt     *time.Time       `db:"processed_at" json:"processed_at,omitempty"`
	CreatedAt       time.Time        `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time        `db:"updated_at" json:"updated_at"`
}

// TopUpStatus represents the status of a wallet top-up
type TopUpStatus string

const (
	TopUpStatusPending   TopUpStatus = "pending"
	TopUpStatusSuccess   TopUpStatus = "success"
	TopUpStatusFailed    TopUpStatus = "failed"
	TopUpStatusAbandoned TopUpStatus = "abandoned"
)

// TopUp represents a wallet top-up/funding request
type TopUp struct {
	ID              uuid.UUID    `db:"id" json:"id"`
	UserID          uuid.UUID    `db:"user_id" json:"user_id"`
	WalletID        uuid.UUID    `db:"wallet_id" json:"wallet_id"`
	Amount          float64      `db:"amount" json:"amount"`
	PaymentMethod   string       `db:"payment_method" json:"payment_method"` // "card", "bank_transfer"
	PaymentRef      string       `db:"payment_ref" json:"payment_ref"`       // Paystack reference
	PaymentProvider string       `db:"payment_provider" json:"payment_provider"`
	Status          TopUpStatus  `db:"status" json:"status"`
	PaidAt          *time.Time   `db:"paid_at" json:"paid_at,omitempty"`
	CreatedAt       time.Time    `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time    `db:"updated_at" json:"updated_at"`
}
