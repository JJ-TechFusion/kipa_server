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

// RiderService handles rider profile and delivery job operations
type RiderService interface {
	// Profile management
	CreateProfile(ctx context.Context, userID uuid.UUID, req *CreateRiderProfileRequest) (*domain.RiderProfile, error)
	GetProfile(ctx context.Context, userID uuid.UUID) (*domain.RiderProfile, error)
	UpdateProfile(ctx context.Context, userID uuid.UUID, req *UpdateRiderProfileRequest) (*domain.RiderProfile, error)

	// Online status
	GoOnline(ctx context.Context, userID uuid.UUID) error
	GoOffline(ctx context.Context, userID uuid.UUID) error
	UpdateLocation(ctx context.Context, userID uuid.UUID, lat, lng, heading, speed float64) error

	// Job management
	GetAvailableJobs(ctx context.Context, userID uuid.UUID) ([]*domain.DeliveryJob, error)
	AcceptJob(ctx context.Context, userID uuid.UUID, jobID uuid.UUID) (*domain.DeliveryJob, error)
	GetActiveJob(ctx context.Context, userID uuid.UUID) (*domain.DeliveryJob, error)
	ConfirmPickup(ctx context.Context, userID uuid.UUID, jobID uuid.UUID, code string) error
	ConfirmDelivery(ctx context.Context, userID uuid.UUID, jobID uuid.UUID, code string) error
	GetJobHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.DeliveryJob, error)

	// Earnings
	GetEarnings(ctx context.Context, userID uuid.UUID) (*RiderEarnings, error)
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

type RiderServiceImpl struct {
	riderRepo         repository.RiderProfileRepository
	jobRepo           repository.DeliveryJobRepository
	offerRepo         repository.RiderJobOfferRepository
	prRepo            repository.PaymentRequestRepository
	escrowService     EscrowService
	walletService     WalletService
	commissionService RiderCommissionService
	errandService     ErrandService
	locationService   LocationService
	logger            *zap.Logger
	config            *config.Config
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
	logger *zap.Logger,
	config *config.Config,
) RiderService {
	return &RiderServiceImpl{
		riderRepo:         riderRepo,
		jobRepo:           jobRepo,
		offerRepo:         offerRepo,
		prRepo:            prRepo,
		escrowService:     escrowService,
		walletService:     walletService,
		commissionService: commissionService,
		errandService:     errandService,
		locationService:   locationService,
		logger:            logger,
		config:            config,
	}
}

// ... CreateProfile ...

// UpdateLocation updates a rider's current location
func (s *RiderServiceImpl) UpdateLocation(ctx context.Context, userID uuid.UUID, lat, lng, heading, speed float64) error {
	profile, err := s.riderRepo.GetByUserID(ctx, userID)
	if err != nil {
		return err
	}

	// Update rider's current location
	if err := s.riderRepo.UpdateLocation(ctx, profile.ID, lat, lng); err != nil {
		return fmt.Errorf("failed to update location: %w", err)
	}

	// If rider has an active job, record location history for tracking
	job, err := s.jobRepo.GetActiveByRiderID(ctx, profile.ID)
	if err == nil && job != nil {
		s.locationService.HandleLocationUpdate(ctx, profile.ID, job.ID.String(), lat, lng, heading, speed)
	}

	return nil
}

// CreateProfile creates a new rider profile
func (s *RiderServiceImpl) CreateProfile(ctx context.Context, userID uuid.UUID, req *CreateRiderProfileRequest) (*domain.RiderProfile, error) {
	// Check if profile already exists
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

// GetProfile gets a rider's profile
func (s *RiderServiceImpl) GetProfile(ctx context.Context, userID uuid.UUID) (*domain.RiderProfile, error) {
	return s.riderRepo.GetByUserID(ctx, userID)
}

// UpdateProfile updates a rider's profile
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

// GoOnline sets a rider as online and available for deliveries
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

// GoOffline sets a rider as offline
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

	s.logger.Info("rider went offline",
		zap.String("user_id", userID.String()),
	)

	return nil
}



// GetAvailableJobs gets delivery jobs available to a rider (searching status, matching vehicle type)
func (s *RiderServiceImpl) GetAvailableJobs(ctx context.Context, userID uuid.UUID) ([]*domain.DeliveryJob, error) {
	profile, err := s.riderRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if !profile.IsOnline {
		return nil, domain.ErrRiderNotOnline
	}

	// If rider has a vehicle type set, filter jobs by it
	if profile.VehicleType != nil && *profile.VehicleType != "" {
		vehicleType := domain.VehicleType(*profile.VehicleType)
		return s.jobRepo.GetSearchingJobsByVehicleType(ctx, vehicleType, 50)
	}

	// Fallback: get all jobs (for backwards compatibility)
	return s.jobRepo.GetSearchingJobs(ctx, 50)
}

