package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

const (
	riderLocationsKey    = "rider_locations"
	riderLocationDataKey = "rider_location_data:"
	riderLocationTTL     = 5 * time.Minute // Rider location expires if not updated
)

type RiderLocation struct {
	RiderID   uuid.UUID `json:"rider_id"`
	Lat       float64   `json:"lat"`
	Lng       float64   `json:"lng"`
	Heading   float64   `json:"heading"`
	Speed     float64   `json:"speed"`
	UpdatedAt int64     `json:"updated_at"`
}

type NearbyRider struct {
	RiderID    uuid.UUID `json:"rider_id"`
	Lat        float64   `json:"lat"`
	Lng        float64   `json:"lng"`
	Heading    float64   `json:"heading"`
	Speed      float64   `json:"speed"`
	DistanceKM float64   `json:"distance_km"`
}

type RiderLocationRepo struct {
	client *redis.Client
}

func NewRiderLocationRepo(client *redis.Client) *RiderLocationRepo {
	return &RiderLocationRepo{client: client}
}
func (r *RiderLocationRepo) UpdateLocation(ctx context.Context, riderID uuid.UUID, lat, lng, heading, speed float64) error {
	pipe := r.client.Pipeline()
	pipe.GeoAdd(ctx, riderLocationsKey, &redis.GeoLocation{
		Name:      riderID.String(),
		Longitude: lng,
		Latitude:  lat,
	})
	dataKey := riderLocationDataKey + riderID.String()
	pipe.HSet(ctx, dataKey, map[string]interface{}{
		"heading":    heading,
		"speed":      speed,
		"updated_at": time.Now().Unix(),
	})
	pipe.Expire(ctx, dataKey, riderLocationTTL)

	_, err := pipe.Exec(ctx)
	return err
}
func (r *RiderLocationRepo) RemoveLocation(ctx context.Context, riderID uuid.UUID) error {
	pipe := r.client.Pipeline()
	pipe.ZRem(ctx, riderLocationsKey, riderID.String())
	pipe.Del(ctx, riderLocationDataKey+riderID.String())
	_, err := pipe.Exec(ctx)
	return err
}
func (r *RiderLocationRepo) GetNearbyRiders(ctx context.Context, lat, lng, radiusKM float64, limit int) ([]NearbyRider, error) {
	locations, err := r.client.GeoRadius(ctx, riderLocationsKey, lng, lat, &redis.GeoRadiusQuery{
		Radius:    radiusKM,
		Unit:      "km",
		WithCoord: true,
		WithDist:  true,
		Count:     limit,
		Sort:      "ASC", // Nearest first
	}).Result()

	if err != nil {
		return nil, fmt.Errorf("failed to query nearby riders: %w", err)
	}

	riders := make([]NearbyRider, 0, len(locations))
	for _, loc := range locations {
		riderID, err := uuid.Parse(loc.Name)
		if err != nil {
			continue
		}

		rider := NearbyRider{
			RiderID:    riderID,
			Lat:        loc.Latitude,
			Lng:        loc.Longitude,
			DistanceKM: loc.Dist,
		}
		dataKey := riderLocationDataKey + riderID.String()
		data, err := r.client.HGetAll(ctx, dataKey).Result()
		if err == nil && len(data) > 0 {
			if heading, ok := data["heading"]; ok {
				fmt.Sscanf(heading, "%f", &rider.Heading)
			}
			if speed, ok := data["speed"]; ok {
				fmt.Sscanf(speed, "%f", &rider.Speed)
			}
		}

		riders = append(riders, rider)
	}

	return riders, nil
}
func (r *RiderLocationRepo) GetRiderLocation(ctx context.Context, riderID uuid.UUID) (*RiderLocation, error) {
	positions, err := r.client.GeoPos(ctx, riderLocationsKey, riderID.String()).Result()
	if err != nil {
		return nil, err
	}

	if len(positions) == 0 || positions[0] == nil {
		return nil, nil
	}

	loc := &RiderLocation{
		RiderID: riderID,
		Lat:     positions[0].Latitude,
		Lng:     positions[0].Longitude,
	}
	dataKey := riderLocationDataKey + riderID.String()
	data, err := r.client.HGetAll(ctx, dataKey).Result()
	if err == nil && len(data) > 0 {
		if heading, ok := data["heading"]; ok {
			fmt.Sscanf(heading, "%f", &loc.Heading)
		}
		if speed, ok := data["speed"]; ok {
			fmt.Sscanf(speed, "%f", &loc.Speed)
		}
		if updatedAt, ok := data["updated_at"]; ok {
			fmt.Sscanf(updatedAt, "%d", &loc.UpdatedAt)
		}
	}

	return loc, nil
}
