package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/config"
	"kipa/internal/domain"
	"kipa/internal/repository"
)

// DriverService handles inter-state driver operations
type DriverService interface {
	// Profile management
	CreateProfile(ctx context.Context, userID uuid.UUID, req *CreateDriverProfileRequest) (*domain.DriverProfile, error)
	GetProfile(ctx context.Context, userID uuid.UUID) (*domain.DriverProfile, error)
	GetProfileByID(ctx context.Context, id uuid.UUID) (*domain.DriverProfile, error)
	UpdateProfile(ctx context.Context, userID uuid.UUID, req *UpdateDriverProfileRequest) (*domain.DriverProfile, error)

	// Online/Offline
	GoOnline(ctx context.Context, userID uuid.UUID, originCity, destinationCity string) (*domain.DriverTrip, error)
	GoOffline(ctx context.Context, userID uuid.UUID) error

	// Available deliveries
	GetAvailableDeliveries(ctx context.Context, userID uuid.UUID) ([]*domain.DriverDelivery, error)
	AcceptDelivery(ctx context.Context, userID uuid.UUID, deliveryID uuid.UUID) (*domain.DriverDelivery, error)

	// Trip management
	GetCurrentTrip(ctx context.Context, userID uuid.UUID) (*domain.DriverTrip, error)
	GetTripDeliveries(ctx context.Context, userID uuid.UUID) ([]*domain.DriverDelivery, error)

	// Manual status updates (no GPS tracking)
	MarkAtPickup(ctx context.Context, userID uuid.UUID, deliveryID uuid.UUID) error
	VerifyPickupCode(ctx context.Context, userID uuid.UUID, deliveryID uuid.UUID, code string) error
	MarkDeparted(ctx context.Context, userID uuid.UUID) error
	MarkArrived(ctx context.Context, userID uuid.UUID) error
	VerifyDropoffCode(ctx context.Context, userID uuid.UUID, deliveryID uuid.UUID, code string) error
	MarkBuyerUnreachable(ctx context.Context, userID uuid.UUID, deliveryID uuid.UUID) error

	// History
	GetTripHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.DriverTrip, error)
	GetDeliveryHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.DriverDelivery, error)
	GetEarnings(ctx context.Context, userID uuid.UUID) (*DriverEarnings, error)

	// Seller endpoints
	GetDeliveriesBySeller(ctx context.Context, sellerID uuid.UUID, limit, offset int) ([]*domain.DriverDelivery, error)
	GetDeliveryForSeller(ctx context.Context, sellerID, deliveryID uuid.UUID) (*domain.DriverDelivery, error)
	GetPickupCodeForSeller(ctx context.Context, sellerID, deliveryID uuid.UUID) (string, error)

	// Buyer endpoints
	GetDeliveriesByBuyer(ctx context.Context, buyerID uuid.UUID, limit, offset int) ([]*domain.DriverDelivery, error)
	GetDeliveryForBuyer(ctx context.Context, buyerID, deliveryID uuid.UUID) (*domain.DriverDelivery, error)
	GetDropoffCodeForBuyer(ctx context.Context, buyerID, deliveryID uuid.UUID) (string, error)

	// Confirmation and completion (Phase 4)
	ConfirmDelivery(ctx context.Context, buyerID, deliveryID uuid.UUID) error
	DisputeDelivery(ctx context.Context, buyerID, deliveryID uuid.UUID, reason string) error
	ProcessDriverPayment(ctx context.Context, deliveryID uuid.UUID) error

	// Edge cases (Phase 5)
	CancelDelivery(ctx context.Context, sellerID, deliveryID uuid.UUID) error
	ProcessExpiredUnreachable(ctx context.Context, deliveryID uuid.UUID) error
	InitiateReturnToSeller(ctx context.Context, deliveryID uuid.UUID) error
	ProcessAbandonedTrip(ctx context.Context, tripID uuid.UUID) error
	ExpireAwaitingDriverDelivery(ctx context.Context, deliveryID uuid.UUID) error

	// For background jobs
	GetExpiredConfirmationWindows(ctx context.Context) ([]*domain.DriverDelivery, error)
	AutoConfirmDelivery(ctx context.Context, deliveryID uuid.UUID) error
	GetExpiredUnreachableDeliveries(ctx context.Context) ([]*domain.DriverDelivery, error)
	GetStaleAwaitingDriver(ctx context.Context, hoursThreshold int) ([]*domain.DriverDelivery, error)
	GetAbandonedTrips(ctx context.Context, hoursThreshold int) ([]*domain.DriverTrip, error)
	GetStaleDepartures(ctx context.Context, hoursThreshold int) ([]*domain.DriverTrip, error)
}

// Request/Response DTOs

type CreateDriverProfileRequest struct {
	VehicleType     string `json:"vehicle_type" binding:"required,oneof=car van bus"`
	LicensePlate    string `json:"license_plate" binding:"required"`
	NINNumber       string `json:"nin_number" binding:"required"`
	NINPhotoURL     string `json:"nin_photo_url" binding:"required"`
	LicenseNumber   string `json:"license_number" binding:"required"`
	LicensePhotoURL string `json:"license_photo_url" binding:"required"`
	VehiclePhotoURL string `json:"vehicle_photo_url" binding:"required"`
	FacePhotoURL    string `json:"face_photo_url" binding:"required"`
}

type UpdateDriverProfileRequest struct {
	VehicleType     *string `json:"vehicle_type"`
	LicensePlate    *string `json:"license_plate"`
	VehiclePhotoURL *string `json:"vehicle_photo_url"`
}

type DriverEarnings struct {
	TotalEarnings   float64 `json:"total_earnings"`
	TotalDeliveries int     `json:"total_deliveries"`
	Rating          float64 `json:"rating"`
}

// DriverServiceImpl implements DriverService
type DriverServiceImpl struct {
	driverRepo       repository.DriverProfileRepository
	tripRepo         repository.DriverTripRepository
	deliveryRepo     repository.DriverDeliveryRepository
	eventRepo        repository.DriverDeliveryEventRepository
	prRepo           repository.PaymentRequestRepository
	escrowService    EscrowService
	walletService    WalletService
	notificationSvc  NotificationService
	config           *config.Config
	logger           *zap.Logger
}

func NewDriverService(
	driverRepo repository.DriverProfileRepository,
	tripRepo repository.DriverTripRepository,
	deliveryRepo repository.DriverDeliveryRepository,
	eventRepo repository.DriverDeliveryEventRepository,
	prRepo repository.PaymentRequestRepository,
	escrowService EscrowService,
	walletService WalletService,
	notificationSvc NotificationService,
	cfg *config.Config,
	logger *zap.Logger,
) DriverService {
	return &DriverServiceImpl{
		driverRepo:      driverRepo,
		tripRepo:        tripRepo,
		deliveryRepo:    deliveryRepo,
		walletService:   walletService,
		eventRepo:       eventRepo,
		prRepo:          prRepo,
		escrowService:   escrowService,
		notificationSvc: notificationSvc,
		config:          cfg,
		logger:          logger,
	}
}

// ============================================
// PROFILE MANAGEMENT
// ============================================

func (s *DriverServiceImpl) CreateProfile(ctx context.Context, userID uuid.UUID, req *CreateDriverProfileRequest) (*domain.DriverProfile, error) {
	existing, err := s.driverRepo.GetByUserID(ctx, userID)
	if err == nil && existing != nil {
		return nil, domain.ErrDriverProfileExists
	}

	profile := &domain.DriverProfile{
		UserID:           userID,
		Status:           domain.DriverStatusOffline,
		VehicleType:      domain.DriverVehicleType(req.VehicleType),
		LicensePlate:     req.LicensePlate,
		MaxCarryingValue: 500000, // Default ₦500k
		MaxPackageCount:  20,     // Default 20 packages
		Rating:           5.0,
		IsVerified:       false, // Requires admin verification
		NINNumber:        &req.NINNumber,
		NINPhotoURL:      &req.NINPhotoURL,
		LicenseNumber:    &req.LicenseNumber,
		LicensePhotoURL:  &req.LicensePhotoURL,
		VehiclePhotoURL:  &req.VehiclePhotoURL,
		FacePhotoURL:     &req.FacePhotoURL,
	}

	if err := s.driverRepo.Create(ctx, profile); err != nil {
		return nil, fmt.Errorf("failed to create driver profile: %w", err)
	}

	s.logger.Info("driver profile created",
		zap.String("user_id", userID.String()),
		zap.String("profile_id", profile.ID.String()),
	)

	return profile, nil
}

