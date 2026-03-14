package domain

import (
	"time"

	"github.com/google/uuid"
)

type PlatformSettings struct {
	ID          uuid.UUID `db:"id" json:"id"`
	SettingKey  string    `db:"setting_key" json:"setting_key"`
	SettingValue []byte   `db:"setting_value" json:"-"` // JSONB
	Description *string   `db:"description" json:"description,omitempty"`
	UpdatedBy   *uuid.UUID `db:"updated_by" json:"updated_by,omitempty"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}

type DeliveryPricing struct {
	BaseFee            float64            `json:"base_fee"`
	PerKMFee           float64            `json:"per_km_fee"`
	VehicleMultipliers map[string]float64 `json:"vehicle_multipliers"`
}

type PlatformFees struct {
	BuyerFeeRate     float64 `json:"buyer_fee_rate"`
	SellerFeeRate    float64 `json:"seller_fee_rate"`
	RiderPayoutRate  float64 `json:"rider_payout_rate"`
}

type CommissionSettings struct {
	MaxUnpaidRides int     `json:"max_unpaid_rides"`
	CommissionRate float64 `json:"commission_rate"`
}

type TimerSettings struct {
	DisputeWindowMinutes     int `json:"dispute_window_minutes"`      // Default: 1440 (24 hours)
	RiderWaitTimeMinutes     int `json:"rider_wait_time_minutes"`     // Default: 45
	RebookWindowMinutes      int `json:"rebook_window_minutes"`       // Default: 45
	PaymentLinkExpiryHours   int `json:"payment_link_expiry_hours"`   // Default: 24
	RiderSearchTimeoutMins   int `json:"rider_search_timeout_mins"`   // Default: 30
}

type FAQQuestion struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type FAQCategory struct {
	Category  string        `json:"category"`
	Questions []FAQQuestion `json:"questions"`
}

type FAQSettings struct {
	Categories []FAQCategory `json:"categories"`
}
