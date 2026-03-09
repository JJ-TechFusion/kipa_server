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
	"kipa/internal/integration/flutterwave"
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
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
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
	userRepo := postgres.NewUserRepo(pgClient.DB)
	otpRepo := postgres.NewOTPRepo(pgClient.DB)
	idempotencyRepo := postgres.NewIdempotencyRepo(pgClient.DB)
	sessionRepo := redisrepo.NewSessionRepo(redisClient.Client, cfg.JWT.RefreshTokenExpiry)
	riderLocationRepo := redisrepo.NewRiderLocationRepo(redisClient.Client)
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
	bankAccountRepo := postgres.NewBankAccountRepo(pgClient.DB)
	virtualAccountRepo := postgres.NewVirtualAccountRepo(pgClient.DB)
	withdrawalRepo := postgres.NewWithdrawalRepo(pgClient.DB)
	topUpRepo := postgres.NewTopUpRepo(pgClient.DB)
	flwVirtualAccountRepo := postgres.NewFlutterwaveVirtualAccountRepo(pgClient.DB)
	orderRepo := postgres.NewOrderRepo(pgClient.DB)
	riderCommissionRepo := postgres.NewRiderCommissionRepo(pgClient.DB)
	commissionPaymentRepo := postgres.NewCommissionPaymentRepo(pgClient.DB)
	errandRepo := postgres.NewErrandRepo(pgClient.DB)
	driverProfileRepo := postgres.NewDriverProfileRepo(pgClient.DB)
	driverTripRepo := postgres.NewDriverTripRepo(pgClient.DB)
	driverDeliveryRepo := postgres.NewDriverDeliveryRepo(pgClient.DB)
	driverDeliveryEventRepo := postgres.NewDriverDeliveryEventRepo(pgClient.DB)
	driverOfferRepo := postgres.NewDriverOfferRepo(pgClient.DB)
	deviceTokenRepo := postgres.NewDeviceTokenRepo(pgClient.DB)
	notificationLogRepo := postgres.NewNotificationLogRepo(pgClient.DB)
	chatRepo := postgres.NewChatMessageRepo(pgClient.DB)
	hasher := util.NewBcryptHasher(cfg.OTP.BcryptCost)
	otpGenerator := util.NewSecureOTPGenerator()
	jwtManager := util.NewJWTManager(&cfg.JWT)
	verifyProvider := sms.NewTwilioVerifyClient(&cfg.Twilio, logger)
	paystackClient := paystack.NewClient(&cfg.Paystack, logger)
	flutterwaveClient := flutterwave.NewClient(&cfg.Flutterwave, logger)
	s3Client, err := storage.NewS3Client(&cfg.S3)
	if err != nil {
		logger.Warn("Failed to initialize S3 client, uploads will be disabled", zap.Error(err))
	}
	var fcmClient *fcm.Client
	if cfg.Firebase.CredentialsPath != "" {
		fcmClient, err = fcm.NewClient(cfg.Firebase.CredentialsPath, logger)
		if err != nil {
			logger.Warn("Failed to initialize FCM client, push notifications will be disabled", zap.Error(err))
		}
	} else {
		logger.Info("FCM credentials not configured, push notifications disabled")
	}
	authService := service.NewAuthService(
		userRepo,
		otpRepo,
		idempotencyRepo,
		sessionRepo,
		escrowRepo,
		deliveryJobRepo,
		walletRepo,
		verifyProvider,
		jwtManager,
		hasher,
		otpGenerator,
		cfg,
		logger,
	)
	var mapsClient maps.MapsClient
	if cfg.GoogleMaps.APIKey != "" {
		mapsClient = maps.NewGoogleMapsClient(&maps.Config{
			APIKey: cfg.GoogleMaps.APIKey,
		}, logger)
	}
	feeCalculator := service.NewFeeCalculatorService(deliveryZoneRepo, platformSettingsRepo, mapsClient, cfg, logger)
	wsHub := websocket.NewHub(logger)
	go wsHub.Run()
	notificationService := service.NewNotificationService(
		fcmClient,
		deviceTokenRepo,
		notificationLogRepo,
		logger,
	)
	walletFundingConfig := service.WalletFundingConfig{
		MinTopUpAmount:      100,  // ₦100 minimum
		MinWithdrawalAmount: 1000, // ₦1000 minimum
		WithdrawalFee:       50,   // ₦50 fixed fee
		WithdrawalFeeType:   "fixed",
	}
	walletFundingService := service.NewWalletFundingService(
		bankAccountRepo,
		virtualAccountRepo,
		withdrawalRepo,
		topUpRepo,
		walletRepo,
		walletTxRepo,
		userRepo,
		flwVirtualAccountRepo,
		paystackClient,
		flutterwaveClient,
		notificationService,
		walletFundingConfig,
		logger,
	)
	prService := service.NewPaymentRequestService(prRepo, prEventRepo, userRepo, deliveryJobRepo, feeCalculator, wsHub, cfg, logger)
	walletService := service.NewWalletService(
		walletRepo,
		walletTxRepo,
		userRepo,
		sessionRepo,
		verifyProvider,
		notificationService,
		cfg,
		logger,
	)
	escrowService := service.NewEscrowService(escrowRepo, walletService, feeCalculator, cfg, logger)
	logisticsDeliveryRepo := postgres.NewLogisticsDeliveryRepo(pgClient.DB)
	logisticsEventRepo := postgres.NewLogisticsDeliveryEventRepo(pgClient.DB)
	logisticsEvidenceRepo := postgres.NewLogisticsEvidenceRepo(pgClient.DB)
	disputeRepo := postgres.NewDisputeRepo(pgClient.DB)
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
	orderService := service.NewOrderService(
		orderRepo,
		prRepo,
		userRepo,
		escrowService,
		walletService,
		logisticsService,
		notificationService,
		paystackClient,
		flutterwaveClient,
		cfg,
		logger,
	)
	commissionService := service.NewRiderCommissionService(
		riderCommissionRepo,
		commissionPaymentRepo,
		riderProfileRepo,
		walletService,
		logger,
	)
	errandService := service.NewErrandService(
		errandRepo,
		deliveryJobRepo,
		riderProfileRepo,
		userRepo,
		feeCalculator,
		wsHub,
		notificationService,
		logger,
	)
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
	uploadService := service.NewUploadService(s3Client, logger)
	riderService := service.NewRiderService(
		riderProfileRepo,
		deliveryJobRepo,
		riderJobOfferRepo,
		prRepo,
		userRepo,
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
	chatService := service.NewChatService(
		chatRepo,
		deliveryJobRepo,
		prRepo,
		riderProfileRepo,
		userRepo,
		errandRepo,
		notificationService,
		logger,
	)
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
	auditLogRepo := postgres.NewAuditLogRepo(pgClient.DB)
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
	disputeService := service.NewDisputeService(
		disputeRepo,
		escrowRepo,
		userRepo,
		prRepo,
		logger,
	)
	supportConversationRepo := postgres.NewSupportConversationRepo(pgClient.DB)
	supportMessageRepo := postgres.NewSupportMessageRepo(pgClient.DB)
	supportChatService := service.NewSupportChatService(
		supportConversationRepo,
		supportMessageRepo,
		disputeRepo,
		escrowRepo,
		prRepo,
		userRepo,
		notificationService,
		logger,
	)
	authHandler := handler.NewAuthHandler(authService)
	authMiddleware := middleware.NewAuthMiddleware(authService)
	adminMiddleware := middleware.NewAdminMiddleware(userRepo)
	prHandler := handler.NewPaymentRequestHandler(prService)
	transactionHandler := handler.NewTransactionHandler(prService, orderService, userRepo, deliveryJobRepo)
	buyerHandler := handler.NewBuyerHandler(prService, orderService, escrowService, userRepo, deliveryJobRepo, disputeRepo, disputeResolutionService, driverService)
	riderHandler := handler.NewRiderHandler(riderService)
	commissionHandler := handler.NewRiderCommissionHandler(commissionService)
	walletHandler := handler.NewWalletHandler(walletService, escrowService)
	walletFundingHandler := handler.NewWalletFundingHandler(
		walletFundingService,
		walletService,
		authService,
		userRepo,
		paystackClient,
		flutterwaveClient,
		logger,
	)
	webhookHandler := handler.NewWebhookHandler(paystackClient, flutterwaveClient, walletFundingService, orderService, logger)
	sellerHandler := handler.NewSellerHandler(orderService, prService, escrowService, deliveryJobRepo, disputeResolutionService, driverService)
	errandHandler := handler.NewErrandHandler(errandService)
	disputeHandler := handler.NewDisputeHandler(disputeService)
	uploadHandler := handler.NewUploadHandler(uploadService)
	adminHandler := handler.NewAdminHandler(adminService, disputeResolutionService, deliveryJobRepo, prService)
	notificationHandler := handler.NewNotificationHandler(notificationService, logger)
	chatHandler := handler.NewChatHandler(chatService, logger)
	supportChatHandler := handler.NewSupportChatHandler(supportChatService, logger)
	logisticsHandler := handler.NewLogisticsHandler(logisticsService)
	driverHandler := handler.NewDriverHandler(driverService)
	if cfg.Server.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(gin.Logger())
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	v1 := router.Group("/api/v1")
	authHandler.RegisterRoutes(v1, authMiddleware)
	prHandler.RegisterRoutes(v1, authMiddleware)
	transactionHandler.RegisterRoutes(v1, authMiddleware)
	buyerHandler.RegisterRoutes(v1, authMiddleware)
	sellerHandler.RegisterRoutes(v1, authMiddleware)
	riderHandler.RegisterRoutes(v1, authMiddleware)
	commissionHandler.RegisterRoutes(v1, authMiddleware)
	errandHandler.RegisterRoutes(v1, authMiddleware)
	disputeHandler.RegisterRoutes(v1, authMiddleware)
	walletHandler.RegisterRoutes(v1, authMiddleware)
	walletFundingHandler.RegisterRoutes(v1, authMiddleware)
	webhookHandler.RegisterRoutes(v1)
	uploadHandler.RegisterRoutes(v1, authMiddleware)
	adminHandler.RegisterRoutes(v1, authMiddleware, adminMiddleware)
	chatHandler.RegisterRoutes(v1, authMiddleware)
	supportChatHandler.RegisterRoutes(v1, authMiddleware, adminMiddleware)
	logisticsHandler.RegisterRoutes(v1, authMiddleware, adminMiddleware)
	driverHandler.RegisterRoutes(v1, authMiddleware)
	usersRoutes := v1.Group("/users")
	usersRoutes.Use(authMiddleware.RequireAuth())
	notificationHandler.RegisterRoutes(usersRoutes)
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
	trackingHandler := handler.NewTrackingHandler(deliveryJobRepo, locationHistoryRepo, errandRepo, riderProfileRepo, userRepo)
	trackingHandler.RegisterRoutes(v1, authMiddleware)
	riderLocationAdapter := worker.NewRiderLocationAdapter(riderLocationRepo)
	apiJobRunner := worker.NewAPIJobRunner(
		deliveryJobRepo,
		riderLocationAdapter,
		wsHub,
		logger,
	)
	apiJobCtx, apiJobCancel := context.WithCancel(context.Background())
	apiJobRunner.Start(apiJobCtx)
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}
	go func() {
		logger.Info("Starting server",
			zap.String("port", cfg.Server.Port),
			zap.String("environment", cfg.Server.Environment),
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start server", zap.Error(err))
		}
	}()
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")
	apiJobCancel()
	apiJobRunner.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server exited")
}