func (s *DriverServiceImpl) GetProfile(ctx context.Context, userID uuid.UUID) (*domain.DriverProfile, error) {
	return s.driverRepo.GetByUserID(ctx, userID)
}

func (s *DriverServiceImpl) GetProfileByID(ctx context.Context, id uuid.UUID) (*domain.DriverProfile, error) {
	return s.driverRepo.GetByID(ctx, id)
}

func (s *DriverServiceImpl) UpdateProfile(ctx context.Context, userID uuid.UUID, req *UpdateDriverProfileRequest) (*domain.DriverProfile, error) {
	profile, err := s.driverRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if req.VehicleType != nil {
		profile.VehicleType = domain.DriverVehicleType(*req.VehicleType)
	}
	if req.LicensePlate != nil {
		profile.LicensePlate = *req.LicensePlate
	}
	if req.VehiclePhotoURL != nil {
		profile.VehiclePhotoURL = req.VehiclePhotoURL
	}

	if err := s.driverRepo.Update(ctx, profile); err != nil {
		return nil, fmt.Errorf("failed to update driver profile: %w", err)
	}

	return profile, nil
}

// ============================================
// ONLINE/OFFLINE
// ============================================

func (s *DriverServiceImpl) GoOnline(ctx context.Context, userID uuid.UUID, originCity, destinationCity string) (*domain.DriverTrip, error) {
	profile, err := s.driverRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if !profile.IsVerified {
		return nil, domain.ErrDriverNotVerified
	}

	if profile.Status != domain.DriverStatusOffline {
		return nil, domain.ErrDriverAlreadyOnline
	}

	// Check for existing active trip
	existingTrip, err := s.tripRepo.GetActiveByDriverID(ctx, profile.ID)
	if err == nil && existingTrip != nil {
		return nil, domain.ErrDriverHasActiveTrip
	}

	// Create new trip for this route
	acceptingUntil := time.Now().Add(4 * time.Hour) // 4 hour window to accept packages
	trip := &domain.DriverTrip{
		DriverID:        profile.ID,
		OriginCity:      originCity,
		DestinationCity: destinationCity,
		Status:          domain.DriverTripStatusAccepting,
		AcceptingUntil:  &acceptingUntil,
	}

	if err := s.tripRepo.Create(ctx, trip); err != nil {
		return nil, fmt.Errorf("failed to create trip: %w", err)
	}

	// Update driver profile
	profile.Status = domain.DriverStatusIdle
	profile.CurrentTripID = &trip.ID
	profile.OriginCity = &originCity
	profile.DestinationCity = &destinationCity

	if err := s.driverRepo.Update(ctx, profile); err != nil {
		return nil, fmt.Errorf("failed to update driver status: %w", err)
	}

	s.logger.Info("driver went online",
		zap.String("user_id", userID.String()),
		zap.String("trip_id", trip.ID.String()),
		zap.String("origin", originCity),
		zap.String("destination", destinationCity),
	)

	return trip, nil
}

func (s *DriverServiceImpl) GoOffline(ctx context.Context, userID uuid.UUID) error {
	profile, err := s.driverRepo.GetByUserID(ctx, userID)
	if err != nil {
		return err
	}

	if err := profile.GoOffline(); err != nil {
		return err
	}

	// If there's an accepting trip with no packages, cancel it
	if profile.CurrentTripID != nil {
		trip, err := s.tripRepo.GetByID(ctx, *profile.CurrentTripID)
		if err == nil && trip.Status == domain.DriverTripStatusAccepting && trip.TotalPackages == 0 {
			trip.Status = domain.DriverTripStatusCancelled
			if err := s.tripRepo.Update(ctx, trip); err != nil {
				s.logger.Warn("failed to cancel empty trip", zap.Error(err))
			}
		}
	}

	if err := s.driverRepo.Update(ctx, profile); err != nil {
		return fmt.Errorf("failed to update driver status: %w", err)
	}

	s.logger.Info("driver went offline",
		zap.String("user_id", userID.String()),
	)

	return nil
}

// ============================================
// AVAILABLE DELIVERIES
// ============================================

func (s *DriverServiceImpl) GetAvailableDeliveries(ctx context.Context, userID uuid.UUID) ([]*domain.DriverDelivery, error) {
	profile, err := s.driverRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if !profile.Status.CanAcceptDeliveries() {
		return nil, domain.ErrDriverNotAccepting
	}

	if profile.OriginCity == nil || profile.DestinationCity == nil {
		return nil, fmt.Errorf("driver route not set")
	}

	// Get deliveries matching driver's route and within capacity
	availableCapacity := profile.AvailableCapacity()
	deliveries, err := s.deliveryRepo.GetAvailableForRoute(ctx, *profile.OriginCity, *profile.DestinationCity, availableCapacity, 50)
	if err != nil {
		return nil, err
	}

	return deliveries, nil
}

func (s *DriverServiceImpl) AcceptDelivery(ctx context.Context, userID uuid.UUID, deliveryID uuid.UUID) (*domain.DriverDelivery, error) {
	profile, err := s.driverRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if !profile.Status.CanAcceptDeliveries() {
		return nil, domain.ErrDriverNotAccepting
	}

	delivery, err := s.deliveryRepo.GetByID(ctx, deliveryID)
	if err != nil {
		return nil, err
	}

	if delivery.Status != domain.DDStatusAwaitingDriver {
		return nil, domain.ErrDeliveryAlreadyAssigned
	}

	// Check capacity
	if !profile.CanAcceptDelivery(delivery.ItemValue) {
		return nil, domain.ErrDriverCapacityExceeded
	}

	// Verify route matches
	if profile.OriginCity == nil || profile.DestinationCity == nil {
		return nil, fmt.Errorf("driver route not set")
	}
	if delivery.OriginCity != *profile.OriginCity || delivery.DestinationCity != *profile.DestinationCity {
		return nil, domain.ErrRouteMismatch
	}

	trip, err := s.tripRepo.GetByID(ctx, *profile.CurrentTripID)
	if err != nil {
		return nil, fmt.Errorf("failed to get driver trip: %w", err)
	}

	// Assign driver to delivery
	now := time.Now()
	delivery.DriverID = &profile.ID
	delivery.TripID = &trip.ID
	delivery.Status = domain.DDStatusDriverAssigned
	delivery.DriverAssignedAt = &now

	// Generate pickup code
	pickupCode := generateDriverCode()
	delivery.PickupCode = &pickupCode
	delivery.PickupCodeGeneratedAt = &now

	if err := s.deliveryRepo.Update(ctx, delivery); err != nil {
		return nil, fmt.Errorf("failed to update delivery: %w", err)
	}

	// Update driver capacity
	profile.AddCapacity(delivery.ItemValue)
	if profile.Status == domain.DriverStatusIdle {
		profile.Status = domain.DriverStatusLoading
	}
	if err := s.driverRepo.Update(ctx, profile); err != nil {
		s.logger.Error("failed to update driver capacity", zap.Error(err))
	}

	// Update trip aggregates
	if err := s.tripRepo.AddDelivery(ctx, trip.ID, delivery.ItemValue, delivery.DriverFee); err != nil {
		s.logger.Error("failed to update trip aggregates", zap.Error(err))
	}

	// Update trip status if needed
	if trip.Status == domain.DriverTripStatusAccepting {
		trip.Status = domain.DriverTripStatusLoading
		if err := s.tripRepo.Update(ctx, trip); err != nil {
			s.logger.Error("failed to update trip status", zap.Error(err))
		}
	}

	// Record event
	s.recordDeliveryEvent(ctx, delivery.ID, nil, domain.DDStatusDriverAssigned, domain.DDActorDriver, &profile.UserID, "Driver accepted delivery", nil)

	s.logger.Info("driver accepted delivery",
		zap.String("driver_id", profile.ID.String()),
		zap.String("delivery_id", deliveryID.String()),
		zap.Float64("item_value", delivery.ItemValue),
	)

	// Notify seller with pickup code
	if s.notificationSvc != nil {
		go s.notificationSvc.SendToUser(
			context.Background(),
			delivery.SellerID,
			"Driver Assigned",
			fmt.Sprintf("A driver has accepted your delivery. Pickup code: %s", pickupCode),
			map[string]string{
				"delivery_id": deliveryID.String(),
				"pickup_code": pickupCode,
			},
			domain.NotificationRiderAssigned,
		)
	}

	return delivery, nil
}

