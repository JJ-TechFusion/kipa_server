package worker

import (
	"context"
	"time"

	"go.uber.org/zap"

	"kipa/internal/domain"
)

func (p *JobProcessor) runRebookExpiryJob(ctx context.Context) {
	ticker := time.NewTicker(p.config.RebookExpiryCheckInterval)
	defer ticker.Stop()

	// Run immediately on start
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

	// Find payment requests with expired rebook deadlines
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
	// Update status to rebook_expired
	if err := p.paymentRequestRepo.UpdateStatus(ctx, pr.ID, domain.PRStatusRebookExpired); err != nil {
		return err
	}

	// Calculate 5% unavailability fee
	unavailabilityFee := pr.ItemPrice * 0.05
	if err := p.paymentRequestRepo.SetUnavailabilityFee(ctx, pr.ID, unavailabilityFee); err != nil {
		p.logger.Error("rebook expiry job: failed to set unavailability fee",
			zap.String("payment_request_id", pr.ID.String()),
			zap.Error(err),
		)
	}

	// Process refund minus 5% if escrow service available
	if p.escrowService != nil && pr.EscrowID != nil {
		// Note: The actual partial refund logic would need to be implemented
		// in the escrow service. For now, we log and let admin handle it.
		p.logger.Info("rebook expiry job: rebook expired, needs partial refund",
			zap.String("payment_request_id", pr.ID.String()),
			zap.String("escrow_id", pr.EscrowID.String()),
			zap.Float64("unavailability_fee", unavailabilityFee),
			zap.Float64("refund_amount", pr.ItemPrice-unavailabilityFee),
		)
	}

	p.logger.Info("rebook expiry job: processed expired rebook",
		zap.String("payment_request_id", pr.ID.String()),
		zap.Float64("unavailability_fee", unavailabilityFee),
	)

	return nil
}
