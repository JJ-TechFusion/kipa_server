package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/config"
	"kipa/internal/domain"
	"kipa/internal/repository"
)
type RiderService interface {
	CreateProfile(ctx context.Context, userID uuid.UUID, req *CreateRiderProfileRequest) (*domain.RiderProfile, error)
	GetProfile(ctx context.Context, userID uuid.UUID) (*domain.RiderProfile, error)
	UpdateProfile(ctx context.Context, userID uuid.UUID, req *UpdateRiderProfileRequest) (*domain.RiderProfile, error)
	GoOnline(ctx context.Context, userID uuid.UUID) error
	GoOffline(ctx context.Context, userID uuid.UUID) error
	UpdateLocation(ctx context.Context, userID uuid.UUID, lat, lng, heading, speed float64) error
	GetAvailableJobs(ctx context.Context, userID uuid.UUID) ([]*domain.DeliveryJob, error)
	AcceptJob(ctx context.Context, userID uuid.UUID, jobID uuid.UUID) (*domain.DeliveryJob, error)
	GetActiveJob(ctx context.Context, userID uuid.UUID) (*domain.DeliveryJob, error)
	ConfirmPickup(ctx context.Context, userID uuid.UUID, jobID uuid.UUID, code string) error
	ConfirmDelivery(ctx context.Context, userID uuid.UUID, jobID uuid.UUID, code string) error
	GetJobHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.DeliveryJob, error)
	GetEarnings(ctx context.Context, userID uuid.UUID) (*RiderEarnings, error)

	// Unavailability handling
	MarkSellerUnavailable(ctx context.Context, userID uuid.UUID, jobID uuid.UUID) error
	MarkBuyerUnavailable(ctx context.Context, userID uuid.UUID, jobID uuid.UUID) (*ForcedReturnInfo, error)
	CompleteForcedReturn(ctx context.Context, userID uuid.UUID, jobID uuid.UUID, code string) error
}

type CreateRiderProfileRequest struct {
	LicenseNumber       string `json:"license_number" binding:"required"`
	NINNumber           string `json:"nin_number" binding:"required"`
	LicensePhotoURL     string `json:"license_photo_url" binding:"required"`
	NINPhotoURL         string `json:"nin_photo_url" binding:"required"`
	FaceVerificationURL string `json:"face_verification_url" binding:"required"`
	VehicleType         string `json:"vehicle_type" binding:"required"`
	VehiclePlate        string `json:"vehicle_plate" binding:"required"`
	VehiclePhotoURL     string `json:"vehicle_photo_url" binding:"required"`
}

type UpdateRiderProfileRequest struct {
	VehicleType     *string `json:"vehicle_type"`
	VehiclePlate    *string `json:"vehicle_plate"`
	VehiclePhotoURL *string `json:"vehicle_photo_url"`
}

type RiderEarnings struct {
	TotalEarnings    float64 `json:"total_earnings"`
	TotalDeliveries  int     `json:"total_deliveries"`
	AvailableBalance float64 `json:"available_balance"`
	Rating           float64 `json:"rating"`
}

type ForcedReturnInfo struct {
	ForcedReturnJobID string `json:"forced_return_job_id"`
	ReturnCode        string `json:"return_code"`
	RebookDeadline    string `json:"rebook_deadline"`
}

type JobStatusBroadcaster interface {
	BroadcastJobStatus(jobID, status string)
}

type RiderLocationCache interface {
	UpdateLocation(ctx context.Context, riderID uuid.UUID, lat, lng, heading, speed float64) error
	RemoveLocation(ctx context.Context, riderID uuid.UUID) error
}

type RiderServiceImpl struct {
	riderRepo            repository.RiderProfileRepository
	jobRepo              repository.DeliveryJobRepository
	offerRepo            repository.RiderJobOfferRepository
	prRepo               repository.PaymentRequestRepository
	escrowService        EscrowService
	walletService        WalletService
	commissionService    RiderCommissionService
	errandService        ErrandService
	locationService      LocationService
	notificationService  NotificationService
	broadcaster          JobStatusBroadcaster
	riderLocationCache   RiderLocationCache
	logger               *zap.Logger
	config               *config.Config
}