// ============================================
// TRIP MANAGEMENT
// ============================================

func (s *DriverServiceImpl) GetCurrentTrip(ctx context.Context, userID uuid.UUID) (*domain.DriverTrip, error) {
	profile, err := s.driverRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if profile.CurrentTripID == nil {
		return nil, domain.ErrNoActiveTrip
	}

	return s.tripRepo.GetByID(ctx, *profile.CurrentTripID)
}

func (s *DriverServiceImpl) GetTripDeliveries(ctx context.Context, userID uuid.UUID) ([]*domain.DriverDelivery, error) {
	profile, err := s.driverRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if profile.CurrentTripID == nil {
		return nil, domain.ErrNoActiveTrip
	}

	return s.deliveryRepo.GetByTripID(ctx, *profile.CurrentTripID)
}

// ============================================
// MANUAL STATUS UPDATES
// ============================================

func (s *DriverServiceImpl) MarkAtPickup(ctx context.Context, userID uuid.UUID, deliveryID uuid.UUID) error {
	profile, err := s.driverRepo.GetByUserID(ctx, userID)
	if err != nil {
		return err
	}

	delivery, err := s.deliveryRepo.GetByID(ctx, deliveryID)
	if err != nil {
		return err
	}

	if delivery.DriverID == nil || *delivery.DriverID != profile.ID {
		return domain.ErrUnauthorized
	}

	if delivery.Status != domain.DDStatusDriverAssigned {
		return domain.ErrInvalidStatusTransition
	}

	now := time.Now()
	oldStatus := delivery.Status
	delivery.Status = domain.DDStatusAwaitingPickup
	delivery.AtPickupAt = &now

	if err := s.deliveryRepo.Update(ctx, delivery); err != nil {
		return fmt.Errorf("failed to update delivery: %w", err)
	}

	s.recordDeliveryEvent(ctx, delivery.ID, &oldStatus, delivery.Status, domain.DDActorDriver, &profile.UserID, "Driver arrived at pickup", nil)

	s.logger.Info("driver marked at pickup",
		zap.String("delivery_id", deliveryID.String()),
	)

	// Notify seller that driver has arrived
	if s.notificationSvc != nil {
		go s.notificationSvc.SendToUser(
			context.Background(),
			delivery.SellerID,
			"Driver Arrived",
			"The driver has arrived for pickup. Please provide the pickup code.",
			map[string]string{"delivery_id": deliveryID.String()},
			domain.NotificationRiderAssigned,
		)
	}

	return nil
}

func (s *DriverServiceImpl) VerifyPickupCode(ctx context.Context, userID uuid.UUID, deliveryID uuid.UUID, code string) error {
	profile, err := s.driverRepo.GetByUserID(ctx, userID)
	if err != nil {
		return err
	}

	delivery, err := s.deliveryRepo.GetByID(ctx, deliveryID)
	if err != nil {
		return err
	}

	if delivery.DriverID == nil || *delivery.DriverID != profile.ID {
		return domain.ErrUnauthorized
	}

	if !delivery.CanVerifyPickupCode() {
		if delivery.IsPickupCodeLocked() {
			return domain.ErrPickupCodeLocked
		}
		return domain.ErrInvalidStatusTransition
	}

	// Check code expiry (24 hours)
	if delivery.PickupCodeGeneratedAt != nil {
		if time.Since(*delivery.PickupCodeGeneratedAt) > domain.CodeExpiryHours*time.Hour {
			return domain.ErrPickupCodeExpired
		}
	}

	// Check code
	if delivery.PickupCode == nil || *delivery.PickupCode != code {
		locked := delivery.IncrementPickupAttempts()
		if err := s.deliveryRepo.Update(ctx, delivery); err != nil {
			s.logger.Error("failed to update pickup attempts", zap.Error(err))
		}

		if locked {
			oldStatus := delivery.Status
			delivery.Status = domain.DDStatusPickupCodeLockout
			if err := s.deliveryRepo.Update(ctx, delivery); err != nil {
				s.logger.Error("failed to update status to lockout", zap.Error(err))
			}
			s.recordDeliveryEvent(ctx, delivery.ID, &oldStatus, delivery.Status, domain.DDActorSystem, nil, "Pickup code lockout - max attempts exceeded", nil)

			// Notify admin about lockout
			s.notifyAdminLockout(ctx, delivery, "pickup", profile.ID)

			return domain.ErrPickupCodeLocked
		}

		return domain.ErrInvalidPickupCode
	}

	// Code verified - mark as picked up
	now := time.Now()
	oldStatus := delivery.Status
	delivery.Status = domain.DDStatusPickedUp
	delivery.PickedUpAt = &now

	// Generate dropoff code for buyer
	dropoffCode := generateDriverCode()
	delivery.DropoffCode = &dropoffCode
	delivery.DropoffCodeGeneratedAt = &now

	if err := s.deliveryRepo.Update(ctx, delivery); err != nil {
		return fmt.Errorf("failed to update delivery: %w", err)
	}

	s.recordDeliveryEvent(ctx, delivery.ID, &oldStatus, delivery.Status, domain.DDActorDriver, &profile.UserID, "Pickup code verified", nil)

	s.logger.Info("pickup code verified",
		zap.String("delivery_id", deliveryID.String()),
	)

	// Notify buyer with dropoff code
	if s.notificationSvc != nil && delivery.BuyerID != nil {
		go s.notificationSvc.SendToUser(
			context.Background(),
			*delivery.BuyerID,
			"Item Picked Up",
			fmt.Sprintf("Your item has been picked up and is on the way. Your dropoff code: %s", dropoffCode),
			map[string]string{
				"delivery_id":  deliveryID.String(),
				"dropoff_code": dropoffCode,
			},
			domain.NotificationOrderPickedUp,
		)
	}

	return nil
}

