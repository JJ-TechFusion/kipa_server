package flutterwave

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

func NewClient(cfg *config.FlutterwaveConfig, logger *zap.Logger) *Client {
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

func (c *Client) doRequest(ctx context.Context, method, endpoint string, body interface{}, queryParams map[string]string) ([]byte, error) {
	fullURL := fmt.Sprintf("%s%s", c.baseURL, endpoint)

	if len(queryParams) > 0 {
		u, err := url.Parse(fullURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse URL: %w", err)
		}
		q := u.Query()
		for k, v := range queryParams {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
		fullURL = u.String()
	}

	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, reqBody)
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

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	c.logger.Debug("flutterwave API response",
		zap.String("endpoint", endpoint),
		zap.Int("status_code", resp.StatusCode),
		zap.String("body", string(respBody)),
	)

	if resp.StatusCode >= 400 {
		return respBody, fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func (c *Client) InitializePayment(ctx context.Context, req *InitializePaymentRequest) (*APIResponse[InitializePaymentData], error) {
	respBody, err := c.doRequest(ctx, http.MethodPost, "/payments", req, nil)
	if err != nil {
		return nil, err
	}

	var result APIResponse[InitializePaymentData]
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Status != "success" {
		return &result, fmt.Errorf("flutterwave payment initialization failed: %s", result.Message)
	}

	return &result, nil
}

func (c *Client) VerifyTransaction(ctx context.Context, txRef string) (*APIResponse[VerifyTransactionData], error) {
	endpoint := fmt.Sprintf("/transactions/verify_by_reference?tx_ref=%s", url.QueryEscape(txRef))
	respBody, err := c.doRequest(ctx, http.MethodGet, endpoint, nil, nil)
	if err != nil {
		return nil, err
	}

	var result APIResponse[VerifyTransactionData]
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

func (c *Client) CreateSubaccount(ctx context.Context, req *CreateSubaccountRequest) (*APIResponse[SubaccountData], error) {
	respBody, err := c.doRequest(ctx, http.MethodPost, "/payout-subaccounts", req, nil)
	if err != nil {
		return nil, err
	}

	var result APIResponse[SubaccountData]
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Status != "success" {
		return &result, fmt.Errorf("failed to create subaccount: %s", result.Message)
	}

	return &result, nil
}

func (c *Client) GetSubaccountBalance(ctx context.Context, accountRef string) (*APIResponse[SubaccountBalanceData], error) {
	endpoint := fmt.Sprintf("/payout-subaccounts/%s/balances", url.PathEscape(accountRef))
	respBody, err := c.doRequest(ctx, http.MethodGet, endpoint, nil, map[string]string{"currency": "NGN"})
	if err != nil {
		return nil, err
	}

	var result APIResponse[SubaccountBalanceData]
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

func (c *Client) FundSubaccount(ctx context.Context, accountRef string, amount float64, reference, narration string) (*APIResponse[FundSubaccountData], error) {
	req := &FundSubaccountRequest{
		AccountReference: accountRef,
		Amount:           amount,
		Currency:         "NGN",
		Reference:        reference,
		Narration:        narration,
	}

	respBody, err := c.doRequest(ctx, http.MethodPost, "/payout-subaccounts/transfer", req, nil)
	if err != nil {
		return nil, err
	}

	var result APIResponse[FundSubaccountData]
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Status != "success" {
		return &result, fmt.Errorf("failed to fund subaccount: %s", result.Message)
	}

	return &result, nil
}


func (c *Client) InitiateTransfer(ctx context.Context, req *InitiateTransferRequest) (*APIResponse[TransferData], error) {
	if req.Currency == "" {
		req.Currency = "NGN"
	}

	respBody, err := c.doRequest(ctx, http.MethodPost, "/transfers", req, nil)
	if err != nil {
		return nil, err
	}

	var result APIResponse[TransferData]
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Status != "success" {
		return &result, fmt.Errorf("failed to initiate transfer: %s", result.Message)
	}

	return &result, nil
}

func (c *Client) GetTransfer(ctx context.Context, transferID int64) (*APIResponse[GetTransferData], error) {
	endpoint := fmt.Sprintf("/transfers/%d", transferID)
	respBody, err := c.doRequest(ctx, http.MethodGet, endpoint, nil, nil)
	if err != nil {
		return nil, err
	}

	var result APIResponse[GetTransferData]
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

func (c *Client) ResolveBankAccount(ctx context.Context, accountNumber, bankCode string) (*APIResponse[ResolveAccountData], error) {
	req := map[string]string{
		"account_number": accountNumber,
		"account_bank":   bankCode,
	}

	respBody, err := c.doRequest(ctx, http.MethodPost, "/accounts/resolve", req, nil)
	if err != nil {
		return nil, err
	}

	var result APIResponse[ResolveAccountData]
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Status != "success" {
		return &result, fmt.Errorf("failed to resolve account: %s", result.Message)
	}

	return &result, nil
}

func (c *Client) ListBanks(ctx context.Context, country string) (*APIResponse[[]BankData], error) {
	endpoint := fmt.Sprintf("/banks/%s", country)
	respBody, err := c.doRequest(ctx, http.MethodGet, endpoint, nil, nil)
	if err != nil {
		return nil, err
	}

	var result APIResponse[[]BankData]
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

func (c *Client) VerifyWebhookSignature(headerHash string) bool {
	if c.secretKey == "" {
		c.logger.Warn("secret key not configured, skipping signature verification")
		return false
	}

	return headerHash == c.secretKey
}

func (c *Client) ParseWebhookEvent(body []byte) (*WebhookEvent, error) {
	var event WebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, fmt.Errorf("failed to parse webhook event: %w", err)
	}
	return &event, nil
}

func (c *Client) GetChargeDataFromEvent(event *WebhookEvent) (*ChargeWebhookData, error) {
	var data ChargeWebhookData
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return nil, fmt.Errorf("failed to parse charge data: %w", err)
	}
	return &data, nil
}

func (c *Client) GetTransferDataFromEvent(event *WebhookEvent) (*TransferWebhookData, error) {
	var data TransferWebhookData
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return nil, fmt.Errorf("failed to parse transfer data: %w", err)
	}
	return &data, nil
}

func (c *Client) GetPublicKey() string {
	return c.publicKey
}

func AmountToKobo(naira float64) int64 {
	return int64(naira * 100)
}

// KoboToNaira converts kobo to naira
func KoboToNaira(kobo int64) float64 {
	return float64(kobo) / 100
}
