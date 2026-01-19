package worker

import (
	"context"
	"time"

	"go.uber.org/zap"

	"kipa/internal/repository/postgres"
)

type LocationCleanupJob struct {
	locationHistoryRepo *postgres.LocationHistoryRepo
	retentionDays       int
	logger              *zap.Logger
}

func NewLocationCleanupJob(
	locationHistoryRepo *postgres.LocationHistoryRepo,
	retentionDays int,
	logger *zap.Logger,
) *LocationCleanupJob {
	return &LocationCleanupJob{
		locationHistoryRepo: locationHistoryRepo,
		retentionDays:       retentionDays,
		logger:              logger,
	}
}

// Run executes the cleanup job
func (j *LocationCleanupJob) Run(ctx context.Context) error {
	before := time.Now().AddDate(0, 0, -j.retentionDays)
	
	count, err := j.locationHistoryRepo.DeleteOlderThan(ctx, before)
	if err != nil {
		j.logger.Error("failed to cleanup location history",
			zap.Error(err),
			zap.Int("retention_days", j.retentionDays),
		)
		return err
	}
	
	if count > 0 {
		j.logger.Info("cleaned up old location history",
			zap.Int64("deleted_count", count),
			zap.Int("retention_days", j.retentionDays),
		)
	}
	
	return nil
}

// Schedule returns the cron schedule for this job (daily at 3am)
func (j *LocationCleanupJob) Schedule() string {
	return "0 3 * * *" // 3:00 AM daily
}

// Name returns the job name
func (j *LocationCleanupJob) Name() string {
	return "location_cleanup"
}
