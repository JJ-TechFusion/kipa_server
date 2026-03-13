package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/domain"
	"kipa/internal/repository"
)

type ErrandService interface {
	Create(ctx context.Context, userID uuid.UUID, req *CreateErrandRequest) (*domain.Errand, error)
	Confirm(ctx context.Context, userID uuid.UUID, errandID uuid.UUID) (*domain.Errand, error)
	Cancel(ctx context.Context, userID uuid.UUID, errandID uuid.UUID) error
	GetByID(ctx context.Context, userID uuid.UUID, errandID uuid.UUID) (*domain.Errand, error)
	GetWithDeliveryDetails(ctx context.Context, userID uuid.UUID, errandID uuid.UUID) (*ErrandWithDelivery, error)
	List(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.Errand, error)
	GetActive(ctx context.Context, userID uuid.UUID) ([]*domain.Errand, error)

	OnJobAccepted(ctx context.Context, errandID uuid.UUID, riderID uuid.UUID) error
	OnJobPickedUp(ctx context.Context, errandID uuid.UUID) error
	OnJobDelivered(ctx context.Context, errandID uuid.UUID) error

	CompleteErrand(ctx context.Context, userID uuid.UUID, errandID uuid.UUID, code string) (*domain.Errand, error)
	GetByErrandID(ctx context.Context, errandID uuid.UUID) (*domain.Errand, error)
}
type ErrandWithDelivery struct {
	Errand      *domain.Errand      `json:"errand"`
	DeliveryJob *ErrandDeliveryInfo `json:"delivery_job,omitempty"`
}
type ErrandDeliveryInfo struct {
	ID          uuid.UUID        `json:"id"`
	Status      string           `json:"status"`
	PickupCode  *string          `json:"pickup_code,omitempty"`
	DropoffCode *string          `json:"dropoff_code,omitempty"`
	Rider       *ErrandRiderInfo `json:"rider,omitempty"`
}
type ErrandRiderInfo struct {
	ID          uuid.UUID `json:"id"`
	FirstName   string    `json:"first_name"`
	LastName    string    `json:"last_name"`
	PhoneNumber string    `json:"phone_number"`
	PhotoURL    *string   `json:"photo_url,omitempty"`
	VehicleType string    `json:"vehicle_type,omitempty"`
	Rating      *float64  `json:"rating,omitempty"`
}

type CreateErrandRequest struct {
	PickupAddress      string  `json:"pickup_address" binding:"required"`
	PickupLat          float64 `json:"pickup_lat" binding:"required"`
	PickupLng          float64 `json:"pickup_lng" binding:"required"`
	PickupContactName  *string `json:"pickup_contact_name"`
	PickupContactPhone *string `json:"pickup_contact_phone"`
	PickupInstructions *string `json:"pickup_instructions"`

	DropoffAddress      string  `json:"dropoff_address" binding:"required"`
	DropoffLat          float64 `json:"dropoff_lat" binding:"required"`
	DropoffLng          float64 `json:"dropoff_lng" binding:"required"`
	DropoffContactName  *string `json:"dropoff_contact_name"`
	DropoffContactPhone *string `json:"dropoff_contact_phone"`

	PackageDescription *string  `json:"package_description"`
	PackageWeightKg    *float64 `json:"package_weight_kg"`
	PackagePhotoURL    *string  `json:"package_photo_url"`

	VehicleType domain.VehicleType `json:"vehicle_type" binding:"required"`
}

type ErrandServiceImpl struct {
	errandRepo      repository.ErrandRepository
	jobRepo         repository.DeliveryJobRepository
	riderRepo       repository.RiderProfileRepository
	userRepo        repository.UserRepository
	feeCalculator   FeeCalculatorService
	broadcaster     JobStatusBroadcaster
	notificationSvc NotificationService
	logger          *zap.Logger
}

func NewErrandService(
	errandRepo repository.ErrandRepository,
	jobRepo repository.DeliveryJobRepository,
	riderRepo repository.RiderProfileRepository,
	userRepo repository.UserRepository,
	feeCalculator FeeCalculatorService,
	broadcaster JobStatusBroadcaster,
	notificationSvc NotificationService,
	logger *zap.Logger,
) ErrandService {
	return &ErrandServiceImpl{
		errandRepo:      errandRepo,
		jobRepo:         jobRepo,
		riderRepo:       riderRepo,
		userRepo:        userRepo,
		feeCalculator:   feeCalculator,
		broadcaster:     broadcaster,
		notificationSvc: notificationSvc,
		logger:          logger,
	}
}

