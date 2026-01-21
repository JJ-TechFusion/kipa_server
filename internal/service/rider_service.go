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
	job, err := s.jobRepo.GetActiveByRiderID(ctx, profile.ID)
	if err == nil && job != nil {
		s.locationService.HandleLocationUpdate(ctx, profile.ID, job.ID.String(), lat, lng, heading, speed)
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
func generateVerificationCode() string {
	return fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
}
var _ RiderService = (*RiderServiceImpl)(nil)
