package maps

import "context"

type MapsClient interface {
	GetETA(ctx context.Context, originLat, originLng, destLat, destLng float64) (*ETAResult, error)
}
var _ MapsClient = (*GoogleMapsClient)(nil)