func (s *DriverServiceImpl) MarkDeparted(ctx context.Context, userID uuid.UUID) error {
	profile, err := s.driverRepo.GetByUserID(ctx, userID)
	if err != nil {
		return err
	}

	if profile.CurrentTripID == nil {
		return domain.ErrNoActiveTrip
	}

	trip, err := s.tripRepo.GetByID(ctx, *profile.CurrentTripID)
	if err != nil {
		return err
	}

	if !trip.CanDepart() {
		return fmt.Errorf("cannot depart: trip status is %s or no packages picked up", trip.Status)
	}

	// Verify all deliveries in trip are picked up
	deliveries, err := s.deliveryRepo.GetByTripID(ctx, trip.ID)
	if err != nil {
		return err
	}

	for _, d := range deliveries {
		if d.Status != domain.DDStatusPickedUp {
			return fmt.Errorf("delivery %s not yet picked up (status: %s)", d.ID, d.Status)
		}
	}

	// Mark trip as in transit
	now := time.Now()
	trip.Status = domain.DriverTripStatusInTransit
	trip.DepartedAt = &now

	if err := s.tripRepo.Update(ctx, trip); err != nil {
		return fmt.Errorf("failed to update trip: %w", err)
	}

	// Update all deliveries to in_transit
	for _, d := range deliveries {
		oldStatus := d.Status
		d.Status = domain.DDStatusInTransit
		d.DepartedAt = &now
		if err := s.deliveryRepo.Update(ctx, d); err != nil {
			s.logger.Error("failed to update delivery status", zap.Error(err), zap.String("delivery_id", d.ID.String()))
		}
		s.recordDeliveryEvent(ctx, d.ID, &oldStatus, d.Status, domain.DDActorDriver, &profile.UserID, "Driver departed", nil)
	}

	// Update driver status
	profile.Status = domain.DriverStatusInTransit
	if err := s.driverRepo.Update(ctx, profile); err != nil {
		s.logger.Error("failed to update driver status", zap.Error(err))
	}

	s.logger.Info("driver departed",
		zap.String("trip_id", trip.ID.String()),
		zap.Int("packages", len(deliveries)),
	)

	// Notify all buyers
	if s.notificationSvc != nil {
		for _, d := range deliveries {
			if d.BuyerID != nil {
				go s.notificationSvc.SendToUser(
					context.Background(),
					*d.BuyerID,
					"Driver Departed",
					fmt.Sprintf("Your item is on the way to %s", trip.DestinationCity),
					map[string]string{"delivery_id": d.ID.String()},
					domain.NotificationOrderPickedUp,
				)
			}
		}
	}

	return nil
}

func (s *DriverServiceImpl) MarkArrived(ctx context.Context, userID uuid.UUID) error {
	profile, err := s.driverRepo.GetByUserID(ctx, userID)
	if err != nil {
		return err
	}

	if profile.CurrentTripID == nil {
		return domain.ErrNoActiveTrip
	}

	trip, err := s.tripRepo.GetByID(ctx, *profile.CurrentTripID)
	if err != nil {
		return err
	}

	if !trip.CanMarkArrived() {
		return fmt.Errorf("cannot mark arrived: trip status is %s", trip.Status)
	}

	now := time.Now()
	trip.Status = domain.DriverTripStatusArrived
	trip.ArrivedAt = &now

	if err := s.tripRepo.Update(ctx, trip); err != nil {
		return fmt.Errorf("failed to update trip: %w", err)
	}

	// Update all in-transit deliveries to arrived
	deliveries, err := s.deliveryRepo.GetByTripID(ctx, trip.ID)
	if err != nil {
		return err
	}

	for _, d := range deliveries {
		if d.Status == domain.DDStatusInTransit {
			oldStatus := d.Status
			d.Status = domain.DDStatusArrived
			d.ArrivedAt = &now
			if err := s.deliveryRepo.Update(ctx, d); err != nil {
				s.logger.Error("failed to update delivery status", zap.Error(err), zap.String("delivery_id", d.ID.String()))
			}
			s.recordDeliveryEvent(ctx, d.ID, &oldStatus, d.Status, domain.DDActorDriver, &profile.UserID, "Driver arrived at destination", nil)
		}
	}

	// Update driver status
	profile.Status = domain.DriverStatusDelivering
	if err := s.driverRepo.Update(ctx, profile); err != nil {
		s.logger.Error("failed to update driver status", zap.Error(err))
	}

	// Update trip status
	trip.Status = domain.DriverTripStatusDelivering
	if err := s.tripRepo.Update(ctx, trip); err != nil {
		s.logger.Error("failed to update trip status", zap.Error(err))
	}

	s.logger.Info("driver arrived at destination",
		zap.String("trip_id", trip.ID.String()),
	)

	// Notify all buyers
	if s.notificationSvc != nil {
		for _, d := range deliveries {
			if d.BuyerID != nil {
				go s.notificationSvc.SendToUser(
					context.Background(),
					*d.BuyerID,
					"Driver Arrived",
					fmt.Sprintf("The driver has arrived in %s. Please be ready to receive your delivery.", trip.DestinationCity),
					map[string]string{"delivery_id": d.ID.String()},
					domain.NotificationOrderDelivered,
				)
			}
		}
	}

	return nil
}

func (s *DriverServiceImpl) VerifyDropoffCode(ctx context.Context, userID uuid.UUID, deliveryID uuid.UUID, code string) error {
	profile, err := s.driverRepo.GetByUserID(ctx, userID)
	if err != nil {
		return err
	}

	delivery, err := s.deliveryRepo.GetByID(ctx, deliveryID)
	if err != nil {
		return err
	}

	if delivery.DriverID == nil || *delivery.DriverID != profile.ID {
		return domain.ErrUnauthorized
	}

	if !delivery.CanVerifyDropoffCode() {
		if delivery.IsDropoffCodeLocked() {
			return domain.ErrDropoffCodeLocked
		}
		return domain.ErrInvalidStatusTransition
	}

	// Check code expiry (24 hours)
	if delivery.DropoffCodeGeneratedAt != nil {
		if time.Since(*delivery.DropoffCodeGeneratedAt) > domain.CodeExpiryHours*time.Hour {
			return domain.ErrDropoffCodeExpired
		}
	}

	// Check code
	if delivery.DropoffCode == nil || *delivery.DropoffCode != code {
		locked := delivery.IncrementDropoffAttempts()
		if err := s.deliveryRepo.Update(ctx, delivery); err != nil {
			s.logger.Error("failed to update dropoff attempts", zap.Error(err))
		}

		if locked {
			oldStatus := delivery.Status
			delivery.Status = domain.DDStatusDropoffCodeLockout
			if err := s.deliveryRepo.Update(ctx, delivery); err != nil {
				s.logger.Error("failed to update status to lockout", zap.Error(err))
			}
			s.recordDeliveryEvent(ctx, delivery.ID, &oldStatus, delivery.Status, domain.DDActorSystem, nil, "Dropoff code lockout - max attempts exceeded", nil)

			// Notify admin about lockout
			s.notifyAdminLockout(ctx, delivery, "dropoff", profile.ID)

			return domain.ErrDropoffCodeLocked
		}

		return domain.ErrInvalidDropoffCode
	}

	// Code verified - start confirmation window
	now := time.Now()
	oldStatus := delivery.Status
	delivery.Status = domain.DDStatusConfirmationWindow
	delivery.DeliveredAt = &now

	if err := s.deliveryRepo.Update(ctx, delivery); err != nil {
		return fmt.Errorf("failed to update delivery: %w", err)
	}

	s.recordDeliveryEvent(ctx, delivery.ID, &oldStatus, delivery.Status, domain.DDActorDriver, &profile.UserID, "Dropoff code verified", nil)

	s.logger.Info("dropoff code verified",
		zap.String("delivery_id", deliveryID.String()),
	)

	// Notify buyer about confirmation window
	if s.notificationSvc != nil && delivery.BuyerID != nil {
		go s.notificationSvc.SendToUser(
			context.Background(),
			*delivery.BuyerID,
			"Item Delivered",
			"Your item has been delivered. Please confirm receipt or open a dispute if there are issues.",
			map[string]string{"delivery_id": deliveryID.String()},
			domain.NotificationOrderDelivered,
		)
	}

	// Check if all deliveries in trip are done
	s.checkTripCompletion(ctx, profile, delivery.TripID)

	return nil
}

