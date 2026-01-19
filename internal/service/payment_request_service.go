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
	PickupLat      *float64            `json:"pickup_lat"`
	PickupLng      *float64            `json:"pickup_lng"`
	DropoffAddress string              `json:"dropoff_address"`
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
	pr, err := s.prRepo.GetByID(ctx, prID)
	if err != nil {
		return nil, err
	}
	if pr.SellerID != sellerID {
		return nil, domain.ErrUnauthorized
	}
	if pr.Status != domain.PRStatusDraft {
		return nil, domain.ErrInvalidStatusTransition
	}
	pr.DeliveryType = req.DeliveryType

	if req.PickupAddress != "" {
		pr.PickupAddress = &req.PickupAddress
	}
	if req.DropoffAddress != "" {
		pr.DropoffAddress = &req.DropoffAddress
	}
	if req.DeliveryType == domain.DeliveryTypeIntraState && req.PickupLat != nil && req.DropoffLat != nil {
		vehicleType := req.VehicleType
		if vehicleType == "" {
			vehicleType = domain.VehicleTypeMotorcycle
		}
		pr.VehicleType = &vehicleType

		estimate, err := s.feeCalculator.CalculateDeliveryFeeWithVehicle(ctx, *req.PickupLat, *req.PickupLng, *req.DropoffLat, *req.DropoffLng, vehicleType)
		if err != nil {
			s.logger.Warn("failed to calculate delivery fee, using default",
				zap.Error(err),
			)
			defaultFee := s.feeCalculator.ApplyVehicleTypeMultiplier(2500.0, vehicleType)
			pr.EstimatedDeliveryFee = &defaultFee
		} else {
			pr.EstimatedDeliveryFee = &estimate.TotalFee
		}
	} else if req.DeliveryType == domain.DeliveryTypeInterState {
		defaultInterStateFee := 5000.0
		pr.EstimatedDeliveryFee = &defaultInterStateFee
	} else {
		zero := 0.0
		pr.EstimatedDeliveryFee = &zero
	}
	buyerFee := s.feeCalculator.CalculateBuyerFee(pr.ItemPrice)
	sellerFee := s.feeCalculator.CalculateSellerFee(pr.ItemPrice)
	pr.BuyerServiceFee = &buyerFee
	pr.SellerPlatformFee = &sellerFee
	deliveryFee := 0.0
	if pr.EstimatedDeliveryFee != nil {
		deliveryFee = *pr.EstimatedDeliveryFee
	}
	estimatedTotal := s.feeCalculator.CalculateTotalForBuyer(pr.ItemPrice, deliveryFee)
	pr.EstimatedTotal = &estimatedTotal
	paymentCode := s.generatePaymentCode()
	pr.PaymentCode = &paymentCode
	linkExpiry := time.Now().Add(7 * 24 * time.Hour)
	pr.LinkExpiresAt = &linkExpiry
	oldStatus := pr.Status
	pr.Status = domain.PRStatusLinkActive
	now := time.Now()
	pr.LinkGeneratedAt = &now

	if err := s.prRepo.Update(ctx, pr); err != nil {
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
			DropoffAddress:      stringValue(pr.DropoffAddress),
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
var _ PaymentRequestService = (*PaymentRequestServiceImpl)(nil)
