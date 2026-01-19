package maps

import "context"

type MapsClient interface {
	// GetETA calculates estimated time of arrival from origin to destination
	GetETA(ctx context.Context, originLat, originLng, destLat, destLng float64) (*ETAResult, error)
}

// Ensure GoogleMapsClient implements MapsClient
var _ MapsClient = (*GoogleMapsClient)(nil)
