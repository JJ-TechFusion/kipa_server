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

// ErrandService handles errand operations
type ErrandService interface {

	Create(ctx context.Context, userID uuid.UUID, req *CreateErrandRequest) (*domain.Errand, error)
	Confirm(ctx context.Context, userID uuid.UUID, errandID uuid.UUID) (*domain.Errand, error)
	Cancel(ctx context.Context, userID uuid.UUID, errandID uuid.UUID) error
	GetByID(ctx context.Context, userID uuid.UUID, errandID uuid.UUID) (*domain.Errand, error)
	List(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.Errand, error)
	GetActive(ctx context.Context, userID uuid.UUID) ([]*domain.Errand, error)

	// Called by rider service when job is accepted
	OnJobAccepted(ctx context.Context, errandID uuid.UUID, riderID uuid.UUID) error
	OnJobPickedUp(ctx context.Context, errandID uuid.UUID) error
	OnJobDelivered(ctx context.Context, errandID uuid.UUID) error

	// Recipient confirmation
	CompleteErrand(ctx context.Context, userID uuid.UUID, errandID uuid.UUID, code string) (*domain.Errand, error)
}

type CreateErrandRequest struct {
	// Pickup
	PickupAddress      string  `json:"pickup_address" binding:"required"`
	PickupLat          float64 `json:"pickup_lat" binding:"required"`
	PickupLng          float64 `json:"pickup_lng" binding:"required"`
	PickupInstructions *string `json:"pickup_instructions"`

	// Dropoff
	DropoffAddress string  `json:"dropoff_address" binding:"required"`
	DropoffLat     float64 `json:"dropoff_lat" binding:"required"`
	DropoffLng     float64 `json:"dropoff_lng" binding:"required"`

	// Package
	PackageDescription string   `json:"package_description" binding:"required"`
	PackageWeightKg    *float64 `json:"package_weight_kg"`
	PackagePhotoURL    *string  `json:"package_photo_url"`

	// Vehicle
	VehicleType domain.VehicleType `json:"vehicle_type" binding:"required"`
}

type ErrandServiceImpl struct {
	errandRepo    repository.ErrandRepository
	jobRepo       repository.DeliveryJobRepository
	feeCalculator FeeCalculatorService
	logger        *zap.Logger
}

func NewErrandService(
	errandRepo repository.ErrandRepository,
	jobRepo repository.DeliveryJobRepository,
	feeCalculator FeeCalculatorService,
	logger *zap.Logger,
) ErrandService {
	return &ErrandServiceImpl{
		errandRepo:    errandRepo,
		jobRepo:       jobRepo,
		feeCalculator: feeCalculator,
		logger:        logger,
	}
}

// Create creates a new errand request in draft status
func (s *ErrandServiceImpl) Create(ctx context.Context, userID uuid.UUID, req *CreateErrandRequest) (*domain.Errand, error) {
	// Validate vehicle type
	if !req.VehicleType.IsValid() {
		return nil, domain.ErrInvalidVehicleType
	}

	// Calculate delivery fee based on distance and vehicle type
	feeEstimate, err := s.feeCalculator.CalculateDeliveryFeeWithVehicle(
		ctx,
		req.PickupLat, req.PickupLng,
		req.DropoffLat, req.DropoffLng,
		req.VehicleType,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate delivery fee: %w", err)
	}

	// Generate dropoff code
	dropoffCode := generateDropoffCode()

	errand := &domain.Errand{
		ID:                 uuid.New(),
		UserID:             userID,
		PickupAddress:      req.PickupAddress,
		PickupLat:          req.PickupLat,
		PickupLng:          req.PickupLng,
		PickupInstructions: req.PickupInstructions,
		DropoffAddress:     req.DropoffAddress,
		DropoffLat:         req.DropoffLat,
		DropoffLng:         req.DropoffLng,
		PackageDescription: req.PackageDescription,
		PackageWeightKg:    req.PackageWeightKg,
		PackagePhotoURL:    req.PackagePhotoURL,
		VehicleType:        req.VehicleType,
		DistanceKM:         &feeEstimate.DistanceKM,
		DeliveryFee:        feeEstimate.TotalFee,
		Status:             domain.ErrandStatusDraft,
		DropoffCode:        &dropoffCode,
	}

	if err := s.errandRepo.Create(ctx, errand); err != nil {
		return nil, fmt.Errorf("failed to create errand: %w", err)
	}

	s.logger.Info("errand created",
		zap.String("errand_id", errand.ID.String()),
		zap.String("user_id", userID.String()),
		zap.Float64("delivery_fee", errand.DeliveryFee),
	)

	return errand, nil
}

