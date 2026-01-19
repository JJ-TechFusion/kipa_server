package worker

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"go.uber.org/zap"

	"kipa/internal/repository"
	"kipa/internal/service"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// WorkerConfig holds configuration for background workers
type WorkerConfig struct {
	// Job intervals
	EscrowCheckInterval      time.Duration
	RiderSearchCheckInterval time.Duration
	LinkExpiryCheckInterval  time.Duration
	RadiusExpandInterval     time.Duration
	TimerExpiryCheckInterval time.Duration

	// Timeouts and thresholds
	RiderSearchTimeoutMins   int
	RadiusExpandSteps        []int // e.g., [3, 5, 8, 12] km
	RadiusExpandIntervalMins int
	RiderOfferTimeoutMins    int // How long a rider has to accept an offer

	// Resilience settings
	MaxJobRestarts    int           // Maximum times to restart a panicked job (0 = unlimited)
	InitialBackoff    time.Duration // Initial backoff after panic
	MaxBackoff        time.Duration // Maximum backoff between restarts
	JitterPercent     int           // Percentage of jitter to add to intervals (0-100)

	// Reminder settings
	ReminderCooldownMins int // Minimum minutes between reminder notifications to same seller
}

// DefaultWorkerConfig returns sensible defaults
func DefaultWorkerConfig() *WorkerConfig {
	return &WorkerConfig{
		EscrowCheckInterval:      1 * time.Minute,
		RiderSearchCheckInterval: 30 * time.Second,
		LinkExpiryCheckInterval:  5 * time.Minute,
		RadiusExpandInterval:     2 * time.Minute,
		TimerExpiryCheckInterval: 1 * time.Minute,

		RiderSearchTimeoutMins:   15,
		RadiusExpandSteps:        []int{3, 5, 8, 12},
		RadiusExpandIntervalMins: 3,
		RiderOfferTimeoutMins:    2, // 2 minutes to accept an offer

		MaxJobRestarts: 10,              // Restart up to 10 times before giving up
		InitialBackoff: 1 * time.Second, // Start with 1 second backoff
		MaxBackoff:     5 * time.Minute, // Cap at 5 minutes
		JitterPercent:  20,              // Add up to 20% jitter

		ReminderCooldownMins: 60, // 1 hour between reminders
	}
}

// DistributedLock provides distributed locking for multi-instance deployments
type DistributedLock interface {
	Lock(ctx context.Context, name string, ttl time.Duration) (release func(), acquired bool)
}

