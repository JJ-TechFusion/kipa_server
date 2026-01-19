package domain

import (
	"time"

	"github.com/google/uuid"
)

// DeliveryZone represents a geographic area with delivery configuration
type DeliveryZone struct {
	ID      uuid.UUID `db:"id" json:"id"`
	Name    string    `db:"name" json:"name"`
	City    string    `db:"city" json:"city"`
	State   string    `db:"state" json:"state"`
	Polygon []byte    `db:"polygon" json:"-"` // PostGIS GEOGRAPHY POLYGON

	// Rider search configuration
	InitialSearchRadiusKM        int `db:"initial_search_radius_km" json:"initial_search_radius_km"`
	MaxSearchRadiusKM            int `db:"max_search_radius_km" json:"max_search_radius_km"`
	RadiusExpandIntervalSeconds  int `db:"radius_expand_interval_seconds" json:"radius_expand_interval_seconds"`

	// Fee configuration
	BaseDeliveryFee float64 `db:"base_delivery_fee" json:"base_delivery_fee"`
	PerKMFee        float64 `db:"per_km_fee" json:"per_km_fee"`

	IsActive  bool      `db:"is_active" json:"is_active"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// CalculateDeliveryFee calculates the estimated delivery fee based on distance
func (z *DeliveryZone) CalculateDeliveryFee(distanceKM float64) float64 {
	return z.BaseDeliveryFee + (distanceKM * z.PerKMFee)
}

// GetSearchTimeoutDuration returns the total time for rider search before timeout
func (z *DeliveryZone) GetSearchTimeoutDuration() time.Duration {
	// Calculate how many expansions can happen
	expansions := (z.MaxSearchRadiusKM - z.InitialSearchRadiusKM) / 2
	if expansions < 0 {
		expansions = 0
	}
	// Add 1 for the initial search
	totalIntervals := expansions + 1
	return time.Duration(totalIntervals*z.RadiusExpandIntervalSeconds) * time.Second
}
