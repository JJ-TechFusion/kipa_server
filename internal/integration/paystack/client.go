package paystack

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"

	"kipa/internal/config"
)
type Client struct {
	httpClient *http.Client
	secretKey  string
	publicKey  string
	baseURL    string
	logger     *zap.Logger
}
func NewClient(cfg *config.PaystackConfig, logger *zap.Logger) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		secretKey: cfg.SecretKey,
		publicKey: cfg.PublicKey,
		baseURL:   cfg.BaseURL,
		logger:    logger,
	}
}
type TransactionStatus string

const (
	TransactionStatusSuccess   TransactionStatus = "success"
	TransactionStatusFailed    TransactionStatus = "failed"
	TransactionStatusAbandoned TransactionStatus = "abandoned"
	TransactionStatusPending   TransactionStatus = "pending"
)
type VerifyTransactionResponse struct {
	Status  bool             `json:"status"`
	Message string           `json:"message"`
	Data    *TransactionData `json:"data"`
}
type TransactionData struct {
	ID              int64             `json:"id"`
	Domain          string            `json:"domain"`
	Status          TransactionStatus `json:"status"`
	Reference       string            `json:"reference"`
	Amount          int64             `json:"amount"` // Amount in kobo (smallest currency unit)
	GatewayResponse string            `json:"gateway_response"`
	PaidAt          *time.Time        `json:"paid_at"`
	CreatedAt       time.Time         `json:"created_at"`
	Channel         string            `json:"channel"`
	Currency        string            `json:"currency"`
	IPAddress       string            `json:"ip_address"`
	Metadata        json.RawMessage   `json:"metadata"`
	Customer        CustomerData      `json:"customer"`
	Authorization   AuthorizationData `json:"authorization"`
}
type CustomerData struct {
	ID           int64  `json:"id"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	Email        string `json:"email"`
	CustomerCode string `json:"customer_code"`
	Phone        string `json:"phone"`
}
type AuthorizationData struct {
	AuthorizationCode string `json:"authorization_code"`
	Bin               string `json:"bin"`
	Last4             string `json:"last4"`
	ExpMonth          string `json:"exp_month"`
	ExpYear           string `json:"exp_year"`
	Channel           string `json:"channel"`
	CardType          string `json:"card_type"`
	Bank              string `json:"bank"`
	CountryCode       string `json:"country_code"`
	Brand             string `json:"brand"`
	Reusable          bool   `json:"reusable"`
	Signature         string `json:"signature"`
	AccountName       string `json:"account_name"`
}
type InitializeTransactionRequest struct {
	Email       string                 `json:"email"`
	Amount      int64                  `json:"amount"` // Amount in kobo
	Reference   string                 `json:"reference,omitempty"`
	CallbackURL string                 `json:"callback_url,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Channels    []string               `json:"channels,omitempty"`
}
type InitializeTransactionResponse struct {
	Status  bool            `json:"status"`
	Message string          `json:"message"`
	Data    *InitializeData `json:"data"`
}
type InitializeData struct {
	AuthorizationURL string `json:"authorization_url"`
	AccessCode       string `json:"access_code"`
	Reference        string `json:"reference"`
}
type WebhookEvent struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}
func (c *Client) VerifyTransaction(ctx context.Context, reference string) (*VerifyTransactionResponse, error) {
	url := fmt.Sprintf("%s/transaction/verify/%s", c.baseURL, reference)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.secretKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	c.logger.Debug("paystack verify response",
		zap.String("reference", reference),
		zap.Int("status_code", resp.StatusCode),
		zap.String("body", string(body)),
	)

	var result VerifyTransactionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Status {
		return &result, fmt.Errorf("paystack verification failed: %s", result.Message)
	}

	return &result, nil
}
func (c *Client) InitializeTransaction(ctx context.Context, req *InitializeTransactionRequest) (*InitializeTransactionResponse, error) {
	url := fmt.Sprintf("%s/transaction/initialize", c.baseURL)

	jsonBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.secretKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	c.logger.Debug("paystack initialize response",
		zap.Int("status_code", resp.StatusCode),
		zap.String("body", string(body)),
	)

	var result InitializeTransactionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Status {
		return &result, fmt.Errorf("paystack initialization failed: %s", result.Message)
	}

	return &result, nil
}
func (c *Client) VerifyWebhookSignature(body []byte, signature string) bool {
	if c.secretKey == "" {
		c.logger.Warn("secret key not configured, skipping signature verification")
		return false
	}

	mac := hmac.New(sha512.New, []byte(c.secretKey))
	mac.Write(body)
	expectedSignature := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expectedSignature), []byte(signature))
}
func (c *Client) ParseWebhookEvent(body []byte) (*WebhookEvent, error) {
	var event WebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, fmt.Errorf("failed to parse webhook event: %w", err)
	}
	return &event, nil
}
func (c *Client) GetTransactionDataFromEvent(event *WebhookEvent) (*TransactionData, error) {
	var data TransactionData
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return nil, fmt.Errorf("failed to parse transaction data: %w", err)
	}
	return &data, nil
}
func AmountToKobo(naira float64) int64 {
	return int64(naira * 100)
}
func KoboToNaira(kobo int64) float64 {
	return float64(kobo) / 100
}
func (c *Client) GetPublicKey() string {
	return c.publicKey
}
type CreateCustomerRequest struct {
	Email     string                 `json:"email"`
	FirstName string                 `json:"first_name,omitempty"`
	LastName  string                 `json:"last_name,omitempty"`
	Phone     string                 `json:"phone,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}
type CreateCustomerResponse struct {
	Status  bool                `json:"status"`
	Message string              `json:"message"`
	Data    *CustomerDetailData `json:"data"`
}
type CustomerDetailData struct {
	ID                       int64       `json:"id"`
	FirstName                string      `json:"first_name"`
	LastName                 string      `json:"last_name"`
	Email                    string      `json:"email"`
	CustomerCode             string      `json:"customer_code"`
	Phone                    string      `json:"phone"`
	Metadata                 interface{} `json:"metadata"`
	RiskAction               string      `json:"risk_action"`
	InternationalFormatPhone string      `json:"international_format_phone"`
}
func (c *Client) CreateCustomer(ctx context.Context, req *CreateCustomerRequest) (*CreateCustomerResponse, error) {
	url := fmt.Sprintf("%s/customer", c.baseURL)

	jsonBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.secretKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	c.logger.Debug("paystack create customer response",
		zap.Int("status_code", resp.StatusCode),
		zap.String("body", string(body)),
	)

	var result CreateCustomerResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Status {
		return &result, fmt.Errorf("failed to create customer: %s", result.Message)
	}

	return &result, nil
}
type CreateDVARequest struct {
	Customer      string `json:"customer"`                 // Customer ID or code
	PreferredBank string `json:"preferred_bank,omitempty"` // "wema-bank", "titan-paystack", etc.
}
type CreateDVAResponse struct {
	Status  bool     `json:"status"`
	Message string   `json:"message"`
	Data    *DVAData `json:"data"`
}
type DVAData struct {
	Bank            DVABankData     `json:"bank"`
	AccountName     string          `json:"account_name"`
	AccountNumber   string          `json:"account_number"`
	Assigned        bool            `json:"assigned"`
	Currency        string          `json:"currency"`
	Metadata        interface{}     `json:"metadata"`
	Active          bool            `json:"active"`
	ID              int64           `json:"id"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	Assignment      DVAAssignment   `json:"assignment"`
	Customer        CustomerData    `json:"customer"`
}
type DVABankData struct {
	Name string `json:"name"`
	ID   int64  `json:"id"`
	Slug string `json:"slug"`
}
type DVAAssignment struct {
	Integration  int64  `json:"integration"`
	AssigneeID   int64  `json:"assignee_id"`
	AssigneeType string `json:"assignee_type"`
	Expired      bool   `json:"expired"`
	AccountType  string `json:"account_type"`
}
func (c *Client) CreateDedicatedVirtualAccount(ctx context.Context, req *CreateDVARequest) (*CreateDVAResponse, error) {
	url := fmt.Sprintf("%s/dedicated_account", c.baseURL)

	jsonBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.secretKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	c.logger.Debug("paystack create DVA response",
		zap.Int("status_code", resp.StatusCode),
		zap.String("body", string(body)),
	)

	var result CreateDVAResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Status {
		return &result, fmt.Errorf("failed to create DVA: %s", result.Message)
	}

	return &result, nil
}
type ListBanksResponse struct {
	Status  bool       `json:"status"`
	Message string     `json:"message"`
	Data    []BankData `json:"data"`
}
type BankData struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	Code      string `json:"code"`
	Longcode  string `json:"longcode"`
	Gateway   string `json:"gateway"`
	Active    bool   `json:"active"`
	Country   string `json:"country"`
	Currency  string `json:"currency"`
	Type      string `json:"type"`
	IsDeleted bool   `json:"is_deleted"`
}
func (c *Client) ListBanks(ctx context.Context, country string) (*ListBanksResponse, error) {
	url := fmt.Sprintf("%s/bank?country=%s", c.baseURL, country)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.secretKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result ListBanksResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}
