package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

// ETACache caches ETA data per delivery job
type ETACache struct {
	client *redis.Client
}

// CachedETA represents cached ETA data
type CachedETA struct {
	ETAMinutes   int       `json:"eta_minutes"`
	DistanceKM   float64   `json:"distance_km"`
	RiderLat     float64   `json:"rider_lat"`
	RiderLng     float64   `json:"rider_lng"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// NewETACache creates a new ETA cache
func NewETACache(client *redis.Client) *ETACache {
	return &ETACache{client: client}
}

func (c *ETACache) key(jobID uuid.UUID) string {
	return fmt.Sprintf("eta:%s", jobID.String())
}

// Set caches ETA for a job
func (c *ETACache) Set(ctx context.Context, jobID uuid.UUID, eta *CachedETA) error {
	eta.UpdatedAt = time.Now()
	
	data, err := json.Marshal(eta)
	if err != nil {
		return err
	}
	
	// Cache for 60 seconds
	return c.client.Set(ctx, c.key(jobID), data, 60*time.Second).Err()
}

// Get retrieves cached ETA for a job
func (c *ETACache) Get(ctx context.Context, jobID uuid.UUID) (*CachedETA, error) {
	data, err := c.client.Get(ctx, c.key(jobID)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // Not found
		}
		return nil, err
	}
	
	var eta CachedETA
	if err := json.Unmarshal(data, &eta); err != nil {
		return nil, err
	}
	
	return &eta, nil
}

// Delete removes cached ETA for a job
func (c *ETACache) Delete(ctx context.Context, jobID uuid.UUID) error {
	return c.client.Del(ctx, c.key(jobID)).Err()
}
