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

	if p.paymentRequestRepo == nil {
		return
	}
	expiredRequests, err := p.paymentRequestRepo.GetExpiredRebookRequests(ctx)
	if err != nil {
		p.logger.Error("rebook expiry job: failed to get expired rebook requests",
			zap.Error(err),
		)
		return
	}

	if len(expiredRequests) == 0 {
		p.logger.Debug("rebook expiry job: no expired rebook requests found")
		return
	}

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

	p.logger.Info("rebook expiry job: completed",
		zap.Int("processed", processedCount),
	)
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