func (s *DriverServiceImpl) MarkBuyerUnreachable(ctx context.Context, userID uuid.UUID, deliveryID uuid.UUID) error {
	profile, err := s.driverRepo.GetByUserID(ctx, userID)
	if err != nil {
		return err
	}

	delivery, err := s.deliveryRepo.GetByID(ctx, deliveryID)
	if err != nil {
		return err
	}

	if delivery.DriverID == nil || *delivery.DriverID != profile.ID {
		return domain.ErrUnauthorized
	}

	if delivery.Status != domain.DDStatusArrived {
		return domain.ErrInvalidStatusTransition
	}

	now := time.Now()
	deadline := now.Add(domain.BuyerUnreachableWindowHours * time.Hour)

	oldStatus := delivery.Status
	delivery.Status = domain.DDStatusBuyerUnreachable
	delivery.BuyerUnreachableAt = &now
	delivery.UnreachableDeadline = &deadline

	if err := s.deliveryRepo.Update(ctx, delivery); err != nil {
		return fmt.Errorf("failed to update delivery: %w", err)
	}

	s.recordDeliveryEvent(ctx, delivery.ID, &oldStatus, delivery.Status, domain.DDActorDriver, &profile.UserID, "Buyer marked unreachable", nil)

	s.logger.Info("buyer marked unreachable",
		zap.String("delivery_id", deliveryID.String()),
		zap.Time("deadline", deadline),
	)

	// Notify buyer urgently
	if s.notificationSvc != nil && delivery.BuyerID != nil {
		go s.notificationSvc.SendToUser(
			context.Background(),
			*delivery.BuyerID,
			"URGENT: Delivery Waiting",
			fmt.Sprintf("The driver is waiting! Please respond within 24 hours or your item will be returned. Deadline: %s", deadline.Format("Jan 2, 3:04 PM")),
			map[string]string{
				"delivery_id": deliveryID.String(),
				"deadline":    deadline.Format(time.RFC3339),
			},
			domain.NotificationDeliveryFailed,
		)
	}

	return nil
}

// ============================================
// HISTORY
// ============================================

func (s *DriverServiceImpl) GetTripHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.DriverTrip, error) {
	profile, err := s.driverRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return s.tripRepo.GetCompletedByDriverID(ctx, profile.ID, limit, offset)
}

func (s *DriverServiceImpl) GetDeliveryHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.DriverDelivery, error) {
	profile, err := s.driverRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	statuses := []domain.DriverDeliveryStatus{
		domain.DDStatusCompleted,
		domain.DDStatusDriverPaid,
	}
	return s.deliveryRepo.GetByDriverID(ctx, profile.ID, statuses, limit, offset)
}

func (s *DriverServiceImpl) GetEarnings(ctx context.Context, userID uuid.UUID) (*DriverEarnings, error) {
	profile, err := s.driverRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &DriverEarnings{
		TotalEarnings:   profile.TotalEarnings,
		TotalDeliveries: profile.TotalDeliveries,
		Rating:          profile.Rating,
	}, nil
}

// ============================================
// SELLER ENDPOINTS
// ============================================

func (s *DriverServiceImpl) GetDeliveriesBySeller(ctx context.Context, sellerID uuid.UUID, limit, offset int) ([]*domain.DriverDelivery, error) {
	return s.deliveryRepo.GetBySellerID(ctx, sellerID, limit, offset)
}

func (s *DriverServiceImpl) GetDeliveryForSeller(ctx context.Context, sellerID, deliveryID uuid.UUID) (*domain.DriverDelivery, error) {
	delivery, err := s.deliveryRepo.GetByID(ctx, deliveryID)
	if err != nil {
		return nil, err
	}

	pr, err := s.prRepo.GetByID(ctx, delivery.PaymentRequestID)
	if err != nil {
		return nil, err
	}

	if pr.SellerID != sellerID {
		return nil, domain.ErrUnauthorized
	}

	return delivery, nil
}

func (s *DriverServiceImpl) GetPickupCodeForSeller(ctx context.Context, sellerID, deliveryID uuid.UUID) (string, error) {
	delivery, err := s.GetDeliveryForSeller(ctx, sellerID, deliveryID)
	if err != nil {
		return "", err
	}

	if delivery.PickupCode == nil {
		return "", fmt.Errorf("pickup code not yet generated")
	}

	// Only show code when driver is at pickup (awaiting verification)
	if delivery.Status != domain.DDStatusAwaitingPickup {
		return "", fmt.Errorf("driver has not arrived at pickup location")
	}

	return *delivery.PickupCode, nil
}

// ============================================
// BUYER ENDPOINTS
// ============================================

func (s *DriverServiceImpl) GetDeliveriesByBuyer(ctx context.Context, buyerID uuid.UUID, limit, offset int) ([]*domain.DriverDelivery, error) {
	return s.deliveryRepo.GetByBuyerID(ctx, buyerID, limit, offset)
}

func (s *DriverServiceImpl) GetDeliveryForBuyer(ctx context.Context, buyerID, deliveryID uuid.UUID) (*domain.DriverDelivery, error) {
	delivery, err := s.deliveryRepo.GetByID(ctx, deliveryID)
	if err != nil {
		return nil, err
	}

	pr, err := s.prRepo.GetByID(ctx, delivery.PaymentRequestID)
	if err != nil {
		return nil, err
	}

	if pr.BuyerID == nil || *pr.BuyerID != buyerID {
		return nil, domain.ErrUnauthorized
	}

	return delivery, nil
}

func (s *DriverServiceImpl) GetDropoffCodeForBuyer(ctx context.Context, buyerID, deliveryID uuid.UUID) (string, error) {
	delivery, err := s.GetDeliveryForBuyer(ctx, buyerID, deliveryID)
	if err != nil {
		return "", err
	}

	if delivery.DropoffCode == nil {
		return "", fmt.Errorf("dropoff code not yet generated")
	}

	// Only show code when driver has arrived at destination
	if delivery.Status != domain.DDStatusArrived {
		return "", fmt.Errorf("driver has not arrived at dropoff location")
	}

	return *delivery.DropoffCode, nil
}

// ============================================
// CONFIRMATION AND COMPLETION (Phase 4)
// ============================================

func (s *DriverServiceImpl) ConfirmDelivery(ctx context.Context, buyerID, deliveryID uuid.UUID) error {
	delivery, err := s.GetDeliveryForBuyer(ctx, buyerID, deliveryID)
	if err != nil {
		return err
	}

	if delivery.Status != domain.DDStatusConfirmationWindow {
		return fmt.Errorf("delivery is not in confirmation window")
	}

	// Buyer can confirm anytime after delivery (no time constraint)

	now := time.Now()
	oldStatus := delivery.Status
	delivery.Status = domain.DDStatusCompleted
	delivery.CompletedAt = &now

	if err := s.deliveryRepo.Update(ctx, delivery); err != nil {
		return err
	}

	// Release escrow to seller
	if delivery.EscrowID != nil {
		if err := s.escrowService.Release(ctx, *delivery.EscrowID); err != nil {
			s.logger.Error("failed to release escrow", zap.Error(err), zap.String("escrow_id", delivery.EscrowID.String()))
		}
	}

	// Record event
	s.recordDeliveryEvent(ctx, deliveryID, &oldStatus, domain.DDStatusCompleted, domain.DDActorBuyer, &buyerID, "buyer confirmed delivery", nil)

	// Notify seller and driver
	if s.notificationSvc != nil {
		pr, _ := s.prRepo.GetByID(ctx, delivery.PaymentRequestID)
		if pr != nil {
			_ = s.notificationSvc.SendToUser(ctx, pr.SellerID, "Delivery Confirmed",
				"Buyer has confirmed receipt. Payment will be released.", nil, domain.NotificationOrderDelivered)
		}
		if delivery.DriverID != nil {
			profile, _ := s.driverRepo.GetByID(ctx, *delivery.DriverID)
			if profile != nil {
				_ = s.notificationSvc.SendToUser(ctx, profile.UserID, "Delivery Completed",
					"Buyer confirmed delivery. Your payment is being processed.", nil, domain.NotificationOrderDelivered)
			}
		}
	}

	// Process driver payment
	go func() {
		if err := s.ProcessDriverPayment(context.Background(), deliveryID); err != nil {
			s.logger.Error("failed to process driver payment", zap.Error(err), zap.String("delivery_id", deliveryID.String()))
		}
	}()

	return nil
}

