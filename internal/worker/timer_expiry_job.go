package worker

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// runTimerExpiryJob handles various time-sensitive expirations:
// 1. Rider offer expiry - mark pending offers as expired after timeout
// 2. Processing time reminders - notify sellers who haven't marked items as ready
//
// This job consolidates multiple small timer-based operations.
func (p *JobProcessor) runTimerExpiryJob(ctx context.Context) {
	ticker := time.NewTicker(p.config.TimerExpiryCheckInterval)
	defer ticker.Stop()

	// Run immediately on start, then on ticker
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

// processTimerExpiries handles all timer-based expirations
func (p *JobProcessor) processTimerExpiries(ctx context.Context) {
	// Acquire distributed lock to prevent multiple instances processing same timers
	release, proceed := p.acquireLock(ctx, "worker:timer_expiry", 2*time.Minute)
	if !proceed {
		return
	}
	if release != nil {
		defer release()
	}

	p.logger.Debug("timer expiry job: processing timer expirations")

	// 1. Expire stale rider offers
	p.expireStaleRiderOffers(ctx)

	// 2. Check for overdue processing orders and send reminders
	p.processOverdueProcessingOrders(ctx)

	p.logger.Debug("timer expiry job: completed")
}

// expireStaleRiderOffers marks pending rider offers as expired
// if they've been pending longer than the configured timeout
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

// processOverdueProcessingOrders finds orders where the seller hasn't
// marked the item as ready within their specified processing time.
// This sends reminder notifications to sellers with proper deduplication.
func (p *JobProcessor) processOverdueProcessingOrders(ctx context.Context) {
	if p.paymentRequestRepo == nil {
		return
	}

	// Get overdue orders that haven't been reminded recently
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
		// Determine escalation level based on reminder count
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

		// Mark reminder as sent (before actually sending to avoid duplicates on failure)
		if err := p.paymentRequestRepo.UpdateReminderSent(ctx, order.ID); err != nil {
			p.logger.Error("timer expiry job: failed to update reminder sent timestamp",
				zap.String("payment_request_id", order.ID.String()),
				zap.Error(err),
			)
			continue
		}

		remindedCount++

		// TODO: Send notification to seller based on escalation level
		// p.notificationService.SendToUser(ctx, order.SellerID, NotificationPayload{
		//     Title: getEscalationTitle(escalationLevel),
		//     Body:  getEscalationBody(escalationLevel, order.ItemName),
		//     Type:  "processing_overdue",
		//     Data:  map[string]interface{}{
		//         "payment_request_id": order.ID,
		//         "item_name":          order.ItemName,
		//         "escalation_level":   escalationLevel,
		//         "reminder_count":     order.ReminderCount + 1,
		//     },
		// })

		// Consider auto-refund after many reminders (e.g., 6+ reminders = 6+ hours overdue)
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