func NewRiderService(
	riderRepo repository.RiderProfileRepository,
	jobRepo repository.DeliveryJobRepository,
	offerRepo repository.RiderJobOfferRepository,
	prRepo repository.PaymentRequestRepository,
	escrowService EscrowService,
	walletService WalletService,
	commissionService RiderCommissionService,
	errandService ErrandService,
	locationService LocationService,
	notificationService NotificationService,
	broadcaster JobStatusBroadcaster,
	riderLocationCache RiderLocationCache,
	logger *zap.Logger,
	config *config.Config,
) RiderService {
	return &RiderServiceImpl{
		riderRepo:            riderRepo,
		jobRepo:              jobRepo,
		offerRepo:            offerRepo,
		prRepo:               prRepo,
		escrowService:        escrowService,
		walletService:        walletService,
		commissionService:    commissionService,
		errandService:        errandService,
		locationService:      locationService,
		notificationService:  notificationService,
		broadcaster:          broadcaster,
		riderLocationCache:   riderLocationCache,
		logger:               logger,
		config:               config,
	}
}
func (s *RiderServiceImpl) UpdateLocation(ctx context.Context, userID uuid.UUID, lat, lng, heading, speed float64) error {
	profile, err := s.riderRepo.GetByUserID(ctx, userID)
	if err != nil {
		return err
	}
	if err := s.riderRepo.UpdateLocation(ctx, profile.ID, lat, lng); err != nil {
		return fmt.Errorf("failed to update location: %w", err)
	}

	// Store in Redis for nearby riders feature (only if online)
	if profile.IsOnline && s.riderLocationCache != nil {
		if err := s.riderLocationCache.UpdateLocation(ctx, profile.ID, lat, lng, heading, speed); err != nil {
			s.logger.Warn("failed to update rider location in cache", zap.Error(err))
		}
	}

	job, err := s.jobRepo.GetActiveByRiderID(ctx, profile.ID)
	if err != nil {
		s.logger.Info("no active job found for rider",
			zap.String("rider_profile_id", profile.ID.String()),
			zap.Error(err),
		)
	} else if job != nil {
		s.logger.Info("broadcasting location update",
			zap.String("rider_profile_id", profile.ID.String()),
			zap.String("job_id", job.ID.String()),
			zap.String("job_status", string(job.Status)),
		)
		s.locationService.HandleLocationUpdate(ctx, profile.ID, job.ID.String(), lat, lng, heading, speed)
	} else {
		s.logger.Info("job is nil for rider",
			zap.String("rider_profile_id", profile.ID.String()),
		)
	}

	return nil
}
func (s *RiderServiceImpl) CreateProfile(ctx context.Context, userID uuid.UUID, req *CreateRiderProfileRequest) (*domain.RiderProfile, error) {
	existing, err := s.riderRepo.GetByUserID(ctx, userID)
	if err == nil && existing != nil {
		return nil, fmt.Errorf("rider profile already exists")
	}

	profile := &domain.RiderProfile{
		UserID:              userID,
		LicenseNumber:       &req.LicenseNumber,
		NINNumber:           &req.NINNumber,
		LicensePhotoURL:     &req.LicensePhotoURL,
		NINPhotoURL:         &req.NINPhotoURL,
		FaceVerificationURL: &req.FaceVerificationURL,
		VehicleType:         &req.VehicleType,
		VehiclePlate:        &req.VehiclePlate,
		VehiclePhotoURL:     &req.VehiclePhotoURL,
		Status:              domain.RiderStatusPendingVerification,
		Rating:              5.0,
		TotalDeliveries:     0,
		TotalEarnings:       0,
	}

	if err := s.riderRepo.Create(ctx, profile); err != nil {
		return nil, fmt.Errorf("failed to create rider profile: %w", err)
	}

	s.logger.Info("rider profile created",
		zap.String("user_id", userID.String()),
		zap.String("profile_id", profile.ID.String()),
	)

	return profile, nil
}
func (s *RiderServiceImpl) GetProfile(ctx context.Context, userID uuid.UUID) (*domain.RiderProfile, error) {
	return s.riderRepo.GetByUserID(ctx, userID)
}
func (s *RiderServiceImpl) UpdateProfile(ctx context.Context, userID uuid.UUID, req *UpdateRiderProfileRequest) (*domain.RiderProfile, error) {
	profile, err := s.riderRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if req.VehicleType != nil {
		profile.VehicleType = req.VehicleType
	}
	if req.VehiclePlate != nil {
		profile.VehiclePlate = req.VehiclePlate
	}
	if req.VehiclePhotoURL != nil {
		profile.VehiclePhotoURL = req.VehiclePhotoURL
	}

	if err := s.riderRepo.Update(ctx, profile); err != nil {
		return nil, fmt.Errorf("failed to update rider profile: %w", err)
	}

	return profile, nil
}
func (s *RiderServiceImpl) GoOnline(ctx context.Context, userID uuid.UUID) error {
	profile, err := s.riderRepo.GetByUserID(ctx, userID)
	if err != nil {
		return err
	}

	if !profile.CanGoOnline() {
		return domain.ErrRiderNotVerified
	}

	if profile.IsOnline {
		return domain.ErrRiderAlreadyOnline
	}

	if err := s.riderRepo.SetOnline(ctx, profile.ID, true); err != nil {
		return fmt.Errorf("failed to go online: %w", err)
	}

	s.logger.Info("rider went online",
		zap.String("user_id", userID.String()),
	)

	return nil
}
func (s *RiderServiceImpl) GoOffline(ctx context.Context, userID uuid.UUID) error {
	profile, err := s.riderRepo.GetByUserID(ctx, userID)
	if err != nil {
		return err
	}

	if !profile.IsOnline {
		return nil // Already offline, no-op
	}

	if err := s.riderRepo.SetOnline(ctx, profile.ID, false); err != nil {
		return fmt.Errorf("failed to go offline: %w", err)
	}

	// Remove from Redis location cache
	if s.riderLocationCache != nil {
		if err := s.riderLocationCache.RemoveLocation(ctx, profile.ID); err != nil {
			s.logger.Warn("failed to remove rider location from cache", zap.Error(err))
		}
	}

	s.logger.Info("rider went offline",
		zap.String("user_id", userID.String()),
	)

	return nil
}
func (s *RiderServiceImpl) GetAvailableJobs(ctx context.Context, userID uuid.UUID) ([]*domain.DeliveryJob, error) {
	profile, err := s.riderRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if !profile.IsOnline {
		return nil, domain.ErrRiderNotOnline
	}
	if profile.VehicleType != nil && *profile.VehicleType != "" {
		vehicleType := domain.VehicleType(*profile.VehicleType)
		return s.jobRepo.GetSearchingJobsByVehicleType(ctx, vehicleType, 50)
	}
	return s.jobRepo.GetSearchingJobs(ctx, 50)
}
func (s *RiderServiceImpl) AcceptJob(ctx context.Context, userID uuid.UUID, jobID uuid.UUID) (*domain.DeliveryJob, error) {
	profile, err := s.riderRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if profile.IsCommissionBlocked {
		s.logger.Warn("rider blocked for unpaid commission",
			zap.String("user_id", userID.String()),
			zap.Float64("unpaid_commission", profile.UnpaidCommission),
			zap.Int("unpaid_count", profile.UnpaidDeliveryCount),
		)
		return nil, domain.ErrRiderBlockedForCommission
	}

	if !profile.IsOnline {
		return nil, domain.ErrRiderNotOnline
	}

	job, err := s.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		return nil, err
	}

	if job.Status != domain.DJStatusSearching {
		return nil, domain.ErrJobAlreadyAccepted
	}
	if job.RequiredVehicleType != "" {
		if profile.VehicleType == nil || *profile.VehicleType == "" {
			return nil, domain.ErrVehicleTypeMismatch
		}
		if domain.VehicleType(*profile.VehicleType) != job.RequiredVehicleType {
			return nil, domain.ErrVehicleTypeMismatch
		}
	}
	now := time.Now()
	if err := s.jobRepo.AssignRider(ctx, jobID, profile.ID); err != nil {
		return nil, fmt.Errorf("failed to assign rider: %w", err)
	}
	job.RiderID = &profile.ID
	job.Status = domain.DJStatusAccepted
	job.AcceptedAt = &now
	pickupCode := generateVerificationCode()
	dropoffCode := generateVerificationCode()
	job.PickupCode = &pickupCode
	job.DropoffCode = &dropoffCode

	if err := s.jobRepo.Update(ctx, job); err != nil {
		return nil, fmt.Errorf("failed to update job: %w", err)
	}

	s.broadcastJobStatus(jobID, domain.DJStatusAccepted)

	if job.SourceType != nil && *job.SourceType == domain.DeliverySourceErrand && job.ErrandID != nil {
		if err := s.errandService.OnJobAccepted(ctx, *job.ErrandID, profile.ID); err != nil {
			s.logger.Warn("failed to update errand status to accepted", zap.Error(err))
		}
	} else {
		if err := s.prRepo.UpdateStatus(ctx, job.PaymentRequestID, domain.PRStatusRiderAssigned); err != nil {
			s.logger.Warn("failed to update payment request status to rider_assigned", zap.Error(err))
		}
	}

	s.logger.Info("rider accepted job",
		zap.String("user_id", userID.String()),
		zap.String("job_id", jobID.String()),
	)

	// Notify buyer and seller that rider is assigned
	if s.notificationService != nil {
		pr, _ := s.prRepo.GetByID(ctx, job.PaymentRequestID)
		if pr != nil {
			data := map[string]string{"job_id": jobID.String(), "payment_request_id": pr.ID.String()}
			if pr.BuyerID != nil {
				go s.notificationService.SendToUser(ctx, *pr.BuyerID, "Rider Assigned", "A rider is on the way to pick up your order", data, domain.NotificationRiderAssigned)
			}
			go s.notificationService.SendToUser(ctx, pr.SellerID, "Rider Assigned", "A rider is on the way to collect the item", data, domain.NotificationRiderAssigned)
		}
	}

	return job, nil
}
func (s *RiderServiceImpl) GetActiveJob(ctx context.Context, userID uuid.UUID) (*domain.DeliveryJob, error) {
	profile, err := s.riderRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return s.jobRepo.GetActiveByRiderID(ctx, profile.ID)
}
func (s *RiderServiceImpl) ConfirmPickup(ctx context.Context, userID uuid.UUID, jobID uuid.UUID, code string) error {
	profile, err := s.riderRepo.GetByUserID(ctx, userID)
	if err != nil {
		return err
	}

	job, err := s.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		return err
	}
	if job.RiderID == nil || *job.RiderID != profile.ID {
		return domain.ErrUnauthorized
	}

	if job.Status != domain.DJStatusAccepted && job.Status != domain.DJStatusEnRoutePickup {
		return domain.ErrInvalidStatusTransition
	}
	if job.PickupCode == nil || *job.PickupCode != code {
		return domain.ErrInvalidPickupCode
	}

	now := time.Now()
	job.Status = domain.DJStatusPickedUp
	job.PickedUpAt = &now

	if err := s.jobRepo.Update(ctx, job); err != nil {
		return fmt.Errorf("failed to update job: %w", err)
	}

	s.broadcastJobStatus(jobID, domain.DJStatusPickedUp)

	if job.SourceType != nil && *job.SourceType == domain.DeliverySourceErrand && job.ErrandID != nil {
		if err := s.errandService.OnJobPickedUp(ctx, *job.ErrandID); err != nil {
			s.logger.Warn("failed to update errand status to picked_up", zap.Error(err))
		}
	} else {
		pr, err := s.prRepo.GetByID(ctx, job.PaymentRequestID)
		if err != nil {
			s.logger.Warn("failed to get payment request for escrow update", zap.Error(err))
		} else if pr.EscrowID != nil {
			if err := s.escrowService.MarkDeliveryInProgress(ctx, *pr.EscrowID); err != nil {
				s.logger.Warn("failed to mark escrow as delivery in progress", zap.Error(err))
			}
		}
		if err := s.prRepo.UpdateStatus(ctx, job.PaymentRequestID, domain.PRStatusInDelivery); err != nil {
			s.logger.Warn("failed to update payment request status to in_delivery", zap.Error(err))
		}
	}

	s.logger.Info("rider confirmed pickup",
		zap.String("job_id", jobID.String()),
	)

	// Notify buyer that order is picked up
	if s.notificationService != nil {
		pr, _ := s.prRepo.GetByID(ctx, job.PaymentRequestID)
		if pr != nil && pr.BuyerID != nil {
			data := map[string]string{"job_id": jobID.String()}
			go s.notificationService.SendToUser(ctx, *pr.BuyerID, "Order Picked Up", "Your order is on the way!", data, domain.NotificationOrderPickedUp)
		}
	}

	return nil
}
func (s *RiderServiceImpl) ConfirmDelivery(ctx context.Context, userID uuid.UUID, jobID uuid.UUID, code string) error {
	profile, err := s.riderRepo.GetByUserID(ctx, userID)
	if err != nil {
		return err
	}

	job, err := s.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		return err
	}
	if job.RiderID == nil || *job.RiderID != profile.ID {
		return domain.ErrUnauthorized
	}

	if job.Status != domain.DJStatusPickedUp && job.Status != domain.DJStatusInTransit {
		return domain.ErrInvalidStatusTransition
	}
	if job.DropoffCode == nil || *job.DropoffCode != code {
		return domain.ErrInvalidDropoffCode
	}

	now := time.Now()
	job.Status = domain.DJStatusDelivered
	job.DeliveredAt = &now

	if err := s.jobRepo.Update(ctx, job); err != nil {
		return fmt.Errorf("failed to update job: %w", err)
	}

	s.broadcastJobStatus(jobID, domain.DJStatusDelivered)

	if err := s.riderRepo.IncrementDeliveries(ctx, profile.ID); err != nil {
		s.logger.Error("failed to increment deliveries", zap.Error(err))
	}
	if job.DeliveryFee > 0 {
		if err := s.commissionService.RecordCommission(ctx, profile.ID, jobID, job.DeliveryFee); err != nil {
			s.logger.Error("failed to record rider commission",
				zap.Error(err),
				zap.String("job_id", jobID.String()),
			)
		}
		if err := s.commissionService.CheckAndUpdateBlockStatus(ctx, profile.ID); err != nil {
			s.logger.Error("failed to check commission block status", zap.Error(err))
		}
		riderTake := domain.CalculateRiderTake(job.DeliveryFee)
		if err := s.riderRepo.AddEarnings(ctx, profile.ID, riderTake); err != nil {
			s.logger.Error("failed to update rider earnings", zap.Error(err))
		}
	}
	if job.SourceType != nil && *job.SourceType == domain.DeliverySourceErrand && job.ErrandID != nil {
		if err := s.errandService.OnJobDelivered(ctx, *job.ErrandID); err != nil {
			s.logger.Error("failed to update errand status to delivered", zap.Error(err))
		}
	} else {
		pr, err := s.prRepo.GetByID(ctx, job.PaymentRequestID)
		if err != nil {
			s.logger.Error("failed to get payment request for escrow update", zap.Error(err))
		} else if pr.EscrowID != nil {
			s.logger.Info("calling StartConfirmationWindow",
				zap.String("escrow_id", pr.EscrowID.String()),
				zap.Float64("delivery_fee", job.DeliveryFee),
			)
			if err := s.escrowService.StartConfirmationWindow(ctx, *pr.EscrowID, job.DeliveryFee); err != nil {
				s.logger.Error("failed to start escrow confirmation window",
					zap.Error(err),
					zap.String("escrow_id", pr.EscrowID.String()),
				)
			}
		} else {
			s.logger.Warn("payment request has no escrow ID",
				zap.String("pr_id", job.PaymentRequestID.String()),
			)
		}
		if err := s.prRepo.UpdateStatus(ctx, job.PaymentRequestID, domain.PRStatusConfirmationWindow); err != nil {
			s.logger.Error("failed to update payment request status to confirmation_window", zap.Error(err))
		}
	}

	s.logger.Info("rider confirmed delivery",
		zap.String("job_id", jobID.String()),
		zap.Float64("delivery_fee", job.DeliveryFee),
	)

	// Notify buyer and seller that order is delivered
	if s.notificationService != nil {
		pr, _ := s.prRepo.GetByID(ctx, job.PaymentRequestID)
		if pr != nil {
			data := map[string]string{"job_id": jobID.String(), "payment_request_id": pr.ID.String()}
			if pr.BuyerID != nil {
				go s.notificationService.SendToUser(ctx, *pr.BuyerID, "Order Delivered", "Your order has been delivered. Please confirm receipt.", data, domain.NotificationOrderDelivered)
			}
			go s.notificationService.SendToUser(ctx, pr.SellerID, "Order Delivered", "Your item has been delivered to the buyer", data, domain.NotificationOrderDelivered)
		}
	}

	return nil
}
func (s *RiderServiceImpl) GetJobHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.DeliveryJob, error) {
	profile, err := s.riderRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	statuses := []domain.DeliveryJobStatus{
		domain.DJStatusDelivered,
		domain.DJStatusPaidOut,
	}
	return s.jobRepo.GetByRiderID(ctx, profile.ID, statuses, limit, offset)
}
func (s *RiderServiceImpl) GetEarnings(ctx context.Context, userID uuid.UUID) (*RiderEarnings, error) {
	profile, err := s.riderRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	balance, err := s.walletService.GetBalance(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &RiderEarnings{
		TotalEarnings:    profile.TotalEarnings,
		TotalDeliveries:  profile.TotalDeliveries,
		AvailableBalance: balance.AvailableBalance,
		Rating:           profile.Rating,
	}, nil
}

func (s *RiderServiceImpl) MarkSellerUnavailable(ctx context.Context, userID uuid.UUID, jobID uuid.UUID) error {
	profile, err := s.riderRepo.GetByUserID(ctx, userID)
	if err != nil {
		return err
	}

	job, err := s.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		return err
	}

	if job.RiderID == nil || *job.RiderID != profile.ID {
		return domain.ErrUnauthorized
	}

	if !job.CanMarkSellerUnavailable() {
		return fmt.Errorf("cannot mark seller unavailable in current status: %s", job.Status)
	}

	now := time.Now()
	job.Status = domain.DJStatusSellerUnavailable
	job.UnavailableMarkedAt = &now

	if err := s.jobRepo.Update(ctx, job); err != nil {
		return fmt.Errorf("failed to update job: %w", err)
	}

	s.broadcastJobStatus(jobID, domain.DJStatusSellerUnavailable)

	// Update payment request status and refund buyer
	pr, err := s.prRepo.GetByID(ctx, job.PaymentRequestID)
	if err != nil {
		return fmt.Errorf("failed to get payment request: %w", err)
	}

	if err := s.prRepo.UpdateStatus(ctx, pr.ID, domain.PRStatusCancelledSellerUnavailable); err != nil {
		s.logger.Error("failed to update payment request status", zap.Error(err))
	}

	// Full refund to buyer
	if pr.EscrowID != nil {
		if err := s.escrowService.Refund(ctx, *pr.EscrowID, "Seller unavailable at pickup"); err != nil {
			s.logger.Error("failed to refund buyer", zap.Error(err))
		}
	}

	s.logger.Info("seller marked unavailable",
		zap.String("job_id", jobID.String()),
		zap.String("rider_id", profile.ID.String()),
	)

	// Notify buyer
	if s.notificationService != nil && pr.BuyerID != nil {
		data := map[string]string{"job_id": jobID.String(), "payment_request_id": pr.ID.String()}
		go s.notificationService.SendToUser(ctx, *pr.BuyerID, "Order Cancelled",
			"The seller was unavailable for pickup. Your payment has been refunded.",
			data, domain.NotificationOrderCancelled)
	}

	return nil
}

