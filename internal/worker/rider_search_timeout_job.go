package worker

import (
	"context"
	"time"

	"go.uber.org/zap"

	"kipa/internal/domain"
)

// runRiderSearchTimeoutJob periodically checks for delivery jobs that have been
// searching for a rider beyond the timeout threshold.
//
// Flow:
// 1. Query jobs WHERE status = 'searching' AND search_timeout_at <= NOW()
// 2. For each job:
//    - Update delivery_job.status = 'timeout'
//    - Update payment_request.status = 'no_rider_found'
//    - (Future) Send notification to seller
func (p *JobProcessor) runRiderSearchTimeoutJob(ctx context.Context) {
	ticker := time.NewTicker(p.config.RiderSearchCheckInterval)
	defer ticker.Stop()

	// Run immediately on start, then on ticker
	p.processRiderSearchTimeouts(ctx)

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("rider search timeout job: context cancelled")
			return
		case <-p.stopCh:
			p.logger.Info("rider search timeout job: stop signal received")
			return
		case <-ticker.C:
			p.processRiderSearchTimeouts(ctx)
		}
	}
}

// processRiderSearchTimeouts handles the actual timeout processing
func (p *JobProcessor) processRiderSearchTimeouts(ctx context.Context) {
	if p.deliveryJobRepo == nil {
		p.logger.Warn("rider search timeout job: delivery job repo not configured")
		return
	}

	// Acquire distributed lock to prevent multiple instances processing same jobs
	release, proceed := p.acquireLock(ctx, "worker:rider_search_timeout", 1*time.Minute)
	if !proceed {
		return
	}
	if release != nil {
		defer release()
	}

	p.logger.Debug("rider search timeout job: checking for timed out jobs")

	// Get all jobs that have exceeded their search timeout
	timedOutJobs, err := p.deliveryJobRepo.GetTimedOutJobs(ctx)
	if err != nil {
		p.logger.Error("rider search timeout job: failed to get timed out jobs",
			zap.Error(err),
		)
		return
	}

	if len(timedOutJobs) == 0 {
		p.logger.Debug("rider search timeout job: no timed out jobs found")
		return
	}

	p.logger.Info("rider search timeout job: processing timed out jobs",
		zap.Int("count", len(timedOutJobs)),
	)

	for _, job := range timedOutJobs {
		if err := p.processTimedOutJob(ctx, job); err != nil {
			p.logger.Error("rider search timeout job: failed to process job",
				zap.String("job_id", job.ID.String()),
				zap.Error(err),
			)
			continue
		}
	}

	p.logger.Info("rider search timeout job: completed",
		zap.Int("processed", len(timedOutJobs)),
	)
}

// processTimedOutJob handles a single timed out delivery job
func (p *JobProcessor) processTimedOutJob(ctx context.Context, job *domain.DeliveryJob) error {
	// Update delivery job status to timeout
	if err := p.deliveryJobRepo.UpdateStatus(ctx, job.ID, domain.DJStatusTimeout); err != nil {
		return err
	}

	p.logger.Info("rider search timeout job: marked job as timed out",
		zap.String("job_id", job.ID.String()),
		zap.String("payment_request_id", job.PaymentRequestID.String()),
	)

	// Expire any pending rider offers for this job
	if p.riderOfferRepo != nil {
		if err := p.riderOfferRepo.ExpirePendingOffers(ctx, job.ID); err != nil {
			p.logger.Error("rider search timeout job: failed to expire pending offers",
				zap.String("job_id", job.ID.String()),
				zap.Error(err),
			)
			// Don't return error - continue with other cleanup
		}
	}

	// Update payment request status to no_rider_found
	if p.paymentRequestRepo != nil {
		if err := p.paymentRequestRepo.UpdateStatus(ctx, job.PaymentRequestID, domain.PRStatusNoRiderFound); err != nil {
			p.logger.Error("rider search timeout job: failed to update payment request status",
				zap.String("job_id", job.ID.String()),
				zap.String("payment_request_id", job.PaymentRequestID.String()),
				zap.Error(err),
			)
			// Don't return error - the job was already marked as timed out
		}
	}

	// TODO: Send notification to seller
	// p.notificationService.SendToUser(ctx, sellerID, NotificationPayload{
	//     Title: "No Riders Available",
	//     Body:  "We couldn't find a rider for your order. Would you like to retry or cancel?",
	//     Type:  "rider_search_timeout",
	//     Data:  map[string]interface{}{"payment_request_id": job.PaymentRequestID},
	// })

	return nil
}
