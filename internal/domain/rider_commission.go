package domain

import (
	"math"
	"time"

	"github.com/google/uuid"
)
type RiderCommissionStatus string

const (
	CommissionStatusPending RiderCommissionStatus = "pending"
	CommissionStatusPaid    RiderCommissionStatus = "paid"
	CommissionStatusWaived  RiderCommissionStatus = "waived"
)
const (
	RiderCommissionRate      = 0.15 // 15% platform commission
	MaxUnpaidRidesBeforeBlock = 5   // Grace period: rides 1-5
)
type RiderCommission struct {
	ID             uuid.UUID `db:"id" json:"id"`
	RiderProfileID uuid.UUID `db:"rider_profile_id" json:"rider_profile_id"`
	DeliveryJobID  uuid.UUID `db:"delivery_job_id" json:"delivery_job_id"`

	DeliveryFee      float64 `db:"delivery_fee" json:"delivery_fee"`
	CommissionRate   float64 `db:"commission_rate" json:"commission_rate"`
	CommissionAmount float64 `db:"commission_amount" json:"commission_amount"`

	Status  RiderCommissionStatus `db:"status" json:"status"`
	PaidAt  *time.Time            `db:"paid_at" json:"paid_at,omitempty"`
	PaidVia *string               `db:"paid_via" json:"paid_via,omitempty"` // "wallet", "admin_waive"

	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
type CommissionPayment struct {
	ID             uuid.UUID `db:"id" json:"id"`
	RiderProfileID uuid.UUID `db:"rider_profile_id" json:"rider_profile_id"`

	Amount          float64 `db:"amount" json:"amount"`
	CommissionsPaid int     `db:"commissions_paid" json:"commissions_paid"`

	PaymentMethod       string     `db:"payment_method" json:"payment_method"`
	WalletTransactionID *uuid.UUID `db:"wallet_transaction_id" json:"wallet_transaction_id,omitempty"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
func CalculateCommission(deliveryFee float64) float64 {
	return roundToTwoDecimals(deliveryFee * RiderCommissionRate)
}
func CalculateRiderTake(deliveryFee float64) float64 {
	return roundToTwoDecimals(deliveryFee - CalculateCommission(deliveryFee))
}
func NewRiderCommission(riderProfileID, deliveryJobID uuid.UUID, deliveryFee float64) *RiderCommission {
	commissionAmount := CalculateCommission(deliveryFee)

	return &RiderCommission{
		ID:               uuid.New(),
		RiderProfileID:   riderProfileID,
		DeliveryJobID:    deliveryJobID,
		DeliveryFee:      deliveryFee,
		CommissionRate:   RiderCommissionRate,
		CommissionAmount: commissionAmount,
		Status:           CommissionStatusPending,
		CreatedAt:        time.Now(),
	}
}
type RiderFeeBreakdown struct {
	DeliveryFee        float64 `json:"delivery_fee"`        // Full fee from buyer
	PlatformCommission float64 `json:"platform_commission"` // 15% to platform
	RiderTake          float64 `json:"rider_take"`          // What rider keeps (85%)
	CommissionRate     float64 `json:"commission_rate"`     // 0.15
}
func NewRiderFeeBreakdown(deliveryFee float64) *RiderFeeBreakdown {
	commission := CalculateCommission(deliveryFee)
	return &RiderFeeBreakdown{
		DeliveryFee:        deliveryFee,
		PlatformCommission: commission,
		RiderTake:          deliveryFee - commission,
		CommissionRate:     RiderCommissionRate,
	}
}
func roundToTwoDecimals(val float64) float64 {
	return math.Round(val*100) / 100
}