func (s *ErrandServiceImpl) Create(ctx context.Context, userID uuid.UUID, req *CreateErrandRequest) (*domain.Errand, error) {

	if !req.VehicleType.IsValid() {
		return nil, domain.ErrInvalidVehicleType
	}

	feeEstimate, err := s.feeCalculator.CalculateDeliveryFeeWithVehicle(
		ctx,
		req.PickupLat, req.PickupLng,
		req.DropoffLat, req.DropoffLng,
		req.VehicleType,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate delivery fee: %w", err)
	}

	dropoffCode := generateDropoffCode()

	errand := &domain.Errand{
		ID:                  uuid.New(),
		UserID:              userID,
		PickupAddress:       req.PickupAddress,
		PickupLat:           req.PickupLat,
		PickupLng:           req.PickupLng,
		PickupContactName:   req.PickupContactName,
		PickupContactPhone:  req.PickupContactPhone,
		PickupInstructions:  req.PickupInstructions,
		DropoffAddress:      req.DropoffAddress,
		DropoffLat:          req.DropoffLat,
		DropoffLng:          req.DropoffLng,
		DropoffContactName:  req.DropoffContactName,
		DropoffContactPhone: req.DropoffContactPhone,
		PackageDescription:  req.PackageDescription,
		PackageWeightKg:     req.PackageWeightKg,
		PackagePhotoURL:     req.PackagePhotoURL,
		VehicleType:         req.VehicleType,
		DistanceKM:          &feeEstimate.DistanceKM,
		DeliveryFee:         feeEstimate.TotalFee,
		Status:              domain.ErrandStatusDraft,
		DropoffCode:         &dropoffCode,
	}

	if err := s.errandRepo.Create(ctx, errand); err != nil {
		s.logger.Error("failed to create errand in database",
			zap.Error(err),
			zap.String("user_id", userID.String()),
		)
		return nil, fmt.Errorf("failed to create errand: %w", err)
	}

	s.logger.Info("errand created",
		zap.String("errand_id", errand.ID.String()),
		zap.String("user_id", userID.String()),
		zap.Float64("delivery_fee", errand.DeliveryFee),
	)

	return errand, nil
}
func (s *ErrandServiceImpl) Confirm(ctx context.Context, userID uuid.UUID, errandID uuid.UUID) (*domain.Errand, error) {
	errand, err := s.errandRepo.GetByID(ctx, errandID)
	if err != nil {
		return nil, err
	}

	if errand.UserID != userID {
		return nil, domain.ErrUnauthorized
	}

	if !errand.CanConfirm() {
		return nil, domain.ErrErrandNotConfirmable
	}

	sourceType := domain.DeliverySourceErrand
	job := &domain.DeliveryJob{
		ID:                  uuid.New(),
		SourceType:          &sourceType,
		ErrandID:            &errandID,
		PickupAddress:       errand.PickupAddress,
		PickupLat:           errand.PickupLat,
		PickupLng:           errand.PickupLng,
		DropoffAddress:      errand.DropoffAddress,
		DropoffLat:          errand.DropoffLat,
		DropoffLng:          errand.DropoffLng,
		DistanceKM:          errand.DistanceKM,
		DeliveryFee:         errand.DeliveryFee,
		RequiredVehicleType: errand.VehicleType,
		DropoffCode:         errand.DropoffCode,
		Status:              domain.DJStatusSearching,
		SearchRadiusKM:      5, // Default initial radius
	}

	riderPayout := errand.DeliveryFee
	job.RiderPayout = &riderPayout
	platformFee := domain.CalculateCommission(errand.DeliveryFee)
	job.PlatformDeliveryFee = &platformFee

	now := time.Now()
	job.SearchStartedAt = &now

	if err := s.jobRepo.Create(ctx, job); err != nil {
		return nil, fmt.Errorf("failed to create delivery job: %w", err)
	}

	if s.broadcaster != nil {
		s.broadcaster.BroadcastJobStatus(job.ID.String(), string(domain.DJStatusSearching))
	}

	errand.Status = domain.ErrandStatusSearching
	errand.ConfirmedAt = &now
	errand.DeliveryJobID = &job.ID

	if err := s.errandRepo.Update(ctx, errand); err != nil {
		return nil, fmt.Errorf("failed to update errand: %w", err)
	}

	s.logger.Info("errand confirmed, searching for rider",
		zap.String("errand_id", errandID.String()),
		zap.String("job_id", job.ID.String()),
	)

	return errand, nil
}
func (s *ErrandServiceImpl) Cancel(ctx context.Context, userID uuid.UUID, errandID uuid.UUID) error {
	errand, err := s.errandRepo.GetByID(ctx, errandID)
	if err != nil {
		return err
	}

	if errand.UserID != userID {
		return domain.ErrUnauthorized
	}

	if !errand.CanCancel() {
		return domain.ErrErrandNotCancellable
	}

	now := time.Now()
	errand.Status = domain.ErrandStatusCancelled
	errand.CancelledAt = &now

	if err := s.errandRepo.Update(ctx, errand); err != nil {
		return fmt.Errorf("failed to cancel errand: %w", err)
	}

	if errand.DeliveryJobID != nil {
		if err := s.jobRepo.UpdateStatus(ctx, *errand.DeliveryJobID, domain.DJStatusCancelled); err != nil {
			s.logger.Error("failed to cancel delivery job", zap.Error(err))
		} else if s.broadcaster != nil {
			s.broadcaster.BroadcastJobStatus(errand.DeliveryJobID.String(), string(domain.DJStatusCancelled))
		}
	}
	if s.notificationSvc != nil && errand.RiderID != nil {
		riderProfile, _ := s.riderRepo.GetByID(ctx, *errand.RiderID)
		if riderProfile != nil {
			go s.notificationSvc.SendToUser(
				context.Background(),
				riderProfile.UserID,
				"Errand Cancelled",
				"The customer has cancelled this errand.",
				map[string]string{"errand_id": errandID.String()},
				domain.NotificationOrderCancelled,
			)
		}
	}

	s.logger.Info("errand cancelled",
		zap.String("errand_id", errandID.String()),
	)

	return nil
}

