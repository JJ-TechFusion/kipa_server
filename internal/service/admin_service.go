package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/domain"
	"kipa/internal/repository"
)

type AdminService interface {
	// Rider Management
	VerifyRider(ctx context.Context, riderID uuid.UUID, adminID uuid.UUID) error
	SuspendRider(ctx context.Context, riderID uuid.UUID, reason string, adminID uuid.UUID) error
	UnsuspendRider(ctx context.Context, riderID uuid.UUID, adminID uuid.UUID) error
	GetPendingRiders(ctx context.Context, limit, offset int) ([]*domain.RiderProfile, error)
	GetRiderStatistics(ctx context.Context) (*RiderStatistics, error)

	// Driver Management (Inter-state)
	VerifyDriver(ctx context.Context, driverID uuid.UUID, adminID uuid.UUID) error
	SuspendDriver(ctx context.Context, driverID uuid.UUID, reason string, adminID uuid.UUID) error
	UnsuspendDriver(ctx context.Context, driverID uuid.UUID, adminID uuid.UUID) error
	GetPendingDrivers(ctx context.Context, limit, offset int) ([]*domain.DriverProfile, error)
	GetDriverStatistics(ctx context.Context) (*repository.DriverStatistics, error)

	// User Management
	ListUsers(ctx context.Context, filters *UserFilters, limit, offset int) ([]*domain.User, error)
	SuspendUser(ctx context.Context, userID uuid.UUID, reason string, adminID uuid.UUID) error
	UnsuspendUser(ctx context.Context, userID uuid.UUID, adminID uuid.UUID) error
	VerifySeller(ctx context.Context, userID uuid.UUID, adminID uuid.UUID) error

	// Platform Settings
	GetDeliveryPricing(ctx context.Context) (*domain.DeliveryPricing, error)
	UpdateDeliveryPricing(ctx context.Context, pricing *domain.DeliveryPricing, adminID uuid.UUID) error
	GetPlatformFees(ctx context.Context) (*domain.PlatformFees, error)
	UpdatePlatformFees(ctx context.Context, fees *domain.PlatformFees, adminID uuid.UUID) error

	// Analytics
	GetPlatformRevenue(ctx context.Context, startDate, endDate *string) (*repository.RevenueAnalytics, error)
	GetUserStatistics(ctx context.Context) (*UserStatistics, error)
	GetActiveDeliveries(ctx context.Context) (*ActiveDeliveriesStats, error)
	GetRiderPerformance(ctx context.Context, riderID *uuid.UUID) ([]*RiderPerformance, error)

	// Payment Request Management
	ListPaymentRequests(ctx context.Context, filters *PaymentRequestFilters, limit, offset int) ([]*domain.PaymentRequest, error)
	CancelPaymentRequest(ctx context.Context, prID uuid.UUID, reason string, adminID uuid.UUID) error
	GetExpiredPaymentRequests(ctx context.Context, limit, offset int) ([]*domain.PaymentRequest, error)

	// System Health
	CheckDatabaseHealth(ctx context.Context) (*HealthStatus, error)
	CheckRedisHealth(ctx context.Context) (*HealthStatus, error)
	CheckExternalServices(ctx context.Context) (*ExternalServicesHealth, error)

	// Audit Logging
	ListAuditLogs(ctx context.Context, filters *repository.AuditLogFilters, limit, offset int) ([]*domain.AuditLog, error)
	GetUserAuditTrail(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.AuditLog, error)

	// Dispute Management
	ListDisputes(ctx context.Context, filters *repository.DisputeFilters, limit, offset int) ([]*domain.Dispute, error)
	GetDispute(ctx context.Context, disputeID uuid.UUID) (*domain.DisputeDetails, error)
	ResolveDispute(ctx context.Context, disputeID uuid.UUID, outcome domain.DisputeOutcome, notes string, adminID uuid.UUID) error
	ProcessRefund(ctx context.Context, disputeID uuid.UUID, refundType string, amount float64, reason string, adminID uuid.UUID) error
	GetDisputeStatistics(ctx context.Context) (*repository.DisputeStatistics, error)

	// Notification Management
	SendBroadcast(ctx context.Context, title, body, topic string, adminID uuid.UUID) error

	// Commission Management
	GetCommissionSettings(ctx context.Context) (*domain.CommissionSettings, error)
	UpdateCommissionSettings(ctx context.Context, settings *domain.CommissionSettings, adminID uuid.UUID) error

	// Wallet Management
	GetUserWallet(ctx context.Context, userID uuid.UUID) (*domain.Wallet, error)
	AdjustWallet(ctx context.Context, userID uuid.UUID, amount float64, adjustmentType string, reason string, adminID uuid.UUID) error
	GetWalletTransactions(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.WalletTransaction, error)
}

type UserFilters struct {
	IsSeller    *bool
	IsBuyer     *bool
	IsRider     *bool
	IsSuspended *bool
	PhoneNumber *string
}

type RiderStatistics struct {
	TotalRiders     int     `json:"total_riders"`
	VerifiedRiders  int     `json:"verified_riders"`
	PendingRiders   int     `json:"pending_riders"`
	SuspendedRiders int     `json:"suspended_riders"`
	OnlineRiders    int     `json:"online_riders"`
	AverageRating   float64 `json:"average_rating"`
	TotalDeliveries int     `json:"total_deliveries"`
}

type UserStatistics struct {
	TotalUsers     int `json:"total_users"`
	TotalSellers   int `json:"total_sellers"`
	TotalBuyers    int `json:"total_buyers"`
	TotalRiders    int `json:"total_riders"`
	SuspendedUsers int `json:"suspended_users"`
}

type AdminServiceImpl struct {
	userRepo            repository.UserRepository
	riderRepo           repository.RiderProfileRepository
	driverRepo          repository.DriverProfileRepository
	settingsRepo        repository.PlatformSettingsRepository
	walletTxRepo        repository.WalletTransactionRepository
	walletRepo          repository.WalletRepository
	prRepo              repository.PaymentRequestRepository
	deliveryJobRepo     repository.DeliveryJobRepository
	auditLogRepo        repository.AuditLogRepository
	disputeRepo         repository.DisputeRepository
	escrowRepo          repository.EscrowRepository
	redisClient         RedisHealthChecker
	notificationService NotificationService
	logger              *zap.Logger
}