type ResolveAccountRequest struct {
	AccountNumber string `json:"account_number"`
	BankCode      string `json:"bank_code"`
}
type ResolveAccountResponse struct {
	Status  bool               `json:"status"`
	Message string             `json:"message"`
	Data    *ResolvedAccountData `json:"data"`
}
type ResolvedAccountData struct {
	AccountNumber string `json:"account_number"`
	AccountName   string `json:"account_name"`
	BankID        int64  `json:"bank_id"`
}
func (c *Client) ResolveAccountNumber(ctx context.Context, accountNumber, bankCode string) (*ResolveAccountResponse, error) {
	url := fmt.Sprintf("%s/bank/resolve?account_number=%s&bank_code=%s", c.baseURL, accountNumber, bankCode)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.secretKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	c.logger.Debug("paystack resolve account response",
		zap.Int("status_code", resp.StatusCode),
		zap.String("body", string(body)),
	)

	var result ResolveAccountResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Status {
		return &result, fmt.Errorf("failed to resolve account: %s", result.Message)
	}

	return &result, nil
}
type CreateTransferRecipientRequest struct {
	Type          string                 `json:"type"`           // "nuban" for Nigerian banks
	Name          string                 `json:"name"`           // Account name
	AccountNumber string                 `json:"account_number"`
	BankCode      string                 `json:"bank_code"`
	Currency      string                 `json:"currency"`       // "NGN"
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}
type CreateTransferRecipientResponse struct {
	Status  bool           `json:"status"`
	Message string         `json:"message"`
	Data    *RecipientData `json:"data"`
}
type RecipientData struct {
	Active        bool        `json:"active"`
	CreatedAt     time.Time   `json:"created_at"`
	Currency      string      `json:"currency"`
	Domain        string      `json:"domain"`
	ID            int64       `json:"id"`
	Integration   int64       `json:"integration"`
	Name          string      `json:"name"`
	RecipientCode string      `json:"recipient_code"`
	Type          string      `json:"type"`
	UpdatedAt     time.Time   `json:"updated_at"`
	IsDeleted     bool        `json:"is_deleted"`
	Details       BankDetails `json:"details"`
}
type BankDetails struct {
	AuthorizationCode string `json:"authorization_code"`
	AccountNumber     string `json:"account_number"`
	AccountName       string `json:"account_name"`
	BankCode          string `json:"bank_code"`
	BankName          string `json:"bank_name"`
}
func (c *Client) CreateTransferRecipient(ctx context.Context, req *CreateTransferRecipientRequest) (*CreateTransferRecipientResponse, error) {
	url := fmt.Sprintf("%s/transferrecipient", c.baseURL)

	jsonBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.secretKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	c.logger.Debug("paystack create recipient response",
		zap.Int("status_code", resp.StatusCode),
		zap.String("body", string(body)),
	)

	var result CreateTransferRecipientResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Status {
		return &result, fmt.Errorf("failed to create recipient: %s", result.Message)
	}

	return &result, nil
}
type InitiateTransferRequest struct {
	Source    string `json:"source"`    // "balance"
	Amount    int64  `json:"amount"`    // Amount in kobo
	Recipient string `json:"recipient"` // Recipient code
	Reason    string `json:"reason,omitempty"`
	Reference string `json:"reference,omitempty"`
}
type InitiateTransferResponse struct {
	Status  bool          `json:"status"`
	Message string        `json:"message"`
	Data    *TransferData `json:"data"`
}
type TransferData struct {
	Integration   int64     `json:"integration"`
	Domain        string    `json:"domain"`
	Amount        int64     `json:"amount"`
	Currency      string    `json:"currency"`
	Source        string    `json:"source"`
	Reason        string    `json:"reason"`
	Recipient     int64     `json:"recipient"`
	Status        string    `json:"status"`
	TransferCode  string    `json:"transfer_code"`
	ID            int64     `json:"id"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	Reference     string    `json:"reference"`
}
func (c *Client) InitiateTransfer(ctx context.Context, req *InitiateTransferRequest) (*InitiateTransferResponse, error) {
	url := fmt.Sprintf("%s/transfer", c.baseURL)

	jsonBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.secretKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	c.logger.Debug("paystack initiate transfer response",
		zap.Int("status_code", resp.StatusCode),
		zap.String("body", string(body)),
	)

	var result InitiateTransferResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Status {
		return &result, fmt.Errorf("failed to initiate transfer: %s", result.Message)
	}

	return &result, nil
}
func (c *Client) VerifyTransfer(ctx context.Context, reference string) (*InitiateTransferResponse, error) {
	url := fmt.Sprintf("%s/transfer/verify/%s", c.baseURL, reference)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.secretKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result InitiateTransferResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}
