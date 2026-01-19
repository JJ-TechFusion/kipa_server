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
	"kipa/internal/integration/maps"
	"kipa/internal/integration/paystack"
	"kipa/internal/integration/sms"
	"kipa/internal/integration/storage"
	"kipa/internal/repository/postgres"
	redisrepo "kipa/internal/repository/redis"
	"kipa/internal/service"
	"kipa/internal/util"
	"kipa/internal/websocket"
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

	// Initialize utilities
	hasher := util.NewBcryptHasher(cfg.OTP.BcryptCost)
	otpGenerator := util.NewSecureOTPGenerator()
	jwtManager := util.NewJWTManager(&cfg.JWT)

	// Initialize SMS provider (Termii)
	smsProvider := sms.NewTermiiClient(&cfg.Termii, logger)

	// Initialize Paystack client
	paystackClient := paystack.NewClient(&cfg.Paystack, logger)

	// Initialize S3 storage client
	s3Client, err := storage.NewS3Client(&cfg.S3)
	if err != nil {
		logger.Warn("Failed to initialize S3 client, uploads will be disabled", zap.Error(err))
	}

	// Initialize services
	authService := service.NewAuthService(
		userRepo,
		otpRepo,
		idempotencyRepo,
		sessionRepo,
		smsProvider,
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

	// Fee calculator service (uses Google Maps for road distance)
	feeCalculator := service.NewFeeCalculatorService(deliveryZoneRepo, mapsClient, cfg, logger)

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
	prService := service.NewPaymentRequestService(prRepo, prEventRepo, userRepo, deliveryJobRepo, feeCalculator, cfg, logger)

	// Escrow service
	escrowService := service.NewEscrowService(escrowRepo, walletService, feeCalculator, cfg, logger)

	// Order service (for payment processing)
	orderService := service.NewOrderService(
		orderRepo,
		prRepo,
		userRepo,
		escrowService,
		walletService,
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
		logger,
	)

	// Location service handles location updates, ETA, and storage
	locationHistoryRepo := postgres.NewLocationHistoryRepo(pgClient.DB)
	djRepo := deliveryJobRepo.(*postgres.DeliveryJobRepo)
	locationService := service.NewLocationService(
		locationHistoryRepo,
		djRepo,
		mapsClient,
		&service.LocationServiceConfig{
			ETAThrottleSeconds: 30,
			RateLimitMs:        1000,
		},
		logger,
	)

	// Rider service
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
		logger,
		cfg,
	)

	// Upload service
	uploadService := service.NewUploadService(s3Client, logger)

	// Initialize handlers
	authHandler := handler.NewAuthHandler(authService)
	authMiddleware := middleware.NewAuthMiddleware(authService)

	// New handlers for platform features
	prHandler := handler.NewPaymentRequestHandler(prService)
	buyerHandler := handler.NewBuyerHandler(prService, orderService, escrowService, userRepo, deliveryJobRepo)
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
	sellerHandler := handler.NewSellerHandler(orderService, prService, escrowService, deliveryJobRepo)
	errandHandler := handler.NewErrandHandler(errandService)
	uploadHandler := handler.NewUploadHandler(uploadService)

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
	buyerHandler.RegisterRoutes(v1, authMiddleware)
	sellerHandler.RegisterRoutes(v1, authMiddleware)
	riderHandler.RegisterRoutes(v1, authMiddleware)
	commissionHandler.RegisterRoutes(v1, authMiddleware)
	errandHandler.RegisterRoutes(v1, authMiddleware)
	walletHandler.RegisterRoutes(v1, authMiddleware)
	walletFundingHandler.RegisterRoutes(v1, authMiddleware)
	webhookHandler.RegisterRoutes(v1)
	uploadHandler.RegisterRoutes(v1, authMiddleware)

	// WebSocket handler
	wsHub := websocket.NewHub(logger)
	go wsHub.Run()



	// Set up WebSocket location handler
	wsHub.SetLocationHandler(func(riderID uuid.UUID, jobID string, lat, lng, heading, speed float64) {
		locationService.HandleLocationUpdate(context.Background(), riderID, jobID, lat, lng, heading, speed)
	})

	wsHandler := websocket.NewHandler(wsHub, authService, logger)
	wsHandler.RegisterRoutes(v1)

	// Tracking handler for REST-based location queries
	trackingHandler := handler.NewTrackingHandler(deliveryJobRepo, locationHistoryRepo, errandRepo)
	trackingHandler.RegisterRoutes(v1, authMiddleware)

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

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server exited")
}
