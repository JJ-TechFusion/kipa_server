package websocket

import (
	"time"

	"github.com/google/uuid"
)
const (
	TypeLocationUpdate = "location_update"
	TypeSubscribe      = "subscribe"
	TypeUnsubscribe    = "unsubscribe"
	TypePing           = "ping"
	TypeRiderLocation   = "rider_location"
	TypeJobStatus       = "job_status"
	TypeNewJobAvailable = "new_job_available"
	TypePong            = "pong"
	TypeError           = "error"
)
type BaseMessage struct {
	Type string `json:"type"`
}
type LocationUpdate struct {
	Type    string  `json:"type"`
	JobID   string  `json:"job_id"`
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
	Heading float64 `json:"heading"` // 0-360 degrees
	Speed   float64 `json:"speed"`   // km/h
}
type Subscribe struct {
	Type  string `json:"type"`
	JobID string `json:"job_id"`
}
type Unsubscribe struct {
	Type  string `json:"type"`
	JobID string `json:"job_id"`
}

type JobStatusUpdate struct {
	Type   string `json:"type"`
	JobID  string `json:"job_id"`
	Status string `json:"status"`
}
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
type ErrorMessage struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}
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
