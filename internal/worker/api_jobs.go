package worker

import (
	"context"
	"time"

	"go.uber.org/zap"

	"kipa/internal/domain"
	"kipa/internal/repository"
	"kipa/internal/websocket"
)
type APIJobRunner struct {
	deliveryJobRepo         repository.DeliveryJobRepository
	nearbyRidersFinder      NearbyRidersFinder
	nearbyRidersBroadcaster NearbyRidersBroadcaster
	logger                  *zap.Logger
	stopCh                  chan struct{}
}

func NewAPIJobRunner(
	deliveryJobRepo repository.DeliveryJobRepository,
	nearbyRidersFinder NearbyRidersFinder,
	nearbyRidersBroadcaster NearbyRidersBroadcaster,
	logger *zap.Logger,
) *APIJobRunner {
	return &APIJobRunner{
		deliveryJobRepo:         deliveryJobRepo,
		nearbyRidersFinder:      nearbyRidersFinder,
		nearbyRidersBroadcaster: nearbyRidersBroadcaster,
		logger:                  logger.Named("api-jobs"),
		stopCh:                  make(chan struct{}),
	}
}

func (r *APIJobRunner) Start(ctx context.Context) {
	r.logger.Info("starting API background jobs")
	go r.runNearbyRidersJob(ctx)
}

func (r *APIJobRunner) Stop() {
	close(r.stopCh)
	r.logger.Info("API background jobs stopped")
}

func (r *APIJobRunner) runNearbyRidersJob(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("nearby riders job: context cancelled")
			return
		case <-r.stopCh:
			r.logger.Info("nearby riders job: stop signal received")
			return
		case <-ticker.C:
			r.broadcastNearbyRidersToSearchingJobs(ctx)
		}
	}
}

func (r *APIJobRunner) broadcastNearbyRidersToSearchingJobs(ctx context.Context) {
	if r.nearbyRidersFinder == nil || r.nearbyRidersBroadcaster == nil {
		return
	}
	jobs, err := r.deliveryJobRepo.GetSearchingJobs(ctx, 100)
	if err != nil {
		r.logger.Error("nearby riders job: failed to get searching jobs", zap.Error(err))
		return
	}

	if len(jobs) == 0 {
		return
	}

	for _, job := range jobs {
		r.broadcastNearbyRidersForJob(ctx, job)
	}
}

func (r *APIJobRunner) broadcastNearbyRidersForJob(ctx context.Context, job *domain.DeliveryJob) {
	coords, err := r.deliveryJobRepo.GetJobCoordinates(ctx, job.ID)
	if err != nil || coords == nil {
		return
	}
	radiusKM := float64(job.SearchRadiusKM)
	if radiusKM == 0 {
		radiusKM = 5.0 // Default 5km
	}
	riders, err := r.nearbyRidersFinder.GetNearbyRiders(ctx, coords.PickupLat, coords.PickupLng, radiusKM, 20)
	if err != nil {
		r.logger.Warn("nearby riders job: failed to find nearby riders",
			zap.String("job_id", job.ID.String()),
			zap.Error(err),
		)
		return
	}
	wsRiders := make([]websocket.NearbyRiderInfo, len(riders))
	for i, rd := range riders {
		wsRiders[i] = websocket.NearbyRiderInfo{
			RiderID:    rd.RiderID.String(),
			Lat:        rd.Lat,
			Lng:        rd.Lng,
			Heading:    rd.Heading,
			Speed:      rd.Speed,
			DistanceKM: rd.DistanceKM,
		}
	}
	r.nearbyRidersBroadcaster.BroadcastNearbyRiders(job.ID.String(), wsRiders)
}