type RedisHealthChecker interface {
	Ping(ctx context.Context) error
}

type ActiveDeliveriesStats struct {
	TotalActive     int     `json:"total_active"`
	Searching       int     `json:"searching"`
	Assigned        int     `json:"assigned"`
	InTransit       int     `json:"in_transit"`
	AverageDistance float64 `json:"average_distance_km"`
}

type RiderPerformance struct {
	RiderID         uuid.UUID `json:"rider_id"`
	RiderName       string    `json:"rider_name"`
	TotalDeliveries int       `json:"total_deliveries"`
	CompletedToday  int       `json:"completed_today"`
	AverageRating   float64   `json:"average_rating"`
	TotalEarnings   float64   `json:"total_earnings"`
	OnlineStatus    bool      `json:"online_status"`
}

type PaymentRequestFilters struct {
	Status       *domain.PaymentRequestStatus
	SellerID     *uuid.UUID
	BuyerID      *uuid.UUID
	DeliveryType *domain.DeliveryType
}

type HealthStatus struct {
	Status    string `json:"status"` // "healthy", "degraded", "down"
	Message   string `json:"message,omitempty"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
}

type ExternalServicesHealth struct {
	Paystack   *HealthStatus `json:"paystack"`
	GoogleMaps *HealthStatus `json:"google_maps"`
	Twilio     *HealthStatus `json:"twilio"`
}

func NewAdminService(
	userRepo repository.UserRepository,
	riderRepo repository.RiderProfileRepository,
	driverRepo repository.DriverProfileRepository,
	settingsRepo repository.PlatformSettingsRepository,
	walletTxRepo repository.WalletTransactionRepository,
	walletRepo repository.WalletRepository,
	prRepo repository.PaymentRequestRepository,
	deliveryJobRepo repository.DeliveryJobRepository,
	auditLogRepo repository.AuditLogRepository,
	disputeRepo repository.DisputeRepository,
	escrowRepo repository.EscrowRepository,
	notificationService NotificationService,
	logger *zap.Logger,
) AdminService {
	return &AdminServiceImpl{
		userRepo:            userRepo,
		riderRepo:           riderRepo,
		driverRepo:          driverRepo,
		settingsRepo:        settingsRepo,
		walletTxRepo:        walletTxRepo,
		walletRepo:          walletRepo,
		prRepo:              prRepo,
		deliveryJobRepo:     deliveryJobRepo,
		auditLogRepo:        auditLogRepo,
		disputeRepo:         disputeRepo,
		escrowRepo:          escrowRepo,
		notificationService: notificationService,
		logger:              logger,
	}
}

// Rider Management

func (s *AdminServiceImpl) VerifyRider(ctx context.Context, riderID uuid.UUID, adminID uuid.UUID) error {
	rider, err := s.riderRepo.GetByID(ctx, riderID)
	if err != nil {
		return fmt.Errorf("failed to get rider: %w", err)
	}

	if rider.Status == domain.RiderStatusVerified {
		return fmt.Errorf("rider already verified")
	}

	if err := s.riderRepo.UpdateStatus(ctx, riderID, domain.RiderStatusVerified); err != nil {
		return fmt.Errorf("failed to update rider status: %w", err)
	}

	if err := s.userRepo.SetIsRider(ctx, rider.UserID); err != nil {
		s.logger.Warn("failed to set is_rider for verified rider",
			zap.String("user_id", rider.UserID.String()),
			zap.Error(err),
		)
	}

	s.logger.Info("rider verified by admin",
		zap.String("rider_id", riderID.String()),
		zap.String("user_id", rider.UserID.String()),
		zap.String("admin_id", adminID.String()),
	)

	return nil
}

func (s *AdminServiceImpl) SuspendRider(ctx context.Context, riderID uuid.UUID, reason string, adminID uuid.UUID) error {
	rider, err := s.riderRepo.GetByID(ctx, riderID)
	if err != nil {
		return fmt.Errorf("failed to get rider: %w", err)
	}

	if rider.Status == domain.RiderStatusSuspended {
		return fmt.Errorf("rider already suspended")
	}

	if err := s.riderRepo.UpdateStatus(ctx, riderID, domain.RiderStatusSuspended); err != nil {
		return fmt.Errorf("failed to suspend rider: %w", err)
	}

	// Also suspend the user account
	if err := s.userRepo.Suspend(ctx, rider.UserID, reason); err != nil {
		s.logger.Error("failed to suspend rider's user account", zap.Error(err))
	}

	s.logger.Info("rider suspended by admin",
		zap.String("rider_id", riderID.String()),
		zap.String("admin_id", adminID.String()),
		zap.String("reason", reason),
	)

	return nil
}

func (s *AdminServiceImpl) UnsuspendRider(ctx context.Context, riderID uuid.UUID, adminID uuid.UUID) error {
	rider, err := s.riderRepo.GetByID(ctx, riderID)
	if err != nil {
		return fmt.Errorf("failed to get rider: %w", err)
	}

	if rider.Status != domain.RiderStatusSuspended {
		return fmt.Errorf("rider is not suspended")
	}

	// Restore to verified status
	if err := s.riderRepo.UpdateStatus(ctx, riderID, domain.RiderStatusVerified); err != nil {
		return fmt.Errorf("failed to unsuspend rider: %w", err)
	}

	// Also unsuspend the user account
	if err := s.userRepo.Unsuspend(ctx, rider.UserID); err != nil {
		s.logger.Error("failed to unsuspend rider's user account", zap.Error(err))
	}

	s.logger.Info("rider unsuspended by admin",
		zap.String("rider_id", riderID.String()),
		zap.String("admin_id", adminID.String()),
	)

	return nil
}

func (s *AdminServiceImpl) GetPendingRiders(ctx context.Context, limit, offset int) ([]*domain.RiderProfile, error) {
	return s.riderRepo.GetByStatus(ctx, domain.RiderStatusPendingVerification, limit, offset)
}

func (s *AdminServiceImpl) GetRiderStatistics(ctx context.Context) (*RiderStatistics, error) {
	stats, err := s.riderRepo.GetStatistics(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get rider statistics: %w", err)
	}

	return &RiderStatistics{
		TotalRiders:     stats.TotalRiders,
		VerifiedRiders:  stats.VerifiedRiders,
		PendingRiders:   stats.PendingRiders,
		SuspendedRiders: stats.SuspendedRiders,
		OnlineRiders:    stats.OnlineRiders,
		AverageRating:   stats.AverageRating,
		TotalDeliveries: stats.TotalDeliveries,
	}, nil
}

// Driver Management

func (s *AdminServiceImpl) VerifyDriver(ctx context.Context, driverID uuid.UUID, adminID uuid.UUID) error {
	driver, err := s.driverRepo.GetByID(ctx, driverID)
	if err != nil {
		return fmt.Errorf("failed to get driver: %w", err)
	}

	if driver.IsVerified {
		return fmt.Errorf("driver already verified")
	}

	// For drivers, verification involves setting is_verified = true
	// and potentially updating status from loading/idle
	// The repo update will handle the fields.
	// However, we need to construct a robust update or just add a Verify method to repo if not exists.
	// Looking at driver_repo.go, Create inserts is_verified. Update updates is_verified.
	// Let's check if there is a specific Verify method or we just use Update.
	// The repository structure is common. Let's assume we update the profile.

	driver.IsVerified = true
	driver.Status = domain.DriverStatusIdle // Set to idle so they can start working
	
	if err := s.driverRepo.Update(ctx, driver); err != nil {
		return fmt.Errorf("failed to verify driver: %w", err)
	}

	s.logger.Info("driver verified by admin",
		zap.String("driver_id", driverID.String()),
		zap.String("user_id", driver.UserID.String()),
		zap.String("admin_id", adminID.String()),
	)

	return nil
}

func (s *AdminServiceImpl) SuspendDriver(ctx context.Context, driverID uuid.UUID, reason string, adminID uuid.UUID) error {
	driver, err := s.driverRepo.GetByID(ctx, driverID)
	if err != nil {
		return fmt.Errorf("failed to get driver: %w", err)
	}

	// We don't have a specific "suspended" status in DriverStatus enum for inter-state drivers in the file viewed earlier?
	// Checking internal/domain/driver.go:
	// const (
	// 	DriverStatusOffline    DriverStatus = "offline"
	// 	DriverStatusIdle       DriverStatus = "idle"
	// 	DriverStatusLoading    DriverStatus = "loading"
	// 	DriverStatusInTransit  DriverStatus = "in_transit"
	// 	DriverStatusDelivering DriverStatus = "delivering"
	// )
	// It appears there is NO "suspended" status for drivers in the domain model I viewed. 
	// However, usually we suspend the USER account.
	// But the requirement implies acting on the driver profile. 
	// Let's re-read the domain/driver.go carefully. 
	// Ah, I missed if there was a status. 
	// Wait, I viewed domain/driver.go. 
	// The available statuses are: Offline, Idle, Loading, InTransit, Delivering.
	// There is NO Suspended status for driver profile itself in the enum.
	// AND there is `IsVerified` bool.
	// BUT, `rider` has `Suspended` status.
	// For drivers, we might just suspend the USER.
	// OR, maybe I should add Suspended status to DriverStatus?
	// Modifying domain enums might be risky if DB has constraints.
	// The `driver_profiles` table definition in `driver_repo.go` shows: `status type is inferred`.
	// Let's look at `SuspendRider` implementation. It suspends rider profile AND user.
	// addressing the user's specific error: "failed to get rider: rider not found" implies they called the RIDER endpoint.
	// The user wants to manage drivers.
	// If I cannot set DriverStatus to Suspended, I can at least suspend the User.
	// OR, I can set IsVerified = false?
	// Let's err on the side of suspending the USER for now, as that blocks access.
	// AND updating the plan to reflect 'SuspendUser' reuse or clarify limitation?
	// No, the user expects parallel functionality. 
	// Let's look at `driver_repo.go` again to see what `UpdateStatus` accepts. It takes `domain.DriverStatus`.
	
	// Assumption: I should add `DriverStatusSuspended` to `domain/driver.go` and `database_schema.sql` if possible?
	// Altering enum types in Postgres requires migration. I cannot easily do that here without user permission/task for migration.
	// However, looking at the code, `AdminHandler.SuspendRider` calls `adminService.SuspendRider`.
	// `VerifyRider` sets `Verified` status.
	// For Driver, `Verify` sets `IsVerified=true`.
	// For Suspend, maybe just Suspend the User ID?
	// Let's check if the USER has `is_rider` flag.
	// Drivers (inter-state) also use `users` table.
	// If I suspend the user, they can't log in. That works.
	// But what about tracking "Suspended Drivers"?
	// If I can't change the enum, I will just suspend the user.
	// BUT, for `GetStatistics`, we likely need to count them.
	// `rider_repo.go` has `COUNT(*) FILTER (WHERE status = 'suspended')`.
	// `driver_repo.go` has `GetStatistics` query:
	// `COUNT(*) as total_drivers`
	// `COUNT(*) FILTER (WHERE is_verified = true) as verified_drivers`
	// `COUNT(*) FILTER (WHERE status IN ('idle', 'loading')) as online_drivers`
	// It does NOT have defined count for suspended.
	
	// Implementation decision:
	// `SuspendDriver` will suspend the underlying USER. 
	// And maybe set `IsVerified = false` as well to prevent them from working even if user is unsuspended?
	// I will just suspend the user for now.
	
	if err := s.userRepo.Suspend(ctx, driver.UserID, reason); err != nil {
		return fmt.Errorf("failed to suspend driver's user account: %w", err)
	}
	
	// Optionally set status to Offline
	s.driverRepo.UpdateStatus(ctx, driverID, domain.DriverStatusOffline)

	s.logger.Info("driver suspended by admin (user suspended)",
		zap.String("driver_id", driverID.String()),
		zap.String("admin_id", adminID.String()),
		zap.String("reason", reason),
	)

	return nil
}

func (s *AdminServiceImpl) UnsuspendDriver(ctx context.Context, driverID uuid.UUID, adminID uuid.UUID) error {
	driver, err := s.driverRepo.GetByID(ctx, driverID)
	if err != nil {
		return fmt.Errorf("failed to get driver: %w", err)
	}

	if err := s.userRepo.Unsuspend(ctx, driver.UserID); err != nil {
		return fmt.Errorf("failed to unsuspend driver's user account: %w", err)
	}

	s.logger.Info("driver unsuspended by admin",
		zap.String("driver_id", driverID.String()),
		zap.String("admin_id", adminID.String()),
	)

	return nil
}

func (s *AdminServiceImpl) GetPendingDrivers(ctx context.Context, limit, offset int) ([]*domain.DriverProfile, error) {
	// Driver profile doesn't have "Pending" status in enum basically.
	// But it has `IsVerified` boolean. 
	// A pending driver is likely one where `IsVerified` is false.
	// `driver_repo.go` does not have a `GetByVerified` method but has `GetByStatus`.
	// But `GetByStatus` uses the enum.
	// I need to filter by `is_verified = false`.
	// `driver_repo.go` does NOT have a method for filtering by `is_verified`.
	// However, `GetStatistics` uses it.
	// I should probably add `GetPending` to `driverRepo`?
	// OR use `GetByStatus` if there is a 'pending' status I missed.
	// There is NO 'pending' status.
	// I will add a method to `DriverProfileRepo` to `GetPending(limit, offset)` which queries `WHERE is_verified = false`.
	// Since I cannot modify repo easily without checking if it exists, I'll assume I need to check `driver_repo.go` again to potentially ADD that method.
	// I checked `driver_repo.go` and it only has `GetByStatus`.
	// Wait, I can't add methods to the interface if I don't modify the interface definition too.
	// The `driverRepo` implementation is in `internal/repository/postgres/driver_repo.go` and interface in `internal/repository/interfaces.go`.
	
	// Actually, looking at `driver_repo.go` `GetStatistics`, it counts `is_verified = true`.
	// So `is_verified = false` IS pending.
	// But there is no method to fetch them list-wise.
	// I might have to add it to `driver_repo.go`.
	// I checked `driver_repo.go` content in previous turn. It does NOT have `GetPending`.
	// It has `GetByStatus`.
	
	// For now, to avoid expanding scope too much, I will rely on the fact that I need to add `GetPending` to `DriverProfileRepository` interface and implementation.
	// I will do that in a separate tool call.
	// For this chunk, I will assume the method exists `GetPending(ctx, limit, offset)`.
	
	// WAIT, I should modify the repository first? 
	// Or I can just check in `admin_service.go` if I can use existing methods?
	// No existing method fits. 
	// I will add `GetPending` to repo.
	
	return s.driverRepo.GetPending(ctx, limit, offset)
}

func (s *AdminServiceImpl) GetDriverStatistics(ctx context.Context) (*repository.DriverStatistics, error) {
	return s.driverRepo.GetStatistics(ctx)
}

// User Management

func (s *AdminServiceImpl) ListUsers(ctx context.Context, filters *UserFilters, limit, offset int) ([]*domain.User, error) {
	// Convert filters to repository format
	return s.userRepo.List(ctx, filters, limit, offset)
}

func (s *AdminServiceImpl) SuspendUser(ctx context.Context, userID uuid.UUID, reason string, adminID uuid.UUID) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	if user.IsSuspended {
		return fmt.Errorf("user already suspended")
	}

	if err := s.userRepo.Suspend(ctx, userID, reason); err != nil {
		return fmt.Errorf("failed to suspend user: %w", err)
	}

	s.logger.Info("user suspended by admin",
		zap.String("user_id", userID.String()),
		zap.String("admin_id", adminID.String()),
		zap.String("reason", reason),
	)

	return nil
}

func (s *AdminServiceImpl) UnsuspendUser(ctx context.Context, userID uuid.UUID, adminID uuid.UUID) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	if !user.IsSuspended {
		return fmt.Errorf("user is not suspended")
	}

	if err := s.userRepo.Unsuspend(ctx, userID); err != nil {
		return fmt.Errorf("failed to unsuspend user: %w", err)
	}

	s.logger.Info("user unsuspended by admin",
		zap.String("user_id", userID.String()),
		zap.String("admin_id", adminID.String()),
	)

	return nil
}

func (s *AdminServiceImpl) VerifySeller(ctx context.Context, userID uuid.UUID, adminID uuid.UUID) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	if !user.IsSeller {
		return fmt.Errorf("user is not a seller")
	}

	if user.SellerVerified {
		return fmt.Errorf("seller already verified")
	}

	if err := s.userRepo.VerifySeller(ctx, userID); err != nil {
		return fmt.Errorf("failed to verify seller: %w", err)
	}

	s.logger.Info("seller verified by admin",
		zap.String("user_id", userID.String()),
		zap.String("admin_id", adminID.String()),
	)

	return nil
}

// Platform Settings

func (s *AdminServiceImpl) GetDeliveryPricing(ctx context.Context) (*domain.DeliveryPricing, error) {
	settings, err := s.settingsRepo.Get(ctx, "delivery_pricing")
	if err != nil {
		return nil, fmt.Errorf("failed to get delivery pricing: %w", err)
	}

	var pricing domain.DeliveryPricing
	if err := json.Unmarshal(settings.SettingValue, &pricing); err != nil {
		return nil, fmt.Errorf("failed to unmarshal delivery pricing: %w", err)
	}

	return &pricing, nil
}

func (s *AdminServiceImpl) UpdateDeliveryPricing(ctx context.Context, pricing *domain.DeliveryPricing, adminID uuid.UUID) error {

	if pricing.BaseFee < 0 || pricing.PerKMFee < 0 {
		return fmt.Errorf("pricing values cannot be negative")
	}

	value, err := json.Marshal(pricing)
	if err != nil {
		return fmt.Errorf("failed to marshal delivery pricing: %w", err)
	}

	desc := "Base delivery fee and per-kilometer pricing with vehicle type multipliers"
	if err := s.settingsRepo.Set(ctx, "delivery_pricing", value, &desc, &adminID); err != nil {
		return fmt.Errorf("failed to update delivery pricing: %w", err)
	}

	s.logger.Info("delivery pricing updated by admin",
		zap.String("admin_id", adminID.String()),
		zap.Float64("base_fee", pricing.BaseFee),
		zap.Float64("per_km_fee", pricing.PerKMFee),
	)

	return nil
}

func (s *AdminServiceImpl) GetPlatformFees(ctx context.Context) (*domain.PlatformFees, error) {
	settings, err := s.settingsRepo.Get(ctx, "platform_fees")
	if err != nil {
		return nil, fmt.Errorf("failed to get platform fees: %w", err)
	}

	var fees domain.PlatformFees
	if err := json.Unmarshal(settings.SettingValue, &fees); err != nil {
		return nil, fmt.Errorf("failed to unmarshal platform fees: %w", err)
	}

	return &fees, nil
}

func (s *AdminServiceImpl) UpdatePlatformFees(ctx context.Context, fees *domain.PlatformFees, adminID uuid.UUID) error {
	// Validate fees
	if fees.BuyerFeeRate < 0 || fees.BuyerFeeRate > 1 ||
		fees.SellerFeeRate < 0 || fees.SellerFeeRate > 1 ||
		fees.RiderPayoutRate < 0 || fees.RiderPayoutRate > 1 {
		return fmt.Errorf("fee rates must be between 0 and 1")
	}

	value, err := json.Marshal(fees)
	if err != nil {
		return fmt.Errorf("failed to marshal platform fees: %w", err)
	}

	desc := "Platform commission rates for buyers, sellers, and rider payouts"
	if err := s.settingsRepo.Set(ctx, "platform_fees", value, &desc, &adminID); err != nil {
		return fmt.Errorf("failed to update platform fees: %w", err)
	}

	s.logger.Info("platform fees updated by admin",
		zap.String("admin_id", adminID.String()),
		zap.Float64("buyer_fee_rate", fees.BuyerFeeRate),
		zap.Float64("seller_fee_rate", fees.SellerFeeRate),
		zap.Float64("rider_payout_rate", fees.RiderPayoutRate),
	)

	return nil
}

// Analytics

func (s *AdminServiceImpl) GetPlatformRevenue(ctx context.Context, startDate, endDate *string) (*repository.RevenueAnalytics, error) {
	var start, end *time.Time
	if startDate != nil {
		t, err := time.Parse("2006-01-02", *startDate)
		if err == nil {
			start = &t
		}
	}
	if endDate != nil {
		t, err := time.Parse("2006-01-02", *endDate)
		if err == nil {
			end = &t
		}
	}

	return s.walletTxRepo.GetRevenueAnalytics(ctx, start, end)
}

func (s *AdminServiceImpl) GetUserStatistics(ctx context.Context) (*UserStatistics, error) {
	stats, err := s.userRepo.GetStatistics(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get user statistics: %w", err)
	}

	return &UserStatistics{
		TotalUsers:     stats.TotalUsers,
		TotalSellers:   stats.TotalSellers,
		TotalBuyers:    stats.TotalBuyers,
		TotalRiders:    stats.TotalRiders,
		SuspendedUsers: stats.SuspendedUsers,
	}, nil
}

// Additional Analytics

func (s *AdminServiceImpl) GetActiveDeliveries(ctx context.Context) (*ActiveDeliveriesStats, error) {
	// Get active delivery jobs
	jobs, err := s.deliveryJobRepo.GetSearchingJobs(ctx, 1000)
	if err != nil {
		return nil, fmt.Errorf("failed to get active deliveries: %w", err)
	}

	stats := &ActiveDeliveriesStats{
		TotalActive: len(jobs),
	}

	// Count by status
	for _, job := range jobs {
		switch job.Status {
		case domain.DJStatusSearching:
			stats.Searching++
		case domain.DJStatusAccepted:
			stats.Assigned++
		case domain.DJStatusPickedUp, domain.DJStatusInTransit:
			stats.InTransit++
		}
	}

	return stats, nil
}

func (s *AdminServiceImpl) GetRiderPerformance(ctx context.Context, riderID *uuid.UUID) ([]*RiderPerformance, error) {
	var riders []*domain.RiderProfile
	var err error

	if riderID != nil {
		rider, err := s.riderRepo.GetByID(ctx, *riderID)
		if err != nil {
			return nil, fmt.Errorf("failed to get rider: %w", err)
		}
		riders = []*domain.RiderProfile{rider}
	} else {
		riders, err = s.riderRepo.GetByStatus(ctx, domain.RiderStatusVerified, 100, 0)
		if err != nil {
			return nil, fmt.Errorf("failed to get riders: %w", err)
		}
	}

	performance := make([]*RiderPerformance, 0, len(riders))
	for _, rider := range riders {
		user, err := s.userRepo.GetByID(ctx, rider.UserID)
		if err != nil {
			s.logger.Warn("failed to get user for rider", zap.Error(err))
			continue
		}

		name := user.FullName()
		if name == "" {
			name = user.PhoneNumber
		}

		completedToday := 0
		if s.deliveryJobRepo != nil {
			count, err := s.deliveryJobRepo.GetCompletedTodayByRiderID(ctx, rider.ID)
			if err == nil {
				completedToday = count
			}
		}

		performance = append(performance, &RiderPerformance{
			RiderID:         rider.ID,
			RiderName:       name,
			TotalDeliveries: rider.TotalDeliveries,
			CompletedToday:  completedToday,
			AverageRating:   rider.Rating,
			TotalEarnings:   rider.TotalEarnings,
			OnlineStatus:    rider.IsOnline,
		})
	}

	return performance, nil
}

// Payment Request Management

func (s *AdminServiceImpl) ListPaymentRequests(ctx context.Context, filters *PaymentRequestFilters, limit, offset int) ([]*domain.PaymentRequest, error) {
	if filters != nil && filters.SellerID != nil {
		return s.prRepo.GetBySellerID(ctx, *filters.SellerID, limit, offset)
	}
	if filters != nil && filters.BuyerID != nil {
		return s.prRepo.GetByBuyerID(ctx, *filters.BuyerID, limit, offset)
	}

	return s.prRepo.GetAll(ctx, limit, offset)
}

func (s *AdminServiceImpl) CancelPaymentRequest(ctx context.Context, prID uuid.UUID, reason string, adminID uuid.UUID) error {
	pr, err := s.prRepo.GetByID(ctx, prID)
	if err != nil {
		return fmt.Errorf("failed to get payment request: %w", err)
	}

	if pr.Status != domain.PRStatusDraft && pr.Status != domain.PRStatusLinkActive {
		return fmt.Errorf("payment request cannot be cancelled in current status: %s", pr.Status)
	}

	if err := s.prRepo.UpdateStatus(ctx, prID, domain.PRStatusCancelled); err != nil {
		return fmt.Errorf("failed to cancel payment request: %w", err)
	}

	s.logger.Info("payment request cancelled by admin",
		zap.String("pr_id", prID.String()),
		zap.String("admin_id", adminID.String()),
		zap.String("reason", reason),
	)

	return nil
}

func (s *AdminServiceImpl) GetExpiredPaymentRequests(ctx context.Context, limit, offset int) ([]*domain.PaymentRequest, error) {
	return s.prRepo.GetExpiredLinks(ctx)
}

func (s *AdminServiceImpl) CheckDatabaseHealth(ctx context.Context) (*HealthStatus, error) {

	_, err := s.userRepo.GetStatistics(ctx)
	if err != nil {
		return &HealthStatus{
			Status:  "down",
			Message: fmt.Sprintf("database error: %v", err),
		}, nil
	}

	return &HealthStatus{
		Status:  "healthy",
		Message: "database connection OK",
	}, nil
}

func (s *AdminServiceImpl) CheckRedisHealth(ctx context.Context) (*HealthStatus, error) {
	if s.redisClient == nil {
		return &HealthStatus{
			Status:  "unknown",
			Message: "redis client not configured",
		}, nil
	}

	err := s.redisClient.Ping(ctx)
	if err != nil {
		return &HealthStatus{
			Status:  "down",
			Message: fmt.Sprintf("redis ping failed: %v", err),
		}, nil
	}

	return &HealthStatus{
		Status:  "healthy",
		Message: "redis connection OK",
	}, nil
}

func (s *AdminServiceImpl) CheckExternalServices(ctx context.Context) (*ExternalServicesHealth, error) {
	return &ExternalServicesHealth{
		Paystack:   s.checkHTTPEndpoint("https://api.paystack.co"),
		GoogleMaps: s.checkHTTPEndpoint("https://maps.googleapis.com"),
		Twilio:     s.checkHTTPEndpoint("https://api.twilio.com"),
	}, nil
}

func (s *AdminServiceImpl) checkHTTPEndpoint(url string) *HealthStatus {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return &HealthStatus{
			Status:  "down",
			Message: fmt.Sprintf("connection failed: %v", err),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 500 {
		return &HealthStatus{
			Status:  "healthy",
			Message: fmt.Sprintf("reachable (HTTP %d)", resp.StatusCode),
		}
	}

	return &HealthStatus{
		Status:  "degraded",
		Message: fmt.Sprintf("unexpected status: HTTP %d", resp.StatusCode),
	}
}

// Audit Logging

func (s *AdminServiceImpl) ListAuditLogs(ctx context.Context, filters *repository.AuditLogFilters, limit, offset int) ([]*domain.AuditLog, error) {
	if s.auditLogRepo == nil {
		return []*domain.AuditLog{}, nil
	}
	return s.auditLogRepo.List(ctx, filters, limit, offset)
}

func (s *AdminServiceImpl) GetUserAuditTrail(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.AuditLog, error) {
	if s.auditLogRepo == nil {
		return []*domain.AuditLog{}, nil
	}
	return s.auditLogRepo.GetByTarget(ctx, domain.AuditTargetUser, userID, limit, offset)
}

// logAdminAction creates an audit log entry for an admin action
func (s *AdminServiceImpl) logAdminAction(ctx context.Context, adminID uuid.UUID, action domain.AuditAction, targetType domain.AuditTargetType, targetID *uuid.UUID, details map[string]any) {
	if s.auditLogRepo == nil {
		return
	}

	log := domain.NewAuditLog(adminID, action, targetType, targetID).WithDetails(details)
	if err := s.auditLogRepo.Create(ctx, log); err != nil {
		s.logger.Error("failed to create audit log", zap.Error(err))
	}
}

// Dispute Management

func (s *AdminServiceImpl) ListDisputes(ctx context.Context, filters *repository.DisputeFilters, limit, offset int) ([]*domain.Dispute, error) {
	if s.disputeRepo == nil {
		return []*domain.Dispute{}, nil
	}
	return s.disputeRepo.List(ctx, filters, limit, offset)
}

func (s *AdminServiceImpl) GetDispute(ctx context.Context, disputeID uuid.UUID) (*domain.DisputeDetails, error) {
	if s.disputeRepo == nil {
		return nil, fmt.Errorf("dispute repository not configured")
	}

	dispute, err := s.disputeRepo.GetByID(ctx, disputeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get dispute: %w", err)
	}
	if dispute == nil {
		return nil, fmt.Errorf("dispute not found")
	}

	escrow, err := s.escrowRepo.GetByID(ctx, dispute.EscrowID)
	if err != nil {
		return nil, fmt.Errorf("failed to get escrow: %w", err)
	}

	buyer, _ := s.userRepo.GetByID(ctx, escrow.BuyerID)
	seller, _ := s.userRepo.GetByID(ctx, escrow.SellerID)

	pr, _ := s.prRepo.GetByID(ctx, escrow.PaymentRequestID)

	return &domain.DisputeDetails{
		Dispute:        dispute,
		Escrow:         escrow,
		PaymentRequest: pr,
		Buyer:          buyer,
		Seller:         seller,
	}, nil
}

func (s *AdminServiceImpl) ResolveDispute(ctx context.Context, disputeID uuid.UUID, outcome domain.DisputeOutcome, notes string, adminID uuid.UUID) error {
	if s.disputeRepo == nil {
		return fmt.Errorf("dispute repository not configured")
	}

	dispute, err := s.disputeRepo.GetByID(ctx, disputeID)
	if err != nil {
		return fmt.Errorf("failed to get dispute: %w", err)
	}
	if dispute == nil {
		return fmt.Errorf("dispute not found")
	}

	if !dispute.CanResolve() {
		return fmt.Errorf("dispute cannot be resolved in current state")
	}

	dispute.Resolve(adminID, outcome, notes)

	if err := s.disputeRepo.Update(ctx, dispute); err != nil {
		return fmt.Errorf("failed to update dispute: %w", err)
	}

	s.logAdminAction(ctx, adminID, domain.AuditActionResolveDispute, domain.AuditTargetDispute, &disputeID, map[string]any{
		"outcome": outcome,
		"notes":   notes,
	})

	s.logger.Info("dispute resolved",
		zap.String("dispute_id", disputeID.String()),
		zap.String("outcome", string(outcome)),
		zap.String("admin_id", adminID.String()),
	)

	// Notify buyer and seller of dispute resolution
	if s.notificationService != nil {
		escrow, _ := s.escrowRepo.GetByID(ctx, dispute.EscrowID)
		if escrow != nil {
			data := map[string]string{"dispute_id": disputeID.String(), "outcome": string(outcome)}

			buyerMsg := fmt.Sprintf("Dispute for your order has been resolved: %s", outcome)
			sellerMsg := fmt.Sprintf("Dispute for an order was resolved: %s", outcome)

			go s.notificationService.SendToUser(ctx, escrow.BuyerID, "Dispute Resolved", buyerMsg, data, domain.NotificationDisputeResolved)
			go s.notificationService.SendToUser(ctx, escrow.SellerID, "Dispute Resolved", sellerMsg, data, domain.NotificationDisputeResolved)
		}
	}

	return nil
}

func (s *AdminServiceImpl) ProcessRefund(ctx context.Context, disputeID uuid.UUID, refundType string, amount float64, reason string, adminID uuid.UUID) error {
	if s.disputeRepo == nil || s.escrowRepo == nil {
		return fmt.Errorf("required repositories not configured")
	}

	dispute, err := s.disputeRepo.GetByID(ctx, disputeID)
	if err != nil {
		return fmt.Errorf("failed to get dispute: %w", err)
	}
	if dispute == nil {
		return fmt.Errorf("dispute not found")
	}

	escrow, err := s.escrowRepo.GetByID(ctx, dispute.EscrowID)
	if err != nil {
		return fmt.Errorf("failed to get escrow: %w", err)
	}

	// Determine refund amount
	var refundAmount float64
	var newStatus domain.EscrowStatus
	if refundType == "full" {
		refundAmount = escrow.TotalLocked
		newStatus = domain.EscrowStatusRefunded
	} else {
		// Partial refund uses the specified amount
		if amount <= 0 || amount > escrow.TotalLocked {
			return fmt.Errorf("invalid refund amount: must be between 0 and %.2f", escrow.TotalLocked)
		}
		refundAmount = amount
		newStatus = domain.EscrowStatusPartialRefund
	}

	if s.walletRepo != nil && s.walletTxRepo != nil {
		buyerWallet, err := s.walletRepo.GetOrCreate(ctx, escrow.BuyerID)
		if err != nil {
			return fmt.Errorf("failed to get or create buyer wallet: %w", err)
		}

		buyerWallet.Credit(refundAmount)
		if err := s.walletRepo.Update(ctx, buyerWallet); err != nil {
			return fmt.Errorf("failed to update buyer wallet: %w", err)
		}

		refundDesc := fmt.Sprintf("Refund for dispute: %s", reason)
		buyerTx := &domain.WalletTransaction{
			ID:           uuid.New(),
			WalletID:     buyerWallet.ID,
			Type:         domain.WalletTxRefund,
			Amount:       refundAmount,
			BalanceAfter: buyerWallet.AvailableBalance,
			Description:  &refundDesc,
		}
		if err := s.walletTxRepo.Create(ctx, buyerTx); err != nil {
			return fmt.Errorf("failed to create refund transaction: %w", err)
		}

		// Notify buyer about refund
		if s.notificationService != nil {
			data := map[string]string{"dispute_id": disputeID.String(), "amount": fmt.Sprintf("%.2f", refundAmount)}
			msg := fmt.Sprintf("A refund of %.2f has been credited to your wallet: %s", refundAmount, reason)
			go s.notificationService.SendToUser(ctx, escrow.BuyerID, "Refund Processed", msg, data, domain.NotificationRefundProcessed)
		}

		s.logger.Info("buyer wallet credited with refund",
			zap.String("buyer_id", escrow.BuyerID.String()),
			zap.Float64("refund_amount", refundAmount),
			zap.Float64("new_balance", buyerWallet.AvailableBalance),
		)
	}

	if err := s.escrowRepo.UpdateStatus(ctx, escrow.ID, newStatus); err != nil {
		return fmt.Errorf("failed to update escrow status: %w", err)
	}

	
	if dispute.Status != domain.DisputeStatusResolved {
		dispute.Status = domain.DisputeStatusResolved
		outcome := domain.DisputeOutcomeBuyerWins
		if refundType != "full" {
			outcome = domain.DisputeOutcomePartialRefund
		}
		dispute.Outcome = outcome
		now := time.Now()
		dispute.ResolvedAt = &now
		dispute.ResolvedBy = &adminID
		notes := fmt.Sprintf("Refund processed: %s - %.2f", refundType, refundAmount)
		dispute.ResolutionNotes = &notes
		if err := s.disputeRepo.Update(ctx, dispute); err != nil {
			s.logger.Warn("failed to update dispute status after refund", zap.Error(err))
		}
	}

	s.logAdminAction(ctx, adminID, domain.AuditActionProcessRefund, domain.AuditTargetDispute, &disputeID, map[string]any{
		"refund_type":   refundType,
		"refund_amount": refundAmount,
		"buyer_id":      escrow.BuyerID.String(),
		"reason":        reason,
	})

	s.logger.Info("refund processed",
		zap.String("dispute_id", disputeID.String()),
		zap.String("refund_type", refundType),
		zap.Float64("refund_amount", refundAmount),
		zap.String("buyer_id", escrow.BuyerID.String()),
		zap.String("admin_id", adminID.String()),
	)

	return nil
}


// Notification Management

func (s *AdminServiceImpl) SendBroadcast(ctx context.Context, title, body, topic string, adminID uuid.UUID) error {
	if s.notificationService == nil {
		return fmt.Errorf("notification service not configured")
	}

	if topic == "" {
		topic = "all_users" // Default topic
	}

	if err := s.notificationService.SendBroadcast(ctx, topic, title, body, nil); err != nil {
		return fmt.Errorf("failed to send broadcast: %w", err)
	}

	s.logAdminAction(ctx, adminID, domain.AuditActionBroadcastNotification, domain.AuditTargetPlatform, nil, map[string]any{
		"action": "broadcast_notification",
		"title":  title,
		"topic":  topic,
	})

	s.logger.Info("admin sent broadcast notification",
		zap.String("admin_id", adminID.String()),
		zap.String("topic", topic),
	)

	return nil
}

func (s *AdminServiceImpl) GetDisputeStatistics(ctx context.Context) (*repository.DisputeStatistics, error) {
	if s.disputeRepo == nil {
		return &repository.DisputeStatistics{}, nil
	}
	return s.disputeRepo.GetStatistics(ctx)
}

// Commission Management

func (s *AdminServiceImpl) GetCommissionSettings(ctx context.Context) (*domain.CommissionSettings, error) {
	data, err := s.settingsRepo.Get(ctx, "commission_settings")
	if err != nil {
		return nil, fmt.Errorf("failed to get commission settings: %w", err)
	}

	if data == nil {
		// Return defaults
		return &domain.CommissionSettings{
			MaxUnpaidRides: 5,
			CommissionRate: 0.15,
		}, nil
	}

	var settings domain.CommissionSettings
	if err := json.Unmarshal(data.SettingValue, &settings); err != nil {
		return nil, fmt.Errorf("failed to parse commission settings: %w", err)
	}

	return &settings, nil
}

func (s *AdminServiceImpl) UpdateCommissionSettings(ctx context.Context, settings *domain.CommissionSettings, adminID uuid.UUID) error {
	data, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("failed to marshal commission settings: %w", err)
	}

	desc := "Commission settings"
	if err := s.settingsRepo.Set(ctx, "commission_settings", data, &desc, &adminID); err != nil {
		return fmt.Errorf("failed to save commission settings: %w", err)
	}

	s.logger.Info("commission settings updated",
		zap.Int("max_unpaid_rides", settings.MaxUnpaidRides),
		zap.Float64("commission_rate", settings.CommissionRate),
		zap.String("admin_id", adminID.String()),
	)

	return nil
}

// Wallet Management

func (s *AdminServiceImpl) GetUserWallet(ctx context.Context, userID uuid.UUID) (*domain.Wallet, error) {
	if s.walletRepo == nil {
		return nil, fmt.Errorf("wallet repository not configured")
	}

	wallet, err := s.walletRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet: %w", err)
	}

	return wallet, nil
}

func (s *AdminServiceImpl) AdjustWallet(ctx context.Context, userID uuid.UUID, amount float64, adjustmentType string, reason string, adminID uuid.UUID) error {
	if s.walletRepo == nil || s.walletTxRepo == nil {
		return fmt.Errorf("wallet repositories not configured")
	}

	wallet, err := s.walletRepo.GetByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get wallet: %w", err)
	}

	// Apply adjustment
	switch adjustmentType {
	case "credit":
		wallet.Credit(amount)
	case "debit":
		if err := wallet.Debit(amount); err != nil {
			return fmt.Errorf("failed to debit wallet: %w", err)
		}
	default:
		return fmt.Errorf("invalid adjustment type: must be 'credit' or 'debit'")
	}

	// Update wallet
	if err := s.walletRepo.Update(ctx, wallet); err != nil {
		return fmt.Errorf("failed to update wallet: %w", err)
	}

	// Create transaction record
	tx := &domain.WalletTransaction{
		ID:           uuid.New(),
		WalletID:     wallet.ID,
		Type:         domain.WalletTxAdjustment,
		Amount:       amount,
		BalanceAfter: wallet.AvailableBalance,
		Description:  &reason,
	}
	if err := s.walletTxRepo.Create(ctx, tx); err != nil {
		return fmt.Errorf("failed to create transaction: %w", err)
	}

	// Audit log
	s.logAdminAction(ctx, adminID, domain.AuditActionAdjustWallet, domain.AuditTargetWallet, &userID, map[string]any{
		"adjustment_type": adjustmentType,
		"amount":          amount,
		"reason":          reason,
	})

	s.logger.Info("wallet adjusted",
		zap.String("user_id", userID.String()),
		zap.String("adjustment_type", adjustmentType),
		zap.Float64("amount", amount),
		zap.String("admin_id", adminID.String()),
	)

	return nil
}

func (s *AdminServiceImpl) GetWalletTransactions(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.WalletTransaction, error) {
	if s.walletRepo == nil || s.walletTxRepo == nil {
		return []*domain.WalletTransaction{}, nil
	}

	wallet, err := s.walletRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet: %w", err)
	}

	return s.walletTxRepo.GetByWalletID(ctx, wallet.ID, limit, offset)
}

var _ AdminService = (*AdminServiceImpl)(nil)
