package worker

import (
	"context"
	"time"

	"go.uber.org/zap"

	"kipa/internal/domain"
)
func (p *JobProcessor) runLinkExpiryJob(ctx context.Context) {
	ticker := time.NewTicker(p.config.LinkExpiryCheckInterval)
	defer ticker.Stop()
	p.processExpiredLinks(ctx)

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("link expiry job: context cancelled")
			return
		case <-p.stopCh:
			p.logger.Info("link expiry job: stop signal received")
			return
		case <-ticker.C:
			p.processExpiredLinks(ctx)
		}
	}
}
func (p *JobProcessor) processExpiredLinks(ctx context.Context) {
	if p.paymentRequestRepo == nil {
		p.logger.Warn("link expiry job: payment request repo not configured")
		return
	}
	release, proceed := p.acquireLock(ctx, "worker:link_expiry", 5*time.Minute)
	if !proceed {
		return
	}
	if release != nil {
		defer release()
	}

	p.logger.Debug("link expiry job: checking for expired links")
	expiredLinks, err := p.paymentRequestRepo.GetExpiredLinks(ctx)
	if err != nil {
		p.logger.Error("link expiry job: failed to get expired links",
			zap.Error(err),
		)
		return
	}

	if len(expiredLinks) == 0 {
		p.logger.Debug("link expiry job: no expired links found")
		return
	}

	p.logger.Info("link expiry job: processing expired links",
		zap.Int("count", len(expiredLinks)),
	)

	expiredCount := 0
	for _, pr := range expiredLinks {
		if err := p.paymentRequestRepo.UpdateStatus(ctx, pr.ID, domain.PRStatusExpired); err != nil {
			p.logger.Error("link expiry job: failed to expire link",
				zap.String("payment_request_id", pr.ID.String()),
				zap.Error(err),
			)
			continue
		}

		expiredCount++
		paymentCode := ""
		if pr.PaymentCode != nil {
			paymentCode = *pr.PaymentCode
		}
		p.logger.Debug("link expiry job: marked link as expired",
			zap.String("payment_request_id", pr.ID.String()),
			zap.String("payment_code", paymentCode),
		)
	}

	p.logger.Info("link expiry job: completed",
		zap.Int("expired", expiredCount),
	)
}
