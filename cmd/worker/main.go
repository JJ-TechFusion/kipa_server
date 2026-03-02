package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"kipa/internal/config"
	"kipa/internal/database"
	"kipa/internal/integration/fcm"
	"kipa/internal/repository/postgres"
	redisrepo "kipa/internal/repository/redis"
	"kipa/internal/service"
	"kipa/internal/worker"
)

func main() {
	// Initialize logger
	log, err := zap.NewProduction()
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}
	defer func() {
		_ = log.Sync()
	}()

	log.Info("starting Kipa background worker")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("failed to load config", zap.Error(err))
	}

	if !cfg.Worker.Enabled {
		log.Info("worker is disabled in configuration, exiting")
		return
	}

	// Connect to database
	dbClient, err := database.NewPostgresClient(&cfg.Database)
	if err != nil {
		log.Fatal("failed to connect to database", zap.Error(err))
	}
	defer dbClient.Close()
	log.Info("connected to database")

	db := dbClient.DB

	// Connect to Redis (for distributed locking)
	redisClient, err := database.NewRedisClient(&cfg.Redis)
	if err != nil {
		log.Fatal("failed to connect to Redis", zap.Error(err))
	}
	defer redisClient.Close()
	log.Info("connected to Redis")

	// Initialize repositories
	escrowRepo := postgres.NewEscrowRepo(db)
	walletRepo := postgres.NewWalletRepo(db)
	walletTxRepo := postgres.NewWalletTransactionRepo(db)
	deliveryJobRepo := postgres.NewDeliveryJobRepo(db)
	paymentRequestRepo := postgres.NewPaymentRequestRepo(db)
	deliveryZoneRepo := postgres.NewDeliveryZoneRepo(db)
	riderOfferRepo := postgres.NewRiderJobOfferRepo(db)
	platformSettingsRepo := postgres.NewPlatformSettingsRepo(db)

	// Driver delivery repositories (for inter-state auto-confirm job)
	driverProfileRepo := postgres.NewDriverProfileRepo(db)
	driverTripRepo := postgres.NewDriverTripRepo(db)
	driverDeliveryRepo := postgres.NewDriverDeliveryRepo(db)
	driverDeliveryEventRepo := postgres.NewDriverDeliveryEventRepo(db)
	driverOfferRepo := postgres.NewDriverOfferRepo(db)

	// Initialize services
	// Note: Wallet service in worker doesn't need PIN reset capabilities,
	// so we pass nil for userRepo, sessionRepo, verifyProvider, notificationService, and config
	walletService := service.NewWalletService(walletRepo, walletTxRepo, nil, nil, nil, nil, nil, log)
	feeCalculator := service.NewFeeCalculatorService(deliveryZoneRepo, platformSettingsRepo, nil, cfg, log)
	escrowService := service.NewEscrowService(escrowRepo, walletService, feeCalculator, cfg, log)

	// Driver service (for inter-state delivery auto-confirm and offer expiry)
	driverService := service.NewDriverService(
		driverProfileRepo,
		driverTripRepo,
		driverDeliveryRepo,
		driverDeliveryEventRepo,
		driverOfferRepo,
		paymentRequestRepo,
		escrowService,
		walletService,
		nil, // notification service not needed for auto-confirm in worker
		cfg,
		log,
	)


	// Initialize notification service repos
	deviceTokenRepo := postgres.NewDeviceTokenRepo(db)
	notificationLogRepo := postgres.NewNotificationLogRepo(db)

	// Initialize FCM client for push notifications
	var fcmClient *fcm.Client
	if cfg.Firebase.CredentialsPath != "" {
		var err error
		fcmClient, err = fcm.NewClient(cfg.Firebase.CredentialsPath, log)
		if err != nil {
			log.Warn("failed to initialize FCM client, notifications will be disabled", zap.Error(err))
		}
	}

	// Initialize notification service
	notificationService := service.NewNotificationService(
		fcmClient,
		deviceTokenRepo,
		notificationLogRepo,
		log,
	)

	// Initialize distributed lock
	distributedLock := redisrepo.NewDistributedLock(redisClient.Client)

	// Convert config to worker config
	workerConfig := &worker.WorkerConfig{
		EscrowCheckInterval:      cfg.Worker.EscrowCheckInterval,
		RiderSearchCheckInterval: cfg.Worker.RiderSearchCheckInterval,
		LinkExpiryCheckInterval:  cfg.Worker.LinkExpiryCheckInterval,
		RadiusExpandInterval:     cfg.Worker.RadiusExpandInterval,
		TimerExpiryCheckInterval: cfg.Worker.TimerExpiryCheckInterval,
		RiderSearchTimeoutMins:   cfg.Worker.RiderSearchTimeoutMins,
		RadiusExpandSteps:        cfg.Worker.RadiusExpandSteps,
		RadiusExpandIntervalMins: cfg.Worker.RadiusExpandIntervalMins,
		RiderOfferTimeoutMins:    cfg.Worker.RiderOfferTimeoutMins,
		MaxJobRestarts:           cfg.Worker.MaxJobRestarts,
		InitialBackoff:           cfg.Worker.InitialBackoff,
		MaxBackoff:           cfg.Worker.MaxBackoff,
		JitterPercent:        cfg.Worker.JitterPercent,
		ReminderCooldownMins: cfg.Worker.ReminderCooldownMins,
	}

	// Initialize job processor
	// Note: broadcaster is nil in worker - WebSocket broadcasts happen only from API server
	processor := worker.NewJobProcessor(
		escrowService,
		deliveryJobRepo,
		paymentRequestRepo,
		riderOfferRepo,
		nil, // broadcaster - not available in worker process
		workerConfig,
		log,
	)

	// Enable distributed locking for multi-instance deployments
	processor.SetDistributedLock(distributedLock)

	// Set driver service for auto-confirm job
	processor.SetDriverService(driverService)

	// Set notification service for rebook expiry notifications
	processor.SetNotificationService(notificationService)

	// Create context that can be cancelled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the job processor
	processor.Start(ctx)

	log.Info("background worker started",
		zap.Duration("escrow_check_interval", cfg.Worker.EscrowCheckInterval),
		zap.Duration("rider_search_check_interval", cfg.Worker.RiderSearchCheckInterval),
		zap.Duration("link_expiry_check_interval", cfg.Worker.LinkExpiryCheckInterval),
	)

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh

	log.Info("received shutdown signal", zap.String("signal", sig.String()))

	// Cancel context to signal all goroutines to stop
	cancel()

	// Wait for all jobs to complete
	processor.Stop()

	log.Info("background worker stopped gracefully")
}
