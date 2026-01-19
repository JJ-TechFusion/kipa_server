package worker

import (
	"context"
	"time"

	"go.uber.org/zap"

	"kipa/internal/domain"
)

// runRadiusExpandJob periodically expands the search radius for delivery jobs
// that haven't found a rider yet. This increases the pool of potential riders.
//
// Flow:
// 1. Query jobs WHERE status = 'searching' AND haven't expanded recently
// 2. For each job, expand radius to next tier
// 3. Radius tiers: 3km → 5km → 8km → 12km (configurable)
func (p *JobProcessor) runRadiusExpandJob(ctx context.Context) {
	ticker := time.NewTicker(p.config.RadiusExpandInterval)
	defer ticker.Stop()

	// Run immediately on start, then on ticker
	p.processRadiusExpansions(ctx)

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("radius expand job: context cancelled")
			return
		case <-p.stopCh:
			p.logger.Info("radius expand job: stop signal received")
			return
		case <-ticker.C:
			p.processRadiusExpansions(ctx)
		}
	}
}

// processRadiusExpansions handles the radius expansion logic
func (p *JobProcessor) processRadiusExpansions(ctx context.Context) {
	if p.deliveryJobRepo == nil {
		p.logger.Warn("radius expand job: delivery job repo not configured")
		return
	}

	// Acquire distributed lock to prevent multiple instances processing same jobs
	release, proceed := p.acquireLock(ctx, "worker:radius_expand", 2*time.Minute)
	if !proceed {
		return
	}
	if release != nil {
		defer release()
	}

	p.logger.Debug("radius expand job: checking for jobs needing radius expansion")

	// Get jobs that need radius expansion
	jobs, err := p.deliveryJobRepo.GetJobsForRadiusExpansion(ctx, p.config.RadiusExpandIntervalMins)
	if err != nil {
		p.logger.Error("radius expand job: failed to get jobs for expansion",
			zap.Error(err),
		)
		return
	}

	if len(jobs) == 0 {
		p.logger.Debug("radius expand job: no jobs need radius expansion")
		return
	}

	p.logger.Info("radius expand job: processing radius expansions",
		zap.Int("count", len(jobs)),
	)

	expandedCount := 0
	for _, job := range jobs {
		newRadius := p.getNextRadius(job.SearchRadiusKM)
		if newRadius == job.SearchRadiusKM {
			// Already at max radius, skip
			continue
		}

		if err := p.expandJobRadius(ctx, job, newRadius); err != nil {
			p.logger.Error("radius expand job: failed to expand radius",
				zap.String("job_id", job.ID.String()),
				zap.Error(err),
			)
			continue
		}

		expandedCount++
	}

	if expandedCount > 0 {
		p.logger.Info("radius expand job: completed",
			zap.Int("expanded", expandedCount),
		)
	}
}

// getNextRadius returns the next radius tier, or the current radius if already at max
func (p *JobProcessor) getNextRadius(currentRadius int) int {
	steps := p.config.RadiusExpandSteps
	if len(steps) == 0 {
		steps = []int{3, 5, 8, 12} // Default steps
	}

	for _, step := range steps {
		if step > currentRadius {
			return step
		}
	}

	// Already at or beyond max radius
	return currentRadius
}

// expandJobRadius updates the job's search radius
func (p *JobProcessor) expandJobRadius(ctx context.Context, job *domain.DeliveryJob, newRadius int) error {
	if err := p.deliveryJobRepo.UpdateSearchRadius(ctx, job.ID, newRadius); err != nil {
		return err
	}

	p.logger.Info("radius expand job: expanded search radius",
		zap.String("job_id", job.ID.String()),
		zap.Int("old_radius", job.SearchRadiusKM),
		zap.Int("new_radius", newRadius),
	)

	// TODO: Send notification to seller about expanded search
	// p.notificationService.SendToUser(ctx, sellerID, NotificationPayload{
	//     Title: "Expanding Rider Search",
	//     Body:  "We're expanding the search area to find a rider for your delivery.",
	//     Type:  "radius_expanded",
	// })

	return nil
}
