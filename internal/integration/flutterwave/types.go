package flutterwave

import (
	"encoding/json"
	"time"
)

type APIResponse[T any] struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Data    T      `json:"data,omitempty"`
}


type InitializePaymentRequest struct {
	TxRef          string                 `json:"tx_ref"`
	Amount         float64                `json:"amount"`
	Currency       string                 `json:"currency"`
	RedirectURL    string                 `json:"redirect_url"`
	Customer       CustomerInfo           `json:"customer"`
	PaymentOptions string                 `json:"payment_options,omitempty"`
	Customizations *PaymentCustomization  `json:"customizations,omitempty"`
	Meta           map[string]interface{} `json:"meta,omitempty"`
}

type CustomerInfo struct {
	Email       string `json:"email"`
	PhoneNumber string `json:"phonenumber,omitempty"`
	Name        string `json:"name,omitempty"`
}

type PaymentCustomization struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Logo        string `json:"logo,omitempty"`
}

type InitializePaymentData struct {
	Link string `json:"link"`
}

type VerifyTransactionData struct {
	ID                int64           `json:"id"`
	TxRef             string          `json:"tx_ref"`
	FlwRef            string          `json:"flw_ref"`
	DeviceFingerprint string          `json:"device_fingerprint"`
	Amount            float64         `json:"amount"`
	Currency          string          `json:"currency"`
	ChargedAmount     float64         `json:"charged_amount"`
	AppFee            float64         `json:"app_fee"`
	MerchantFee       float64         `json:"merchant_fee"`
	ProcessorResponse string          `json:"processor_response"`
	AuthModel         string          `json:"auth_model"`
	IP                string          `json:"ip"`
	Narration         string          `json:"narration"`
	Status            string          `json:"status"`
	PaymentType       string          `json:"payment_type"`
	CreatedAt         time.Time       `json:"created_at"`
	AccountID         int64           `json:"account_id"`
	Customer          CustomerData    `json:"customer"`
	Meta              json.RawMessage `json:"meta"`
}

type CustomerData struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	PhoneNumber string `json:"phone_number"`
	Email       string `json:"email"`
	CreatedAt   string `json:"created_at"`
}

type CreateSubaccountRequest struct {
	AccountName string `json:"account_name"`
	Email       string `json:"email"`
	Country     string `json:"country"`
	BankCode    string `json:"bank_code,omitempty"` // "035" (WEMA) or "232" (Sterling)
}

type SubaccountData struct {
	ID               int64  `json:"id"`
	AccountReference string `json:"account_reference"`
	AccountName      string `json:"account_name"`
	BarterID         string `json:"barter_id"` // Needed for wallet-to-wallet transfers
	Email            string `json:"email"`
	Country          string `json:"country"`
	Nuban            string `json:"nuban"` // Virtual account number
	BankName         string `json:"bank_name"`
	BankCode         string `json:"bank_code"`
	Status           string `json:"status"`
	CreatedAt        string `json:"created_at"`
}

type SubaccountBalanceData struct {
	Currency         string  `json:"currency"`
	AvailableBalance float64 `json:"available_balance"`
	LedgerBalance    float64 `json:"ledger_balance"`
}


type InitiateTransferRequest struct {
	AccountBank      string                 `json:"account_bank"`
	AccountNumber    string                 `json:"account_number"`
	Amount           float64                `json:"amount"`
	Currency         string                 `json:"currency"`
	Narration        string                 `json:"narration,omitempty"`
	Reference        string                 `json:"reference"`
	CallbackURL      string                 `json:"callback_url,omitempty"`
	DebitSubaccount  string                 `json:"debit_subaccount,omitempty"` // Account reference for subaccount debits
	DebitCurrency    string                 `json:"debit_currency,omitempty"`
	BeneficiaryName  string                 `json:"beneficiary_name,omitempty"`
	Meta             map[string]interface{} `json:"meta,omitempty"`
}

