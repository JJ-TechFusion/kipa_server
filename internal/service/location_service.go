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
	BroadcastJobStatus(jobID, status string)
}

type LocationService interface {
	HandleLocationUpdate(ctx context.Context, riderID uuid.UUID, jobID string, lat, lng, heading, speed float64)
	RecordLocation(ctx context.Context, riderID uuid.UUID, jobID string, lat, lng, heading, speed float64) *LocationETAResult
	CleanupOldLocations(ctx context.Context, olderThanDays int) (int64, error)
}

type LocationETAResult struct {
	ETAMinutes          *int
	DistanceRemainingKM *float64
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
		etaLastCalc:         make(map[string]time.Time),
		lastUpdate:          make(map[string]time.Time),
		broadcaster:         broadcaster,
		logger:              logger,
	}
}
func (s *LocationServiceImpl) HandleLocationUpdate(ctx context.Context, riderID uuid.UUID, jobID string, lat, lng, heading, speed float64) {
	etaResult := s.RecordLocation(ctx, riderID, jobID, lat, lng, heading, speed)

	if s.broadcaster != nil {
		location := &domain.RiderLocation{
			Type:      domain.TypeRiderLocation,
			JobID:     jobID,
			RiderID:   riderID.String(),
			Lat:       lat,
			Lng:       lng,
			Heading:   heading,
			Speed:     speed,
			Timestamp: time.Now().Unix(),
		}
		if etaResult != nil {
			location.ETAMinutes = etaResult.ETAMinutes
			location.DistanceRemainingKM = etaResult.DistanceRemainingKM
		}
		s.broadcaster.BroadcastToJob(jobID, location)
	}
}
func (s *LocationServiceImpl) RecordLocation(ctx context.Context, riderID uuid.UUID, jobID string, lat, lng, heading, speed float64) *LocationETAResult {
	clientKey := riderID.String()
	s.rateMu.Lock()
	lastUpdate, exists := s.lastUpdate[clientKey]
	if exists && time.Since(lastUpdate) < time.Second {
		s.rateMu.Unlock()
		return nil
	}
	s.lastUpdate[clientKey] = time.Now()
	s.rateMu.Unlock()
	jobUUID, err := uuid.Parse(jobID)
	if err != nil {
		return nil
	}
	s.autoUpdateJobStatus(ctx, jobUUID, jobID, speed)

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
			job, err := s.deliveryJobRepo.GetByID(ctx, jobUUID)
			if err == nil && job != nil {
				coords, err := s.deliveryJobRepo.GetJobCoordinates(ctx, jobUUID)
				if err == nil && coords != nil {
					var destLat, destLng float64
					if job.Status == domain.DJStatusAccepted || job.Status == domain.DJStatusEnRoutePickup {
						destLat, destLng = coords.PickupLat, coords.PickupLng
					} else {
						destLat, destLng = coords.DropoffLat, coords.DropoffLng
					}
					eta, err := s.mapsClient.GetETA(ctx, lat, lng, destLat, destLng)
					if err == nil {
						etaMinutes = &eta.DurationMinutes
						distanceRemaining = &eta.DistanceKM
					}
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

	return &LocationETAResult{
		ETAMinutes:          etaMinutes,
		DistanceRemainingKM: distanceRemaining,
	}
}
func (s *LocationServiceImpl) CleanupOldLocations(ctx context.Context, olderThanDays int) (int64, error) {
	before := time.Now().AddDate(0, 0, -olderThanDays)
	return s.locationHistoryRepo.DeleteOlderThan(ctx, before)
}
const minMovingSpeedKmh = 5.0

func (s *LocationServiceImpl) autoUpdateJobStatus(ctx context.Context, jobUUID uuid.UUID, jobID string, speed float64) {
	if speed < minMovingSpeedKmh {
		return
	}

	job, err := s.deliveryJobRepo.GetByID(ctx, jobUUID)
	if err != nil {
		return
	}

	var newStatus domain.DeliveryJobStatus

	switch job.Status {
	case domain.DJStatusAccepted:
		newStatus = domain.DJStatusEnRoutePickup
	case domain.DJStatusPickedUp:
		newStatus = domain.DJStatusInTransit
	default:
		return // No auto-transition needed
	}
	if err := s.deliveryJobRepo.UpdateStatus(ctx, jobUUID, newStatus); err != nil {
		s.logger.Warn("failed to auto-update job status",
			zap.String("job_id", jobID),
			zap.String("new_status", string(newStatus)),
			zap.Error(err),
		)
		return
	}

	s.logger.Info("auto-updated job status based on movement",
		zap.String("job_id", jobID),
		zap.String("old_status", string(job.Status)),
		zap.String("new_status", string(newStatus)),
		zap.Float64("speed_kmh", speed),
	)
	if s.broadcaster != nil {
		s.broadcaster.BroadcastJobStatus(jobID, string(newStatus))
	}
}
