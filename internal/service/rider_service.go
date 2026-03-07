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
	GetAvailableJobByID(ctx context.Context, userID uuid.UUID, jobID uuid.UUID) (*domain.DeliveryJob, *domain.PaymentRequest, error)
	AcceptJob(ctx context.Context, userID uuid.UUID, jobID uuid.UUID) (*domain.DeliveryJob, error)
	GetActiveJob(ctx context.Context, userID uuid.UUID) (*ActiveJobDetails, error)
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
	VehicleBrand        string `json:"vehicle_brand" binding:"required"`
	VehicleColor        string `json:"vehicle_color" binding:"required"`
	VehiclePlate        string `json:"vehicle_plate" binding:"required"`
	VehiclePhotoURL     string `json:"vehicle_photo_url" binding:"required"`
}

type UpdateRiderProfileRequest struct {
	VehicleType     *string `json:"vehicle_type"`
	VehicleBrand    *string `json:"vehicle_brand"`
	VehicleColor    *string `json:"vehicle_color"`
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

type ContactInfo struct {
	Name  string `json:"name"`
	Phone string `json:"phone"`
}

type ActiveJobDetails struct {
	Job            *domain.DeliveryJob
	PaymentRequest *domain.PaymentRequest
	Buyer          *ContactInfo
	Seller         *ContactInfo
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
	userRepo             repository.UserRepository
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
	userRepo repository.UserRepository,
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
		userRepo:             userRepo,
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
		VehicleBrand:        &req.VehicleBrand,
		VehicleColor:        &req.VehicleColor,
		VehiclePlate:        &req.VehiclePlate,
		VehiclePhotoURL:     &req.VehiclePhotoURL,
		Status:              domain.RiderStatusPendingVerification,
		Rating:              5.0,
		TotalDeliveries:     0,
		TotalEarnings:       0,
	}

	if err := s.riderRepo.Create(ctx, profile); err != nil {
		s.logger.Error("failed to create rider profile", zap.Error(err), zap.String("user_id", userID.String()))
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
	if req.VehicleBrand != nil {
		profile.VehicleBrand = req.VehicleBrand
	}
	if req.VehicleColor != nil {
		profile.VehicleColor = req.VehicleColor
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

func (s *RiderServiceImpl) GetAvailableJobByID(ctx context.Context, userID uuid.UUID, jobID uuid.UUID) (*domain.DeliveryJob, *domain.PaymentRequest, error) {
	profile, err := s.riderRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, nil, err
	}

	if !profile.IsOnline {
		return nil, nil, domain.ErrRiderNotOnline
	}

	job, err := s.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		return nil, nil, err
	}

	// Verify job is still available (searching status)
	if job.Status != domain.DJStatusSearching {
		return nil, nil, domain.ErrJobAlreadyAccepted
	}

	// Verify vehicle type matches if required
	if job.RequiredVehicleType != "" {
		if profile.VehicleType == nil || *profile.VehicleType == "" {
			return nil, nil, domain.ErrVehicleTypeMismatch
		}
		if domain.VehicleType(*profile.VehicleType) != job.RequiredVehicleType {
			return nil, nil, domain.ErrVehicleTypeMismatch
		}
	}

	// Get payment request for package details
	var pr *domain.PaymentRequest
	if job.PaymentRequestID != uuid.Nil {
		pr, err = s.prRepo.GetByID(ctx, job.PaymentRequestID)
		if err != nil {
			s.logger.Warn("failed to get payment request for job",
				zap.String("job_id", jobID.String()),
				zap.Error(err),
			)
		}
	}

	return job, pr, nil
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
	} else if job.IsReturn {
		// For return jobs, update status to return_in_progress
		if err := s.prRepo.UpdateStatus(ctx, job.PaymentRequestID, domain.PRStatusReturnInProgress); err != nil {
			s.logger.Warn("failed to update payment request status to return_in_progress", zap.Error(err))
		}
	} else {
		if err := s.prRepo.UpdateStatus(ctx, job.PaymentRequestID, domain.PRStatusRiderAssigned); err != nil {
			s.logger.Warn("failed to update payment request status to rider_assigned", zap.Error(err))
		}
	}

	s.logger.Info("rider accepted job",
		zap.String("user_id", userID.String()),
		zap.String("job_id", jobID.String()),
		zap.Bool("is_return", job.IsReturn),
	)

	// Notify buyer and seller that rider is assigned
	if s.notificationService != nil {
		pr, _ := s.prRepo.GetByID(ctx, job.PaymentRequestID)
		if pr != nil {
			data := map[string]string{"job_id": jobID.String(), "payment_request_id": pr.ID.String()}

			if job.IsReturn {
				// For return jobs: roles are reversed
				// Buyer is at pickup (gets pickup code), Seller is at dropoff (gets dropoff code)
				if pr.BuyerID != nil {
					data["pickup_code"] = pickupCode
					go s.notificationService.SendToUser(context.Background(), *pr.BuyerID, "Return Rider Assigned",
						fmt.Sprintf("A rider is on the way to pick up the item for return. Your pickup code: %s", pickupCode),
						data, domain.NotificationRiderAssigned)
				}
				data["dropoff_code"] = dropoffCode
				go s.notificationService.SendToUser(context.Background(), pr.SellerID, "Return Rider Assigned",
					fmt.Sprintf("A rider is picking up the item from the buyer for return. Your dropoff code: %s", dropoffCode),
					data, domain.NotificationRiderAssigned)
			} else {
				// Normal delivery: Seller is at pickup, Buyer is at dropoff
				if pr.BuyerID != nil {
					go s.notificationService.SendToUser(context.Background(), *pr.BuyerID, "Rider Assigned", "A rider is on the way to pick up your order", data, domain.NotificationRiderAssigned)
				}
				go s.notificationService.SendToUser(context.Background(), pr.SellerID, "Rider Assigned", "A rider is on the way to collect the item", data, domain.NotificationRiderAssigned)
			}
		}
	}

	return job, nil
}
func (s *RiderServiceImpl) GetActiveJob(ctx context.Context, userID uuid.UUID) (*ActiveJobDetails, error) {
	profile, err := s.riderRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	job, err := s.jobRepo.GetActiveByRiderID(ctx, profile.ID)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, nil
	}

	details := &ActiveJobDetails{
		Job: job,
	}

	// Get payment request for package details and user IDs
	if job.PaymentRequestID != uuid.Nil {
		pr, err := s.prRepo.GetByID(ctx, job.PaymentRequestID)
		if err != nil {
			s.logger.Warn("failed to get payment request for active job",
				zap.String("job_id", job.ID.String()),
				zap.Error(err),
			)
		} else {
			details.PaymentRequest = pr

			// Get seller info
			seller, err := s.userRepo.GetByID(ctx, pr.SellerID)
			if err == nil && seller != nil {
				details.Seller = &ContactInfo{
					Name:  seller.FullName(),
					Phone: seller.PhoneNumber,
				}
			}

			// Get buyer info
			if pr.BuyerID != nil {
				buyer, err := s.userRepo.GetByID(ctx, *pr.BuyerID)
				if err == nil && buyer != nil {
					details.Buyer = &ContactInfo{
						Name:  buyer.FullName(),
						Phone: buyer.PhoneNumber,
					}
				}
			}
		}
	}

	return details, nil
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
	} else if job.IsReturn {
		// For return jobs, PR status stays as return_in_progress
		// No escrow update needed as we're returning item
		s.logger.Info("return pickup confirmed, PR status unchanged",
			zap.String("job_id", jobID.String()),
		)
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
		zap.Bool("is_return", job.IsReturn),
	)

	// Notify appropriate party based on job type
	if s.notificationService != nil {
		pr, _ := s.prRepo.GetByID(ctx, job.PaymentRequestID)
		if pr != nil {
			data := map[string]string{"job_id": jobID.String()}

			if job.IsReturn {
				// For return: Buyer was at pickup, now notify seller item is on the way back
				go s.notificationService.SendToUser(context.Background(), pr.SellerID, "Return Picked Up",
					"The item has been picked up from the buyer and is on the way back to you.",
					data, domain.NotificationOrderPickedUp)
				if pr.BuyerID != nil {
					go s.notificationService.SendToUser(context.Background(), *pr.BuyerID, "Return Picked Up",
						"The item has been picked up for return. It's on the way to the seller.",
						data, domain.NotificationOrderPickedUp)
				}
			} else if pr.BuyerID != nil {
				// Normal delivery: notify buyer
				go s.notificationService.SendToUser(context.Background(), *pr.BuyerID, "Order Picked Up", "Your order is on the way!", data, domain.NotificationOrderPickedUp)
			}
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

	// Check if this is a return job - handle differently
	if job.IsReturn {
		return s.confirmReturnDelivery(ctx, profile, job, now)
	}

	// Normal delivery flow
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
				go s.notificationService.SendToUser(context.Background(), *pr.BuyerID, "Order Delivered", "Your order has been delivered. Please confirm receipt.", data, domain.NotificationOrderDelivered)
			}
			go s.notificationService.SendToUser(context.Background(), pr.SellerID, "Order Delivered", "Your item has been delivered to the buyer", data, domain.NotificationOrderDelivered)
		}
	}

	return nil
}