func (s *ErrandServiceImpl) GetByID(ctx context.Context, userID uuid.UUID, errandID uuid.UUID) (*domain.Errand, error) {
	errand, err := s.errandRepo.GetByID(ctx, errandID)
	if err != nil {
		return nil, err
	}
	if errand.UserID != userID {
		return nil, domain.ErrUnauthorized
	}

	return errand, nil
}

func (s *ErrandServiceImpl) GetWithDeliveryDetails(ctx context.Context, userID uuid.UUID, errandID uuid.UUID) (*ErrandWithDelivery, error) {
	errand, err := s.errandRepo.GetByID(ctx, errandID)
	if err != nil {
		return nil, err
	}
	if errand.UserID != userID {
		return nil, domain.ErrUnauthorized
	}

	result := &ErrandWithDelivery{
		Errand: errand,
	}
	if errand.DeliveryJobID != nil {
		job, err := s.jobRepo.GetByID(ctx, *errand.DeliveryJobID)
		if err != nil {
			s.logger.Warn("failed to fetch delivery job for errand",
				zap.String("errand_id", errandID.String()),
				zap.Error(err),
			)
		} else {
			deliveryInfo := &ErrandDeliveryInfo{
				ID:          job.ID,
				Status:      string(job.Status),
				PickupCode:  job.PickupCode,
				DropoffCode: job.DropoffCode,
			}
			if job.RiderID != nil {
				riderProfile, err := s.riderRepo.GetByID(ctx, *job.RiderID)
				if err != nil {
					s.logger.Warn("failed to fetch rider profile for errand",
						zap.String("rider_id", job.RiderID.String()),
						zap.Error(err),
					)
				} else {
					user, err := s.userRepo.GetByID(ctx, riderProfile.UserID)
					if err != nil {
						s.logger.Warn("failed to fetch rider user for errand",
							zap.String("user_id", riderProfile.UserID.String()),
							zap.Error(err),
						)
					} else {
						firstName := ""
						lastName := ""
						if user.FirstName != nil {
							firstName = *user.FirstName
						}
						if user.LastName != nil {
							lastName = *user.LastName
						}
						vehicleType := ""
						if riderProfile.VehicleType != nil {
							vehicleType = *riderProfile.VehicleType
						}
						deliveryInfo.Rider = &ErrandRiderInfo{
							ID:          riderProfile.UserID,
							FirstName:   firstName,
							LastName:    lastName,
							PhoneNumber: user.PhoneNumber,
							PhotoURL:    user.ProfilePhotoURL,
							VehicleType: vehicleType,
							Rating:      &riderProfile.Rating,
						}
					}
				}
			}

			result.DeliveryJob = deliveryInfo
		}
	}

	return result, nil
}

