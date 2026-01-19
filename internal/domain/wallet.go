package domain

import (
	"time"

	"github.com/google/uuid"
)

// WalletTransactionType represents the type of wallet transaction
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

// Wallet represents a user's balance
type Wallet struct {
	ID               uuid.UUID `db:"id" json:"id"`
	UserID           uuid.UUID `db:"user_id" json:"user_id"`
	AvailableBalance float64   `db:"available_balance" json:"available_balance"`
	LockedBalance    float64   `db:"locked_balance" json:"locked_balance"`
	TotalEarned      float64   `db:"total_earned" json:"total_earned"`
	TotalWithdrawn   float64   `db:"total_withdrawn" json:"total_withdrawn"`
	CreatedAt        time.Time `db:"created_at" json:"created_at"`
	UpdatedAt        time.Time `db:"updated_at" json:"updated_at"`
}

// WalletTransaction represents a single transaction on a wallet
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

// TotalBalance returns the sum of available and locked balances
func (w *Wallet) TotalBalance() float64 {
	return w.AvailableBalance + w.LockedBalance
}

// CanWithdraw checks if the wallet has enough available balance for withdrawal
func (w *Wallet) CanWithdraw(amount float64) bool {
	return w.AvailableBalance >= amount && amount > 0
}

// Credit adds funds to the wallet's available balance
func (w *Wallet) Credit(amount float64) {
	w.AvailableBalance += amount
	w.TotalEarned += amount
}

// Debit removes funds from the wallet's available balance
func (w *Wallet) Debit(amount float64) error {
	if w.AvailableBalance < amount {
		return ErrInsufficientBalance
	}
	w.AvailableBalance -= amount
	return nil
}

// Lock moves funds from available to locked balance
func (w *Wallet) Lock(amount float64) error {
	if w.AvailableBalance < amount {
		return ErrInsufficientBalance
	}
	w.AvailableBalance -= amount
	w.LockedBalance += amount
	return nil
}

// Unlock moves funds from locked back to available balance
func (w *Wallet) Unlock(amount float64) error {
	if w.LockedBalance < amount {
		return ErrInsufficientLockedBalance
	}
	w.LockedBalance -= amount
	w.AvailableBalance += amount
	return nil
}

// DeductLocked removes funds from locked balance (e.g., for escrow release)
func (w *Wallet) DeductLocked(amount float64) error {
	if w.LockedBalance < amount {
		return ErrInsufficientLockedBalance
	}
	w.LockedBalance -= amount
	return nil
}

// RecordWithdrawal updates the total withdrawn amount
func (w *Wallet) RecordWithdrawal(amount float64) {
	w.TotalWithdrawn += amount
}
