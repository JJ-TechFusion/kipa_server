package worker

import (
	"context"

	redisrepo "kipa/internal/repository/redis"
)

// RiderLocationAdapter adapts RiderLocationRepo to NearbyRidersFinder interface
type RiderLocationAdapter struct {
	repo *redisrepo.RiderLocationRepo
}

func NewRiderLocationAdapter(repo *redisrepo.RiderLocationRepo) *RiderLocationAdapter {
	return &RiderLocationAdapter{repo: repo}
}

func (a *RiderLocationAdapter) GetNearbyRiders(ctx context.Context, lat, lng, radiusKM float64, limit int) ([]NearbyRider, error) {
	redisRiders, err := a.repo.GetNearbyRiders(ctx, lat, lng, radiusKM, limit)
	if err != nil {
		return nil, err
	}

	riders := make([]NearbyRider, len(redisRiders))
	for i, r := range redisRiders {
		riders[i] = NearbyRider{
			RiderID:    r.RiderID,
			Lat:        r.Lat,
			Lng:        r.Lng,
			Heading:    r.Heading,
			Speed:      r.Speed,
			DistanceKM: r.DistanceKM,
		}
	}

	return riders, nil
}
