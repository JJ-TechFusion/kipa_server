package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/config"
	"kipa/internal/domain"
	"kipa/internal/repository"
)


type PaymentRequestService interface {

	CreateDraft(ctx context.Context, sellerID uuid.UUID, req *CreateDraftRequest) (*domain.PaymentRequest, error)

	ConfigureFulfillment(ctx context.Context, prID uuid.UUID, sellerID uuid.UUID, req *ConfigureFulfillmentRequest) (*domain.PaymentRequest, error)

	
	GetByID(ctx context.Context, id uuid.UUID) (*domain.PaymentRequest, error)

	
	GetByPaymentCode(ctx context.Context, code string) (*domain.PaymentRequest, error)

	GetBySellerID(ctx context.Context, sellerID uuid.UUID, limit, offset int) ([]*domain.PaymentRequest, error)

	GetByBuyerID(ctx context.Context, buyerID uuid.UUID, limit, offset int) ([]*domain.PaymentRequest, error)

	GetHistoryBySellerID(ctx context.Context, sellerID uuid.UUID, limit, offset int) ([]*domain.PaymentRequest, error)

	Edit(ctx context.Context, prID uuid.UUID, sellerID uuid.UUID, req *EditPaymentRequestRequest) (*domain.PaymentRequest, error)

	MarkReadyForPickup(ctx context.Context, prID uuid.UUID, sellerID uuid.UUID) (*domain.PaymentRequest, error)

	UpdateStatus(ctx context.Context, prID uuid.UUID, status domain.PaymentRequestStatus) error

	
	Cancel(ctx context.Context, prID uuid.UUID, sellerID uuid.UUID) error

	SetBuyer(ctx context.Context, prID uuid.UUID, buyerID uuid.UUID, escrowID uuid.UUID) error

	MarkDelivered(ctx context.Context, prID uuid.UUID) error

	MarkCompleted(ctx context.Context, prID uuid.UUID) error

	// Rebook delivery after buyer unavailability
	RebookDelivery(ctx context.Context, prID uuid.UUID, buyerID uuid.UUID, req *RebookDeliveryRequest) (*domain.DeliveryJob, error)
	GetRebookStatus(ctx context.Context, prID uuid.UUID, buyerID uuid.UUID) (*RebookStatusResponse, error)
}

type RebookDeliveryRequest struct {
	DropoffAddress string   `json:"dropoff_address" binding:"required"`
	DropoffLat     float64  `json:"dropoff_lat" binding:"required"`
	DropoffLng     float64  `json:"dropoff_lng" binding:"required"`
}

type RebookStatusResponse struct {
	Status          string     `json:"status"`
	RebookDeadline  *time.Time `json:"rebook_deadline,omitempty"`
	RebookAttempts  int        `json:"rebook_attempts"`
	CanRebook       bool       `json:"can_rebook"`
	TimeRemaining   string     `json:"time_remaining,omitempty"`
}

type CreateDraftRequest struct {
	ItemName            string   `json:"item_name" binding:"required"`
	ItemDescription     string   `json:"item_description"`
	ItemPrice           float64  `json:"item_price" binding:"required,gt=0"`
	ItemImages          []string `json:"item_images"`
	ProcessingTimeHours *int     `json:"processing_time_hours"`
	IsReusable          bool     `json:"is_reusable"` // If true, link can be used by multiple buyers
	MaxUses             *int     `json:"max_uses"`    // Max number of times link can be used (null = unlimited if reusable)
}

type ConfigureFulfillmentRequest struct {
	DeliveryType   domain.DeliveryType `json:"delivery_type" binding:"required"`
	PickupAddress  string              `json:"pickup_address"`
	PickupState    string              `json:"pickup_state"`  // Required for inter_state delivery
	PickupLat      *float64            `json:"pickup_lat"`
	PickupLng      *float64            `json:"pickup_lng"`
	DropoffAddress string              `json:"dropoff_address"`
	DropoffState   string              `json:"dropoff_state"` // Required for inter_state delivery
	DropoffLat     *float64            `json:"dropoff_lat"`
	DropoffLng     *float64            `json:"dropoff_lng"`
	VehicleType    domain.VehicleType  `json:"vehicle_type"` // Optional: motorcycle (default) or car
}