func (s *DriverServiceImpl) DisputeDelivery(ctx context.Context, buyerID, deliveryID uuid.UUID, reason string) error {
	delivery, err := s.GetDeliveryForBuyer(ctx, buyerID, deliveryID)
	if err != nil {
		return err
	}

	// Can dispute during confirmation window or within 48h after
	if delivery.Status != domain.DDStatusConfirmationWindow && delivery.Status != domain.DDStatusCompleted {
		return fmt.Errorf("cannot dispute delivery in current status")
	}

	// If completed, check 48h window
	if delivery.Status == domain.DDStatusCompleted {
		if delivery.CompletedAt != nil {
			disputeDeadline := delivery.CompletedAt.Add(48 * time.Hour)
			if time.Now().After(disputeDeadline) {
				return fmt.Errorf("dispute window has expired (48h after completion)")
			}
		}
	}

	oldStatus := delivery.Status
	delivery.Status = domain.DDStatusDisputed

	if err := s.deliveryRepo.Update(ctx, delivery); err != nil {
		return err
	}

	// Put escrow in dispute state
	if delivery.EscrowID != nil {
		if err := s.escrowService.Dispute(ctx, *delivery.EscrowID, reason); err != nil {
			s.logger.Error("failed to dispute escrow", zap.Error(err), zap.String("escrow_id", delivery.EscrowID.String()))
		}
	}

	// Record event
	s.recordDeliveryEvent(ctx, deliveryID, &oldStatus, domain.DDStatusDisputed, domain.DDActorBuyer, &buyerID, reason, nil)

	// Notify all parties
	if s.notificationSvc != nil {
		pr, _ := s.prRepo.GetByID(ctx, delivery.PaymentRequestID)
		if pr != nil {
			_ = s.notificationSvc.SendToUser(ctx, pr.SellerID, "Delivery Disputed",
				"Buyer has raised a dispute. Admin will review.", nil, domain.NotificationDisputeOpened)
		}
		if delivery.DriverID != nil {
			profile, _ := s.driverRepo.GetByID(ctx, *delivery.DriverID)
			if profile != nil {
				_ = s.notificationSvc.SendToUser(ctx, profile.UserID, "Delivery Disputed",
					"A dispute has been raised for this delivery.", nil, domain.NotificationDisputeOpened)
			}
		}
		_ = s.notificationSvc.SendToAdmins(ctx, "New Dispute",
			fmt.Sprintf("Delivery %s disputed: %s", deliveryID.String()[:8], reason), map[string]string{
				"type":        "driver_delivery_dispute",
				"delivery_id": deliveryID.String(),
			})
	}

	return nil
}

func (s *DriverServiceImpl) ProcessDriverPayment(ctx context.Context, deliveryID uuid.UUID) error {
	delivery, err := s.deliveryRepo.GetByID(ctx, deliveryID)
	if err != nil {
		return err
	}

	if delivery.Status != domain.DDStatusCompleted {
		return fmt.Errorf("delivery must be completed before driver payment")
	}

	if delivery.DriverID == nil {
		return fmt.Errorf("no driver assigned to this delivery")
	}

	profile, err := s.driverRepo.GetByID(ctx, *delivery.DriverID)
	if err != nil {
		return err
	}

	// Calculate driver payout (85% of driver fee, 15% platform fee)
	platformFeePercent := 15.0
	driverPayout := delivery.DriverFee * (100 - platformFeePercent) / 100

	// Credit driver wallet (with idempotency key)
	idempotencyKey := fmt.Sprintf("driver_payout_%s", deliveryID.String())
	_, err = s.walletService.Credit(ctx, profile.UserID, driverPayout, domain.WalletTxDeliveryPayout,
		"driver_delivery", deliveryID, fmt.Sprintf("Driver delivery payment: %s", deliveryID.String()[:8]), idempotencyKey)
	if err != nil {
		return fmt.Errorf("failed to credit driver wallet: %w", err)
	}

	// Update delivery status to driver_paid
	oldStatus := delivery.Status
	delivery.Status = domain.DDStatusDriverPaid
	if err := s.deliveryRepo.Update(ctx, delivery); err != nil {
		return err
	}

	// Update driver profile stats
	profile.TotalDeliveries++
	profile.TotalEarnings += driverPayout
	if err := s.driverRepo.Update(ctx, profile); err != nil {
		s.logger.Error("failed to update driver stats", zap.Error(err))
	}

	// Record event
	s.recordDeliveryEvent(ctx, deliveryID, &oldStatus, domain.DDStatusDriverPaid, domain.DDActorSystem, nil,
		fmt.Sprintf("driver paid %.2f", driverPayout), nil)

	// Notify driver
	if s.notificationSvc != nil {
		_ = s.notificationSvc.SendToUser(ctx, profile.UserID, "Payment Received",
			fmt.Sprintf("₦%.2f has been credited to your wallet for delivery %s", driverPayout, deliveryID.String()[:8]),
			nil, domain.NotificationTopUpSuccess)
	}

	s.logger.Info("driver payment processed",
		zap.String("delivery_id", deliveryID.String()),
		zap.String("driver_id", profile.ID.String()),
		zap.Float64("payout", driverPayout),
	)

	return nil
}

func (s *DriverServiceImpl) GetExpiredConfirmationWindows(ctx context.Context) ([]*domain.DriverDelivery, error) {
	return s.deliveryRepo.GetExpiredConfirmationWindow(ctx)
}

func (s *DriverServiceImpl) AutoConfirmDelivery(ctx context.Context, deliveryID uuid.UUID) error {
	delivery, err := s.deliveryRepo.GetByID(ctx, deliveryID)
	if err != nil {
		return err
	}

	if delivery.Status != domain.DDStatusConfirmationWindow {
		return fmt.Errorf("delivery is not in confirmation window")
	}

	// Verify auto-confirm window has actually expired (7 days)
	if delivery.DeliveredAt != nil {
		deadline := delivery.DeliveredAt.Add(domain.AutoConfirmDays * 24 * time.Hour)
		if time.Now().Before(deadline) {
			return fmt.Errorf("auto-confirm window has not expired yet")
		}
	}

	now := time.Now()
	oldStatus := delivery.Status
	delivery.Status = domain.DDStatusCompleted
	delivery.CompletedAt = &now

	if err := s.deliveryRepo.Update(ctx, delivery); err != nil {
		return err
	}

	// Release escrow to seller
	if delivery.EscrowID != nil {
		if err := s.escrowService.Release(ctx, *delivery.EscrowID); err != nil {
			s.logger.Error("failed to release escrow on auto-confirm", zap.Error(err))
		}
	}

	// Record event
	s.recordDeliveryEvent(ctx, deliveryID, &oldStatus, domain.DDStatusCompleted, domain.DDActorSystem, nil, "auto-confirmed after 24h", nil)

	// Notify parties
	if s.notificationSvc != nil {
		pr, _ := s.prRepo.GetByID(ctx, delivery.PaymentRequestID)
		if pr != nil {
			_ = s.notificationSvc.SendToUser(ctx, pr.SellerID, "Delivery Auto-Confirmed",
				"Buyer did not respond within 24h. Payment released.", nil, domain.NotificationEscrowReleased)
			if pr.BuyerID != nil {
				_ = s.notificationSvc.SendToUser(ctx, *pr.BuyerID, "Delivery Auto-Confirmed",
					"Your delivery was automatically confirmed after 24h.", nil, domain.NotificationOrderDelivered)
			}
		}
	}

	// Process driver payment
	if err := s.ProcessDriverPayment(ctx, deliveryID); err != nil {
		s.logger.Error("failed to process driver payment on auto-confirm", zap.Error(err))
	}

	s.logger.Info("delivery auto-confirmed",
		zap.String("delivery_id", deliveryID.String()),
	)

	return nil
}

// ============================================
// EDGE CASES (Phase 5)
// ============================================

