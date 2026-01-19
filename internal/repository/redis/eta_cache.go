package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)
type ETACache struct {
	client *redis.Client
}
type CachedETA struct {
	ETAMinutes   int       `json:"eta_minutes"`
	DistanceKM   float64   `json:"distance_km"`
	RiderLat     float64   `json:"rider_lat"`
	RiderLng     float64   `json:"rider_lng"`
	UpdatedAt    time.Time `json:"updated_at"`
}
func NewETACache(client *redis.Client) *ETACache {
	return &ETACache{client: client}
}

func (c *ETACache) key(jobID uuid.UUID) string {
	return fmt.Sprintf("eta:%s", jobID.String())
}
func (c *ETACache) Set(ctx context.Context, jobID uuid.UUID, eta *CachedETA) error {
	eta.UpdatedAt = time.Now()
	
	data, err := json.Marshal(eta)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, c.key(jobID), data, 60*time.Second).Err()
}
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
func (c *ETACache) Delete(ctx context.Context, jobID uuid.UUID) error {
	return c.client.Del(ctx, c.key(jobID)).Err()
}