type EditPaymentRequestRequest struct {
	ItemName            *string  `json:"item_name"`
	ItemDescription     *string  `json:"item_description"`
	ItemPrice           *float64 `json:"item_price" binding:"omitempty,gt=0"`
	ItemImages          []string `json:"item_images"`
	ProcessingTimeHours *int     `json:"processing_time_hours"`
}

type PaymentRequestServiceImpl struct {
	prRepo          repository.PaymentRequestRepository
	eventRepo       repository.PaymentRequestEventRepository
	userRepo        repository.UserRepository
	deliveryJobRepo repository.DeliveryJobRepository
	feeCalculator   FeeCalculatorService
	config          *config.Config
	logger          *zap.Logger
}

func NewPaymentRequestService(
	prRepo repository.PaymentRequestRepository,
	eventRepo repository.PaymentRequestEventRepository,
	userRepo repository.UserRepository,
	deliveryJobRepo repository.DeliveryJobRepository,
	feeCalculator FeeCalculatorService,
	cfg *config.Config,
	logger *zap.Logger,
) PaymentRequestService {
	return &PaymentRequestServiceImpl{
		prRepo:          prRepo,
		eventRepo:       eventRepo,
		userRepo:        userRepo,
		deliveryJobRepo: deliveryJobRepo,
		feeCalculator:   feeCalculator,
		config:          cfg,
		logger:          logger,
	}
}
func (s *PaymentRequestServiceImpl) CreateDraft(ctx context.Context, sellerID uuid.UUID, req *CreateDraftRequest) (*domain.PaymentRequest, error) {
	var itemImagesJSON []byte
	if len(req.ItemImages) > 0 {
		var err error
		itemImagesJSON, err = json.Marshal(req.ItemImages)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal item images: %w", err)
		}
	}
	var itemDescription *string
	if req.ItemDescription != "" {
		itemDescription = &req.ItemDescription
	}

	pr := &domain.PaymentRequest{
		SellerID:            sellerID,
		ItemName:            req.ItemName,
		ItemDescription:     itemDescription,
		ItemPrice:           req.ItemPrice,
		ItemImages:          itemImagesJSON,
		ProcessingTimeHours: req.ProcessingTimeHours,
		DeliveryType:        domain.DeliveryTypeIntraState, // Default, will be set in fulfillment
		Status:              domain.PRStatusDraft,
		IsReusable:          req.IsReusable,
		MaxUses:             req.MaxUses,
		CurrentUses:         0,
	}

	if err := s.prRepo.Create(ctx, pr); err != nil {
		return nil, fmt.Errorf("failed to create payment request: %w", err)
	}
	if err := s.userRepo.SetIsSeller(ctx, sellerID); err != nil {
		s.logger.Warn("failed to set user as seller", zap.Error(err))
	}
	s.recordEvent(ctx, pr.ID, nil, domain.PRStatusDraft, "draft_created", nil)

	s.logger.Info("payment request draft created",
		zap.String("pr_id", pr.ID.String()),
		zap.String("seller_id", sellerID.String()),
	)

	return pr, nil
}
func (s *PaymentRequestServiceImpl) ConfigureFulfillment(ctx context.Context, prID uuid.UUID, sellerID uuid.UUID, req *ConfigureFulfillmentRequest) (*domain.PaymentRequest, error) {
	s.logger.Info("starting fulfillment configuration", 
		zap.String("pr_id", prID.String()),
		zap.String("seller_id", sellerID.String()),
		zap.String("delivery_type", string(req.DeliveryType)),
	)

	pr, err := s.prRepo.GetByID(ctx, prID)
	if err != nil {
		s.logger.Error("failed to get payment request", zap.Error(err), zap.String("pr_id", prID.String()))
		return nil, err
	}
	
	s.logger.Info("payment request retrieved", zap.String("current_status", string(pr.Status)))
	
	if pr.SellerID != sellerID {
		s.logger.Error("unauthorized seller access", 
			zap.String("pr_seller_id", pr.SellerID.String()),
			zap.String("request_seller_id", sellerID.String()),
		)
		return nil, domain.ErrUnauthorized
	}
	if pr.Status != domain.PRStatusDraft {
		s.logger.Error("invalid status transition", 
			zap.String("current_status", string(pr.Status)),
			zap.String("expected_status", string(domain.PRStatusDraft)),
		)
		return nil, domain.ErrInvalidStatusTransition
	}
	pr.DeliveryType = req.DeliveryType

	if req.PickupAddress != "" {
		pr.PickupAddress = &req.PickupAddress
	}
	if req.PickupState != "" {
		pr.PickupState = &req.PickupState
	}
	if req.DropoffAddress != "" {
		pr.DropoffAddress = &req.DropoffAddress
	}
	if req.DropoffState != "" {
		pr.DropoffState = &req.DropoffState
	}
	if req.PickupLat != nil && req.PickupLng != nil {
		pr.PickupLat = *req.PickupLat
		pr.PickupLng = *req.PickupLng
	}
	if req.DropoffLat != nil && req.DropoffLng != nil {
		pr.DropoffLat = *req.DropoffLat
		pr.DropoffLng = *req.DropoffLng
	}

	// Validate state fields for inter-state delivery
	if req.DeliveryType == domain.DeliveryTypeInterState {
		if req.PickupState == "" || req.DropoffState == "" {
			return nil, fmt.Errorf("pickup_state and dropoff_state are required for inter-state delivery")
		}
	}
	
	s.logger.Info("calculating delivery fees", 
		zap.String("delivery_type", string(req.DeliveryType)),
		zap.Bool("has_pickup_coords", req.PickupLat != nil && req.PickupLng != nil),
		zap.Bool("has_dropoff_coords", req.DropoffLat != nil && req.DropoffLng != nil),
	)

	if req.DeliveryType == domain.DeliveryTypeIntraState && req.PickupLat != nil && req.DropoffLat != nil {
		vehicleType := req.VehicleType
		if vehicleType == "" {
			vehicleType = domain.VehicleTypeMotorcycle
		}
		pr.VehicleType = &vehicleType

		s.logger.Info("calculating intra-state delivery fee", 
			zap.String("vehicle_type", string(vehicleType)),
			zap.Float64("pickup_lat", *req.PickupLat),
			zap.Float64("pickup_lng", *req.PickupLng),
			zap.Float64("dropoff_lat", *req.DropoffLat),
			zap.Float64("dropoff_lng", *req.DropoffLng),
		)

		estimate, err := s.feeCalculator.CalculateDeliveryFeeWithVehicle(ctx, *req.PickupLat, *req.PickupLng, *req.DropoffLat, *req.DropoffLng, vehicleType)
		if err != nil {
			s.logger.Warn("failed to calculate delivery fee, using default",
				zap.Error(err),
			)
			defaultFee := s.feeCalculator.ApplyVehicleTypeMultiplier(2500.0, vehicleType)
			pr.EstimatedDeliveryFee = &defaultFee
		} else {
			s.logger.Info("delivery fee calculated successfully", zap.Float64("total_fee", estimate.TotalFee))
			pr.EstimatedDeliveryFee = &estimate.TotalFee
		}
	} else if req.DeliveryType == domain.DeliveryTypeInterState {
		s.logger.Info("using default inter-state fee")
		defaultInterStateFee := 5000.0
		pr.EstimatedDeliveryFee = &defaultInterStateFee
	} else {
		s.logger.Info("no delivery fee required")
		zero := 0.0
		pr.EstimatedDeliveryFee = &zero
	}
	s.logger.Info("calculating platform fees", zap.Float64("item_price", pr.ItemPrice))
	
	buyerFee := s.feeCalculator.CalculateBuyerFee(pr.ItemPrice)
	sellerFee := s.feeCalculator.CalculateSellerFee(pr.ItemPrice)
	pr.BuyerServiceFee = &buyerFee
	pr.SellerPlatformFee = &sellerFee
	
	s.logger.Info("fees calculated", 
		zap.Float64("buyer_fee", buyerFee),
		zap.Float64("seller_fee", sellerFee),
	)
	
	deliveryFee := 0.0
	if pr.EstimatedDeliveryFee != nil {
		deliveryFee = *pr.EstimatedDeliveryFee
	}
	estimatedTotal := s.feeCalculator.CalculateTotalForBuyer(pr.ItemPrice, deliveryFee)
	pr.EstimatedTotal = &estimatedTotal
	
	s.logger.Info("generating payment code and updating status")
	
	paymentCode := s.generatePaymentCode()
	pr.PaymentCode = &paymentCode
	linkExpiry := time.Now().Add(7 * 24 * time.Hour)
	pr.LinkExpiresAt = &linkExpiry
	oldStatus := pr.Status
	pr.Status = domain.PRStatusLinkActive
	now := time.Now()
	pr.LinkGeneratedAt = &now

	s.logger.Info("updating payment request in database", zap.String("pr_id", pr.ID.String()))
	
	if err := s.prRepo.Update(ctx, pr); err != nil {
		s.logger.Error("failed to update payment request in database", zap.Error(err), zap.String("pr_id", pr.ID.String()))
		return nil, fmt.Errorf("failed to update payment request: %w", err)
	}
	s.recordEvent(ctx, pr.ID, &oldStatus, domain.PRStatusLinkActive, "link_generated", nil)

	s.logger.Info("payment link generated",
		zap.String("pr_id", pr.ID.String()),
		zap.String("payment_code", paymentCode),
	)

	return pr, nil
}
func (s *PaymentRequestServiceImpl) GetByID(ctx context.Context, id uuid.UUID) (*domain.PaymentRequest, error) {
	return s.prRepo.GetByID(ctx, id)
}
func (s *PaymentRequestServiceImpl) GetByPaymentCode(ctx context.Context, code string) (*domain.PaymentRequest, error) {
	return s.prRepo.GetByPaymentCode(ctx, code)
}
func (s *PaymentRequestServiceImpl) GetBySellerID(ctx context.Context, sellerID uuid.UUID, limit, offset int) ([]*domain.PaymentRequest, error) {
	return s.prRepo.GetBySellerID(ctx, sellerID, limit, offset)
}
func (s *PaymentRequestServiceImpl) GetByBuyerID(ctx context.Context, buyerID uuid.UUID, limit, offset int) ([]*domain.PaymentRequest, error) {
	return s.prRepo.GetByBuyerID(ctx, buyerID, limit, offset)
}

