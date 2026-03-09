package worker

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/domain"
	"kipa/internal/websocket"
)

type NearbyRidersFinder interface {
	GetNearbyRiders(ctx context.Context, lat, lng, radiusKM float64, limit int) ([]NearbyRider, error)
}

type NearbyRider struct {
	RiderID    uuid.UUID
	Lat        float64
	Lng        float64
	Heading    float64
	Speed      float64
	DistanceKM float64
}

type NearbyRidersBroadcaster interface {
	BroadcastNearbyRiders(jobID string, riders []websocket.NearbyRiderInfo)
}

func (p *JobProcessor) SetNearbyRidersFinder(finder NearbyRidersFinder) {
	p.nearbyRidersFinder = finder
}

func (p *JobProcessor) SetNearbyRidersBroadcaster(broadcaster NearbyRidersBroadcaster) {
	p.nearbyRidersBroadcaster = broadcaster
}

func (p *JobProcessor) runNearbyRidersJob(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second) // Broadcast every 3 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("nearby riders job: context cancelled")
			return
		case <-p.stopCh:
			p.logger.Info("nearby riders job: stop signal received")
			return
		case <-ticker.C:
			p.broadcastNearbyRidersToSearchingJobs(ctx)
		}
	}
}

func (p *JobProcessor) broadcastNearbyRidersToSearchingJobs(ctx context.Context) {
	if p.nearbyRidersFinder == nil || p.nearbyRidersBroadcaster == nil {
		return
	}

	if p.deliveryJobRepo == nil {
		return
	}
	jobs, err := p.deliveryJobRepo.GetSearchingJobs(ctx, 100)
	if err != nil {
		p.logger.Error("nearby riders job: failed to get searching jobs", zap.Error(err))
		return
	}

	if len(jobs) == 0 {
		return
	}

	for _, job := range jobs {
		p.broadcastNearbyRidersForJob(ctx, job)
	}
}

func (p *JobProcessor) broadcastNearbyRidersForJob(ctx context.Context, job *domain.DeliveryJob) {
	coords, err := p.deliveryJobRepo.GetJobCoordinates(ctx, job.ID)
	if err != nil || coords == nil {
		return
	}
	radiusKM := float64(job.SearchRadiusKM)
	if radiusKM == 0 {
		radiusKM = 5.0 // Default 5km
	}
	riders, err := p.nearbyRidersFinder.GetNearbyRiders(ctx, coords.PickupLat, coords.PickupLng, radiusKM, 20)
	if err != nil {
		p.logger.Warn("nearby riders job: failed to find nearby riders",
			zap.String("job_id", job.ID.String()),
			zap.Error(err),
		)
		return
	}
	wsRiders := make([]websocket.NearbyRiderInfo, len(riders))
	for i, r := range riders {
		wsRiders[i] = websocket.NearbyRiderInfo{
			RiderID:    r.RiderID.String(),
			Lat:        r.Lat,
			Lng:        r.Lng,
			Heading:    r.Heading,
			Speed:      r.Speed,
			DistanceKM: r.DistanceKM,
		}
	}
	p.nearbyRidersBroadcaster.BroadcastNearbyRiders(job.ID.String(), wsRiders)
}
