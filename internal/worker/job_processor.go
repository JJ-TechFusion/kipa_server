package worker

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/domain"
	"kipa/internal/repository"
	"kipa/internal/service"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}
type WorkerConfig struct {
	EscrowCheckInterval      time.Duration
	RiderSearchCheckInterval time.Duration
	LinkExpiryCheckInterval  time.Duration
	RadiusExpandInterval     time.Duration
	TimerExpiryCheckInterval time.Duration
	RiderSearchTimeoutMins   int
	RadiusExpandSteps        []int // e.g., [3, 5, 8, 12] km
	RadiusExpandIntervalMins int
	RiderOfferTimeoutMins    int // How long a rider has to accept an offer
	MaxJobRestarts    int           // Maximum times to restart a panicked job (0 = unlimited)
	InitialBackoff    time.Duration // Initial backoff after panic
	MaxBackoff        time.Duration // Maximum backoff between restarts
	JitterPercent     int           // Percentage of jitter to add to intervals (0-100)
	ReminderCooldownMins int // Minimum minutes between reminder notifications to same seller
	LogisticsCheckInterval      time.Duration // Check for exceeded ship deadlines
	LogisticsReminderInterval   time.Duration // Send confirmation reminders
	LogisticsEscalationInterval time.Duration // Auto-escalate unresponsive buyers
	RebookExpiryCheckInterval time.Duration // Check for expired rebook deadlines
}
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
		LogisticsCheckInterval:      15 * time.Minute, // Check ship deadlines every 15 min
		LogisticsReminderInterval:   1 * time.Hour,    // Send reminders hourly
		LogisticsEscalationInterval: 30 * time.Minute, // Check for auto-escalation every 30 min
		RebookExpiryCheckInterval: 5 * time.Minute, // Check expired rebook deadlines every 5 min
	}
}
type DistributedLock interface {
	Lock(ctx context.Context, name string, ttl time.Duration) (release func(), acquired bool)
}
type JobStatusBroadcaster interface {
	BroadcastJobStatus(jobID, status string)
}

type JobProcessor struct {
	escrowService       EscrowServiceInterface
	logisticsService    LogisticsServiceInterface
	driverService       DriverServiceInterface
	notificationService NotificationServiceInterface
	deliveryJobRepo     repository.DeliveryJobRepository
	paymentRequestRepo  repository.PaymentRequestRepository
	riderOfferRepo      repository.RiderJobOfferRepository
	errandRepo          repository.ErrandRepository
	broadcaster         JobStatusBroadcaster
	lock                DistributedLock
	config              *WorkerConfig
	logger              *zap.Logger
	stopCh              chan struct{}
	wg                  sync.WaitGroup
	nearbyRidersFinder      NearbyRidersFinder
	nearbyRidersBroadcaster NearbyRidersBroadcaster
}
type EscrowServiceInterface interface {
	ProcessPendingConfirmationWindows(ctx context.Context) error
}