// Confirm confirms the errand and starts rider search
func (s *ErrandServiceImpl) Confirm(ctx context.Context, userID uuid.UUID, errandID uuid.UUID) (*domain.Errand, error) {
	errand, err := s.errandRepo.GetByID(ctx, errandID)
	if err != nil {
		return nil, err
	}

	// Verify ownership
	if errand.UserID != userID {
		return nil, domain.ErrUnauthorized
	}

	// Check if can confirm
	if !errand.CanConfirm() {
		return nil, domain.ErrErrandNotConfirmable
	}

	// Create delivery job
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

	// Set rider payout (full fee since direct payment)
	riderPayout := errand.DeliveryFee
	job.RiderPayout = &riderPayout

	// Platform fee (15%)
	platformFee := domain.CalculateCommission(errand.DeliveryFee)
	job.PlatformDeliveryFee = &platformFee

	now := time.Now()
	job.SearchStartedAt = &now

	if err := s.jobRepo.Create(ctx, job); err != nil {
		return nil, fmt.Errorf("failed to create delivery job: %w", err)
	}

	// Update errand
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

// Cancel cancels an errand
func (s *ErrandServiceImpl) Cancel(ctx context.Context, userID uuid.UUID, errandID uuid.UUID) error {
	errand, err := s.errandRepo.GetByID(ctx, errandID)
	if err != nil {
		return err
	}

	// Verify ownership
	if errand.UserID != userID {
		return domain.ErrUnauthorized
	}

	// Check if can cancel
	if !errand.CanCancel() {
		return domain.ErrErrandNotCancellable
	}

	now := time.Now()
	errand.Status = domain.ErrandStatusCancelled
	errand.CancelledAt = &now

	if err := s.errandRepo.Update(ctx, errand); err != nil {
		return fmt.Errorf("failed to cancel errand: %w", err)
	}

	// Cancel associated delivery job if exists
	if errand.DeliveryJobID != nil {
		if err := s.jobRepo.UpdateStatus(ctx, *errand.DeliveryJobID, domain.DJStatusCancelled); err != nil {
			s.logger.Error("failed to cancel delivery job", zap.Error(err))
		}
	}

	s.logger.Info("errand cancelled",
		zap.String("errand_id", errandID.String()),
	)

	return nil
}

// GetByID gets an errand by ID
func (s *ErrandServiceImpl) GetByID(ctx context.Context, userID uuid.UUID, errandID uuid.UUID) (*domain.Errand, error) {
	errand, err := s.errandRepo.GetByID(ctx, errandID)
	if err != nil {
		return nil, err
	}

	// Verify ownership
	if errand.UserID != userID {
		return nil, domain.ErrUnauthorized
	}

	return errand, nil
}

// List gets user's errands
func (s *ErrandServiceImpl) List(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.Errand, error) {
	return s.errandRepo.GetByUserID(ctx, userID, limit, offset)
}

// GetActive gets user's active errands
func (s *ErrandServiceImpl) GetActive(ctx context.Context, userID uuid.UUID) ([]*domain.Errand, error) {
	return s.errandRepo.GetActiveByUserID(ctx, userID)
}

// OnJobAccepted called when rider accepts the job
func (s *ErrandServiceImpl) OnJobAccepted(ctx context.Context, errandID uuid.UUID, riderID uuid.UUID) error {
	errand, err := s.errandRepo.GetByID(ctx, errandID)
	if err != nil {
		return err
	}

	now := time.Now()
	errand.Status = domain.ErrandStatusAccepted
	errand.RiderID = &riderID
	errand.AcceptedAt = &now

	return s.errandRepo.Update(ctx, errand)
}

// OnJobPickedUp called when rider confirms pickup
func (s *ErrandServiceImpl) OnJobPickedUp(ctx context.Context, errandID uuid.UUID) error {
	errand, err := s.errandRepo.GetByID(ctx, errandID)
	if err != nil {
		return err
	}

	now := time.Now()
	errand.Status = domain.ErrandStatusPickedUp
	errand.PickedUpAt = &now

	return s.errandRepo.Update(ctx, errand)
}

// OnJobDelivered called when rider confirms delivery
func (s *ErrandServiceImpl) OnJobDelivered(ctx context.Context, errandID uuid.UUID) error {
	errand, err := s.errandRepo.GetByID(ctx, errandID)
	if err != nil {
		return err
	}

	now := time.Now()
	errand.Status = domain.ErrandStatusDelivered
	errand.DeliveredAt = &now

	return s.errandRepo.Update(ctx, errand)
}

// CompleteErrand allows recipient to confirm delivery with dropoff code
func (s *ErrandServiceImpl) CompleteErrand(ctx context.Context, userID uuid.UUID, errandID uuid.UUID, code string) (*domain.Errand, error) {
	errand, err := s.errandRepo.GetByID(ctx, errandID)
	if err != nil {
		return nil, err
	}

	// Verify ownership
	if errand.UserID != userID {
		return nil, domain.ErrUnauthorized
	}

	// Check if can complete
	if !errand.CanComplete() {
		return nil, fmt.Errorf("errand cannot be completed in current status: %s", errand.Status)
	}

	// Validate dropoff code
	if errand.DropoffCode == nil || *errand.DropoffCode != code {
		return nil, fmt.Errorf("invalid dropoff code")
	}

	// Update status
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

	return errand, nil
}

// generateDropoffCode generates a 6-digit verification code
func generateDropoffCode() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(900000))
	return fmt.Sprintf("%06d", n.Int64()+100000)
}

// Ensure ErrandServiceImpl implements ErrandService
var _ ErrandService = (*ErrandServiceImpl)(nil)
