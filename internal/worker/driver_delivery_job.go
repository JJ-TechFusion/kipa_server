package worker

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/domain"
)

// DriverServiceInterface is the subset of DriverService needed by the worker
type DriverServiceInterface interface {
	// Phase 4: Auto-confirm
	GetExpiredConfirmationWindows(ctx context.Context) ([]*domain.DriverDelivery, error)
	AutoConfirmDelivery(ctx context.Context, deliveryID uuid.UUID) error

	// Phase 5: Edge cases
	GetExpiredUnreachableDeliveries(ctx context.Context) ([]*domain.DriverDelivery, error)
	ProcessExpiredUnreachable(ctx context.Context, deliveryID uuid.UUID) error
	GetStaleAwaitingDriver(ctx context.Context, hoursThreshold int) ([]*domain.DriverDelivery, error)
	ExpireAwaitingDriverDelivery(ctx context.Context, deliveryID uuid.UUID) error
	GetAbandonedTrips(ctx context.Context, hoursThreshold int) ([]*domain.DriverTrip, error)
	ProcessAbandonedTrip(ctx context.Context, tripID uuid.UUID) error
	GetStaleDepartures(ctx context.Context, hoursThreshold int) ([]*domain.DriverTrip, error)

	// Driver offer expiry
	ExpireStaleOffers(ctx context.Context) error
}

// runDriverDeliveryAutoConfirmJob runs periodically to auto-confirm deliveries
// whose 24h confirmation window has expired
func (p *JobProcessor) runDriverDeliveryAutoConfirmJob(ctx context.Context) {
	interval := 5 * time.Minute // Check every 5 minutes
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Process immediately on start
	p.processDriverDeliveryAutoConfirm(ctx)

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("driver delivery auto-confirm job: context cancelled")
			return
		case <-p.stopCh:
			p.logger.Info("driver delivery auto-confirm job: stop signal received")
			return
		case <-ticker.C:
			p.processDriverDeliveryAutoConfirm(ctx)
		}
	}
}

func (p *JobProcessor) processDriverDeliveryAutoConfirm(ctx context.Context) {
	if p.driverService == nil {
		return
	}

	release, proceed := p.acquireLock(ctx, "worker:driver_delivery_auto_confirm", 2*time.Minute)
	if !proceed {
		return
	}
	if release != nil {
		defer release()
	}

	p.logger.Debug("driver delivery auto-confirm job: checking for expired confirmation windows")

	deliveries, err := p.driverService.GetExpiredConfirmationWindows(ctx)
	if err != nil {
		p.logger.Error("driver delivery auto-confirm job: failed to get expired deliveries",
			zap.Error(err),
		)
		return
	}

	if len(deliveries) == 0 {
		p.logger.Debug("driver delivery auto-confirm job: no expired deliveries found")
		return
	}

	p.logger.Info("driver delivery auto-confirm job: processing expired deliveries",
		zap.Int("count", len(deliveries)),
	)

	for _, delivery := range deliveries {
		if err := p.driverService.AutoConfirmDelivery(ctx, delivery.ID); err != nil {
			p.logger.Error("driver delivery auto-confirm job: failed to auto-confirm delivery",
				zap.String("delivery_id", delivery.ID.String()),
				zap.Error(err),
			)
			continue
		}

		p.logger.Info("driver delivery auto-confirm job: auto-confirmed delivery",
			zap.String("delivery_id", delivery.ID.String()),
		)
	}

	p.logger.Debug("driver delivery auto-confirm job: completed successfully")
}

// ============================================
// PHASE 5: EDGE CASE MONITOR JOBS
// ============================================

// runDriverDeliveryMonitorJob monitors for edge cases:
// - Buyer unreachable deadline expired (24h)
// - No driver found (48h)
func (p *JobProcessor) runDriverDeliveryMonitorJob(ctx context.Context) {
	interval := 10 * time.Minute
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Process immediately on start
	p.processExpiredUnreachable(ctx)
	p.processStaleAwaitingDriver(ctx)

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("driver delivery monitor job: context cancelled")
			return
		case <-p.stopCh:
			p.logger.Info("driver delivery monitor job: stop signal received")
			return
		case <-ticker.C:
			p.processExpiredUnreachable(ctx)
			p.processStaleAwaitingDriver(ctx)
		}
	}
}