type NotificationServiceInterface interface {
	SendToUser(ctx context.Context, userID uuid.UUID, title, body string, data map[string]string, notificationType domain.NotificationType) error
}
func NewJobProcessor(
	escrowService EscrowServiceInterface,
	deliveryJobRepo repository.DeliveryJobRepository,
	paymentRequestRepo repository.PaymentRequestRepository,
	riderOfferRepo repository.RiderJobOfferRepository,
	broadcaster JobStatusBroadcaster,
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
		broadcaster:        broadcaster,
		config:             config,
		logger:             logger.Named("worker"),
		stopCh:             make(chan struct{}),
	}
}
func (p *JobProcessor) Start(ctx context.Context) {
	p.logger.Info("starting background job processor")
	p.wg.Add(13) // 5 existing + 3 logistics + 1 rebook + 1 driver auto-confirm + 2 driver monitors + 1 nearby riders

	go p.runWithRecovery(ctx, "escrow_timeout", p.runEscrowTimeoutJob)
	go p.runWithRecovery(ctx, "rider_search_timeout", p.runRiderSearchTimeoutJob)
	go p.runWithRecovery(ctx, "link_expiry", p.runLinkExpiryJob)
	go p.runWithRecovery(ctx, "radius_expand", p.runRadiusExpandJob)
	go p.runWithRecovery(ctx, "timer_expiry", p.runTimerExpiryJob)
	go p.runWithRecovery(ctx, "logistics_ship_deadline", p.runLogisticsShipDeadlineJob)
	go p.runWithRecovery(ctx, "logistics_reminder", p.runLogisticsReminderJob)
	go p.runWithRecovery(ctx, "logistics_escalation", p.runLogisticsAutoEscalationJob)
	go p.runWithRecovery(ctx, "rebook_expiry", p.runRebookExpiryJob)
	go p.runWithRecovery(ctx, "driver_delivery_auto_confirm", p.runDriverDeliveryAutoConfirmJob)
	go p.runWithRecovery(ctx, "driver_delivery_monitor", p.runDriverDeliveryMonitorJob)
	go p.runWithRecovery(ctx, "driver_trip_monitor", p.runDriverTripMonitorJob)
	go p.runWithRecovery(ctx, "nearby_riders", p.runNearbyRidersJob)

	p.logger.Info("all background jobs started")
}
func (p *JobProcessor) Stop() {
	p.logger.Info("stopping background job processor")
	close(p.stopCh)
	p.wg.Wait()
	p.logger.Info("all background jobs stopped")
}
func (p *JobProcessor) Wait() {
	p.wg.Wait()
}
func (p *JobProcessor) runWithRecovery(ctx context.Context, jobName string, jobFunc func(context.Context)) {
	defer p.wg.Done()

	restartCount := 0
	backoff := p.config.InitialBackoff

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("job not starting: context cancelled", zap.String("job", jobName))
			return
		case <-p.stopCh:
			p.logger.Info("job not starting: stop signal received", zap.String("job", jobName))
			return
		default:
		}
		panicked := p.runJobWithPanicRecovery(ctx, jobName, jobFunc)

		if !panicked {
			p.logger.Info("job stopped normally", zap.String("job", jobName))
			return
		}
		restartCount++
		if p.config.MaxJobRestarts > 0 && restartCount > p.config.MaxJobRestarts {
			p.logger.Error("job exceeded max restarts, giving up",
				zap.String("job", jobName),
				zap.Int("restart_count", restartCount),
			)
			return
		}
		jitteredBackoff := p.addJitter(backoff)

		p.logger.Warn("job will restart after backoff",
			zap.String("job", jobName),
			zap.Int("restart_count", restartCount),
			zap.Duration("backoff", jitteredBackoff),
		)
		select {
		case <-ctx.Done():
			p.logger.Info("job restart cancelled: context cancelled", zap.String("job", jobName))
			return
		case <-p.stopCh:
			p.logger.Info("job restart cancelled: stop signal received", zap.String("job", jobName))
			return
		case <-time.After(jitteredBackoff):
		}
		backoff = backoff * 2
		if backoff > p.config.MaxBackoff {
			backoff = p.config.MaxBackoff
		}
	}
}
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
func (p *JobProcessor) addJitter(d time.Duration) time.Duration {
	if p.config.JitterPercent <= 0 {
		return d
	}
	jitterFraction := float64(p.config.JitterPercent) / 100.0
	jitter := time.Duration(float64(d) * jitterFraction * rand.Float64())
	return d + jitter
}
func (p *JobProcessor) SetEscrowService(svc service.EscrowService) {
	p.escrowService = svc
}

func (p *JobProcessor) SetLogisticsService(svc LogisticsServiceInterface) {
	p.logisticsService = svc
}

func (p *JobProcessor) SetDriverService(svc DriverServiceInterface) {
	p.driverService = svc
}

func (p *JobProcessor) SetNotificationService(svc NotificationServiceInterface) {
	p.notificationService = svc
}

func (p *JobProcessor) SetErrandRepo(repo repository.ErrandRepository) {
	p.errandRepo = repo
}

func (p *JobProcessor) SetDistributedLock(lock DistributedLock) {
	p.lock = lock
}
func (p *JobProcessor) acquireLock(ctx context.Context, lockName string, ttl time.Duration) (release func(), proceed bool) {
	if p.lock == nil {
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
