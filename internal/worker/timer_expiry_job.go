package worker

import (
	"context"
	"time"

	"go.uber.org/zap"
)
func (p *JobProcessor) runTimerExpiryJob(ctx context.Context) {
	ticker := time.NewTicker(p.config.TimerExpiryCheckInterval)
	defer ticker.Stop()
	p.processTimerExpiries(ctx)

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("timer expiry job: context cancelled")
			return
		case <-p.stopCh:
			p.logger.Info("timer expiry job: stop signal received")
			return
		case <-ticker.C:
			p.processTimerExpiries(ctx)
		}
	}
}
func (p *JobProcessor) processTimerExpiries(ctx context.Context) {
	release, proceed := p.acquireLock(ctx, "worker:timer_expiry", 2*time.Minute)
	if !proceed {
		return
	}
	if release != nil {
		defer release()
	}

	p.logger.Debug("timer expiry job: processing timer expirations")
	p.expireStaleRiderOffers(ctx)
	p.processOverdueProcessingOrders(ctx)

	p.logger.Debug("timer expiry job: completed")
}
func (p *JobProcessor) expireStaleRiderOffers(ctx context.Context) {
	if p.riderOfferRepo == nil {
		return
	}

	expiredCount, err := p.riderOfferRepo.ExpireAllStaleOffers(ctx, p.config.RiderOfferTimeoutMins)
	if err != nil {
		p.logger.Error("timer expiry job: failed to expire stale offers",
			zap.Error(err),
		)
		return
	}

	if expiredCount > 0 {
		p.logger.Info("timer expiry job: expired stale rider offers",
			zap.Int64("count", expiredCount),
			zap.Int("timeout_mins", p.config.RiderOfferTimeoutMins),
		)
	}
}
func (p *JobProcessor) processOverdueProcessingOrders(ctx context.Context) {
	if p.paymentRequestRepo == nil {
		return
	}
	overdueOrders, err := p.paymentRequestRepo.GetOverdueProcessingOrders(ctx, p.config.ReminderCooldownMins)
	if err != nil {
		p.logger.Error("timer expiry job: failed to get overdue processing orders",
			zap.Error(err),
		)
		return
	}

	if len(overdueOrders) == 0 {
		return
	}

	p.logger.Info("timer expiry job: found overdue processing orders",
		zap.Int("count", len(overdueOrders)),
	)

	remindedCount := 0
	for _, order := range overdueOrders {
		escalationLevel := "gentle"
		if order.ReminderCount >= 2 {
			escalationLevel = "warning"
		}
		if order.ReminderCount >= 4 {
			escalationLevel = "urgent"
		}

		p.logger.Warn("timer expiry job: order overdue for processing",
			zap.String("payment_request_id", order.ID.String()),
			zap.String("seller_id", order.SellerID.String()),
			zap.Time("paid_at", *order.PaidAt),
			zap.Int("processing_hours", *order.ProcessingTimeHours),
			zap.Int("reminder_count", order.ReminderCount),
			zap.String("escalation_level", escalationLevel),
		)
		if err := p.paymentRequestRepo.UpdateReminderSent(ctx, order.ID); err != nil {
			p.logger.Error("timer expiry job: failed to update reminder sent timestamp",
				zap.String("payment_request_id", order.ID.String()),
				zap.Error(err),
			)
			continue
		}

		remindedCount++
		if order.ReminderCount >= 6 {
			p.logger.Error("timer expiry job: order severely overdue, consider auto-refund",
				zap.String("payment_request_id", order.ID.String()),
				zap.String("seller_id", order.SellerID.String()),
				zap.Int("reminder_count", order.ReminderCount),
			)
		}
	}

	if remindedCount > 0 {
		p.logger.Info("timer expiry job: sent processing reminders",
			zap.Int("reminded", remindedCount),
		)
	}
}