func (p *JobProcessor) processExpiredUnreachable(ctx context.Context) {
	if p.driverService == nil {
		return
	}

	release, proceed := p.acquireLock(ctx, "worker:driver_expired_unreachable", 2*time.Minute)
	if !proceed {
		return
	}
	if release != nil {
		defer release()
	}

	deliveries, err := p.driverService.GetExpiredUnreachableDeliveries(ctx)
	if err != nil {
		p.logger.Error("driver monitor job: failed to get expired unreachable deliveries",
			zap.Error(err),
		)
		return
	}

	for _, delivery := range deliveries {
		if err := p.driverService.ProcessExpiredUnreachable(ctx, delivery.ID); err != nil {
			p.logger.Error("driver monitor job: failed to process expired unreachable",
				zap.String("delivery_id", delivery.ID.String()),
				zap.Error(err),
			)
			continue
		}
		p.logger.Info("driver monitor job: processed expired unreachable delivery",
			zap.String("delivery_id", delivery.ID.String()),
		)
	}
}

func (p *JobProcessor) processStaleAwaitingDriver(ctx context.Context) {
	if p.driverService == nil {
		return
	}

	release, proceed := p.acquireLock(ctx, "worker:driver_stale_awaiting", 2*time.Minute)
	if !proceed {
		return
	}
	if release != nil {
		defer release()
	}

	// 48 hours threshold for no driver found
	deliveries, err := p.driverService.GetStaleAwaitingDriver(ctx, 48)
	if err != nil {
		p.logger.Error("driver monitor job: failed to get stale awaiting driver deliveries",
			zap.Error(err),
		)
		return
	}

	for _, delivery := range deliveries {
		if err := p.driverService.ExpireAwaitingDriverDelivery(ctx, delivery.ID); err != nil {
			p.logger.Error("driver monitor job: failed to expire awaiting driver delivery",
				zap.String("delivery_id", delivery.ID.String()),
				zap.Error(err),
			)
			continue
		}
		p.logger.Info("driver monitor job: expired awaiting driver delivery",
			zap.String("delivery_id", delivery.ID.String()),
		)
	}
}

// runDriverTripMonitorJob monitors for trip edge cases:
// - Stale departures (scheduled but no departure after 24h)
// - Abandoned trips (in transit but no updates for 72h)
func (p *JobProcessor) runDriverTripMonitorJob(ctx context.Context) {
	interval := 30 * time.Minute
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Process immediately on start
	p.processStaleDepartures(ctx)
	p.processAbandonedTrips(ctx)

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("driver trip monitor job: context cancelled")
			return
		case <-p.stopCh:
			p.logger.Info("driver trip monitor job: stop signal received")
			return
		case <-ticker.C:
			p.processStaleDepartures(ctx)
			p.processAbandonedTrips(ctx)
		}
	}
}

func (p *JobProcessor) processStaleDepartures(ctx context.Context) {
	if p.driverService == nil {
		return
	}

	release, proceed := p.acquireLock(ctx, "worker:driver_stale_departures", 2*time.Minute)
	if !proceed {
		return
	}
	if release != nil {
		defer release()
	}

	// 24 hours threshold for stale departure
	trips, err := p.driverService.GetStaleDepartures(ctx, 24)
	if err != nil {
		p.logger.Error("driver trip monitor job: failed to get stale departures",
			zap.Error(err),
		)
		return
	}

	for _, trip := range trips {
		// Send reminder notification to driver
		p.logger.Warn("driver trip monitor job: stale departure detected",
			zap.String("trip_id", trip.ID.String()),
			zap.String("driver_id", trip.DriverID.String()),
		)
		// TODO: Send notification to driver
	}
}

func (p *JobProcessor) processAbandonedTrips(ctx context.Context) {
	if p.driverService == nil {
		return
	}

	release, proceed := p.acquireLock(ctx, "worker:driver_abandoned_trips", 2*time.Minute)
	if !proceed {
		return
	}
	if release != nil {
		defer release()
	}

	// 72 hours threshold for abandoned trip
	trips, err := p.driverService.GetAbandonedTrips(ctx, 72)
	if err != nil {
		p.logger.Error("driver trip monitor job: failed to get abandoned trips",
			zap.Error(err),
		)
		return
	}

	for _, trip := range trips {
		if err := p.driverService.ProcessAbandonedTrip(ctx, trip.ID); err != nil {
			p.logger.Error("driver trip monitor job: failed to process abandoned trip",
				zap.String("trip_id", trip.ID.String()),
				zap.Error(err),
			)
			continue
		}
		p.logger.Warn("driver trip monitor job: processed abandoned trip",
			zap.String("trip_id", trip.ID.String()),
		)
	}
}
