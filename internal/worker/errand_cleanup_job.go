package worker

import (
	"context"
	"time"

	"go.uber.org/zap"

	"kipa/internal/domain"
)

// runErrandCleanupJob periodically fixes stuck errands where the delivery job
// is completed but the errand status wasn't updated
func (p *JobProcessor) runErrandCleanupJob(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Minute) // Run every 10 minutes
	defer ticker.Stop()

	// Run immediately on startup
	p.processStuckErrands(ctx)

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("errand cleanup job: context cancelled")
			return
		case <-p.stopCh:
			p.logger.Info("errand cleanup job: stop signal received")
			return
		case <-ticker.C:
			p.processStuckErrands(ctx)
		}
	}
}

func (p *JobProcessor) processStuckErrands(ctx context.Context) {
	if p.errandRepo == nil {
		return
	}

	release, proceed := p.acquireLock(ctx, "errand_cleanup", 5*time.Minute)
	if !proceed {
		return
	}
	if release != nil {
		defer release()
	}

	stuckErrands, err := p.errandRepo.GetStuckErrands(ctx)
	if err != nil {
		p.logger.Error("errand cleanup job: failed to get stuck errands", zap.Error(err))
		return
	}

	if len(stuckErrands) == 0 {
		return
	}

	p.logger.Info("errand cleanup job: found stuck errands",
		zap.Int("count", len(stuckErrands)),
	)

	fixedCount := 0
	for _, errand := range stuckErrands {
		if err := p.errandRepo.MarkCompleted(ctx, errand.ID); err != nil {
			p.logger.Error("errand cleanup job: failed to fix stuck errand",
				zap.String("errand_id", errand.ID.String()),
				zap.String("current_status", string(errand.Status)),
				zap.Error(err),
			)
			continue
		}

		fixedCount++
		p.logger.Info("errand cleanup job: fixed stuck errand",
			zap.String("errand_id", errand.ID.String()),
			zap.String("user_id", errand.UserID.String()),
			zap.String("previous_status", string(errand.Status)),
		)

		// Notify the user that their errand has been completed
		if p.notificationService != nil && errand.UserID.String() != "" {
			go p.notificationService.SendToUser(
				context.Background(),
				errand.UserID,
				"Errand Completed",
				"Your errand has been marked as completed.",
				map[string]string{"errand_id": errand.ID.String()},
				domain.NotificationErrandCompleted,
			)
		}
	}

	p.logger.Info("errand cleanup job: completed",
		zap.Int("total_stuck", len(stuckErrands)),
		zap.Int("fixed", fixedCount),
	)
}
