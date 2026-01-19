package sms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"

	"kipa/internal/config"
	"kipa/internal/integration"
)

const (
	termiiBaseURL = "https://api.ng.termii.com/api"
)
type TermiiClient struct {
	httpClient          *http.Client
	apiKey              string
	senderID            string
	logger              *zap.Logger
	enableVoiceFallback bool
}
func NewTermiiClient(cfg *config.TermiiConfig, logger *zap.Logger) *TermiiClient {
	return &TermiiClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		apiKey:              cfg.APIKey,
		senderID:            cfg.SenderID,
		logger:              logger,
		enableVoiceFallback: cfg.EnableVoiceFallback,
	}
}
type termiiSMSRequest struct {
	To      string `json:"to"`
	From    string `json:"from"`
	SMS     string `json:"sms"`
	Type    string `json:"type"`
	Channel string `json:"channel"`
	APIKey  string `json:"api_key"`
}
type termiiVoiceTokenRequest struct {
	APIKey        string `json:"api_key"`
	PhoneNumber   string `json:"phone_number"`
	PinAttempts   int    `json:"pin_attempts"`
	PinTimeToLive int    `json:"pin_time_to_live"`
	PinLength     int    `json:"pin_length"`
}
type termiiVoiceTokenResponse struct {
	PinID   string `json:"pinId"`
	To      string `json:"to"`
	Status  string `json:"status"`
	Message string `json:"message"`
}
type termiiResponse struct {
	MessageID   string `json:"message_id"`
	Message     string `json:"message"`
	Balance     any    `json:"balance"`
	User        string `json:"user"`
	Code        string `json:"code"`
	MessageIDStr string `json:"message_id_str"`
}
func (c *TermiiClient) SendSMS(ctx context.Context, phoneNumber, message string) (*integration.SMSResult, error) {
	reqBody := termiiSMSRequest{
		To:      phoneNumber,
		From:    c.senderID,
		SMS:     message,
		Type:    "plain",
		Channel: "generic",
		APIKey:  c.apiKey,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, termiiBaseURL+"/sms/send", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("failed to send SMS via Termii",
			zap.String("phone", phoneNumber),
			zap.Error(err),
		)
		return &integration.SMSResult{
			Success:  false,
			Provider: "termii_sms",
			Error:    err,
		}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	c.logger.Debug("Termii SMS raw response",
		zap.Int("status_code", resp.StatusCode),
		zap.String("body", string(body)),
	)

	var result termiiResponse
	if err := json.Unmarshal(body, &result); err != nil {
		c.logger.Error("failed to parse Termii response",
			zap.Int("status_code", resp.StatusCode),
			zap.String("body", string(body)),
			zap.Error(err),
		)
		return &integration.SMSResult{
			Success:  false,
			Provider: "termii_sms",
			Error:    fmt.Errorf("failed to parse response: %s", string(body)),
		}, fmt.Errorf("termii error: %s", string(body))
	}
	if resp.StatusCode != http.StatusOK || (result.Code != "" && result.Code != "ok") {
		errMsg := result.Message
		if errMsg == "" {
			errMsg = string(body)
		}
		err := fmt.Errorf("termii error: %s (code: %s, http: %d)", errMsg, result.Code, resp.StatusCode)
		c.logger.Error("Termii SMS failed",
			zap.String("phone", phoneNumber),
			zap.Int("http_status", resp.StatusCode),
			zap.String("response_body", string(body)),
			zap.String("message", result.Message),
			zap.String("code", result.Code),
		)
		return &integration.SMSResult{
			Success:  false,
			Provider: "termii_sms",
			Error:    err,
		}, err
	}

	messageID := result.MessageID
	if messageID == "" {
		messageID = result.MessageIDStr
	}

	c.logger.Info("SMS sent successfully via Termii",
		zap.String("phone", phoneNumber),
		zap.String("message_id", messageID),
	)

	return &integration.SMSResult{
		Success:   true,
		MessageID: messageID,
		Provider:  "termii_sms",
	}, nil
}
func (c *TermiiClient) SendVoiceOTP(ctx context.Context, phoneNumber, otp string) (*integration.SMSResult, error) {
	return &integration.SMSResult{
		Success:  false,
		Provider: "termii_voice",
		Error:    fmt.Errorf("voice OTP not supported with Termii - use SMS"),
	}, fmt.Errorf("voice OTP not supported")
}
func (c *TermiiClient) SendWithFallback(ctx context.Context, phoneNumber, otp string) (*integration.SMSResult, error) {
	message := fmt.Sprintf("Your Kipa verification code is: %s", otp)

	result, err := c.SendSMS(ctx, phoneNumber, message)
	if err != nil {
		c.logger.Error("SMS delivery failed",
			zap.String("phone", phoneNumber),
			zap.Error(err),
		)
	}

	return result, err
}
