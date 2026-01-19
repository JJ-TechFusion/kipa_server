package domain

import (
	"time"

	"github.com/google/uuid"
)
type DeliveryZone struct {
	ID      uuid.UUID `db:"id" json:"id"`
	Name    string    `db:"name" json:"name"`
	City    string    `db:"city" json:"city"`
	State   string    `db:"state" json:"state"`
	Polygon []byte    `db:"polygon" json:"-"` // PostGIS GEOGRAPHY POLYGON
	InitialSearchRadiusKM        int `db:"initial_search_radius_km" json:"initial_search_radius_km"`
	MaxSearchRadiusKM            int `db:"max_search_radius_km" json:"max_search_radius_km"`
	RadiusExpandIntervalSeconds  int `db:"radius_expand_interval_seconds" json:"radius_expand_interval_seconds"`
	BaseDeliveryFee float64 `db:"base_delivery_fee" json:"base_delivery_fee"`
	PerKMFee        float64 `db:"per_km_fee" json:"per_km_fee"`

	IsActive  bool      `db:"is_active" json:"is_active"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
func (z *DeliveryZone) CalculateDeliveryFee(distanceKM float64) float64 {
	return z.BaseDeliveryFee + (distanceKM * z.PerKMFee)
}
func (z *DeliveryZone) GetSearchTimeoutDuration() time.Duration {
	expansions := (z.MaxSearchRadiusKM - z.InitialSearchRadiusKM) / 2
	if expansions < 0 {
		expansions = 0
	}
	totalIntervals := expansions + 1
	return time.Duration(totalIntervals*z.RadiusExpandIntervalSeconds) * time.Second
}