func (s *DriverServiceImpl) CancelDelivery(ctx context.Context, sellerID, deliveryID uuid.UUID) error {
	delivery, err := s.deliveryRepo.GetByID(ctx, deliveryID)
	if err != nil {
		return err
	}

	// Verify seller ownership
	if delivery.SellerID != sellerID {
		return domain.ErrUnauthorized
	}

	// Check if trip has started (departed) - can't cancel after trip starts
	if delivery.TripID != nil {
		trip, err := s.tripRepo.GetByID(ctx, *delivery.TripID)
		if err != nil {
			return fmt.Errorf("failed to get trip: %w", err)
		}
		if !delivery.CanBeCancelledWithTrip(trip.Status) {
			return fmt.Errorf("cannot cancel delivery: trip has already departed")
		}
	} else if !delivery.CanBeCancelled() {
		return fmt.Errorf("cannot cancel delivery in status: %s", delivery.Status)
	}

	oldStatus := delivery.Status
	delivery.Status = domain.DDStatusCancelled

	if err := s.deliveryRepo.Update(ctx, delivery); err != nil {
		return fmt.Errorf("failed to update delivery: %w", err)
	}

	// If driver was assigned, release capacity
	if delivery.DriverID != nil && delivery.TripID != nil {
		// Release driver capacity
		profile, err := s.driverRepo.GetByID(ctx, *delivery.DriverID)
		if err == nil {
			profile.ReleaseCapacity(delivery.ItemValue)
			if err := s.driverRepo.Update(ctx, profile); err != nil {
				s.logger.Error("failed to release driver capacity", zap.Error(err))
			}
		}

		// Remove from trip aggregates
		if err := s.tripRepo.RemoveDelivery(ctx, *delivery.TripID, delivery.ItemValue, delivery.DriverFee); err != nil {
			s.logger.Error("failed to update trip aggregates", zap.Error(err))
		}
	}

	// Refund buyer if escrow exists
	if delivery.EscrowID != nil {
		if err := s.escrowService.Refund(ctx, *delivery.EscrowID, "Seller cancelled delivery"); err != nil {
			s.logger.Error("failed to refund escrow", zap.Error(err), zap.String("escrow_id", delivery.EscrowID.String()))
		}
	}

	// Record event
	s.recordDeliveryEvent(ctx, deliveryID, &oldStatus, domain.DDStatusCancelled, domain.DDActorSeller, &sellerID, "seller cancelled delivery", nil)

	s.logger.Info("delivery cancelled",
		zap.String("delivery_id", deliveryID.String()),
		zap.String("seller_id", sellerID.String()),
	)

	// Notify parties
	if s.notificationSvc != nil {
		if delivery.BuyerID != nil {
			_ = s.notificationSvc.SendToUser(ctx, *delivery.BuyerID, "Delivery Cancelled",
				"The seller has cancelled this delivery. Your payment will be refunded.", nil, domain.NotificationDeliveryFailed)
		}
		if delivery.DriverID != nil {
			profile, _ := s.driverRepo.GetByID(ctx, *delivery.DriverID)
			if profile != nil {
				_ = s.notificationSvc.SendToUser(ctx, profile.UserID, "Delivery Cancelled",
					"A delivery has been cancelled by the seller.", nil, domain.NotificationDeliveryFailed)
			}
		}
	}

	return nil
}

func (s *DriverServiceImpl) ProcessExpiredUnreachable(ctx context.Context, deliveryID uuid.UUID) error {
	delivery, err := s.deliveryRepo.GetByID(ctx, deliveryID)
	if err != nil {
		return err
	}

	if delivery.Status != domain.DDStatusBuyerUnreachable {
		return fmt.Errorf("delivery is not in buyer_unreachable status")
	}

	// Verify deadline has passed
	if delivery.UnreachableDeadline != nil && time.Now().Before(*delivery.UnreachableDeadline) {
		return fmt.Errorf("unreachable deadline has not expired yet")
	}

	// Mark for return to seller
	return s.InitiateReturnToSeller(ctx, deliveryID)
}

func (s *DriverServiceImpl) InitiateReturnToSeller(ctx context.Context, deliveryID uuid.UUID) error {
	delivery, err := s.deliveryRepo.GetByID(ctx, deliveryID)
	if err != nil {
		return err
	}

	if delivery.Status != domain.DDStatusBuyerUnreachable {
		return fmt.Errorf("can only initiate return from buyer_unreachable status")
	}

	oldStatus := delivery.Status
	delivery.Status = domain.DDStatusReturnToSeller

	if err := s.deliveryRepo.Update(ctx, delivery); err != nil {
		return fmt.Errorf("failed to update delivery: %w", err)
	}

	// Record event
	s.recordDeliveryEvent(ctx, deliveryID, &oldStatus, domain.DDStatusReturnToSeller, domain.DDActorSystem, nil, "buyer unreachable deadline expired, returning to seller", nil)

	s.logger.Info("delivery marked for return to seller",
		zap.String("delivery_id", deliveryID.String()),
	)

	// Notify parties
	if s.notificationSvc != nil {
		_ = s.notificationSvc.SendToUser(ctx, delivery.SellerID, "Item Returning",
			"The buyer was unreachable. The item is being returned to you.", nil, domain.NotificationDeliveryFailed)

		if delivery.BuyerID != nil {
			penalty := delivery.CalculatePenalty()
			_ = s.notificationSvc.SendToUser(ctx, *delivery.BuyerID, "Delivery Failed",
				fmt.Sprintf("You were unreachable. The item is being returned. A %.0f%% penalty (₦%.2f) will be deducted from your refund.",
					domain.BuyerUnreachablePenaltyPercent, penalty), nil, domain.NotificationDeliveryFailed)
		}

		if delivery.DriverID != nil {
			profile, _ := s.driverRepo.GetByID(ctx, *delivery.DriverID)
			if profile != nil {
				_ = s.notificationSvc.SendToUser(ctx, profile.UserID, "Return to Seller",
					"The buyer was unreachable. Please return the item to the seller.", nil, domain.NotificationRiderAssigned)
			}
		}
	}

	return nil
}

func (s *DriverServiceImpl) ProcessAbandonedTrip(ctx context.Context, tripID uuid.UUID) error {
	trip, err := s.tripRepo.GetByID(ctx, tripID)
	if err != nil {
		return err
	}

	if trip.Status != domain.DriverTripStatusInTransit {
		return fmt.Errorf("trip is not in transit")
	}

	// Mark trip as abandoned
	trip.Status = domain.DriverTripStatusCancelled
	if err := s.tripRepo.Update(ctx, trip); err != nil {
		return fmt.Errorf("failed to update trip: %w", err)
	}

	// Get driver profile
	profile, err := s.driverRepo.GetByID(ctx, trip.DriverID)
	if err != nil {
		s.logger.Error("failed to get driver profile", zap.Error(err))
	}

	// Process all deliveries in trip
	deliveries, err := s.deliveryRepo.GetByTripID(ctx, tripID)
	if err != nil {
		return fmt.Errorf("failed to get trip deliveries: %w", err)
	}

	for _, delivery := range deliveries {
		if delivery.Status.IsTerminal() {
			continue
		}

		oldStatus := delivery.Status
		delivery.Status = domain.DDStatusRefunded

		if err := s.deliveryRepo.Update(ctx, delivery); err != nil {
			s.logger.Error("failed to update delivery", zap.Error(err), zap.String("delivery_id", delivery.ID.String()))
			continue
		}

		// Refund buyer
		if delivery.EscrowID != nil {
			if err := s.escrowService.Refund(ctx, *delivery.EscrowID, "Trip abandoned - driver unresponsive"); err != nil {
				s.logger.Error("failed to refund escrow", zap.Error(err))
			}
		}

		// Record event
		s.recordDeliveryEvent(ctx, delivery.ID, &oldStatus, domain.DDStatusRefunded, domain.DDActorSystem, nil, "trip abandoned - driver unresponsive for 72h", nil)

		// Notify buyer
		if s.notificationSvc != nil && delivery.BuyerID != nil {
			_ = s.notificationSvc.SendToUser(ctx, *delivery.BuyerID, "Delivery Failed",
				"The driver abandoned the trip. Your payment will be refunded.", nil, domain.NotificationDeliveryFailed)
		}
	}

	// Suspend driver
	if profile != nil {
		profile.Status = domain.DriverStatusOffline
		profile.IsVerified = false // Requires re-verification
		profile.CurrentTripID = nil
		profile.OriginCity = nil
		profile.DestinationCity = nil
		profile.ResetCapacity()
		if err := s.driverRepo.Update(ctx, profile); err != nil {
			s.logger.Error("failed to suspend driver", zap.Error(err))
		}

		if s.notificationSvc != nil {
			_ = s.notificationSvc.SendToUser(ctx, profile.UserID, "Account Suspended",
				"Your driver account has been suspended for abandoning a trip. Contact support.", nil, domain.NotificationUserSuspended)
			_ = s.notificationSvc.SendToAdmins(ctx, "Driver Trip Abandoned",
				fmt.Sprintf("Driver %s abandoned trip %s after 72h. Account suspended.", profile.ID.String()[:8], tripID.String()[:8]),
				map[string]string{
					"type":      "driver_abandoned",
					"driver_id": profile.ID.String(),
					"trip_id":   tripID.String(),
				})
		}
	}

	s.logger.Warn("trip marked as abandoned",
		zap.String("trip_id", tripID.String()),
		zap.Int("affected_deliveries", len(deliveries)),
	)

	return nil
}