// MarkBuyerUnavailable handles when buyer is not available at dropoff
// Flow: create forced return job, set pending_rebook, buyer has 24h to rebook
func (s *RiderServiceImpl) MarkBuyerUnavailable(ctx context.Context, userID uuid.UUID, jobID uuid.UUID) (*ForcedReturnInfo, error) {
	profile, err := s.riderRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	job, err := s.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		return nil, err
	}

	if job.RiderID == nil || *job.RiderID != profile.ID {
		return nil, domain.ErrUnauthorized
	}

	if !job.CanMarkBuyerUnavailable() {
		return nil, fmt.Errorf("cannot mark buyer unavailable in current status: %s", job.Status)
	}

	now := time.Now()
	job.Status = domain.DJStatusBuyerUnavailable
	job.UnavailableMarkedAt = &now

	if err := s.jobRepo.Update(ctx, job); err != nil {
		return nil, fmt.Errorf("failed to update job: %w", err)
	}

	s.broadcastJobStatus(jobID, domain.DJStatusBuyerUnavailable)

	pr, err := s.prRepo.GetByID(ctx, job.PaymentRequestID)
	if err != nil {
		return nil, fmt.Errorf("failed to get payment request: %w", err)
	}

	coords, err := s.jobRepo.GetJobCoordinates(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get job coordinates: %w", err)
	}

	// Create forced return job (swap pickup/dropoff)
	returnJob := &domain.DeliveryJob{
		PaymentRequestID:    job.PaymentRequestID,
		RiderID:             &profile.ID, // Same rider does the return
		IsForcedReturn:      true,
		PickupAddress:       job.DropoffAddress,
		PickupLat:           coords.DropoffLat,
		PickupLng:           coords.DropoffLng,
		DropoffAddress:      job.PickupAddress,
		DropoffLat:          coords.PickupLat,
		DropoffLng:          coords.PickupLng,
		RequiredVehicleType: job.RequiredVehicleType,
		DeliveryFee:         job.DeliveryFee, // Platform pays this
		Status:              domain.DJStatusForcedReturn,
	}

	returnCode := generateVerificationCode()
	returnJob.DropoffCode = &returnCode

	if err := s.jobRepo.Create(ctx, returnJob); err != nil {
		return nil, fmt.Errorf("failed to create forced return job: %w", err)
	}

	// Update payment request
	rebookDeadline := now.Add(24 * time.Hour)
	pr.ForcedReturnJobID = &returnJob.ID
	pr.RebookDeadline = &rebookDeadline
	pr.Status = domain.PRStatusPendingRebook

	if err := s.prRepo.Update(ctx, pr); err != nil {
		return nil, fmt.Errorf("failed to update payment request: %w", err)
	}

	job.Status = domain.DJStatusForcedReturn
	if err := s.jobRepo.Update(ctx, job); err != nil {
		return nil, fmt.Errorf("failed to update original job status: %w", err)
	}

	s.broadcastJobStatus(jobID, domain.DJStatusForcedReturn)

	// Notify buyer and seller
	if s.notificationService != nil {
		data := map[string]string{
			"job_id":             jobID.String(),
			"payment_request_id": pr.ID.String(),
			"rebook_deadline":    rebookDeadline.Format(time.RFC3339),
		}
		if pr.BuyerID != nil {
			go s.notificationService.SendToUser(ctx, *pr.BuyerID, "Delivery Attempt Failed",
				"You were unavailable for delivery. Please rebook within 24 hours or receive a refund minus 5% fee.",
				data, domain.NotificationDeliveryFailed)
		}
		go s.notificationService.SendToUser(ctx, pr.SellerID, "Item Being Returned",
			"The buyer was unavailable. The rider is returning the item to you.",
			data, domain.NotificationItemReturning)
	}

	return &ForcedReturnInfo{
		ForcedReturnJobID: returnJob.ID.String(),
		ReturnCode:        returnCode,
		RebookDeadline:    rebookDeadline.Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}