// confirmReturnDelivery handles delivery confirmation for return jobs
func (s *RiderServiceImpl) confirmReturnDelivery(ctx context.Context, profile *domain.RiderProfile, job *domain.DeliveryJob, now time.Time) error {
	// For return jobs, set status to return_delivered (waiting for seller confirmation)
	job.Status = domain.DJStatusReturnDelivered
	job.DeliveredAt = &now

	if err := s.jobRepo.Update(ctx, job); err != nil {
		return fmt.Errorf("failed to update return job: %w", err)
	}

	s.broadcastJobStatus(job.ID, domain.DJStatusReturnDelivered)

	// Increment rider deliveries
	if err := s.riderRepo.IncrementDeliveries(ctx, profile.ID); err != nil {
		s.logger.Error("failed to increment deliveries for return job", zap.Error(err))
	}

	// Handle rider earnings for return (if any fee)
	if job.DeliveryFee > 0 {
		riderTake := domain.CalculateRiderTake(job.DeliveryFee)
		if err := s.riderRepo.AddEarnings(ctx, profile.ID, riderTake); err != nil {
			s.logger.Error("failed to update rider earnings for return job", zap.Error(err))
		}
	}

	// Update payment request status
	if err := s.prRepo.UpdateStatus(ctx, job.PaymentRequestID, domain.PRStatusReturnDelivered); err != nil {
		s.logger.Error("failed to update payment request status to return_delivered", zap.Error(err))
	}

	s.logger.Info("rider confirmed return delivery",
		zap.String("job_id", job.ID.String()),
		zap.Bool("is_return", job.IsReturn),
	)

	// Notify seller to confirm return receipt
	if s.notificationService != nil {
		pr, _ := s.prRepo.GetByID(ctx, job.PaymentRequestID)
		if pr != nil {
			data := map[string]string{"job_id": job.ID.String(), "payment_request_id": pr.ID.String()}
			go s.notificationService.SendToUser(context.Background(), pr.SellerID, "Return Delivered",
				"The returned item has arrived. Please confirm the condition to complete the return.",
				data, domain.NotificationItemReturned)
			if pr.BuyerID != nil {
				go s.notificationService.SendToUser(context.Background(), *pr.BuyerID, "Return Delivered",
					"Your item has been returned to the seller. Awaiting seller confirmation for refund.",
					data, domain.NotificationItemReturned)
			}
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
		go s.notificationService.SendToUser(context.Background(), *pr.BuyerID, "Order Cancelled",
			"The seller was unavailable for pickup. Your payment has been refunded.",
			data, domain.NotificationOrderCancelled)
	}

	return nil
}

// MarkBuyerUnavailable handles when buyer is not available at dropoff
// Flow: set status to buyer_unavailable, start 45-min timer
// Rider keeps item. Admin/buyer can rebook within 45 min, otherwise forced return initiated
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

	// Set 45-min deadline for rebook (rider keeps item, no return job yet)
	rebookDeadline := now.Add(45 * time.Minute)
	pr.RebookDeadline = &rebookDeadline
	pr.Status = domain.PRStatusPendingRebook

	if err := s.prRepo.Update(ctx, pr); err != nil {
		return nil, fmt.Errorf("failed to update payment request: %w", err)
	}

	// Notify buyer, seller, and admin
	if s.notificationService != nil {
		data := map[string]string{
			"job_id":             jobID.String(),
			"payment_request_id": pr.ID.String(),
			"rebook_deadline":    rebookDeadline.Format(time.RFC3339),
		}
		if pr.BuyerID != nil {
			go s.notificationService.SendToUser(context.Background(), *pr.BuyerID, "Delivery Attempt Failed",
				"You were unavailable for delivery. Please respond within 45 minutes or item will be returned to seller.",
				data, domain.NotificationDeliveryFailed)
		}
		go s.notificationService.SendToUser(context.Background(), pr.SellerID, "Buyer Unavailable",
			"The buyer was unavailable. We're trying to reach them. If unsuccessful, item will be returned to you.",
			data, domain.NotificationBuyerUnavailable)
	}

	s.logger.Info("buyer marked unavailable, 45-min rebook window started",
		zap.String("job_id", jobID.String()),
		zap.String("pr_id", pr.ID.String()),
		zap.Time("rebook_deadline", rebookDeadline),
	)

	return &ForcedReturnInfo{
		RebookDeadline: rebookDeadline.Format("2006-01-02T15:04:05Z07:00"),
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
			go s.notificationService.SendToUser(context.Background(), pr.SellerID, "Item Returned",
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