func (s *DriverServiceImpl) ExpireAwaitingDriverDelivery(ctx context.Context, deliveryID uuid.UUID) error {
	delivery, err := s.deliveryRepo.GetByID(ctx, deliveryID)
	if err != nil {
		return err
	}

	if delivery.Status != domain.DDStatusAwaitingDriver {
		return fmt.Errorf("delivery is not awaiting driver")
	}

	oldStatus := delivery.Status
	delivery.Status = domain.DDStatusExpired

	if err := s.deliveryRepo.Update(ctx, delivery); err != nil {
		return fmt.Errorf("failed to update delivery: %w", err)
	}

	// Refund buyer
	if delivery.EscrowID != nil {
		if err := s.escrowService.Refund(ctx, *delivery.EscrowID, "No driver found within 48h"); err != nil {
			s.logger.Error("failed to refund escrow", zap.Error(err))
		}
	}

	// Record event
	s.recordDeliveryEvent(ctx, deliveryID, &oldStatus, domain.DDStatusExpired, domain.DDActorSystem, nil, "no driver found within 48h", nil)

	s.logger.Info("delivery expired - no driver found",
		zap.String("delivery_id", deliveryID.String()),
	)

	// Notify parties
	if s.notificationSvc != nil {
		_ = s.notificationSvc.SendToUser(ctx, delivery.SellerID, "Delivery Expired",
			"No driver was found for your delivery. The buyer's payment will be refunded.", nil, domain.NotificationDeliveryFailed)
		if delivery.BuyerID != nil {
			_ = s.notificationSvc.SendToUser(ctx, *delivery.BuyerID, "Delivery Cancelled",
				"No driver was available for your delivery. Your payment will be refunded.", nil, domain.NotificationDeliveryFailed)
		}
	}

	return nil
}

func (s *DriverServiceImpl) GetExpiredUnreachableDeliveries(ctx context.Context) ([]*domain.DriverDelivery, error) {
	return s.deliveryRepo.GetExpiredUnreachable(ctx)
}

func (s *DriverServiceImpl) GetStaleAwaitingDriver(ctx context.Context, hoursThreshold int) ([]*domain.DriverDelivery, error) {
	return s.deliveryRepo.GetExpiredAwaiting(ctx, hoursThreshold)
}

func (s *DriverServiceImpl) GetAbandonedTrips(ctx context.Context, hoursThreshold int) ([]*domain.DriverTrip, error) {
	return s.tripRepo.GetAbandonedTrips(ctx, hoursThreshold)
}

func (s *DriverServiceImpl) GetStaleDepartures(ctx context.Context, hoursThreshold int) ([]*domain.DriverTrip, error) {
	return s.tripRepo.GetStaleDepartures(ctx, hoursThreshold)
}

// ============================================
// HELPERS
// ============================================

func (s *DriverServiceImpl) recordDeliveryEvent(ctx context.Context, deliveryID uuid.UUID, fromStatus *domain.DriverDeliveryStatus, toStatus domain.DriverDeliveryStatus, actorType domain.DriverDeliveryActorType, actorID *uuid.UUID, reason string, metadata []byte) {
	event := &domain.DriverDeliveryEvent{
		DeliveryID: deliveryID,
		FromStatus: fromStatus,
		ToStatus:   toStatus,
		ActorType:  actorType,
		ActorID:    actorID,
		Reason:     &reason,
		Metadata:   metadata,
	}

	if err := s.eventRepo.Create(ctx, event); err != nil {
		s.logger.Error("failed to record delivery event", zap.Error(err))
	}
}

func (s *DriverServiceImpl) notifyAdminLockout(ctx context.Context, delivery *domain.DriverDelivery, codeType string, driverID uuid.UUID) {
	s.logger.Warn("verification code lockout",
		zap.String("delivery_id", delivery.ID.String()),
		zap.String("code_type", codeType),
		zap.String("driver_id", driverID.String()),
		zap.String("payment_request_id", delivery.PaymentRequestID.String()),
	)

	// Send admin notification
	if s.notificationSvc != nil {
		title := fmt.Sprintf("%s Code Lockout Alert", codeType)
		body := fmt.Sprintf("Delivery %s: Driver exceeded max %s code attempts. Manual review required.",
			delivery.ID.String()[:8], strings.ToLower(codeType))

		_ = s.notificationSvc.SendToAdmins(ctx, title, body, map[string]string{
			"type":               "code_lockout",
			"delivery_id":        delivery.ID.String(),
			"driver_id":          driverID.String(),
			"code_type":          codeType,
			"payment_request_id": delivery.PaymentRequestID.String(),
		})
	}
}

func (s *DriverServiceImpl) checkTripCompletion(ctx context.Context, profile *domain.DriverProfile, tripID *uuid.UUID) {
	if tripID == nil {
		return
	}

	deliveries, err := s.deliveryRepo.GetByTripID(ctx, *tripID)
	if err != nil {
		s.logger.Error("failed to get trip deliveries", zap.Error(err))
		return
	}

	allDone := true
	for _, d := range deliveries {
		if !d.Status.IsTerminal() && d.Status != domain.DDStatusConfirmationWindow && d.Status != domain.DDStatusCompleted {
			allDone = false
			break
		}
	}

	if allDone {
		now := time.Now()
		trip, err := s.tripRepo.GetByID(ctx, *tripID)
		if err != nil {
			s.logger.Error("failed to get trip", zap.Error(err))
			return
		}

		trip.Status = domain.DriverTripStatusCompleted
		trip.CompletedAt = &now
		if err := s.tripRepo.Update(ctx, trip); err != nil {
			s.logger.Error("failed to complete trip", zap.Error(err))
		}

		// Reset driver
		profile.Status = domain.DriverStatusIdle
		profile.ResetCapacity()
		profile.CurrentTripID = nil
		profile.OriginCity = nil
		profile.DestinationCity = nil
		if err := s.driverRepo.Update(ctx, profile); err != nil {
			s.logger.Error("failed to reset driver", zap.Error(err))
		}

		s.logger.Info("trip completed",
			zap.String("trip_id", tripID.String()),
		)
	}
}

func generateDriverCode() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	}
	return fmt.Sprintf("%06d", n.Int64())
}

var _ DriverService = (*DriverServiceImpl)(nil)
