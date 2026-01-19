package worker

import (
	"context"
	"time"

	"go.uber.org/zap"
)
func (p *JobProcessor) runEscrowTimeoutJob(ctx context.Context) {
	ticker := time.NewTicker(p.config.EscrowCheckInterval)
	defer ticker.Stop()
	p.processEscrowTimeouts(ctx)

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("escrow timeout job: context cancelled")
			return
		case <-p.stopCh:
			p.logger.Info("escrow timeout job: stop signal received")
			return
		case <-ticker.C:
			p.processEscrowTimeouts(ctx)
		}
	}
}
func (p *JobProcessor) processEscrowTimeouts(ctx context.Context) {
	if p.escrowService == nil {
		p.logger.Warn("escrow timeout job: escrow service not configured")
		return
	}
	release, proceed := p.acquireLock(ctx, "worker:escrow_timeout", 2*time.Minute)
	if !proceed {
		return
	}
	if release != nil {
		defer release()
	}

	p.logger.Debug("escrow timeout job: checking for expired confirmation windows")

	if err := p.escrowService.ProcessPendingConfirmationWindows(ctx); err != nil {
		p.logger.Error("escrow timeout job: failed to process pending escrows",
			zap.Error(err),
		)
		return
	}

	p.logger.Debug("escrow timeout job: completed successfully")
}
