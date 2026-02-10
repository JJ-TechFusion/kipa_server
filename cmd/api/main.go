package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/config"
	"kipa/internal/database"
	"kipa/internal/handler"
	"kipa/internal/handler/middleware"
	"kipa/internal/integration/fcm"
	"kipa/internal/integration/maps"
	"kipa/internal/integration/paystack"
	"kipa/internal/integration/sms"
	"kipa/internal/integration/storage"
	"kipa/internal/repository/postgres"
	redisrepo "kipa/internal/repository/redis"
	"kipa/internal/service"
	"kipa/internal/util"
	"kipa/internal/websocket"
	"kipa/internal/worker"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize logger
	var logger *zap.Logger
	if cfg.Server.Environment == "production" {
		logger, err = zap.NewProduction()
	} else {
		logger, err = zap.NewDevelopment()
	}
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Sync()

	// Initialize database clients
	pgClient, err := database.NewPostgresClient(&cfg.Database)
	if err != nil {
		logger.Fatal("Failed to connect to PostgreSQL", zap.Error(err))
	}
	defer pgClient.Close()

	redisClient, err := database.NewRedisClient(&cfg.Redis)
	if err != nil {
		logger.Fatal("Failed to connect to Redis", zap.Error(err))
	}
	defer redisClient.Close()

	// Initialize repositories
	userRepo := postgres.NewUserRepo(pgClient.DB)
	otpRepo := postgres.NewOTPRepo(pgClient.DB)
	idempotencyRepo := postgres.NewIdempotencyRepo(pgClient.DB)
	sessionRepo := redisrepo.NewSessionRepo(redisClient.Client, cfg.JWT.RefreshTokenExpiry)
	riderLocationRepo := redisrepo.NewRiderLocationRepo(redisClient.Client)

	// New repositories for platform features
	walletRepo := postgres.NewWalletRepo(pgClient.DB)
	walletTxRepo := postgres.NewWalletTransactionRepo(pgClient.DB)
	prRepo := postgres.NewPaymentRequestRepo(pgClient.DB)
	prEventRepo := postgres.NewPaymentRequestEventRepo(pgClient.DB)
	escrowRepo := postgres.NewEscrowRepo(pgClient.DB)
	deliveryJobRepo := postgres.NewDeliveryJobRepo(pgClient.DB)
	riderProfileRepo := postgres.NewRiderProfileRepo(pgClient.DB)
	riderJobOfferRepo := postgres.NewRiderJobOfferRepo(pgClient.DB)
	deliveryZoneRepo := postgres.NewDeliveryZoneRepo(pgClient.DB)
	platformSettingsRepo := postgres.NewPlatformSettingsRepo(pgClient.DB)

	// Bank/wallet funding repositories
	bankAccountRepo := postgres.NewBankAccountRepo(pgClient.DB)
	virtualAccountRepo := postgres.NewVirtualAccountRepo(pgClient.DB)
	withdrawalRepo := postgres.NewWithdrawalRepo(pgClient.DB)
	topUpRepo := postgres.NewTopUpRepo(pgClient.DB)

	// Order repository
	orderRepo := postgres.NewOrderRepo(pgClient.DB)

	// Commission repositories
	riderCommissionRepo := postgres.NewRiderCommissionRepo(pgClient.DB)
	commissionPaymentRepo := postgres.NewCommissionPaymentRepo(pgClient.DB)

	// Errand repository
	errandRepo := postgres.NewErrandRepo(pgClient.DB)

	// Driver repositories (inter-state delivery)
	driverProfileRepo := postgres.NewDriverProfileRepo(pgClient.DB)
	driverTripRepo := postgres.NewDriverTripRepo(pgClient.DB)
	driverDeliveryRepo := postgres.NewDriverDeliveryRepo(pgClient.DB)
	driverDeliveryEventRepo := postgres.NewDriverDeliveryEventRepo(pgClient.DB)
	driverOfferRepo := postgres.NewDriverOfferRepo(pgClient.DB)

	// Notification repositories
	deviceTokenRepo := postgres.NewDeviceTokenRepo(pgClient.DB)
	notificationLogRepo := postgres.NewNotificationLogRepo(pgClient.DB)
	chatRepo := postgres.NewChatMessageRepo(pgClient.DB)

	// Initialize utilities
	hasher := util.NewBcryptHasher(cfg.OTP.BcryptCost)
	otpGenerator := util.NewSecureOTPGenerator()
	jwtManager := util.NewJWTManager(&cfg.JWT)

	// Initialize Twilio Verify provider
	verifyProvider := sms.NewTwilioVerifyClient(&cfg.Twilio, logger)

	// Initialize Paystack client
	paystackClient := paystack.NewClient(&cfg.Paystack, logger)

	// Initialize S3 storage client
	s3Client, err := storage.NewS3Client(&cfg.S3)
	if err != nil {
		logger.Warn("Failed to initialize S3 client, uploads will be disabled", zap.Error(err))
	}

	// Initialize FCM client (optional - if credentials not available, notifications will be logged only)
	var fcmClient *fcm.Client
	if cfg.Firebase.CredentialsPath != "" {
		fcmClient, err = fcm.NewClient(cfg.Firebase.CredentialsPath, logger)
		if err != nil {
			logger.Warn("Failed to initialize FCM client, push notifications will be disabled", zap.Error(err))
		}
	} else {
		logger.Info("FCM credentials not configured, push notifications disabled")
	}

	// Initialize services
	authService := service.NewAuthService(
		userRepo,
		otpRepo,
		idempotencyRepo,
		sessionRepo,
		verifyProvider,
		jwtManager,
		hasher,
		otpGenerator,
		cfg,
		logger,
	)

	// Initialize Google Maps client for fee calculation and ETA
	var mapsClient maps.MapsClient
	if cfg.GoogleMaps.APIKey != "" {
		mapsClient = maps.NewGoogleMapsClient(&maps.Config{
			APIKey: cfg.GoogleMaps.APIKey,
		}, logger)
	}

	// Fee calculator service (uses Google Maps for road distance + DB settings)
	feeCalculator := service.NewFeeCalculatorService(deliveryZoneRepo, platformSettingsRepo, mapsClient, cfg, logger)

	// WebSocket hub (created early as many services need it for broadcasting)
	wsHub := websocket.NewHub(logger)
	go wsHub.Run()

	// Wallet service
	walletService := service.NewWalletService(walletRepo, walletTxRepo, logger)

	// Wallet funding service
	walletFundingService := service.NewWalletFundingService(
		bankAccountRepo,
		virtualAccountRepo,
		withdrawalRepo,
		topUpRepo,
		walletRepo,
		walletTxRepo,
		userRepo,
		paystackClient,
		logger,
	)

	// Payment request service
	prService := service.NewPaymentRequestService(prRepo, prEventRepo, userRepo, deliveryJobRepo, feeCalculator, wsHub, cfg, logger)

	// Escrow service
	// Escrow service
	escrowService := service.NewEscrowService(escrowRepo, walletService, feeCalculator, cfg, logger)

	// Notification service
	notificationService := service.NewNotificationService(
		fcmClient,
		deviceTokenRepo,
		notificationLogRepo,
		logger,
	)

	// Logistics delivery repositories (needed for order service)
	logisticsDeliveryRepo := postgres.NewLogisticsDeliveryRepo(pgClient.DB)
	logisticsEventRepo := postgres.NewLogisticsDeliveryEventRepo(pgClient.DB)
	logisticsEvidenceRepo := postgres.NewLogisticsEvidenceRepo(pgClient.DB)

	// Dispute repository (needed for logistics service)
	disputeRepo := postgres.NewDisputeRepo(pgClient.DB)

	// Logistics delivery service (must be created before order service)
	logisticsService := service.NewLogisticsDeliveryService(
		logisticsDeliveryRepo,
		logisticsEventRepo,
		logisticsEvidenceRepo,
		escrowRepo,
		disputeRepo,
		userRepo,
		prRepo,
		walletService,
		notificationService,
		cfg,
		logger,
	)

	// Order service (for payment processing)
	orderService := service.NewOrderService(
		orderRepo,
		prRepo,
		userRepo,
		escrowService,
		walletService,
		logisticsService,
		notificationService,
		paystackClient,
		cfg,
		logger,
	)

	// Commission service
	commissionService := service.NewRiderCommissionService(
		riderCommissionRepo,
		commissionPaymentRepo,
		riderProfileRepo,
		walletService,
		logger,
	)

	// Errand service
	errandService := service.NewErrandService(
		errandRepo,
		deliveryJobRepo,
		feeCalculator,
		wsHub,
		logger,
	)

	// Location service handles location updates, ETA, and storage
	locationHistoryRepo := postgres.NewLocationHistoryRepo(pgClient.DB)
	djRepo := deliveryJobRepo.(*postgres.DeliveryJobRepo)

	locationService := service.NewLocationService(
		locationHistoryRepo,
		djRepo,
		mapsClient,
		wsHub,
		&service.LocationServiceConfig{
			ETAThrottleSeconds: 30,
			RateLimitMs:        1000,
		},
		logger,
	)

	// Upload service
	uploadService := service.NewUploadService(s3Client, logger)

	// Notification service


	// Rider service (needs notification service and broadcaster)
	riderService := service.NewRiderService(
		riderProfileRepo,
		deliveryJobRepo,
		riderJobOfferRepo,
		prRepo,
		escrowService,
		walletService,
		commissionService,
		errandService,
		locationService,
		notificationService,
		wsHub,
		riderLocationRepo,
		logger,
		cfg,
	)

	// Chat service
	chatService := service.NewChatService(
		chatRepo,
		deliveryJobRepo,
		prRepo,
		riderProfileRepo,
		notificationService,
		logger,
	)

	// Driver service (inter-state delivery)
	driverService := service.NewDriverService(
		driverProfileRepo,
		driverTripRepo,
		driverDeliveryRepo,
		driverDeliveryEventRepo,
		driverOfferRepo,
		prRepo,
		escrowService,
		walletService,
		notificationService,
		cfg,
		logger,
	)

	// Audit log repository
	auditLogRepo := postgres.NewAuditLogRepo(pgClient.DB)

	// Admin service
	adminService := service.NewAdminService(
		userRepo,
		riderProfileRepo,
		driverProfileRepo,
		platformSettingsRepo,
		walletTxRepo,
		walletRepo,
		prRepo,
		deliveryJobRepo,
		auditLogRepo,
		disputeRepo,
		escrowRepo,
		notificationService,
		logger,
	)

	// Dispute resolution service (for both intra-state and inter-state returns)
	disputeResolutionService := service.NewDisputeResolutionService(
		disputeRepo,
		escrowRepo,
		prRepo,
		deliveryJobRepo,
		userRepo,
		walletService,
		notificationService,
		feeCalculator,
		wsHub,
		cfg,
		logger,
	)



	// Initialize handlers
	authHandler := handler.NewAuthHandler(authService)
	authMiddleware := middleware.NewAuthMiddleware(authService)
	adminMiddleware := middleware.NewAdminMiddleware(userRepo)

	// New handlers for platform features
	prHandler := handler.NewPaymentRequestHandler(prService)
	transactionHandler := handler.NewTransactionHandler(prService, orderService, userRepo, deliveryJobRepo)
	buyerHandler := handler.NewBuyerHandler(prService, orderService, escrowService, userRepo, deliveryJobRepo, disputeResolutionService, driverService)
	riderHandler := handler.NewRiderHandler(riderService)
	commissionHandler := handler.NewRiderCommissionHandler(commissionService)
	walletHandler := handler.NewWalletHandler(walletService)
	walletFundingHandler := handler.NewWalletFundingHandler(
		walletFundingService,
		walletService,
		authService,
		userRepo,
		paystackClient,
		logger,
	)
	webhookHandler := handler.NewWebhookHandler(paystackClient, walletFundingService, orderService, logger)
	sellerHandler := handler.NewSellerHandler(orderService, prService, escrowService, deliveryJobRepo, disputeResolutionService, driverService)
	errandHandler := handler.NewErrandHandler(errandService)
	uploadHandler := handler.NewUploadHandler(uploadService)
	adminHandler := handler.NewAdminHandler(adminService, disputeResolutionService, deliveryJobRepo)
	notificationHandler := handler.NewNotificationHandler(notificationService, logger)
	chatHandler := handler.NewChatHandler(chatService, logger)
	logisticsHandler := handler.NewLogisticsHandler(logisticsService)
	driverHandler := handler.NewDriverHandler(driverService)

	// Setup Gin router
	if cfg.Server.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(gin.Logger())

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// API v1 routes
	v1 := router.Group("/api/v1")

	// Register all handlers
	authHandler.RegisterRoutes(v1, authMiddleware)
	prHandler.RegisterRoutes(v1, authMiddleware)
	transactionHandler.RegisterRoutes(v1, authMiddleware)
	buyerHandler.RegisterRoutes(v1, authMiddleware)
	sellerHandler.RegisterRoutes(v1, authMiddleware)
	riderHandler.RegisterRoutes(v1, authMiddleware)
	commissionHandler.RegisterRoutes(v1, authMiddleware)
	errandHandler.RegisterRoutes(v1, authMiddleware)
	walletHandler.RegisterRoutes(v1, authMiddleware)
	walletFundingHandler.RegisterRoutes(v1, authMiddleware)
	webhookHandler.RegisterRoutes(v1)
	uploadHandler.RegisterRoutes(v1, authMiddleware)
	adminHandler.RegisterRoutes(v1, authMiddleware, adminMiddleware)
	chatHandler.RegisterRoutes(v1, authMiddleware)
	logisticsHandler.RegisterRoutes(v1, authMiddleware, adminMiddleware)
	driverHandler.RegisterRoutes(v1, authMiddleware)

	// Notification routes (authenticated users)
	usersRoutes := v1.Group("/users")
	usersRoutes.Use(authMiddleware.RequireAuth())
	notificationHandler.RegisterRoutes(usersRoutes)





	// Set up WebSocket location handler
	wsHub.SetLocationHandler(func(riderID uuid.UUID, jobID string, lat, lng, heading, speed float64) {
		locationService.HandleLocationUpdate(context.Background(), riderID, jobID, lat, lng, heading, speed)
	})
	
	wsHub.SetChatHandler(func(senderID uuid.UUID, jobID, content, mediaURL, mediaType string) (string, int64, error) {
		id, err := uuid.Parse(jobID)
		if err != nil {
			return "", 0, fmt.Errorf("invalid job ID")
		}
		
		var mUrl *string
		if mediaURL != "" {
			mUrl = &mediaURL
		}
		
		msg, err := chatService.SendMessage(context.Background(), senderID, id, content, mUrl, mediaType)
		if err != nil {
			return "", 0, err
		}
		
		return msg.ID.String(), msg.CreatedAt.Unix(), nil
	})

	wsHandler := websocket.NewHandler(wsHub, authService, logger)
	wsHandler.RegisterRoutes(v1)

	// Tracking handler for REST-based location queries
	trackingHandler := handler.NewTrackingHandler(deliveryJobRepo, locationHistoryRepo, errandRepo, riderProfileRepo, userRepo)
	trackingHandler.RegisterRoutes(v1, authMiddleware)

	// Start API background jobs (jobs that need WebSocket access)
	riderLocationAdapter := worker.NewRiderLocationAdapter(riderLocationRepo)
	apiJobRunner := worker.NewAPIJobRunner(
		deliveryJobRepo,
		riderLocationAdapter,
		wsHub,
		logger,
	)
	apiJobCtx, apiJobCancel := context.WithCancel(context.Background())
	apiJobRunner.Start(apiJobCtx)

	// Create HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Start server in goroutine
	go func() {
		logger.Info("Starting server",
			zap.String("port", cfg.Server.Port),
			zap.String("environment", cfg.Server.Environment),
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// Stop API background jobs
	apiJobCancel()
	apiJobRunner.Stop()

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server exited")
}
