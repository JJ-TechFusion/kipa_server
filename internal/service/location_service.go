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

// LocationService handles rider location updates, ETA calculation, and broadcasting
type LocationService interface {
	HandleLocationUpdate(ctx context.Context, riderID uuid.UUID, jobID string, lat, lng, heading, speed float64)
	CleanupOldLocations(ctx context.Context, olderThanDays int) (int64, error)
}

// LocationServiceImpl implements LocationService
type LocationServiceImpl struct {
	locationHistoryRepo *postgres.LocationHistoryRepo
	deliveryJobRepo     *postgres.DeliveryJobRepo
	mapsClient          maps.MapsClient

	// ETA throttling
	etaLastCalc   map[string]time.Time
	etaThrottleMs int
	etaMu         sync.Mutex

	// Rate limiting (1 update per second per client)
	lastUpdate map[string]time.Time
	rateMu     sync.Mutex

	logger *zap.Logger
}

// LocationServiceConfig configures the location service
type LocationServiceConfig struct {
	ETAThrottleSeconds int // How often to recalculate ETA (default 30)
	RateLimitMs        int // Minimum ms between updates (default 1000)
}

// NewLocationService creates a new location service
func NewLocationService(
	locationHistoryRepo *postgres.LocationHistoryRepo,
	deliveryJobRepo *postgres.DeliveryJobRepo,
	mapsClient maps.MapsClient,
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
		etaLastCalc:         make(map[string]time.Time),
		etaThrottleMs:       cfg.ETAThrottleSeconds * 1000,
		lastUpdate:          make(map[string]time.Time),
		logger:              logger,
	}
}

// HandleLocationUpdate processes a location update from a rider
func (s *LocationServiceImpl) HandleLocationUpdate(ctx context.Context, riderID uuid.UUID, jobID string, lat, lng, heading, speed float64) {
	// Rate limiting: max 1 update per second per rider
	clientKey := riderID.String()
	s.rateMu.Lock()
	lastUpdate, exists := s.lastUpdate[clientKey]
	if exists && time.Since(lastUpdate) < time.Second {
		s.rateMu.Unlock()
		return 
	}
	s.lastUpdate[clientKey] = time.Now()
	s.rateMu.Unlock()

	// Parse job ID
	jobUUID, err := uuid.Parse(jobID)
	if err != nil {
		return
	}

	// Calculate ETA (throttled)
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

	// Store location
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

// CleanupOldLocations removes location history older than given days
func (s *LocationServiceImpl) CleanupOldLocations(ctx context.Context, olderThanDays int) (int64, error) {
	before := time.Now().AddDate(0, 0, -olderThanDays)
	return s.locationHistoryRepo.DeleteOlderThan(ctx, before)
}