func (s *PaymentRequestServiceImpl) GetHistoryBySellerID(ctx context.Context, sellerID uuid.UUID, limit, offset int) ([]*domain.PaymentRequest, error) {
	return s.prRepo.GetHistoryBySellerID(ctx, sellerID, limit, offset)
}

func (s *PaymentRequestServiceImpl) Edit(ctx context.Context, prID uuid.UUID, sellerID uuid.UUID, req *EditPaymentRequestRequest) (*domain.PaymentRequest, error) {
	pr, err := s.prRepo.GetByID(ctx, prID)
	if err != nil {
		return nil, err
	}

	if pr.SellerID != sellerID {
		return nil, domain.ErrUnauthorized
	}

	if pr.Status != domain.PRStatusDraft && pr.Status != domain.PRStatusLinkActive {
		return nil, domain.ErrPaymentRequestNotEditable
	}

	if req.ItemName != nil {
		pr.ItemName = *req.ItemName
	}
	if req.ItemDescription != nil {
		pr.ItemDescription = req.ItemDescription
	}
	if req.ItemPrice != nil {
		pr.ItemPrice = *req.ItemPrice
	}
	if req.ItemImages != nil {
		itemImagesJSON, err := json.Marshal(req.ItemImages)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal item images: %w", err)
		}
		pr.ItemImages = itemImagesJSON
	}
	if req.ProcessingTimeHours != nil {
		pr.ProcessingTimeHours = req.ProcessingTimeHours
	}

	if err := s.prRepo.Update(ctx, pr); err != nil {
		return nil, fmt.Errorf("failed to update payment request: %w", err)
	}

	s.recordEvent(ctx, pr.ID, nil, pr.Status, "edited", nil)

	s.logger.Info("payment request edited",
		zap.String("pr_id", pr.ID.String()),
		zap.String("seller_id", sellerID.String()),
	)

	return pr, nil
}
func (s *PaymentRequestServiceImpl) MarkReadyForPickup(ctx context.Context, prID uuid.UUID, sellerID uuid.UUID) (*domain.PaymentRequest, error) {
	pr, err := s.prRepo.GetByID(ctx, prID)
	if err != nil {
		return nil, err
	}
	if pr.SellerID != sellerID {
		return nil, domain.ErrUnauthorized
	}
	if pr.Status != domain.PRStatusPaidAwaitingFulfillment {
		return nil, domain.ErrInvalidStatusTransition
	}
	oldStatus := pr.Status
	now := time.Now()
	pr.ReadyForPickupAt = &now

	if pr.DeliveryType == domain.DeliveryTypeIntraState {
		if pr.VehicleType == nil {
			return nil, fmt.Errorf("vehicle type not set during fulfillment")
		}
		vehicleType := *pr.VehicleType
		deliveryFee := 0.0
		if pr.EstimatedDeliveryFee != nil {
			deliveryFee = *pr.EstimatedDeliveryFee
		}

		job := &domain.DeliveryJob{
			ID:                  uuid.New(),
			PaymentRequestID:    pr.ID,
			PickupAddress:       stringValue(pr.PickupAddress),
			PickupLat:           pr.PickupLat,
			PickupLng:           pr.PickupLng,
			DropoffAddress:      stringValue(pr.DropoffAddress),
			DropoffLat:          pr.DropoffLat,
			DropoffLng:          pr.DropoffLng,
			DeliveryFee:         deliveryFee,
			RequiredVehicleType: vehicleType,
			SearchRadiusKM:      5, // Default 5km search radius
			SearchStartedAt:     &now,
			Status:              domain.DJStatusSearching,
		}


		if err := s.deliveryJobRepo.Create(ctx, job); err != nil {
			return nil, fmt.Errorf("failed to create delivery job: %w", err)
		}
		if err := s.prRepo.SetDeliveryJobID(ctx, pr.ID, job.ID); err != nil {
			s.logger.Warn("failed to set delivery job ID on payment request", zap.Error(err))
		}
		pr.DeliveryJobID = &job.ID

		s.logger.Info("delivery job created for rider matching",
			zap.String("job_id", job.ID.String()),
			zap.String("pr_id", prID.String()),
		)

		pr.Status = domain.PRStatusSearchingRider
	} else {
		pr.Status = domain.PRStatusReadyForPickup
	}

	if err := s.prRepo.Update(ctx, pr); err != nil {
		return nil, fmt.Errorf("failed to update payment request: %w", err)
	}
	s.recordEvent(ctx, pr.ID, &oldStatus, pr.Status, "marked_ready_for_pickup", nil)

	s.logger.Info("payment request marked ready for pickup",
		zap.String("pr_id", prID.String()),
		zap.String("delivery_type", string(pr.DeliveryType)),
		zap.String("status", string(pr.Status)),
	)

	return pr, nil
}
func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
func (s *PaymentRequestServiceImpl) UpdateStatus(ctx context.Context, prID uuid.UUID, status domain.PaymentRequestStatus) error {
	pr, err := s.prRepo.GetByID(ctx, prID)
	if err != nil {
		return err
	}

	oldStatus := pr.Status
	if err := s.prRepo.UpdateStatus(ctx, prID, status); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	s.recordEvent(ctx, prID, &oldStatus, status, "status_updated", nil)
	return nil
}
func (s *PaymentRequestServiceImpl) Cancel(ctx context.Context, prID uuid.UUID, sellerID uuid.UUID) error {
	pr, err := s.prRepo.GetByID(ctx, prID)
	if err != nil {
		return err
	}
	if pr.SellerID != sellerID {
		return domain.ErrUnauthorized
	}
	if pr.Status != domain.PRStatusDraft && pr.Status != domain.PRStatusLinkActive {
		return domain.ErrCannotCancel
	}

	oldStatus := pr.Status
	if err := s.prRepo.UpdateStatus(ctx, prID, domain.PRStatusCancelled); err != nil {
		return fmt.Errorf("failed to cancel payment request: %w", err)
	}

	s.recordEvent(ctx, prID, &oldStatus, domain.PRStatusCancelled, "cancelled", nil)

	s.logger.Info("payment request cancelled",
		zap.String("pr_id", prID.String()),
	)

	return nil
}
func (s *PaymentRequestServiceImpl) SetBuyer(ctx context.Context, prID uuid.UUID, buyerID uuid.UUID, escrowID uuid.UUID) error {
	pr, err := s.prRepo.GetByID(ctx, prID)
	if err != nil {
		return err
	}

	oldStatus := pr.Status
	pr.BuyerID = &buyerID
	pr.EscrowID = &escrowID
	pr.Status = domain.PRStatusPaidAwaitingFulfillment
	now := time.Now()
	pr.PaidAt = &now

	if err := s.prRepo.Update(ctx, pr); err != nil {
		return fmt.Errorf("failed to update payment request: %w", err)
	}

	s.recordEvent(ctx, prID, &oldStatus, domain.PRStatusPaidAwaitingFulfillment, "payment_completed", nil)

	return nil
}
func (s *PaymentRequestServiceImpl) MarkDelivered(ctx context.Context, prID uuid.UUID) error {
	pr, err := s.prRepo.GetByID(ctx, prID)
	if err != nil {
		return err
	}

	oldStatus := pr.Status
	now := time.Now()
	pr.DeliveredAt = &now
	pr.Status = domain.PRStatusDelivered

	if err := s.prRepo.Update(ctx, pr); err != nil {
		return fmt.Errorf("failed to update payment request: %w", err)
	}

	s.recordEvent(ctx, prID, &oldStatus, domain.PRStatusDelivered, "delivered", nil)

	return nil
}
func (s *PaymentRequestServiceImpl) MarkCompleted(ctx context.Context, prID uuid.UUID) error {
	pr, err := s.prRepo.GetByID(ctx, prID)
	if err != nil {
		return err
	}

	oldStatus := pr.Status
	now := time.Now()
	pr.CompletedAt = &now
	pr.Status = domain.PRStatusCompleted

	if err := s.prRepo.Update(ctx, pr); err != nil {
		return fmt.Errorf("failed to update payment request: %w", err)
	}

	s.recordEvent(ctx, prID, &oldStatus, domain.PRStatusCompleted, "completed", nil)

	return nil
}
func (s *PaymentRequestServiceImpl) generatePaymentCode() string {
	bytes := make([]byte, 4)
	rand.Read(bytes)
	code := strings.ToUpper(hex.EncodeToString(bytes))
	return fmt.Sprintf("KIP-%s", code)
}
func (s *PaymentRequestServiceImpl) recordEvent(ctx context.Context, prID uuid.UUID, fromStatus *domain.PaymentRequestStatus, toStatus domain.PaymentRequestStatus, reason string, metadata map[string]interface{}) {
	var metadataBytes []byte
	if metadata != nil {
		var err error
		metadataBytes, err = json.Marshal(metadata)
		if err != nil {
			s.logger.Error("failed to marshal event metadata", zap.Error(err))
			return 
		}
	} else {
		metadataBytes = []byte("{}")
	}

	event := &domain.PaymentRequestEvent{
		PaymentRequestID: prID,
		FromStatus:       fromStatus,
		ToStatus:         toStatus,
		Reason:           &reason,
		Metadata:         metadataBytes,
	}

	if err := s.eventRepo.Create(ctx, event); err != nil {
		s.logger.Error("failed to record payment request event",
			zap.Error(err),
			zap.String("pr_id", prID.String()),
			zap.String("reason", reason),
		)
	}
}

