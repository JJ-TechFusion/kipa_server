package worker

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// runEscrowTimeoutJob periodically checks for escrows past their confirmation window
// and auto-releases them to the seller if no dispute was filed.
//
// Flow:
// 1. Query escrows with status = 'confirmation_window' AND confirmation_window_ends_at <= NOW()
// 2. For each escrow, call Release() to credit seller wallet
// 3. Update escrow status to 'released'
func (p *JobProcessor) runEscrowTimeoutJob(ctx context.Context) {
	ticker := time.NewTicker(p.config.EscrowCheckInterval)
	defer ticker.Stop()

	// Run immediately on start, then on ticker
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

// processEscrowTimeouts handles the actual escrow timeout processing
func (p *JobProcessor) processEscrowTimeouts(ctx context.Context) {
	if p.escrowService == nil {
		p.logger.Warn("escrow timeout job: escrow service not configured")
		return
	}

	// Acquire distributed lock to prevent multiple instances processing same escrows
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