func (s *RiderServiceImpl) CompleteForcedReturn(ctx context.Context, userID uuid.UUID, jobID uuid.UUID, code string) error {
	s.logger.Info("starting complete forced return process",
		zap.String("user_id", userID.String()),
		zap.String("job_id", jobID.String()),
		zap.String("provided_code", code),
	)

	profile, err := s.riderRepo.GetByUserID(ctx, userID)
	if err != nil {
		s.logger.Error("failed to get rider profile", zap.Error(err), zap.String("user_id", userID.String()))
		return err
	}
	s.logger.Info("rider profile retrieved", zap.String("profile_id", profile.ID.String()))

	job, err := s.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		s.logger.Error("failed to get delivery job", zap.Error(err), zap.String("job_id", jobID.String()))
		return err
	}
	
	s.logger.Info("delivery job retrieved",
		zap.String("job_status", string(job.Status)),
		zap.Bool("is_forced_return", job.IsForcedReturn),
		zap.String("payment_request_id", job.PaymentRequestID.String()),
		zap.Bool("has_rider_id", job.RiderID != nil),
		zap.Bool("has_dropoff_code", job.DropoffCode != nil),
	)

	if job.RiderID == nil || *job.RiderID != profile.ID {
		s.logger.Error("unauthorized rider access",
			zap.String("job_rider_id", func() string {
				if job.RiderID != nil {
					return job.RiderID.String()
				}
				return "nil"
			}()),
			zap.String("profile_id", profile.ID.String()),
		)
		return domain.ErrUnauthorized
	}

	if !job.IsForcedReturn || job.Status != domain.DJStatusForcedReturn {
		s.logger.Error("invalid job type or status for forced return completion",
			zap.Bool("is_forced_return", job.IsForcedReturn),
			zap.String("current_status", string(job.Status)),
			zap.String("expected_status", string(domain.DJStatusForcedReturn)),
		)
		return fmt.Errorf("not a forced return job or invalid status")
	}

	if job.DropoffCode == nil || *job.DropoffCode != code {
		s.logger.Error("invalid dropoff code",
			zap.String("expected_code", func() string {
				if job.DropoffCode != nil {
					return *job.DropoffCode
				}
				return "nil"
			}()),
			zap.String("provided_code", code),
		)
		return domain.ErrInvalidDropoffCode
	}

	s.logger.Info("validation passed, updating job status to forced return done")
	
	now := time.Now()
	job.Status = domain.DJStatusForcedReturnDone
	job.DeliveredAt = &now

	s.logger.Info("updating job with completion details",
		zap.String("new_status", string(job.Status)),
		zap.Time("delivered_at", now),
	)

	if err := s.jobRepo.Update(ctx, job); err != nil {
		s.logger.Error("failed to update job status", zap.Error(err), zap.String("job_id", jobID.String()))
		return fmt.Errorf("failed to update job: %w", err)
	}
	s.logger.Info("job status updated successfully")

	s.broadcastJobStatus(jobID, domain.DJStatusForcedReturnDone)

	// Platform pays rider for forced return delivery
	if job.DeliveryFee > 0 {
		riderTake := domain.CalculateRiderTake(job.DeliveryFee)
		s.logger.Info("adding rider earnings for forced return",
			zap.Float64("delivery_fee", job.DeliveryFee),
			zap.Float64("rider_take", riderTake),
		)
		if err := s.riderRepo.AddEarnings(ctx, profile.ID, riderTake); err != nil {
			s.logger.Error("failed to add rider earnings for forced return", zap.Error(err))
		} else {
			s.logger.Info("rider earnings added successfully")
		}
	} else {
		s.logger.Info("no delivery fee to pay rider", zap.Float64("delivery_fee", job.DeliveryFee))
	}

	s.logger.Info("forced return completed successfully",
		zap.String("job_id", jobID.String()),
		zap.String("rider_id", profile.ID.String()),
	)


	pr, err := s.prRepo.GetByID(ctx, job.PaymentRequestID)
	if err != nil {
		s.logger.Error("failed to get payment request for notification", zap.Error(err), zap.String("payment_request_id", job.PaymentRequestID.String()))
	} else {
		if s.notificationService != nil {
			data := map[string]string{"job_id": jobID.String(), "payment_request_id": pr.ID.String()}
			go s.notificationService.SendToUser(ctx, pr.SellerID, "Item Returned",
				"The item has been returned to you. Waiting for buyer to rebook delivery.",
				data, domain.NotificationItemReturned)
		} else {
			s.logger.Warn("notification service not available, skipping seller notification")
		}
	}

	return nil
}

func generateVerificationCode() string {
	return fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
}

func (s *RiderServiceImpl) broadcastJobStatus(jobID uuid.UUID, status domain.DeliveryJobStatus) {
	if s.broadcaster != nil {
		s.broadcaster.BroadcastJobStatus(jobID.String(), string(status))
	}
}
var _ RiderService = (*RiderServiceImpl)(nil)
