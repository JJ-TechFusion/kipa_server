package maps

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"go.uber.org/zap"
)

// GoogleMapsClient handles Google Maps API calls
type GoogleMapsClient struct {
	apiKey     string
	httpClient *http.Client
	logger     *zap.Logger
}

// Config for Google Maps
type Config struct {
	APIKey string
}

// NewGoogleMapsClient creates a new client
func NewGoogleMapsClient(cfg *Config, logger *zap.Logger) *GoogleMapsClient {
	return &GoogleMapsClient{
		apiKey: cfg.APIKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// ETAResult contains ETA and distance info
type ETAResult struct {
	DurationSeconds int     `json:"duration_seconds"`
	DurationMinutes int     `json:"duration_minutes"`
	DistanceMeters  int     `json:"distance_meters"`
	DistanceKM      float64 `json:"distance_km"`
	DurationText    string  `json:"duration_text"`
	DistanceText    string  `json:"distance_text"`
}

// DistanceMatrixResponse from Google API
type DistanceMatrixResponse struct {
	Rows []struct {
		Elements []struct {
			Status   string `json:"status"`
			Duration struct {
				Value int    `json:"value"`
				Text  string `json:"text"`
			} `json:"duration"`
			DurationInTraffic struct {
				Value int    `json:"value"`
				Text  string `json:"text"`
			} `json:"duration_in_traffic"`
			Distance struct {
				Value int    `json:"value"`
				Text  string `json:"text"`
			} `json:"distance"`
		} `json:"elements"`
	} `json:"rows"`
	Status string `json:"status"`
}

// GetETA calculates ETA from origin to destination
func (c *GoogleMapsClient) GetETA(ctx context.Context, originLat, originLng, destLat, destLng float64) (*ETAResult, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("google maps API key not configured")
	}

	baseURL := "https://maps.googleapis.com/maps/api/distancematrix/json"
	params := url.Values{
		"origins":        {fmt.Sprintf("%f,%f", originLat, originLng)},
		"destinations":   {fmt.Sprintf("%f,%f", destLat, destLng)},
		"mode":           {"driving"},
		"departure_time": {"now"}, // For traffic-aware ETA
		"key":            {c.apiKey},
	}

	reqURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call distance matrix API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("distance matrix API returned status %d", resp.StatusCode)
	}

	var result DistanceMatrixResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if result.Status != "OK" {
		return nil, fmt.Errorf("distance matrix API error: %s", result.Status)
	}

	if len(result.Rows) == 0 || len(result.Rows[0].Elements) == 0 {
		return nil, fmt.Errorf("no results from distance matrix API")
	}

	element := result.Rows[0].Elements[0]
	if element.Status != "OK" {
		return nil, fmt.Errorf("route not found: %s", element.Status)
	}

	// Prefer traffic-aware duration if available
	durationSeconds := element.Duration.Value
	durationText := element.Duration.Text
	if element.DurationInTraffic.Value > 0 {
		durationSeconds = element.DurationInTraffic.Value
		durationText = element.DurationInTraffic.Text
	}

	return &ETAResult{
		DurationSeconds: durationSeconds,
		DurationMinutes: durationSeconds / 60,
		DistanceMeters:  element.Distance.Value,
		DistanceKM:      float64(element.Distance.Value) / 1000,
		DurationText:    durationText,
		DistanceText:    element.Distance.Text,
	}, nil
}
