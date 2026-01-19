package worker

import (
	"context"
	"time"

	"go.uber.org/zap"

	"kipa/internal/domain"
)
func (p *JobProcessor) runRadiusExpandJob(ctx context.Context) {
	ticker := time.NewTicker(p.config.RadiusExpandInterval)
	defer ticker.Stop()
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
func (p *JobProcessor) processRadiusExpansions(ctx context.Context) {
	if p.deliveryJobRepo == nil {
		p.logger.Warn("radius expand job: delivery job repo not configured")
		return
	}
	release, proceed := p.acquireLock(ctx, "worker:radius_expand", 2*time.Minute)
	if !proceed {
		return
	}
	if release != nil {
		defer release()
	}

	p.logger.Debug("radius expand job: checking for jobs needing radius expansion")
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
	return currentRadius
}
func (p *JobProcessor) expandJobRadius(ctx context.Context, job *domain.DeliveryJob, newRadius int) error {
	if err := p.deliveryJobRepo.UpdateSearchRadius(ctx, job.ID, newRadius); err != nil {
		return err
	}

	p.logger.Info("radius expand job: expanded search radius",
		zap.String("job_id", job.ID.String()),
		zap.Int("old_radius", job.SearchRadiusKM),
		zap.Int("new_radius", newRadius),
	)

	return nil
}
