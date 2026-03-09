package domain

import (
	"time"

	"github.com/google/uuid"
)
type FlutterwaveVirtualAccount struct {
	ID            uuid.UUID `db:"id" json:"id"`
	UserID        uuid.UUID `db:"user_id" json:"user_id"`
	AccountNumber string    `db:"account_number" json:"account_number"` // The virtual account number
	BankName      string    `db:"bank_name" json:"bank_name"`           // e.g., "WEMA BANK"
	TxRef         string    `db:"tx_ref" json:"tx_ref"`                 // Unique reference for lookup
	FlwRef        string    `db:"flw_ref" json:"flw_ref"`               // Flutterwave reference
	OrderRef      string    `db:"order_ref" json:"order_ref"`           // Order reference from Flutterwave
	Email         string    `db:"email" json:"email"`
	IsActive      bool      `db:"is_active" json:"is_active"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time `db:"updated_at" json:"updated_at"`
}