// AcceptJob accepts a delivery job
func (s *RiderServiceImpl) AcceptJob(ctx context.Context, userID uuid.UUID, jobID uuid.UUID) (*domain.DeliveryJob, error) {
	profile, err := s.riderRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Check if rider is blocked for unpaid commission
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

	// Verify rider's vehicle type matches the job's required vehicle type
	if job.RequiredVehicleType != "" {
		if profile.VehicleType == nil || *profile.VehicleType == "" {
			return nil, domain.ErrVehicleTypeMismatch
		}
		if domain.VehicleType(*profile.VehicleType) != job.RequiredVehicleType {
			return nil, domain.ErrVehicleTypeMismatch
		}
	}

	// Assign rider to job
	now := time.Now()
	if err := s.jobRepo.AssignRider(ctx, jobID, profile.ID); err != nil {
		return nil, fmt.Errorf("failed to assign rider: %w", err)
	}

	// Update job
	job.RiderID = &profile.ID
	job.Status = domain.DJStatusAccepted
	job.AcceptedAt = &now

	// Generate verification codes
	pickupCode := generateVerificationCode()
	dropoffCode := generateVerificationCode()
	job.PickupCode = &pickupCode
	job.DropoffCode = &dropoffCode

	if err := s.jobRepo.Update(ctx, job); err != nil {
		return nil, fmt.Errorf("failed to update job: %w", err)
	}

	// Update source status based on type
	if job.SourceType != nil && *job.SourceType == domain.DeliverySourceErrand && job.ErrandID != nil {
		if err := s.errandService.OnJobAccepted(ctx, *job.ErrandID, profile.ID); err != nil {
			s.logger.Warn("failed to update errand status to accepted", zap.Error(err))
		}
	} else {
		// Default to Payment Request
		if err := s.prRepo.UpdateStatus(ctx, job.PaymentRequestID, domain.PRStatusRiderAssigned); err != nil {
			s.logger.Warn("failed to update payment request status to rider_assigned", zap.Error(err))
		}
	}

	s.logger.Info("rider accepted job",
		zap.String("user_id", userID.String()),
		zap.String("job_id", jobID.String()),
	)

	return job, nil
}

// GetActiveJob gets the rider's current active job
func (s *RiderServiceImpl) GetActiveJob(ctx context.Context, userID uuid.UUID) (*domain.DeliveryJob, error) {
	profile, err := s.riderRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return s.jobRepo.GetActiveByRiderID(ctx, profile.ID)
}

// ConfirmPickup confirms item pickup with verification code
func (s *RiderServiceImpl) ConfirmPickup(ctx context.Context, userID uuid.UUID, jobID uuid.UUID, code string) error {
	profile, err := s.riderRepo.GetByUserID(ctx, userID)
	if err != nil {
		return err
	}

	job, err := s.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		return err
	}

	// Verify this is the rider's job
	if job.RiderID == nil || *job.RiderID != profile.ID {
		return domain.ErrUnauthorized
	}

	if job.Status != domain.DJStatusAccepted && job.Status != domain.DJStatusEnRoutePickup {
		return domain.ErrInvalidStatusTransition
	}

	// Verify pickup code
	if job.PickupCode == nil || *job.PickupCode != code {
		return domain.ErrInvalidPickupCode
	}

	now := time.Now()
	job.Status = domain.DJStatusPickedUp
	job.PickedUpAt = &now

	if err := s.jobRepo.Update(ctx, job); err != nil {
		return fmt.Errorf("failed to update job: %w", err)
	}

	// Update source status
	if job.SourceType != nil && *job.SourceType == domain.DeliverySourceErrand && job.ErrandID != nil {
		if err := s.errandService.OnJobPickedUp(ctx, *job.ErrandID); err != nil {
			s.logger.Warn("failed to update errand status to picked_up", zap.Error(err))
		}
	} else {
		// Payment Request logic
		// Mark escrow as delivery in progress
		pr, err := s.prRepo.GetByID(ctx, job.PaymentRequestID)
		if err != nil {
			s.logger.Warn("failed to get payment request for escrow update", zap.Error(err))
		} else if pr.EscrowID != nil {
			if err := s.escrowService.MarkDeliveryInProgress(ctx, *pr.EscrowID); err != nil {
				s.logger.Warn("failed to mark escrow as delivery in progress", zap.Error(err))
			}
		}

		// Update payment request status to in_delivery
		if err := s.prRepo.UpdateStatus(ctx, job.PaymentRequestID, domain.PRStatusInDelivery); err != nil {
			s.logger.Warn("failed to update payment request status to in_delivery", zap.Error(err))
		}
	}



	s.logger.Info("rider confirmed pickup",
		zap.String("job_id", jobID.String()),
	)

	return nil
}

