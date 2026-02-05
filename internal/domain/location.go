package domain

import (
	"time"

	"github.com/google/uuid"
)

type LocationRecord struct {
	ID                  uuid.UUID  `db:"id"`
	RiderID             uuid.UUID  `db:"rider_id"`
	DeliveryJobID       *uuid.UUID `db:"delivery_job_id"`
	Lat                 float64    `db:"lat"`
	Lng                 float64    `db:"lng"`
	Heading             float64    `db:"heading"`
	Speed               float64    `db:"speed"`
	ETAMinutes          *int       `db:"eta_minutes"`
	DistanceRemainingKM *float64   `db:"distance_remaining_km"`
	CreatedAt           time.Time  `db:"created_at"`
}

type RiderLocation struct {
	Type                string   `json:"type"`
	JobID               string   `json:"job_id"`
	RiderID             string   `json:"rider_id"`
	Lat                 float64  `json:"lat"`
	Lng                 float64  `json:"lng"`
	Heading             float64  `json:"heading"`
	Speed               float64  `json:"speed"`
	ETAMinutes          *int     `json:"eta_minutes,omitempty"`
	DistanceRemainingKM *float64 `json:"distance_remaining_km,omitempty"`
	Timestamp           int64    `json:"ts"`
}

const (
	TypeRiderLocation = "rider_location"
)
