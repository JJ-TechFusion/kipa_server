package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

type Config struct {
	BotToken string
	ChatID   string
	Enabled  bool
}

type Client struct {
	config     *Config
	httpClient *http.Client
	logger     *zap.Logger
}

func NewClient(cfg *Config, logger *zap.Logger) *Client {
	return &Client{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

type sendMessageRequest struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode,omitempty"`
}

type sendMessageResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description,omitempty"`
}

func (c *Client) SendMessage(ctx context.Context, text string) error {
	if c.config == nil || !c.config.Enabled || c.config.BotToken == "" || c.config.ChatID == "" {
		return nil
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", c.config.BotToken)

	payload := sendMessageRequest{
		ChatID:    c.config.ChatID,
		Text:      text,
		ParseMode: "HTML",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	var result sendMessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.OK {
		return fmt.Errorf("telegram API error: %s", result.Description)
	}

	c.logger.Debug("telegram message sent",
		zap.String("chat_id", c.config.ChatID),
	)

	return nil
}

func (c *Client) IsEnabled() bool {
	return c.config != nil && c.config.Enabled && c.config.BotToken != "" && c.config.ChatID != ""
}
