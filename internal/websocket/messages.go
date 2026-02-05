package websocket

import (
	"time"

	"github.com/google/uuid"
)
const (
	TypeLocationUpdate  = "location_update"
	TypeSubscribe       = "subscribe"
	TypeUnsubscribe     = "unsubscribe"
	TypePing            = "ping"
	TypeRiderLocation   = "rider_location"
	TypeJobStatus       = "job_status"
	TypeNewJobAvailable = "new_job_available"
	TypeNearbyRiders    = "nearby_riders"
	TypePong            = "pong"
	TypeError           = "error"
	TypeChatMessage     = "chat_message"
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

type ChatMessagePayload struct {
	Type      string `json:"type"`
	JobID     string `json:"job_id"`
	Content   string `json:"content"`
	MediaURL  string `json:"media_url,omitempty"`
	MediaType string `json:"media_type"`
	MessageID string `json:"message_id,omitempty"`
	SenderID  string `json:"sender_id,omitempty"`
	Timestamp int64  `json:"timestamp,omitempty"`
}

type NearbyRiderInfo struct {
	RiderID    string  `json:"rider_id"`
	Lat        float64 `json:"lat"`
	Lng        float64 `json:"lng"`
	Heading    float64 `json:"heading"`
	Speed      float64 `json:"speed"`
	DistanceKM float64 `json:"distance_km"`
}

type NearbyRidersMessage struct {
	Type   string            `json:"type"`
	JobID  string            `json:"job_id"`
	Riders []NearbyRiderInfo `json:"riders"`
}