type TransferData struct {
	ID              int64   `json:"id"`
	AccountNumber   string  `json:"account_number"`
	BankCode        string  `json:"bank_code"`
	FullName        string  `json:"full_name"`
	CreatedAt       string  `json:"created_at"`
	Currency        string  `json:"currency"`
	DebitCurrency   string  `json:"debit_currency"`
	Amount          float64 `json:"amount"`
	Fee             float64 `json:"fee"`
	Status          string  `json:"status"`
	Reference       string  `json:"reference"`
	Meta            interface{} `json:"meta"`
	Narration       string  `json:"narration"`
	CompleteMessage string  `json:"complete_message"`
	RequiresApproval int    `json:"requires_approval"`
	IsApproved      int     `json:"is_approved"`
	BankName        string  `json:"bank_name"`
}

type GetTransferData struct {
	ID            int64   `json:"id"`
	AccountNumber string  `json:"account_number"`
	BankCode      string  `json:"bank_code"`
	BankName      string  `json:"bank_name"`
	FullName      string  `json:"full_name"`
	Currency      string  `json:"currency"`
	Amount        float64 `json:"amount"`
	Fee           float64 `json:"fee"`
	Status        string  `json:"status"`
	Reference     string  `json:"reference"`
	Narration     string  `json:"narration"`
	CreatedAt     string  `json:"created_at"`
}

type ResolveAccountData struct {
	AccountNumber string `json:"account_number"`
	AccountName   string `json:"account_name"`
}

type BankData struct {
	ID   int64  `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}
type StaticAccountData struct {
	StaticAccount string `json:"static_account"`
	BankName      string `json:"bank_name"`
	BankCode      string `json:"bank_code"`
	Currency      string `json:"currency"`
}
type WalletTransferRequest struct {
	AccountBank   string  `json:"account_bank"`   // "flutterwave" for wallet transfers
	AccountNumber string  `json:"account_number"` // Destination wallet's barter_id
	Amount        float64 `json:"amount"`
	Currency      string  `json:"currency"`
	Reference     string  `json:"reference,omitempty"`
	Narration     string  `json:"narration,omitempty"`
}
type UpdateWalletRequest struct {
	AccountName  string `json:"account_name,omitempty"`
	Email        string `json:"email,omitempty"`
	MobileNumber string `json:"mobilenumber,omitempty"`
	Country      string `json:"country,omitempty"`
}
type WalletTransaction struct {
	ID        int64   `json:"id"`
	Type      string  `json:"type"`
	Amount    float64 `json:"amount"`
	Currency  string  `json:"currency"`
	Status    string  `json:"status"`
	Reference string  `json:"reference"`
	Narration string  `json:"narration"`
	CreatedAt string  `json:"created_at"`
}
type WalletTransactionsData struct {
	Transactions []WalletTransaction `json:"transactions"`
}
type ListWalletsData struct {
	PayoutSubaccounts []SubaccountData `json:"payout_subaccounts"`
	Cursor            struct {
		Next         int  `json:"next"`
		Limit        int  `json:"limit"`
		HasMoreItems bool `json:"has_more_items"`
	} `json:"cursor"`
}
type CreateVirtualAccountRequest struct {
	Email       string `json:"email"`
	IsPermanent bool   `json:"is_permanent"`
	TxRef       string `json:"tx_ref"`
	PhoneNumber string `json:"phonenumber,omitempty"`
	FirstName   string `json:"firstname,omitempty"`
	LastName    string `json:"lastname,omitempty"`
	Narration   string `json:"narration,omitempty"`
	BVN         string `json:"bvn,omitempty"`
}
type VirtualAccountData struct {
	ResponseCode    string      `json:"response_code"`
	ResponseMessage string      `json:"response_message"`
	FlwRef          string      `json:"flw_ref"`
	OrderRef        string      `json:"order_ref"`
	AccountNumber   string      `json:"account_number"`
	Frequency       interface{} `json:"frequency"` // Can be string or number from API
	BankName        string      `json:"bank_name"`
	CreatedAt       string      `json:"created_at"`
	ExpiryDate      interface{} `json:"expiry_date"` // Can be string or null
	Note            string      `json:"note"`
	Amount          interface{} `json:"amount"` // Can be string or number
}
