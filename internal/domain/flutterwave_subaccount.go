package domain

import (
	"time"

	"github.com/google/uuid"
)
type FlutterwaveSubaccount struct {
	ID               uuid.UUID  `db:"id" json:"id"`
	UserID           uuid.UUID  `db:"user_id" json:"user_id"`
	AccountReference string     `db:"account_reference" json:"account_reference"`
	AccountBank      *string    `db:"account_bank" json:"account_bank,omitempty"`
	AccountNumber    *string    `db:"account_number" json:"account_number,omitempty"`
	BarterID         *string    `db:"barter_id" json:"barter_id,omitempty"` // For wallet-to-wallet transfers
	BusinessName     *string    `db:"business_name" json:"business_name,omitempty"`
	Email            *string    `db:"email" json:"email,omitempty"`
	IsActive         bool       `db:"is_active" json:"is_active"`
	CreatedAt        time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt        time.Time  `db:"updated_at" json:"updated_at"`
}