// RebookDelivery handles buyer rebooking delivery after unavailability
func (s *PaymentRequestServiceImpl) RebookDelivery(ctx context.Context, prID uuid.UUID, buyerID uuid.UUID, req *RebookDeliveryRequest) (*domain.DeliveryJob, error) {
	pr, err := s.prRepo.GetByID(ctx, prID)
	if err != nil {
		return nil, err
	}

	if pr.BuyerID == nil || *pr.BuyerID != buyerID {
		return nil, domain.ErrUnauthorized
	}

	if pr.Status != domain.PRStatusPendingRebook {
		return nil, fmt.Errorf("payment request is not in pending_rebook status")
	}

	if pr.RebookDeadline != nil && time.Now().After(*pr.RebookDeadline) {
		return nil, fmt.Errorf("rebook deadline has passed")
	}

	// Check max rebook attempts (limit to 1 rebook)
	if pr.RebookAttempts >= 1 {
		return nil, fmt.Errorf("maximum rebook attempts reached")
	}

	// Get seller's pickup address from original request
	if pr.PickupAddress == nil {
		return nil, fmt.Errorf("no pickup address found")
	}

	// Get coordinates for seller's location from forced return job
	var pickupLat, pickupLng float64
	if pr.ForcedReturnJobID != nil {
		coords, err := s.deliveryJobRepo.GetJobCoordinates(ctx, *pr.ForcedReturnJobID)
		if err != nil {
			return nil, fmt.Errorf("failed to get coordinates: %w", err)
		}
		// Seller's location is the dropoff of forced return
		pickupLat = coords.DropoffLat
		pickupLng = coords.DropoffLng
	}

	vehicleType := domain.VehicleTypeMotorcycle
	if pr.VehicleType != nil {
		vehicleType = *pr.VehicleType
	}

	// Create new delivery job
	newJob := &domain.DeliveryJob{
		PaymentRequestID:    prID,
		PickupAddress:       *pr.PickupAddress,
		PickupLat:           pickupLat,
		PickupLng:           pickupLng,
		DropoffAddress:      req.DropoffAddress,
		DropoffLat:          req.DropoffLat,
		DropoffLng:          req.DropoffLng,
		RequiredVehicleType: vehicleType,
		Status:              domain.DJStatusSearching,
		SearchRadiusKM:      5,
	}

	// Calculate delivery fee
	if s.feeCalculator != nil {
		estimate, err := s.feeCalculator.CalculateDeliveryFeeWithVehicle(ctx, pickupLat, pickupLng, req.DropoffLat, req.DropoffLng, vehicleType)
		if err == nil {
			newJob.DeliveryFee = estimate.TotalFee
			newJob.DistanceKM = &estimate.DistanceKM
		} else {
			newJob.DeliveryFee = 2500.0 // Default
		}
	}

	now := time.Now()
	newJob.SearchStartedAt = &now
	timeout := now.Add(30 * time.Minute)
	newJob.SearchTimeoutAt = &timeout

	if err := s.deliveryJobRepo.Create(ctx, newJob); err != nil {
		return nil, fmt.Errorf("failed to create delivery job: %w", err)
	}

	// Update payment request
	oldStatus := pr.Status
	pr.DeliveryJobID = &newJob.ID
	pr.Status = domain.PRStatusSearchingRider
	pr.RebookAttempts++
	pr.DropoffAddress = &req.DropoffAddress

	if err := s.prRepo.Update(ctx, pr); err != nil {
		return nil, fmt.Errorf("failed to update payment request: %w", err)
	}

	s.recordEvent(ctx, prID, &oldStatus, domain.PRStatusSearchingRider, "delivery_rebooked", map[string]interface{}{
		"new_job_id":      newJob.ID.String(),
		"rebook_attempt":  pr.RebookAttempts,
		"dropoff_address": req.DropoffAddress,
	})

	s.logger.Info("delivery rebooked",
		zap.String("pr_id", prID.String()),
		zap.String("new_job_id", newJob.ID.String()),
		zap.Int("rebook_attempt", pr.RebookAttempts),
	)

	return newJob, nil
}

// GetRebookStatus returns the rebook status for a payment request
func (s *PaymentRequestServiceImpl) GetRebookStatus(ctx context.Context, prID uuid.UUID, buyerID uuid.UUID) (*RebookStatusResponse, error) {
	pr, err := s.prRepo.GetByID(ctx, prID)
	if err != nil {
		return nil, err
	}

	if pr.BuyerID == nil || *pr.BuyerID != buyerID {
		return nil, domain.ErrUnauthorized
	}

	resp := &RebookStatusResponse{
		Status:         string(pr.Status),
		RebookDeadline: pr.RebookDeadline,
		RebookAttempts: pr.RebookAttempts,
		CanRebook:      false,
	}

	if pr.Status == domain.PRStatusPendingRebook && pr.RebookDeadline != nil {
		if time.Now().Before(*pr.RebookDeadline) && pr.RebookAttempts < 1 {
			resp.CanRebook = true
			remaining := time.Until(*pr.RebookDeadline)
			hours := int(remaining.Hours())
			minutes := int(remaining.Minutes()) % 60
			resp.TimeRemaining = fmt.Sprintf("%dh %dm", hours, minutes)
		}
	}

	return resp, nil
}

var _ PaymentRequestService = (*PaymentRequestServiceImpl)(nil)
