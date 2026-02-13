package domain

import (
	"time"

	"github.com/google/uuid"
)
type WalletTransactionType string

const (
	WalletTxEscrowFund     WalletTransactionType = "escrow_fund"
	WalletTxEscrowRelease  WalletTransactionType = "escrow_release"
	WalletTxDeliveryPayout WalletTransactionType = "delivery_payout"
	WalletTxPlatformFee    WalletTransactionType = "platform_fee"
	WalletTxWithdrawal     WalletTransactionType = "withdrawal"
	WalletTxTopUp          WalletTransactionType = "top_up"
	WalletTxRefund         WalletTransactionType = "refund"
	WalletTxAdjustment     WalletTransactionType = "adjustment"
)
type Wallet struct {
	ID               uuid.UUID  `db:"id" json:"id"`
	UserID           uuid.UUID  `db:"user_id" json:"user_id"`
	AvailableBalance float64    `db:"available_balance" json:"available_balance"`
	LockedBalance    float64    `db:"locked_balance" json:"locked_balance"`
	TotalEarned      float64    `db:"total_earned" json:"total_earned"`
	TotalWithdrawn   float64    `db:"total_withdrawn" json:"total_withdrawn"`
	PinHash          *string    `db:"pin_hash" json:"-"`
	PinCreatedAt     *time.Time `db:"pin_created_at" json:"pin_created_at,omitempty"`
	PinAttempts      int        `db:"pin_attempts" json:"-"`
	PinLockedUntil   *time.Time `db:"pin_locked_until" json:"-"`
	CreatedAt        time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt        time.Time  `db:"updated_at" json:"updated_at"`
}
type WalletTransaction struct {
	ID             uuid.UUID             `db:"id" json:"id"`
	WalletID       uuid.UUID             `db:"wallet_id" json:"wallet_id"`
	Type           WalletTransactionType `db:"type" json:"type"`
	Amount         float64               `db:"amount" json:"amount"`
	BalanceAfter   float64               `db:"balance_after" json:"balance_after"`
	ReferenceType  *string               `db:"reference_type" json:"reference_type,omitempty"`
	ReferenceID    *uuid.UUID            `db:"reference_id" json:"reference_id,omitempty"`
	Description    *string               `db:"description" json:"description,omitempty"`
	IdempotencyKey *string               `db:"idempotency_key" json:"idempotency_key,omitempty"`
	CreatedAt      time.Time             `db:"created_at" json:"created_at"`
}
func (w *Wallet) TotalBalance() float64 {
	return w.AvailableBalance + w.LockedBalance
}
func (w *Wallet) CanWithdraw(amount float64) bool {
	return w.AvailableBalance >= amount && amount > 0
}
func (w *Wallet) Credit(amount float64) {
	w.AvailableBalance += amount
	w.TotalEarned += amount
}
func (w *Wallet) Debit(amount float64) error {
	if w.AvailableBalance < amount {
		return ErrInsufficientBalance
	}
	w.AvailableBalance -= amount
	return nil
}
func (w *Wallet) Lock(amount float64) error {
	if w.AvailableBalance < amount {
		return ErrInsufficientBalance
	}
	w.AvailableBalance -= amount
	w.LockedBalance += amount
	return nil
}
func (w *Wallet) Unlock(amount float64) error {
	if w.LockedBalance < amount {
		return ErrInsufficientLockedBalance
	}
	w.LockedBalance -= amount
	w.AvailableBalance += amount
	return nil
}
func (w *Wallet) DeductLocked(amount float64) error {
	if w.LockedBalance < amount {
		return ErrInsufficientLockedBalance
	}
	w.LockedBalance -= amount
	return nil
}
func (w *Wallet) RecordWithdrawal(amount float64) {
	w.TotalWithdrawn += amount
}

// HasPin returns true if the wallet has a PIN set
func (w *Wallet) HasPin() bool {
	return w.PinHash != nil && *w.PinHash != ""
}

// IsPinLocked returns true if the wallet is locked due to too many failed attempts
func (w *Wallet) IsPinLocked() bool {
	if w.PinLockedUntil == nil {
		return false
	}
	return time.Now().Before(*w.PinLockedUntil)
}

// IncrementPinAttempts increments failed PIN attempts and locks if threshold reached
func (w *Wallet) IncrementPinAttempts(maxAttempts int, lockDuration time.Duration) {
	w.PinAttempts++
	if w.PinAttempts >= maxAttempts {
		lockUntil := time.Now().Add(lockDuration)
		w.PinLockedUntil = &lockUntil
	}
}

// ResetPinAttempts resets the PIN attempt counter after successful verification
func (w *Wallet) ResetPinAttempts() {
	w.PinAttempts = 0
	w.PinLockedUntil = nil
}
