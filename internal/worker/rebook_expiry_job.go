package worker

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"kipa/internal/domain"
)

func (p *JobProcessor) runRebookExpiryJob(ctx context.Context) {
	ticker := time.NewTicker(p.config.RebookExpiryCheckInterval)
	defer ticker.Stop()
	p.processRebookExpiries(ctx)

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("rebook expiry job: context cancelled")
			return
		case <-p.stopCh:
			p.logger.Info("rebook expiry job: stop signal received")
			return
		case <-ticker.C:
			p.processRebookExpiries(ctx)
		}
	}
}

func (p *JobProcessor) processRebookExpiries(ctx context.Context) {
	release, proceed := p.acquireLock(ctx, "worker:rebook_expiry", 2*time.Minute)
	if !proceed {
		return
	}
	if release != nil {
		defer release()
	}

	p.logger.Debug("rebook expiry job: processing expired rebook windows")

	// Process expired payment request rebooks (intrastate/interstate)
	if p.paymentRequestRepo != nil {
		expiredRequests, err := p.paymentRequestRepo.GetExpiredRebookRequests(ctx)
		if err != nil {
			p.logger.Error("rebook expiry job: failed to get expired rebook requests",
				zap.Error(err),
			)
		} else if len(expiredRequests) > 0 {
			p.logger.Info("rebook expiry job: found expired rebook requests",
				zap.Int("count", len(expiredRequests)),
			)

			processedCount := 0
			for _, pr := range expiredRequests {
				if err := p.processRebookExpiry(ctx, pr); err != nil {
					p.logger.Error("rebook expiry job: failed to process expired rebook",
						zap.String("payment_request_id", pr.ID.String()),
						zap.Error(err),
					)
					continue
				}
				processedCount++
			}

			p.logger.Info("rebook expiry job: payment requests completed",
				zap.Int("processed", processedCount),
			)
		}
	}

	// Process expired errand unavailability windows (45 min wait)
	if p.deliveryJobRepo != nil {
		expiredErrandJobs, err := p.deliveryJobRepo.GetExpiredErrandUnavailableJobs(ctx, 45)
		if err != nil {
			p.logger.Error("rebook expiry job: failed to get expired errand jobs",
				zap.Error(err),
			)
		} else if len(expiredErrandJobs) > 0 {
			p.logger.Info("rebook expiry job: found expired errand unavailable jobs",
				zap.Int("count", len(expiredErrandJobs)),
			)

			processedCount := 0
			for _, job := range expiredErrandJobs {
				if err := p.processErrandUnavailableExpiry(ctx, job); err != nil {
					p.logger.Error("rebook expiry job: failed to process expired errand job",
						zap.String("job_id", job.ID.String()),
						zap.Error(err),
					)
					continue
				}
				processedCount++
			}

			p.logger.Info("rebook expiry job: errand jobs completed",
				zap.Int("processed", processedCount),
			)
		}
	}
}

