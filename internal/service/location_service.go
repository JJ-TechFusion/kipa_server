package service

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/domain"
	"kipa/internal/integration/maps"
	"kipa/internal/repository/postgres"
)

type LocationBroadcaster interface {
	BroadcastToJob(jobID string, msg interface{})
}

type LocationService interface {
	HandleLocationUpdate(ctx context.Context, riderID uuid.UUID, jobID string, lat, lng, heading, speed float64)
	RecordLocation(ctx context.Context, riderID uuid.UUID, jobID string, lat, lng, heading, speed float64)
	CleanupOldLocations(ctx context.Context, olderThanDays int) (int64, error)
}
type LocationServiceImpl struct {
	locationHistoryRepo *postgres.LocationHistoryRepo
	deliveryJobRepo     *postgres.DeliveryJobRepo
	mapsClient          maps.MapsClient
	etaLastCalc   map[string]time.Time
	etaThrottleMs int
	etaMu         sync.Mutex
	lastUpdate map[string]time.Time
	rateMu     sync.Mutex
	broadcaster LocationBroadcaster

	logger *zap.Logger
}
type LocationServiceConfig struct {
	ETAThrottleSeconds int // How often to recalculate ETA (default 30)
	RateLimitMs        int // Minimum ms between updates (default 1000)
}
func NewLocationService(
	locationHistoryRepo *postgres.LocationHistoryRepo,
	deliveryJobRepo *postgres.DeliveryJobRepo,
	mapsClient maps.MapsClient,
	broadcaster LocationBroadcaster,
	cfg *LocationServiceConfig,
	logger *zap.Logger,
) LocationService {
	if cfg == nil {
		cfg = &LocationServiceConfig{
			ETAThrottleSeconds: 30,
			RateLimitMs:        1000,
		}
	}

	return &LocationServiceImpl{
		locationHistoryRepo: locationHistoryRepo,
		deliveryJobRepo:     deliveryJobRepo,
		mapsClient:          mapsClient,
		etaThrottleMs:       cfg.ETAThrottleSeconds * 1000,
		lastUpdate:          make(map[string]time.Time),
		broadcaster:         broadcaster,
		logger:              logger,
	}
}

// HandleLocationUpdate handles location updates from external sources (e.g. HTTP)
// It checks rate limits, calculates ETA, saves to DB, AND broadcasts to WebSocket
func (s *LocationServiceImpl) HandleLocationUpdate(ctx context.Context, riderID uuid.UUID, jobID string, lat, lng, heading, speed float64) {
	s.RecordLocation(ctx, riderID, jobID, lat, lng, heading, speed)

	if s.broadcaster != nil {
		s.broadcaster.BroadcastToJob(jobID, &domain.RiderLocation{
			Type:      domain.TypeRiderLocation,
			JobID:     jobID,
			RiderID:   riderID.String(),
			Lat:       lat,
			Lng:       lng,
			Heading:   heading,
			Speed:     speed,
			Timestamp: time.Now().Unix(),
		})
	}
}

// RecordLocation handles location logic: rate limiting, ETA calculation, and DB storage
// This is called by HandleLocationUpdate (HTTP) and directly by the WebSocket Hub (to avoid loops)
func (s *LocationServiceImpl) RecordLocation(ctx context.Context, riderID uuid.UUID, jobID string, lat, lng, heading, speed float64) {
	clientKey := riderID.String()
	s.rateMu.Lock()
	lastUpdate, exists := s.lastUpdate[clientKey]
	if exists && time.Since(lastUpdate) < time.Second {
		s.rateMu.Unlock()
		return 
	}
	s.lastUpdate[clientKey] = time.Now()
	s.rateMu.Unlock()
	jobUUID, err := uuid.Parse(jobID)
	if err != nil {
		return
	}
	var etaMinutes *int
	var distanceRemaining *float64

	if s.mapsClient != nil {
		s.etaMu.Lock()
		lastCalc, calcExists := s.etaLastCalc[jobID]
		shouldCalc := !calcExists || time.Since(lastCalc) >= time.Duration(s.etaThrottleMs)*time.Millisecond
		if shouldCalc {
			s.etaLastCalc[jobID] = time.Now()
		}
		s.etaMu.Unlock()

		if shouldCalc {
			coords, err := s.deliveryJobRepo.GetDropoffCoordinates(ctx, jobUUID)
			if err == nil && coords != nil {
				eta, err := s.mapsClient.GetETA(ctx, lat, lng, coords.Lat, coords.Lng)
				if err == nil {
					etaMinutes = &eta.DurationMinutes
					distanceRemaining = &eta.DistanceKM
				}
			}
		}
	}
	_ = s.locationHistoryRepo.Create(ctx, &domain.LocationRecord{
		RiderID:             riderID,
		DeliveryJobID:       &jobUUID,
		Lat:                 lat,
		Lng:                 lng,
		Heading:             heading,
		Speed:               speed,
		ETAMinutes:          etaMinutes,
		DistanceRemainingKM: distanceRemaining,
	})
}
func (s *LocationServiceImpl) CleanupOldLocations(ctx context.Context, olderThanDays int) (int64, error) {
	before := time.Now().AddDate(0, 0, -olderThanDays)
	return s.locationHistoryRepo.DeleteOlderThan(ctx, before)
}