// JobProcessor orchestrates all background jobs
type JobProcessor struct {
	// Services
	escrowService EscrowServiceInterface

	// Repositories
	deliveryJobRepo    repository.DeliveryJobRepository
	paymentRequestRepo repository.PaymentRequestRepository
	riderOfferRepo     repository.RiderJobOfferRepository

	// Distributed locking (optional, for multi-instance deployments)
	lock DistributedLock

	// Configuration
	config *WorkerConfig
	logger *zap.Logger

	// Lifecycle management
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// EscrowServiceInterface defines what we need from escrow service
// This avoids circular imports with the service package
type EscrowServiceInterface interface {
	ProcessPendingConfirmationWindows(ctx context.Context) error
}

// NewJobProcessor creates a new job processor
func NewJobProcessor(
	escrowService EscrowServiceInterface,
	deliveryJobRepo repository.DeliveryJobRepository,
	paymentRequestRepo repository.PaymentRequestRepository,
	riderOfferRepo repository.RiderJobOfferRepository,
	config *WorkerConfig,
	logger *zap.Logger,
) *JobProcessor {
	if config == nil {
		config = DefaultWorkerConfig()
	}

	return &JobProcessor{
		escrowService:      escrowService,
		deliveryJobRepo:    deliveryJobRepo,
		paymentRequestRepo: paymentRequestRepo,
		riderOfferRepo:     riderOfferRepo,
		config:             config,
		logger:             logger.Named("worker"),
		stopCh:             make(chan struct{}),
	}
}

// Start begins all background jobs
func (p *JobProcessor) Start(ctx context.Context) {
	p.logger.Info("starting background job processor")

	// Start each job in its own goroutine
	p.wg.Add(5)

	go p.runWithRecovery(ctx, "escrow_timeout", p.runEscrowTimeoutJob)
	go p.runWithRecovery(ctx, "rider_search_timeout", p.runRiderSearchTimeoutJob)
	go p.runWithRecovery(ctx, "link_expiry", p.runLinkExpiryJob)
	go p.runWithRecovery(ctx, "radius_expand", p.runRadiusExpandJob)
	go p.runWithRecovery(ctx, "timer_expiry", p.runTimerExpiryJob)

	p.logger.Info("all background jobs started")
}

// Stop gracefully stops all background jobs
func (p *JobProcessor) Stop() {
	p.logger.Info("stopping background job processor")
	close(p.stopCh)
	p.wg.Wait()
	p.logger.Info("all background jobs stopped")
}

// Wait blocks until all jobs are done
func (p *JobProcessor) Wait() {
	p.wg.Wait()
}

// runWithRecovery wraps a job runner with panic recovery and automatic restart
func (p *JobProcessor) runWithRecovery(ctx context.Context, jobName string, jobFunc func(context.Context)) {
	defer p.wg.Done()

	restartCount := 0
	backoff := p.config.InitialBackoff

	for {
		// Check if we should stop before starting/restarting
		select {
		case <-ctx.Done():
			p.logger.Info("job not starting: context cancelled", zap.String("job", jobName))
			return
		case <-p.stopCh:
			p.logger.Info("job not starting: stop signal received", zap.String("job", jobName))
			return
		default:
		}

		// Run the job with panic recovery
		panicked := p.runJobWithPanicRecovery(ctx, jobName, jobFunc)

		if !panicked {
			// Job exited normally (context cancelled or stop signal)
			p.logger.Info("job stopped normally", zap.String("job", jobName))
			return
		}

		// Job panicked - decide whether to restart
		restartCount++
		if p.config.MaxJobRestarts > 0 && restartCount > p.config.MaxJobRestarts {
			p.logger.Error("job exceeded max restarts, giving up",
				zap.String("job", jobName),
				zap.Int("restart_count", restartCount),
			)
			return
		}

		// Add jitter to backoff
		jitteredBackoff := p.addJitter(backoff)

		p.logger.Warn("job will restart after backoff",
			zap.String("job", jobName),
			zap.Int("restart_count", restartCount),
			zap.Duration("backoff", jitteredBackoff),
		)

		// Wait for backoff duration or stop signal
		select {
		case <-ctx.Done():
			p.logger.Info("job restart cancelled: context cancelled", zap.String("job", jobName))
			return
		case <-p.stopCh:
			p.logger.Info("job restart cancelled: stop signal received", zap.String("job", jobName))
			return
		case <-time.After(jitteredBackoff):
			// Continue to restart
		}

		// Exponential backoff with cap
		backoff = backoff * 2
		if backoff > p.config.MaxBackoff {
			backoff = p.config.MaxBackoff
		}
	}
}

// runJobWithPanicRecovery runs the job and returns true if it panicked
func (p *JobProcessor) runJobWithPanicRecovery(ctx context.Context, jobName string, jobFunc func(context.Context)) (panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			p.logger.Error("job panicked",
				zap.String("job", jobName),
				zap.Any("panic", r),
				zap.Stack("stacktrace"),
			)
			panicked = true
		}
	}()

	p.logger.Info("starting job", zap.String("job", jobName))
	jobFunc(ctx)
	return false
}

// addJitter adds random jitter to a duration based on config
func (p *JobProcessor) addJitter(d time.Duration) time.Duration {
	if p.config.JitterPercent <= 0 {
		return d
	}
	jitterFraction := float64(p.config.JitterPercent) / 100.0
	jitter := time.Duration(float64(d) * jitterFraction * rand.Float64())
	return d + jitter
}

// SetEscrowService allows setting the escrow service (useful for avoiding circular imports)
func (p *JobProcessor) SetEscrowService(svc service.EscrowService) {
	p.escrowService = svc
}

// SetDistributedLock enables distributed locking for multi-instance deployments
func (p *JobProcessor) SetDistributedLock(lock DistributedLock) {
	p.lock = lock
}

// acquireLock attempts to acquire a distributed lock if one is configured.
// Returns a release function (or nil if no lock) and whether to proceed.
// If locking is not configured, always returns true (single-instance mode).
func (p *JobProcessor) acquireLock(ctx context.Context, lockName string, ttl time.Duration) (release func(), proceed bool) {
	if p.lock == nil {
		// No distributed lock configured, proceed in single-instance mode
		return nil, true
	}

	release, acquired := p.lock.Lock(ctx, lockName, ttl)
	if !acquired {
		p.logger.Debug("could not acquire lock, another instance is processing",
			zap.String("lock", lockName),
		)
		return nil, false
	}

	return release, true
}
