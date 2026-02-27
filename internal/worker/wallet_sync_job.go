package worker

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/service"
)

// WalletSyncServiceInterface defines the methods needed for wallet sync job
type WalletSyncServiceInterface interface {
	SyncFlutterwaveWallet(ctx context.Context, userID uuid.UUID) (*service.WalletSyncResult, error)
	GetAllActiveFlutterwaveSubaccounts(ctx context.Context) ([]uuid.UUID, error)
}

func (p *JobProcessor) runWalletSyncJob(ctx context.Context) {
	// Default to 5 minutes if not configured
	interval := p.config.WalletSyncInterval
	if interval == 0 {
		interval = 5 * time.Minute
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run immediately on start
	p.processWalletSync(ctx)

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("wallet sync job: context cancelled")
			return
		case <-p.stopCh:
			p.logger.Info("wallet sync job: stop signal received")
			return
		case <-ticker.C:
			p.processWalletSync(ctx)
		}
	}
}

func (p *JobProcessor) processWalletSync(ctx context.Context) {
	if p.walletSyncService == nil {
		p.logger.Debug("wallet sync job: wallet sync service not configured")
		return
	}

	// Acquire distributed lock to prevent multiple workers from syncing simultaneously
	release, proceed := p.acquireLock(ctx, "worker:wallet_sync", 5*time.Minute)
	if !proceed {
		return
	}
	if release != nil {
		defer release()
	}

	p.logger.Debug("wallet sync job: starting sync for all active subaccounts")

	// Get all users with active Flutterwave subaccounts
	userIDs, err := p.walletSyncService.GetAllActiveFlutterwaveSubaccounts(ctx)
	if err != nil {
		p.logger.Error("wallet sync job: failed to get active subaccounts",
			zap.Error(err),
		)
		return
	}

	if len(userIDs) == 0 {
		p.logger.Debug("wallet sync job: no active subaccounts to sync")
		return
	}

	p.logger.Info("wallet sync job: syncing wallets",
		zap.Int("count", len(userIDs)),
	)

	syncedCount := 0
	errorCount := 0

	for _, userID := range userIDs {
		select {
		case <-ctx.Done():
			p.logger.Info("wallet sync job: context cancelled during sync")
			return
		case <-p.stopCh:
			p.logger.Info("wallet sync job: stop signal during sync")
			return
		default:
		}

		result, err := p.walletSyncService.SyncFlutterwaveWallet(ctx, userID)
		if err != nil {
			p.logger.Warn("wallet sync job: failed to sync wallet",
				zap.String("user_id", userID.String()),
				zap.Error(err),
			)
			errorCount++
			continue
		}

		if result.WasSynced {
			syncedCount++
			p.logger.Info("wallet sync job: wallet synced",
				zap.String("user_id", userID.String()),
				zap.Float64("amount_synced", result.AmountSynced),
			)
		}

		// Small delay between API calls to avoid rate limiting
		time.Sleep(100 * time.Millisecond)
	}

	p.logger.Info("wallet sync job: completed",
		zap.Int("total", len(userIDs)),
		zap.Int("synced", syncedCount),
		zap.Int("errors", errorCount),
	)
}

func (p *JobProcessor) SetWalletSyncService(svc WalletSyncServiceInterface) {
	p.walletSyncService = svc
}