func (p *JobProcessor) processRebookExpiry(ctx context.Context, pr *domain.PaymentRequest) error {
	if pr.DeliveryJobID == nil {
		p.logger.Error("rebook expiry job: no delivery job found",
			zap.String("payment_request_id", pr.ID.String()),
		)
		return nil
	}

	job, err := p.deliveryJobRepo.GetByID(ctx, *pr.DeliveryJobID)
	if err != nil {
		p.logger.Error("rebook expiry job: failed to get delivery job",
			zap.String("payment_request_id", pr.ID.String()),
			zap.Error(err),
		)
		return err
	}
	if job.Status != domain.DJStatusBuyerUnavailable {
		p.logger.Info("rebook expiry job: job not in buyer_unavailable status, skipping",
			zap.String("job_id", job.ID.String()),
			zap.String("current_status", string(job.Status)),
		)
		return nil
	}
	job.IsForcedReturn = true
	job.Status = domain.DJStatusForcedReturn
	dropoffCode := fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	job.DropoffCode = &dropoffCode
	originalPickupAddr := job.PickupAddress
	originalPickupLat := job.PickupLat
	originalPickupLng := job.PickupLng

	job.PickupAddress = job.DropoffAddress // Rider is at buyer's address
	job.PickupLat = job.DropoffLat
	job.PickupLng = job.DropoffLng

	job.DropoffAddress = originalPickupAddr // Return to seller's address
	job.DropoffLat = originalPickupLat
	job.DropoffLng = originalPickupLng

	if err := p.deliveryJobRepo.UpdateForcedReturn(ctx, job); err != nil {
		p.logger.Error("rebook expiry job: failed to update job for forced return",
			zap.String("job_id", job.ID.String()),
			zap.Error(err),
		)
		return err
	}
	if p.broadcaster != nil {
		p.broadcaster.BroadcastJobStatus(job.ID.String(), string(domain.DJStatusForcedReturn))
	}
	if err := p.paymentRequestRepo.UpdateStatus(ctx, pr.ID, domain.PRStatusRebookExpired); err != nil {
		p.logger.Error("rebook expiry job: failed to update payment request status",
			zap.String("payment_request_id", pr.ID.String()),
			zap.Error(err),
		)
	}
	unavailabilityFee := pr.ItemPrice * 0.05
	if err := p.paymentRequestRepo.SetUnavailabilityFee(ctx, pr.ID, unavailabilityFee); err != nil {
		p.logger.Error("rebook expiry job: failed to set unavailability fee",
			zap.String("payment_request_id", pr.ID.String()),
			zap.Error(err),
		)
	}
	if p.notificationService != nil {
		data := map[string]string{
			"job_id":             job.ID.String(),
			"payment_request_id": pr.ID.String(),
			"dropoff_code":       dropoffCode,
		}
		go p.notificationService.SendToUser(ctx, pr.SellerID, "Item Being Returned",
			fmt.Sprintf("The buyer was unreachable. The item is being returned to you. Verification code: %s", dropoffCode),
			data, domain.NotificationItemReturning)
		if pr.BuyerID != nil {
			go p.notificationService.SendToUser(ctx, *pr.BuyerID, "Delivery Cancelled - Item Returning",
				"The rebook window has expired. The item is being returned to the seller. You will receive a partial refund (minus 5% unavailability fee).",
				data, domain.NotificationRebookExpired)
		}
	}

	p.logger.Info("rebook expiry job: forced return initiated",
		zap.String("payment_request_id", pr.ID.String()),
		zap.String("job_id", job.ID.String()),
		zap.String("dropoff_code", dropoffCode),
		zap.Float64("unavailability_fee", unavailabilityFee),
	)

	return nil
}

// processErrandUnavailableExpiry ends an errand job after the 45-min wait window expires.
// For errands, there's no forced return - the ride just ends and parties settle outside the app.
func (p *JobProcessor) processErrandUnavailableExpiry(ctx context.Context, job *domain.DeliveryJob) error {
	if job.ErrandID == nil {
		p.logger.Error("rebook expiry job: errand job has no errand_id",
			zap.String("job_id", job.ID.String()),
		)
		return nil
	}

	// End the delivery job
	job.Status = domain.DJStatusFailed
	if err := p.deliveryJobRepo.UpdateStatus(ctx, job.ID, domain.DJStatusFailed); err != nil {
		p.logger.Error("rebook expiry job: failed to update errand job status",
			zap.String("job_id", job.ID.String()),
			zap.Error(err),
		)
		return err
	}

	if p.broadcaster != nil {
		p.broadcaster.BroadcastJobStatus(job.ID.String(), string(domain.DJStatusFailed))
	}

	// Update errand status
	if p.errandRepo != nil {
		if err := p.errandRepo.UpdateStatus(ctx, *job.ErrandID, domain.ErrandStatusReceiverUnavailable); err != nil {
			p.logger.Error("rebook expiry job: failed to update errand status",
				zap.String("errand_id", job.ErrandID.String()),
				zap.Error(err),
			)
		}
	}

	// Notify the errand requester
	if p.notificationService != nil && p.errandRepo != nil {
		errand, err := p.errandRepo.GetByID(ctx, *job.ErrandID)
		if err == nil && errand != nil {
			data := map[string]string{
				"job_id":    job.ID.String(),
				"errand_id": job.ErrandID.String(),
			}
			go p.notificationService.SendToUser(ctx, errand.UserID, "Errand Ended",
				"The receiver was unavailable and the wait time has expired. The ride has ended. Please settle with the rider directly.",
				data, domain.NotificationErrandCompleted)
		}
	}

	p.logger.Info("rebook expiry job: errand job ended due to receiver unavailable",
		zap.String("job_id", job.ID.String()),
		zap.String("errand_id", job.ErrandID.String()),
	)

	return nil
}
