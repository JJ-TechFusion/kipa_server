package websocket

import (
	"time"

	"github.com/google/uuid"
)

// Message types
const (
	// Client → Server
	TypeLocationUpdate = "location_update"
	TypeSubscribe      = "subscribe"
	TypeUnsubscribe    = "unsubscribe"
	TypePing           = "ping"

	// Server → Client
	TypeRiderLocation   = "rider_location"
	TypeJobStatus       = "job_status"
	TypeNewJobAvailable = "new_job_available"
	TypePong            = "pong"
	TypeError           = "error"
)

// BaseMessage for type detection
type BaseMessage struct {
	Type string `json:"type"`
}

// --- Client → Server Messages ---

// LocationUpdate sent by rider
type LocationUpdate struct {
	Type    string  `json:"type"`
	JobID   string  `json:"job_id"`
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
	Heading float64 `json:"heading"` // 0-360 degrees
	Speed   float64 `json:"speed"`   // km/h
}

// Subscribe to a job's updates
type Subscribe struct {
	Type  string `json:"type"`
	JobID string `json:"job_id"`
}

// Unsubscribe from a job
type Unsubscribe struct {
	Type  string `json:"type"`
	JobID string `json:"job_id"`
}

// --- Server → Client Messages ---

// RiderLocation broadcast to users
type RiderLocation struct {
	Type      string  `json:"type"`
	JobID     string  `json:"job_id"`
	RiderID   string  `json:"rider_id"`
	Lat       float64 `json:"lat"`
	Lng       float64 `json:"lng"`
	Heading   float64 `json:"heading"`
	Speed     float64 `json:"speed"`
	ETA       int     `json:"eta_minutes,omitempty"`
	Timestamp int64   `json:"ts"`
}

// JobStatusUpdate sent when job status changes
type JobStatusUpdate struct {
	Type   string `json:"type"`
	JobID  string `json:"job_id"`
	Status string `json:"status"`
}

// NewJobAvailable sent to online riders
type NewJobAvailable struct {
	Type            string  `json:"type"`
	JobID           string  `json:"job_id"`
	PickupAddress   string  `json:"pickup_address"`
	DropoffAddress  string  `json:"dropoff_address"`
	DeliveryFee     float64 `json:"delivery_fee"`
	DistanceKM      float64 `json:"distance_km"`
	VehicleType     string  `json:"vehicle_type"`
	RiderTake       float64 `json:"rider_take"` // 85%
}

// ErrorMessage for errors
type ErrorMessage struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// --- Internal Types ---

// LocationRecord for database storage
type LocationRecord struct {
	ID                  uuid.UUID
	RiderID             uuid.UUID
	DeliveryJobID       *uuid.UUID
	Lat                 float64
	Lng                 float64
	Heading             float64
	Speed               float64
	ETAMinutes          *int
	DistanceRemainingKM *float64
	CreatedAt           time.Time
}
