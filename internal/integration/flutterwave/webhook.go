package flutterwave

import "encoding/json"

const (
	EventChargeCompleted   = "charge.completed"
	EventTransferCompleted = "transfer.completed"
	EventTransferFailed    = "transfer.failed"
	// Payout Subaccount events
	EventPayoutCredit              = "payout.credit"               // Bank transfer into PSA virtual account
	EventVirtualAccountCredit      = "virtualaccount.credit"       // Alternative event name
	EventPayoutSubaccountCredit    = "payout-subaccount.credit"    // Yet another variation
)

type WebhookEvent struct {
	Event     string          `json:"event"`
	Data      json.RawMessage `json:"data"`
	EventType string          `json:"event.type,omitempty"`
}

type ChargeWebhookData struct {
	ID                int64   `json:"id"`
	TxRef             string  `json:"tx_ref"`
	FlwRef            string  `json:"flw_ref"`
	DeviceFingerprint string  `json:"device_fingerprint"`
	Amount            float64 `json:"amount"`
	Currency          string  `json:"currency"`
	ChargedAmount     float64 `json:"charged_amount"`
	AppFee            float64 `json:"app_fee"`
	MerchantFee       float64 `json:"merchant_fee"`
	ProcessorResponse string  `json:"processor_response"`
	AuthModel         string  `json:"auth_model"`
	IP                string  `json:"ip"`
	Narration         string  `json:"narration"`
	Status            string  `json:"status"`
	PaymentType       string  `json:"payment_type"`
	CreatedAt         string  `json:"created_at"`
	AccountID         int64   `json:"account_id"`
	Customer          struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		PhoneNumber string `json:"phone_number"`
		Email       string `json:"email"`
		CreatedAt   string `json:"created_at"`
	} `json:"customer"`
}

type TransferWebhookData struct {
	ID              int64       `json:"id"`
	AccountNumber   string      `json:"account_number"`
	BankName        string      `json:"bank_name"`
	BankCode        string      `json:"bank_code"`
	FullName        string      `json:"full_name"`
	CreatedAt       string      `json:"created_at"`
	Currency        string      `json:"currency"`
	DebitCurrency   string      `json:"debit_currency"`
	Amount          float64     `json:"amount"`
	Fee             float64     `json:"fee"`
	Status          string      `json:"status"`
	Reference       string      `json:"reference"`
	Meta            interface{} `json:"meta"`
	Narration       string      `json:"narration"`
	CompleteMessage string      `json:"complete_message"`
}

// PayoutCreditWebhookData represents the data for payout subaccount credit events
// This is sent when bank transfer is made to a PSA virtual account (nuban)
type PayoutCreditWebhookData struct {
	ID               int64   `json:"id"`
	AccountReference string  `json:"account_reference"` // PSA account reference
	Amount           float64 `json:"amount"`
	Currency         string  `json:"currency"`
	Status           string  `json:"status"`
	Reference        string  `json:"reference"`
	Narration        string  `json:"narration"`
	SenderBank       string  `json:"sender_bank"`
	SenderName       string  `json:"sender_name"`
	SenderAccount    string  `json:"sender_account"`
	CreatedAt        string  `json:"created_at"`
	// Fields that might be present
	AccountNumber string `json:"account_number"` // The nuban that received the transfer
	BarterID      string `json:"barter_id"`
}

func IsPaymentSuccessEvent(eventType string) bool {
	return eventType == EventChargeCompleted
}

func IsTransferEvent(eventType string) bool {
	switch eventType {
	case EventTransferCompleted, EventTransferFailed:
		return true
	default:
		return false
	}
}

func IsTransferSuccess(eventType string) bool {
	return eventType == EventTransferCompleted
}

// IsPayoutCreditEvent checks if the event is a payout subaccount credit (bank transfer to virtual account)
func IsPayoutCreditEvent(eventType string) bool {
	switch eventType {
	case EventPayoutCredit, EventVirtualAccountCredit, EventPayoutSubaccountCredit:
		return true
	default:
		return false
	}
}