func (s *ErrandServiceImpl) List(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.Errand, error) {
	return s.errandRepo.GetByUserID(ctx, userID, limit, offset)
}
func (s *ErrandServiceImpl) GetActive(ctx context.Context, userID uuid.UUID) ([]*domain.Errand, error) {
	return s.errandRepo.GetActiveByUserID(ctx, userID)
}
func (s *ErrandServiceImpl) OnJobAccepted(ctx context.Context, errandID uuid.UUID, riderID uuid.UUID) error {
	errand, err := s.errandRepo.GetByID(ctx, errandID)
	if err != nil {
		return err
	}

	now := time.Now()
	errand.Status = domain.ErrandStatusAccepted
	errand.RiderID = &riderID
	errand.AcceptedAt = &now

	if err := s.errandRepo.Update(ctx, errand); err != nil {
		return err
	}
	if s.notificationSvc != nil {
		riderProfile, _ := s.riderRepo.GetByID(ctx, riderID)
		riderName := "A rider"
		if riderProfile != nil {
			if user, err := s.userRepo.GetByID(ctx, riderProfile.UserID); err == nil && user.FirstName != nil {
				riderName = *user.FirstName
			}
		}
		go s.notificationSvc.SendToUser(
			context.Background(),
			errand.UserID,
			"Rider Accepted",
			fmt.Sprintf("%s has accepted your errand and is heading to pickup.", riderName),
			map[string]string{"errand_id": errandID.String()},
			domain.NotificationErrandAccepted,
		)
	}

	return nil
}
func (s *ErrandServiceImpl) OnJobPickedUp(ctx context.Context, errandID uuid.UUID) error {
	errand, err := s.errandRepo.GetByID(ctx, errandID)
	if err != nil {
		return err
	}

	now := time.Now()
	errand.Status = domain.ErrandStatusPickedUp
	errand.PickedUpAt = &now

	if err := s.errandRepo.Update(ctx, errand); err != nil {
		return err
	}
	if s.notificationSvc != nil {
		go s.notificationSvc.SendToUser(
			context.Background(),
			errand.UserID,
			"Package Picked Up",
			"Your package has been picked up and is on the way.",
			map[string]string{"errand_id": errandID.String()},
			domain.NotificationErrandPickedUp,
		)
	}

	return nil
}
func (s *ErrandServiceImpl) OnJobDelivered(ctx context.Context, errandID uuid.UUID) error {
	errand, err := s.errandRepo.GetByID(ctx, errandID)
	if err != nil {
		return err
	}

	now := time.Now()
	errand.Status = domain.ErrandStatusCompleted
	errand.DeliveredAt = &now
	errand.CompletedAt = &now

	if err := s.errandRepo.Update(ctx, errand); err != nil {
		return err
	}
	if s.notificationSvc != nil {
		go s.notificationSvc.SendToUser(
			context.Background(),
			errand.UserID,
			"Package Delivered",
			"Your package has been delivered successfully.",
			map[string]string{"errand_id": errandID.String()},
			domain.NotificationErrandCompleted,
		)
	}

	return nil
}
func (s *ErrandServiceImpl) CompleteErrand(ctx context.Context, userID uuid.UUID, errandID uuid.UUID, code string) (*domain.Errand, error) {
	errand, err := s.errandRepo.GetByID(ctx, errandID)
	if err != nil {
		return nil, err
	}

	if errand.UserID != userID {
		return nil, domain.ErrUnauthorized
	}

	if errand.Status == domain.ErrandStatusCompleted {
		return errand, nil
	}

	if !errand.CanComplete() {
		return nil, domain.ErrErrandNotConfirmable
	}

	if errand.DropoffCode == nil || *errand.DropoffCode != code {
		return nil, domain.ErrInvalidDropoffCode
	}

	now := time.Now()
	errand.Status = domain.ErrandStatusCompleted
	errand.CompletedAt = &now

	if err := s.errandRepo.Update(ctx, errand); err != nil {
		return nil, fmt.Errorf("failed to complete errand: %w", err)
	}

	s.logger.Info("errand completed",
		zap.String("errand_id", errandID.String()),
		zap.String("user_id", userID.String()),
	)
	if s.notificationSvc != nil && errand.RiderID != nil {
		riderProfile, _ := s.riderRepo.GetByID(ctx, *errand.RiderID)
		if riderProfile != nil {
			go s.notificationSvc.SendToUser(
				context.Background(),
				riderProfile.UserID,
				"Errand Completed",
				"The recipient has confirmed delivery. Great job!",
				map[string]string{"errand_id": errandID.String()},
				domain.NotificationErrandCompleted,
			)
		}
	}

	return errand, nil
}

func generateDropoffCode() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(900000))
	return fmt.Sprintf("%06d", n.Int64()+100000)
}

func (s *ErrandServiceImpl) GetByErrandID(ctx context.Context, errandID uuid.UUID) (*domain.Errand, error) {
	return s.errandRepo.GetByID(ctx, errandID)
}

var _ ErrandService = (*ErrandServiceImpl)(nil)
