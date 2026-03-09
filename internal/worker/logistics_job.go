package worker

import (
	"context"
	"time"

	"go.uber.org/zap"
)
type LogisticsServiceInterface interface {
	ProcessExceededShipDeadlines(ctx context.Context) error
	ProcessConfirmationReminders(ctx context.Context) error
	ProcessAutoEscalations(ctx context.Context) error
}
func (p *JobProcessor) runLogisticsShipDeadlineJob(ctx context.Context) {
	ticker := time.NewTicker(p.config.LogisticsCheckInterval)
	defer ticker.Stop()
	p.processLogisticsShipDeadlines(ctx)

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("logistics ship deadline job: context cancelled")
			return
		case <-p.stopCh:
			p.logger.Info("logistics ship deadline job: stop signal received")
			return
		case <-ticker.C:
			p.processLogisticsShipDeadlines(ctx)
		}
	}
}

func (p *JobProcessor) processLogisticsShipDeadlines(ctx context.Context) {
	if p.logisticsService == nil {
		return
	}

	release, proceed := p.acquireLock(ctx, "worker:logistics_ship_deadline", 2*time.Minute)
	if !proceed {
		return
	}
	if release != nil {
		defer release()
	}

	p.logger.Debug("logistics ship deadline job: checking for exceeded deadlines")

	if err := p.logisticsService.ProcessExceededShipDeadlines(ctx); err != nil {
		p.logger.Error("logistics ship deadline job: failed to process",
			zap.Error(err),
		)
		return
	}

	p.logger.Debug("logistics ship deadline job: completed successfully")
}
func (p *JobProcessor) runLogisticsReminderJob(ctx context.Context) {
	ticker := time.NewTicker(p.config.LogisticsReminderInterval)
	defer ticker.Stop()
	p.processLogisticsReminders(ctx)

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("logistics reminder job: context cancelled")
			return
		case <-p.stopCh:
			p.logger.Info("logistics reminder job: stop signal received")
			return
		case <-ticker.C:
			p.processLogisticsReminders(ctx)
		}
	}
}

func (p *JobProcessor) processLogisticsReminders(ctx context.Context) {
	if p.logisticsService == nil {
		return
	}

	release, proceed := p.acquireLock(ctx, "worker:logistics_reminder", 2*time.Minute)
	if !proceed {
		return
	}
	if release != nil {
		defer release()
	}

	p.logger.Debug("logistics reminder job: sending confirmation reminders")

	if err := p.logisticsService.ProcessConfirmationReminders(ctx); err != nil {
		p.logger.Error("logistics reminder job: failed to process",
			zap.Error(err),
		)
		return
	}

	p.logger.Debug("logistics reminder job: completed successfully")
}
func (p *JobProcessor) runLogisticsAutoEscalationJob(ctx context.Context) {
	ticker := time.NewTicker(p.config.LogisticsEscalationInterval)
	defer ticker.Stop()
	p.processLogisticsAutoEscalations(ctx)

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("logistics auto-escalation job: context cancelled")
			return
		case <-p.stopCh:
			p.logger.Info("logistics auto-escalation job: stop signal received")
			return
		case <-ticker.C:
			p.processLogisticsAutoEscalations(ctx)
		}
	}
}

func (p *JobProcessor) processLogisticsAutoEscalations(ctx context.Context) {
	if p.logisticsService == nil {
		return
	}

	release, proceed := p.acquireLock(ctx, "worker:logistics_escalation", 2*time.Minute)
	if !proceed {
		return
	}
	if release != nil {
		defer release()
	}

	p.logger.Debug("logistics auto-escalation job: checking for unresponsive buyers")

	if err := p.logisticsService.ProcessAutoEscalations(ctx); err != nil {
		p.logger.Error("logistics auto-escalation job: failed to process",
			zap.Error(err),
		)
		return
	}

	p.logger.Debug("logistics auto-escalation job: completed successfully")
}