// ConfirmDelivery confirms item delivery with verification code
func (s *RiderServiceImpl) ConfirmDelivery(ctx context.Context, userID uuid.UUID, jobID uuid.UUID, code string) error {
	profile, err := s.riderRepo.GetByUserID(ctx, userID)
	if err != nil {
		return err
	}

	job, err := s.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		return err
	}

	// Verify this is the rider's job
	if job.RiderID == nil || *job.RiderID != profile.ID {
		return domain.ErrUnauthorized
	}

	if job.Status != domain.DJStatusPickedUp && job.Status != domain.DJStatusInTransit {
		return domain.ErrInvalidStatusTransition
	}

	// Verify dropoff code
	if job.DropoffCode == nil || *job.DropoffCode != code {
		return domain.ErrInvalidDropoffCode
	}

	now := time.Now()
	job.Status = domain.DJStatusDelivered
	job.DeliveredAt = &now

	if err := s.jobRepo.Update(ctx, job); err != nil {
		return fmt.Errorf("failed to update job: %w", err)
	}

	// Increment rider's delivery count
	if err := s.riderRepo.IncrementDeliveries(ctx, profile.ID); err != nil {
		s.logger.Error("failed to increment deliveries", zap.Error(err))
	}

	// Record commission owed to platform (buyer pays rider directly)
	// Rider collects full delivery fee from buyer, owes platform 15%
	if job.DeliveryFee > 0 {
		if err := s.commissionService.RecordCommission(ctx, profile.ID, jobID, job.DeliveryFee); err != nil {
			s.logger.Error("failed to record rider commission",
				zap.Error(err),
				zap.String("job_id", jobID.String()),
			)
		}

		// Check if rider should now be blocked (after 5 unpaid rides)
		if err := s.commissionService.CheckAndUpdateBlockStatus(ctx, profile.ID); err != nil {
			s.logger.Error("failed to check commission block status", zap.Error(err))
		}

		// Update total earnings (rider's take = full fee - 15% commission)
		riderTake := domain.CalculateRiderTake(job.DeliveryFee)
		if err := s.riderRepo.AddEarnings(ctx, profile.ID, riderTake); err != nil {
			s.logger.Error("failed to update rider earnings", zap.Error(err))
		}
	}

	// Update source status based on type
	if job.SourceType != nil && *job.SourceType == domain.DeliverySourceErrand && job.ErrandID != nil {
		if err := s.errandService.OnJobDelivered(ctx, *job.ErrandID); err != nil {
			s.logger.Error("failed to update errand status to delivered", zap.Error(err))
		}
	} else {
		// Payment Request logic
		// Start escrow confirmation window (24hr for buyer to confirm/dispute)
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

		// Update payment request status to confirmation_window
		if err := s.prRepo.UpdateStatus(ctx, job.PaymentRequestID, domain.PRStatusConfirmationWindow); err != nil {
			s.logger.Error("failed to update payment request status to confirmation_window", zap.Error(err))
		}
	}

	s.logger.Info("rider confirmed delivery",
		zap.String("job_id", jobID.String()),
		zap.Float64("delivery_fee", job.DeliveryFee),
	)

	return nil
}

// GetJobHistory gets a rider's completed jobs
func (s *RiderServiceImpl) GetJobHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.DeliveryJob, error) {
	profile, err := s.riderRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Get all jobs for this rider (completed statuses)
	statuses := []domain.DeliveryJobStatus{
		domain.DJStatusDelivered,
		domain.DJStatusPaidOut,
	}
	return s.jobRepo.GetByRiderID(ctx, profile.ID, statuses, limit, offset)
}

// GetEarnings gets a rider's earnings summary
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

// generateVerificationCode generates a 6-digit verification code
func generateVerificationCode() string {
	return fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
}

// Ensure RiderServiceImpl implements RiderService
var _ RiderService = (*RiderServiceImpl)(nil)
