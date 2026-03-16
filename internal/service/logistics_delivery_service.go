package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/config"
	"kipa/internal/domain"
	"kipa/internal/dto"
	"kipa/internal/repository"
)

const (
	HighValueThreshold                = 1000000.0 // 1 million NGN
	ConfirmationWindowDays            = 2
	SellerDisputeWindowDays           = 2
	ReturnConfirmationWindowDays      = 2
	MinHoursBeforeClaimingDelivery    = 24
)

type LogisticsDeliveryService interface {
	Create(ctx context.Context, req *CreateLogisticsDeliveryRequest) (*domain.LogisticsDelivery, error)

	GetByID(ctx context.Context, id uuid.UUID) (*domain.LogisticsDelivery, error)
	GetDetails(ctx context.Context, id uuid.UUID) (*domain.LogisticsDeliveryDetails, error)
	GetByPaymentRequestID(ctx context.Context, prID uuid.UUID) (*domain.LogisticsDelivery, error)
	ListForSeller(ctx context.Context, sellerID uuid.UUID, status *domain.LogisticsDeliveryStatus, limit, offset int) (*dto.LogisticsDeliveryListResponse, error)
	ListForBuyer(ctx context.Context, buyerID uuid.UUID, status *domain.LogisticsDeliveryStatus, limit, offset int) (*dto.LogisticsDeliveryListResponse, error)
	ListPendingAdminApproval(ctx context.Context, limit, offset int) (*dto.LogisticsDeliveryListResponse, error)
	MarkShipped(ctx context.Context, id uuid.UUID, sellerID uuid.UUID, req *dto.MarkShippedRequest) error
	UpdateTracking(ctx context.Context, id uuid.UUID, sellerID uuid.UUID, req *dto.UpdateTrackingRequest) error
	ClaimDelivery(ctx context.Context, id uuid.UUID, sellerID uuid.UUID, req *dto.ClaimDeliveryRequest) error
	ConfirmReturnReceived(ctx context.Context, id uuid.UUID, sellerID uuid.UUID, req *dto.ConfirmReturnReceivedRequest) error
	ConfirmDelivery(ctx context.Context, id uuid.UUID, buyerID uuid.UUID, req *dto.ConfirmDeliveryRequest) error
	CancelDelivery(ctx context.Context, id uuid.UUID, buyerID uuid.UUID, req *dto.CancelDeliveryRequest) error
	MarkReturnShipped(ctx context.Context, id uuid.UUID, buyerID uuid.UUID, req *dto.MarkReturnShippedRequest) error
	ClaimReturnDelivery(ctx context.Context, id uuid.UUID, buyerID uuid.UUID, req *dto.ClaimReturnDeliveryRequest) error
	OpenDispute(ctx context.Context, id uuid.UUID, openerID uuid.UUID, openerType string, req *dto.OpenDisputeRequest) error
	ResolveDispute(ctx context.Context, id uuid.UUID, adminID uuid.UUID, req *dto.AdminResolveDisputeRequest) error
	ApproveRelease(ctx context.Context, id uuid.UUID, adminID uuid.UUID, req *dto.AdminApproveReleaseRequest) error
	ResolveDamageClaim(ctx context.Context, id uuid.UUID, adminID uuid.UUID, req *dto.AdminResolveDamageClaimRequest) error
	ApproveRefund(ctx context.Context, id uuid.UUID, adminID uuid.UUID, req *dto.AdminApproveRefundRequest) error
	ListDamageClaims(ctx context.Context, limit, offset int) (*dto.LogisticsDeliveryListResponse, error)
	ListPendingRefundApprovals(ctx context.Context, limit, offset int) (*dto.LogisticsDeliveryListResponse, error)

	GetTimeline(ctx context.Context, id uuid.UUID) (*dto.DeliveryTimelineResponse, error)

	UploadEvidence(ctx context.Context, id uuid.UUID, uploaderID uuid.UUID, uploaderType string, req *dto.UploadEvidenceRequest) error

	ProcessExceededShipDeadlines(ctx context.Context) error
	ProcessConfirmationReminders(ctx context.Context) error
	ProcessAutoEscalations(ctx context.Context) error
	ProcessReturnAutoEscalations(ctx context.Context) error
}

type CreateLogisticsDeliveryRequest struct {
	PaymentRequestID uuid.UUID
	EscrowID         uuid.UUID
	SellerID         uuid.UUID
	BuyerID          uuid.UUID
	Carrier          *domain.LogisticsCarrier
	CarrierOtherName *string
	PickupAddress    string
	PickupState      string
	DropoffAddress   string
	DropoffState     string
	ShipDeadline     time.Time
	EscrowAmount     float64
}

type LogisticsDeliveryServiceImpl struct {
	deliveryRepo        repository.LogisticsDeliveryRepository
	eventRepo           repository.LogisticsDeliveryEventRepository
	evidenceRepo        repository.LogisticsEvidenceRepository
	escrowRepo          repository.EscrowRepository
	disputeRepo         repository.DisputeRepository
	userRepo            repository.UserRepository
	prRepo              repository.PaymentRequestRepository
	walletService       WalletService
	notificationService NotificationService
	supportChatService  SupportChatService
	adminAlertSvc       AdminAlertService
	feeCalculator       FeeCalculatorService
	config              *config.Config
	logger              *zap.Logger
}

func NewLogisticsDeliveryService(
	deliveryRepo repository.LogisticsDeliveryRepository,
	eventRepo repository.LogisticsDeliveryEventRepository,
	evidenceRepo repository.LogisticsEvidenceRepository,
	escrowRepo repository.EscrowRepository,
	disputeRepo repository.DisputeRepository,
	userRepo repository.UserRepository,
	prRepo repository.PaymentRequestRepository,
	walletService WalletService,
	notificationService NotificationService,
	feeCalculator FeeCalculatorService,
	cfg *config.Config,
	logger *zap.Logger,
) LogisticsDeliveryService {
	return &LogisticsDeliveryServiceImpl{
		deliveryRepo:        deliveryRepo,
		eventRepo:           eventRepo,
		evidenceRepo:        evidenceRepo,
		escrowRepo:          escrowRepo,
		disputeRepo:         disputeRepo,
		userRepo:            userRepo,
		prRepo:              prRepo,
		walletService:       walletService,
		notificationService: notificationService,
		feeCalculator:       feeCalculator,
		config:              cfg,
		logger:              logger,
	}
}

func (s *LogisticsDeliveryServiceImpl) SetSupportChatService(svc SupportChatService) {
	s.supportChatService = svc
}

func (s *LogisticsDeliveryServiceImpl) SetAdminAlertService(svc AdminAlertService) {
	s.adminAlertSvc = svc
}

func (s *LogisticsDeliveryServiceImpl) Create(ctx context.Context, req *CreateLogisticsDeliveryRequest) (*domain.LogisticsDelivery, error) {
	delivery := &domain.LogisticsDelivery{
		ID:                    uuid.New(),
		PaymentRequestID:      req.PaymentRequestID,
		EscrowID:              req.EscrowID,
		SellerID:              req.SellerID,
		BuyerID:               req.BuyerID,
		Carrier:               req.Carrier,
		CarrierOtherName:      req.CarrierOtherName,
		PickupAddress:         req.PickupAddress,
		PickupState:           req.PickupState,
		DropoffAddress:        req.DropoffAddress,
		DropoffState:          req.DropoffState,
		Status:                domain.LDStatusAwaitingShipment,
		ShipDeadline:          req.ShipDeadline,
		RequiresAdminApproval: req.EscrowAmount >= HighValueThreshold,
	}

	if err := s.deliveryRepo.Create(ctx, delivery); err != nil {
		return nil, fmt.Errorf("failed to create logistics delivery: %w", err)
	}

	s.recordEvent(ctx, delivery.ID, nil, domain.LDStatusAwaitingShipment, nil, "system", "Logistics delivery created", nil)

	s.logger.Info("logistics delivery created",
		zap.String("delivery_id", delivery.ID.String()),
		zap.String("pr_id", req.PaymentRequestID.String()),
		zap.Bool("requires_admin_approval", delivery.RequiresAdminApproval),
	)

	return delivery, nil
}

func (s *LogisticsDeliveryServiceImpl) GetByID(ctx context.Context, id uuid.UUID) (*domain.LogisticsDelivery, error) {
	return s.deliveryRepo.GetByID(ctx, id)
}

func (s *LogisticsDeliveryServiceImpl) GetDetails(ctx context.Context, id uuid.UUID) (*domain.LogisticsDeliveryDetails, error) {
	delivery, err := s.deliveryRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	pr, _ := s.prRepo.GetByID(ctx, delivery.PaymentRequestID)
	escrow, _ := s.escrowRepo.GetByID(ctx, delivery.EscrowID)
	seller, _ := s.userRepo.GetByID(ctx, delivery.SellerID)
	buyer, _ := s.userRepo.GetByID(ctx, delivery.BuyerID)
	events, _ := s.eventRepo.GetByDeliveryID(ctx, id)
	evidence, _ := s.evidenceRepo.GetByDeliveryID(ctx, id)

	details := &domain.LogisticsDeliveryDetails{
		Delivery:       delivery,
		PaymentRequest: pr,
		Escrow:         escrow,
		Seller:         seller,
		Buyer:          buyer,
		Events:         events,
		Evidence:       evidence,
	}

	if delivery.DisputeID != nil {
		dispute, _ := s.disputeRepo.GetByID(ctx, *delivery.DisputeID)
		details.Dispute = dispute
	}

	return details, nil
}

func (s *LogisticsDeliveryServiceImpl) GetByPaymentRequestID(ctx context.Context, prID uuid.UUID) (*domain.LogisticsDelivery, error) {
	return s.deliveryRepo.GetByPaymentRequestID(ctx, prID)
}

func (s *LogisticsDeliveryServiceImpl) ListForSeller(ctx context.Context, sellerID uuid.UUID, status *domain.LogisticsDeliveryStatus, limit, offset int) (*dto.LogisticsDeliveryListResponse, error) {
	if limit == 0 {
		limit = 20
	}

	deliveries, total, err := s.deliveryRepo.GetBySellerID(ctx, sellerID, status, limit, offset)
	if err != nil {
		return nil, err
	}

	responses := make([]dto.LogisticsDeliveryResponse, len(deliveries))
	for i, d := range deliveries {
		pr, _ := s.prRepo.GetByID(ctx, d.PaymentRequestID)
		escrow, _ := s.escrowRepo.GetByID(ctx, d.EscrowID)
		seller, _ := s.userRepo.GetByID(ctx, d.SellerID)
		buyer, _ := s.userRepo.GetByID(ctx, d.BuyerID)
		responses[i] = dto.ToLogisticsDeliveryResponse(d, pr, escrow, seller, buyer, sellerID)
	}

	return &dto.LogisticsDeliveryListResponse{
		Deliveries: responses,
		Total:      total,
		Limit:      limit,
		Offset:     offset,
	}, nil
}

func (s *LogisticsDeliveryServiceImpl) ListForBuyer(ctx context.Context, buyerID uuid.UUID, status *domain.LogisticsDeliveryStatus, limit, offset int) (*dto.LogisticsDeliveryListResponse, error) {
	if limit == 0 {
		limit = 20
	}

	deliveries, total, err := s.deliveryRepo.GetByBuyerID(ctx, buyerID, status, limit, offset)
	if err != nil {
		return nil, err
	}

	responses := make([]dto.LogisticsDeliveryResponse, len(deliveries))
	for i, d := range deliveries {
		pr, _ := s.prRepo.GetByID(ctx, d.PaymentRequestID)
		escrow, _ := s.escrowRepo.GetByID(ctx, d.EscrowID)
		seller, _ := s.userRepo.GetByID(ctx, d.SellerID)
		buyer, _ := s.userRepo.GetByID(ctx, d.BuyerID)
		responses[i] = dto.ToLogisticsDeliveryResponse(d, pr, escrow, seller, buyer, buyerID)
	}

	return &dto.LogisticsDeliveryListResponse{
		Deliveries: responses,
		Total:      total,
		Limit:      limit,
		Offset:     offset,
	}, nil
}

func (s *LogisticsDeliveryServiceImpl) ListPendingAdminApproval(ctx context.Context, limit, offset int) (*dto.LogisticsDeliveryListResponse, error) {
	if limit == 0 {
		limit = 20
	}

	deliveries, total, err := s.deliveryRepo.GetPendingAdminApproval(ctx, limit, offset)
	if err != nil {
		return nil, err
	}

	responses := make([]dto.LogisticsDeliveryResponse, len(deliveries))
	for i, d := range deliveries {
		pr, _ := s.prRepo.GetByID(ctx, d.PaymentRequestID)
		escrow, _ := s.escrowRepo.GetByID(ctx, d.EscrowID)
		seller, _ := s.userRepo.GetByID(ctx, d.SellerID)
		buyer, _ := s.userRepo.GetByID(ctx, d.BuyerID)
		responses[i] = dto.ToLogisticsDeliveryResponse(d, pr, escrow, seller, buyer, uuid.Nil)
	}

	return &dto.LogisticsDeliveryListResponse{
		Deliveries: responses,
		Total:      total,
		Limit:      limit,
		Offset:     offset,
	}, nil
}

func (s *LogisticsDeliveryServiceImpl) MarkShipped(ctx context.Context, id uuid.UUID, sellerID uuid.UUID, req *dto.MarkShippedRequest) error {
	delivery, err := s.deliveryRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if delivery.SellerID != sellerID {
		return domain.ErrUnauthorized
	}

	if !delivery.CanBeShipped() {
		return domain.ErrCannotShipYet
	}

	carrier := domain.LogisticsCarrier(req.Carrier)
	if !carrier.IsValid() {
		return domain.ErrInvalidCarrier
	}

	if carrier == domain.CarrierOther && (req.CarrierOtherName == nil || *req.CarrierOtherName == "") {
		return domain.ErrInvalidCarrier
	}

	var estimatedDelivery *time.Time
	if req.EstimatedDeliveryAt != nil {
		t, err := time.Parse(time.RFC3339, *req.EstimatedDeliveryAt)
		if err == nil {
			estimatedDelivery = &t
		}
	}

	trackingNumber := req.TrackingNumber
	err = s.deliveryRepo.SetShipped(ctx, id, &carrier, req.CarrierOtherName, &trackingNumber, req.TrackingURL, &req.ShipmentReceiptURL, estimatedDelivery)
	if err != nil {
		return err
	}

	fromStatus := delivery.Status
	s.recordEvent(ctx, id, &fromStatus, domain.LDStatusShipped, &sellerID, "seller", "Item shipped to logistics carrier", map[string]interface{}{
		"carrier":         carrier,
		"tracking_number": trackingNumber,
	})

	s.escrowRepo.UpdateStatus(ctx, delivery.EscrowID, domain.EscrowStatusDeliveryInProgress)
	if err := s.prRepo.UpdateStatus(ctx, delivery.PaymentRequestID, domain.PRStatusInDelivery); err != nil {
		s.logger.Warn("failed to update payment request status", zap.Error(err))
	}

	s.sendNotification(ctx, delivery.BuyerID, "Item Shipped", fmt.Sprintf("Your order has been shipped via %s. Tracking: %s", carrier.DisplayName(), trackingNumber))

	s.logger.Info("logistics delivery marked as shipped",
		zap.String("delivery_id", id.String()),
		zap.String("carrier", string(carrier)),
		zap.String("tracking", trackingNumber),
	)

	return nil
}

func (s *LogisticsDeliveryServiceImpl) UpdateTracking(ctx context.Context, id uuid.UUID, sellerID uuid.UUID, req *dto.UpdateTrackingRequest) error {
	delivery, err := s.deliveryRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if delivery.SellerID != sellerID {
		return domain.ErrUnauthorized
	}

	if delivery.Status != domain.LDStatusShipped && delivery.Status != domain.LDStatusDeliveryClaimed {
		return domain.ErrInvalidStatusTransition
	}

	if req.TrackingNumber != nil {
		delivery.TrackingNumber = req.TrackingNumber
	}
	if req.TrackingURL != nil {
		delivery.TrackingURL = req.TrackingURL
	}
	if req.EstimatedDeliveryAt != nil {
		t, err := time.Parse(time.RFC3339, *req.EstimatedDeliveryAt)
		if err == nil {
			delivery.EstimatedDeliveryAt = &t
		}
	}

	if err := s.deliveryRepo.Update(ctx, delivery); err != nil {
		return err
	}

	s.recordEvent(ctx, id, nil, delivery.Status, &sellerID, "seller", "Tracking info updated", nil)

	return nil
}

func (s *LogisticsDeliveryServiceImpl) ClaimDelivery(ctx context.Context, id uuid.UUID, sellerID uuid.UUID, req *dto.ClaimDeliveryRequest) error {
	delivery, err := s.deliveryRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if delivery.SellerID != sellerID {
		return domain.ErrUnauthorized
	}

	if delivery.Status != domain.LDStatusShipped {
		return domain.ErrCannotClaimDelivery
	}

	confirmationDeadline := time.Now().Add(ConfirmationWindowDays * 24 * time.Hour)
	autoEscalationAt := time.Now().Add(ConfirmationWindowDays * 24 * time.Hour)

	err = s.deliveryRepo.SetDeliveryClaimed(ctx, id, confirmationDeadline, autoEscalationAt)
	if err != nil {
		return err
	}

	if req.DeliveryProofURL != nil {
		s.uploadEvidenceInternal(ctx, id, sellerID, "seller", "delivery_photo", *req.DeliveryProofURL, req.Notes)
	}

	fromStatus := delivery.Status
	s.recordEvent(ctx, id, &fromStatus, domain.LDStatusDeliveryClaimed, &sellerID, "seller", "Seller claims item delivered", nil)
	if err := s.prRepo.UpdateStatus(ctx, delivery.PaymentRequestID, domain.PRStatusConfirmationWindow); err != nil {
		s.logger.Warn("failed to update payment request status", zap.Error(err))
	}

	s.sendNotification(ctx, delivery.BuyerID, "Please Confirm Delivery",
		fmt.Sprintf("The seller claims your order has been delivered. Please confirm receipt within %d days or open a dispute if there are issues.", ConfirmationWindowDays))

	s.logger.Info("delivery claimed by seller",
		zap.String("delivery_id", id.String()),
		zap.Time("confirmation_deadline", confirmationDeadline),
	)

	return nil
}

func (s *LogisticsDeliveryServiceImpl) ConfirmReturnReceived(ctx context.Context, id uuid.UUID, sellerID uuid.UUID, req *dto.ConfirmReturnReceivedRequest) error {
	delivery, err := s.deliveryRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if delivery.SellerID != sellerID {
		return domain.ErrUnauthorized
	}

	if !delivery.CanSellerConfirmReturn() {
		return domain.ErrInvalidStatusTransition
	}

	now := time.Now()
	delivery.ReturnReceivedAt = &now
	delivery.ReturnCondition = &req.Condition

	fromStatus := domain.LDStatusReturnDeliveryClaimed
	var newStatus domain.LogisticsDeliveryStatus

	switch req.Condition {
	case "good":
		escrow, err := s.escrowRepo.GetByID(ctx, delivery.EscrowID)
		if err != nil {
			s.logger.Error("failed to get escrow", zap.Error(err))
			return err
		}
		s.logger.Info("escrow retrieved", 
			zap.String("escrow_id", delivery.EscrowID.String()),
			zap.Float64("total_locked", escrow.TotalLocked))
		if escrow != nil && escrow.TotalLocked >= HighValueThreshold {
			newStatus = domain.LDStatusPendingRefundApproval
			s.sendNotification(ctx, delivery.BuyerID, "Return Received",
				"Seller confirmed receipt in good condition. Your refund is pending admin approval.")
		} else {
			s.logger.Info("low value delivery - processing auto-refund")
			
			newStatus = domain.LDStatusReturnConfirmed
			delivery.Status = newStatus
			if err := s.deliveryRepo.Update(ctx, delivery); err != nil {
				return err
			}
			s.recordEvent(ctx, id, &fromStatus, newStatus, &sellerID, "seller", "Return received in good condition", nil)

			if escrow != nil {
				s.processReturnRefund(ctx, delivery, escrow)
			}
			return nil
		}

	case "damaged", "incomplete":
		if req.DamageReason == nil || *req.DamageReason == "" {
			return fmt.Errorf("damage reason required when condition is damaged or incomplete")
		}
		newStatus = domain.LDStatusReturnDamageDisputed
		delivery.DamageClaimReason = req.DamageReason
		for _, url := range req.DamageEvidenceURLs {
			s.uploadEvidenceInternal(ctx, id, sellerID, "seller", "return_damage_photo", url, req.DamageReason)
		}

		s.sendNotification(ctx, delivery.BuyerID, "Return Condition Disputed",
			fmt.Sprintf("Seller claims the returned item is %s. Admin will review and decide.", req.Condition))
		s.sendNotification(ctx, delivery.SellerID, "Damage Claim Submitted",
			"Your damage claim has been submitted. Admin will review and make a decision.")

		// Send admin alert
		if s.adminAlertSvc != nil {
			go func(d *domain.LogisticsDelivery, reason string) {
				seller, _ := s.userRepo.GetByID(context.Background(), d.SellerID)
				pr, _ := s.prRepo.GetByID(context.Background(), d.PaymentRequestID)

				sellerName := ""
				itemName := "Unknown Item"
				var amount float64
				if seller != nil {
					sellerName = seller.FullName()
				}
				if pr != nil {
					itemName = pr.ItemName
					amount = pr.ItemPrice
				}

				s.adminAlertSvc.AlertReturnDamageClaimed(context.Background(), d.ID, sellerName, itemName, amount, reason)
			}(delivery, *req.DamageReason)
		}

	default:
		return fmt.Errorf("invalid condition: %s", req.Condition)
	}

	delivery.Status = newStatus
	if err := s.deliveryRepo.Update(ctx, delivery); err != nil {
		return err
	}

	notes := fmt.Sprintf("Return received in %s condition", req.Condition)
	if req.Notes != nil {
		notes += ": " + *req.Notes
	}
	s.recordEvent(ctx, id, &fromStatus, newStatus, &sellerID, "seller", notes, map[string]interface{}{
		"condition":     req.Condition,
		"damage_reason": req.DamageReason,
	})

	s.logger.Info("return received by seller",
		zap.String("delivery_id", id.String()),
		zap.String("condition", req.Condition),
		zap.String("new_status", string(newStatus)),
	)

	return nil
}

func (s *LogisticsDeliveryServiceImpl) ConfirmDelivery(ctx context.Context, id uuid.UUID, buyerID uuid.UUID, req *dto.ConfirmDeliveryRequest) error {
	delivery, err := s.deliveryRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if delivery.BuyerID != buyerID {
		return domain.ErrUnauthorized
	}

	if !delivery.CanConfirmDelivery() {
		return domain.ErrCannotConfirmDelivery
	}

	err = s.deliveryRepo.SetConfirmed(ctx, id, delivery.RequiresAdminApproval)
	if err != nil {
		return err
	}

	fromStatus := delivery.Status
	var newStatus domain.LogisticsDeliveryStatus
	if delivery.RequiresAdminApproval {
		newStatus = domain.LDStatusPendingAdminApproval
		s.recordEvent(ctx, id, &fromStatus, newStatus, &buyerID, "buyer", "Buyer confirmed delivery - pending admin approval for high value order", nil)
		s.logger.Info("high value delivery confirmed, pending admin approval",
			zap.String("delivery_id", id.String()),
		)

		// Send admin alert for high-value order pending approval
		if s.adminAlertSvc != nil {
			go func() {
				buyer, _ := s.userRepo.GetByID(context.Background(), delivery.BuyerID)
				seller, _ := s.userRepo.GetByID(context.Background(), delivery.SellerID)
				pr, _ := s.prRepo.GetByID(context.Background(), delivery.PaymentRequestID)
				escrow, _ := s.escrowRepo.GetByID(context.Background(), delivery.EscrowID)

				buyerName := ""
				sellerName := ""
				itemName := "Unknown Item"
				amount := 0.0

				if buyer != nil {
					buyerName = buyer.FullName()
				}
				if seller != nil {
					sellerName = seller.FullName()
				}
				if pr != nil {
					itemName = pr.ItemName
				}
				if escrow != nil {
					amount = escrow.ItemAmount
				}

				s.adminAlertSvc.AlertHighValuePendingApproval(context.Background(), id, buyerName, sellerName, itemName, amount)
			}()
		}
	} else {
		newStatus = domain.LDStatusConfirmed
		s.recordEvent(ctx, id, &fromStatus, newStatus, &buyerID, "buyer", "Buyer confirmed delivery", nil)
		s.releaseEscrow(ctx, delivery, buyerID)
	}

	return nil
}

func (s *LogisticsDeliveryServiceImpl) CancelDelivery(ctx context.Context, id uuid.UUID, buyerID uuid.UUID, req *dto.CancelDeliveryRequest) error {
	delivery, err := s.deliveryRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if delivery.BuyerID != buyerID {
		return domain.ErrUnauthorized
	}

	if !delivery.CanBeCancelled() {
		return domain.ErrCannotCancelAfterShipment
	}
	if !delivery.IsShipDeadlineExceeded() {
		return domain.ErrShipDeadlineExceeded
	}

	now := time.Now()
	delivery.Status = domain.LDStatusCancelled
	delivery.CancelledAt = &now
	if err := s.deliveryRepo.Update(ctx, delivery); err != nil {
		return err
	}
	fromStatus := domain.LDStatusAwaitingShipment
	s.recordEvent(ctx, id, &fromStatus, domain.LDStatusCancelled, &buyerID, "buyer", req.Reason, nil)
	escrow, _ := s.escrowRepo.GetByID(ctx, delivery.EscrowID)
	if escrow != nil {
		idempotencyKey := fmt.Sprintf("logistics_cancel_refund_%s", id.String())
		// Refund only item amount - service fee retained by platform
		s.walletService.Credit(ctx, buyerID, escrow.ItemAmount, domain.WalletTxRefund, "logistics_delivery", id, "Order cancelled - seller did not ship on time", idempotencyKey)
		s.escrowRepo.UpdateStatus(ctx, delivery.EscrowID, domain.EscrowStatusRefunded)
	}
	if err := s.prRepo.UpdateStatus(ctx, delivery.PaymentRequestID, domain.PRStatusCancelled); err != nil {
		s.logger.Warn("failed to update payment request status", zap.Error(err))
	}

	s.sendNotification(ctx, delivery.SellerID, "Order Cancelled", "Your order was cancelled because you missed the shipping deadline.")

	s.logger.Info("logistics delivery cancelled",
		zap.String("delivery_id", id.String()),
		zap.String("reason", req.Reason),
	)

	return nil
}

func (s *LogisticsDeliveryServiceImpl) MarkReturnShipped(ctx context.Context, id uuid.UUID, buyerID uuid.UUID, req *dto.MarkReturnShippedRequest) error {
	delivery, err := s.deliveryRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if delivery.BuyerID != buyerID {
		return domain.ErrUnauthorized
	}

	if delivery.Status != domain.LDStatusReturnRequired {
		return domain.ErrInvalidStatusTransition
	}

	err = s.deliveryRepo.SetReturnShipped(ctx, id, req.ReturnCarrier, req.ReturnTrackingNumber)
	if err != nil {
		return err
	}

	fromStatus := delivery.Status
	s.recordEvent(ctx, id, &fromStatus, domain.LDStatusReturnShipped, &buyerID, "buyer", "Return shipped", map[string]interface{}{
		"carrier":         req.ReturnCarrier,
		"tracking_number": req.ReturnTrackingNumber,
	})

	s.sendNotification(ctx, delivery.SellerID, "Return Shipped", fmt.Sprintf("Buyer has shipped the return via %s. Tracking: %s", req.ReturnCarrier, req.ReturnTrackingNumber))

	return nil
}

func (s *LogisticsDeliveryServiceImpl) ClaimReturnDelivery(ctx context.Context, id uuid.UUID, buyerID uuid.UUID, req *dto.ClaimReturnDeliveryRequest) error {
	delivery, err := s.deliveryRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if delivery.BuyerID != buyerID {
		return domain.ErrUnauthorized
	}

	if !delivery.CanClaimReturnDelivery() {
		return domain.ErrCannotClaimReturnDelivery
	}

	now := time.Now()
	confirmationDeadline := now.Add(ReturnConfirmationWindowDays * 24 * time.Hour)
	autoEscalationAt := now.Add(ReturnConfirmationWindowDays * 24 * time.Hour)

	delivery.Status = domain.LDStatusReturnDeliveryClaimed
	delivery.ReturnDeliveryClaimedAt = &now
	delivery.ReturnConfirmationDeadline = &confirmationDeadline
	delivery.ReturnAutoEscalationAt = &autoEscalationAt

	if err := s.deliveryRepo.Update(ctx, delivery); err != nil {
		return err
	}

	if req.DeliveryProofURL != nil {
		s.uploadEvidenceInternal(ctx, id, buyerID, "buyer", "return_delivery_photo", *req.DeliveryProofURL, req.Notes)
	}

	fromStatus := domain.LDStatusReturnShipped
	s.recordEvent(ctx, id, &fromStatus, domain.LDStatusReturnDeliveryClaimed, &buyerID, "buyer", "Buyer claims return was delivered", nil)

	s.sendNotification(ctx, delivery.SellerID, "Please Confirm Return Receipt",
		fmt.Sprintf("The buyer claims the return has been delivered. Please confirm receipt within %d days or open a dispute if there are issues.", ReturnConfirmationWindowDays))

	s.logger.Info("return delivery claimed by buyer",
		zap.String("delivery_id", id.String()),
		zap.Time("confirmation_deadline", confirmationDeadline),
	)

	return nil
}

func (s *LogisticsDeliveryServiceImpl) OpenDispute(ctx context.Context, id uuid.UUID, openerID uuid.UUID, openerType string, req *dto.OpenDisputeRequest) error {
	delivery, err := s.deliveryRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if openerType == "buyer" && delivery.BuyerID != openerID {
		return domain.ErrUnauthorized
	}
	if openerType == "seller" && delivery.SellerID != openerID {
		return domain.ErrUnauthorized
	}
	if openerType == "buyer" && !delivery.CanOpenDispute() {
		return domain.ErrCannotOpenDisputeInState
	}
	if openerType == "seller" && !delivery.CanSellerOpenDispute() {
		return domain.ErrSellerCannotDisputeYet
	}
	if delivery.DisputeID != nil {
		return domain.ErrDisputeAlreadyOpen
	}
	dispute := domain.NewDispute(delivery.EscrowID, openerID, req.Reason, req.EvidenceURLs)
	if err := s.disputeRepo.Create(ctx, dispute); err != nil {
		return fmt.Errorf("failed to create dispute: %w", err)
	}
	err = s.deliveryRepo.SetDispute(ctx, id, dispute.ID)
	if err != nil {
		return err
	}

	// Create support conversation for this dispute
	if s.supportChatService != nil {
		go func() {
			if _, err := s.supportChatService.CreateDisputeConversation(context.Background(), openerID, dispute.ID); err != nil {
				s.logger.Warn("failed to create dispute support conversation",
					zap.Error(err),
					zap.String("dispute_id", dispute.ID.String()),
				)
			}
		}()
	}

	s.escrowRepo.UpdateStatus(ctx, delivery.EscrowID, domain.EscrowStatusDisputed)
	if err := s.prRepo.UpdateStatus(ctx, delivery.PaymentRequestID, domain.PRStatusDisputed); err != nil {
		s.logger.Warn("failed to update payment request status", zap.Error(err))
	}
	fromStatus := delivery.Status
	s.recordEvent(ctx, id, &fromStatus, domain.LDStatusDisputed, &openerID, openerType, req.Reason, nil)

	var notifyID uuid.UUID
	if openerType == "buyer" {
		notifyID = delivery.SellerID
	} else {
		notifyID = delivery.BuyerID
	}
	s.sendNotification(ctx, notifyID, "Dispute Opened", fmt.Sprintf("A dispute has been opened: %s", req.Reason))

	// Send admin alert with tracking details and evidence
	if s.adminAlertSvc != nil {
		go func(d *domain.LogisticsDelivery, disp *domain.Dispute, evidenceURLs []string) {
			opener, _ := s.userRepo.GetByID(context.Background(), disp.OpenedBy)
			pr, _ := s.prRepo.GetByID(context.Background(), d.PaymentRequestID)

			openerName := ""
			itemName := "Unknown Item"
			var amount float64
			if opener != nil {
				openerName = opener.FullName()
			}
			if pr != nil {
				itemName = pr.ItemName
				amount = pr.ItemPrice
			}

			carrier := ""
			if d.Carrier != nil {
				carrier = string(*d.Carrier)
			}
			trackingNumber := ""
			if d.TrackingNumber != nil {
				trackingNumber = *d.TrackingNumber
			}
			trackingURL := ""
			if d.TrackingURL != nil {
				trackingURL = *d.TrackingURL
			}
			shipmentReceiptURL := ""
			if d.ShipmentReceiptURL != nil {
				shipmentReceiptURL = *d.ShipmentReceiptURL
			}

			s.adminAlertSvc.AlertLogisticsDisputeOpened(context.Background(), &LogisticsDisputeDetails{
				DeliveryID:         d.ID,
				DisputeID:          disp.ID,
				OpenerName:         openerName,
				OpenerType:         openerType,
				ItemName:           itemName,
				Amount:             amount,
				Reason:             disp.Reason,
				Carrier:            carrier,
				TrackingNumber:     trackingNumber,
				TrackingURL:        trackingURL,
				ShipmentReceiptURL: shipmentReceiptURL,
				EvidenceURLs:       evidenceURLs,
			})
		}(delivery, dispute, req.EvidenceURLs)
	}

	s.logger.Info("dispute opened",
		zap.String("delivery_id", id.String()),
		zap.String("dispute_id", dispute.ID.String()),
		zap.String("opener_type", openerType),
	)

	return nil
}

func (s *LogisticsDeliveryServiceImpl) ResolveDispute(ctx context.Context, id uuid.UUID, adminID uuid.UUID, req *dto.AdminResolveDisputeRequest) error {
	delivery, err := s.deliveryRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if delivery.Status != domain.LDStatusDisputed {
		return domain.ErrInvalidStatusTransition
	}

	if delivery.DisputeID == nil {
		return fmt.Errorf("no dispute found for this delivery")
	}

	dispute, err := s.disputeRepo.GetByID(ctx, *delivery.DisputeID)
	if err != nil {
		return err
	}

	escrow, err := s.escrowRepo.GetByID(ctx, delivery.EscrowID)
	if err != nil {
		return err
	}

	var outcome domain.DisputeOutcome
	var newStatus domain.LogisticsDeliveryStatus

	switch req.Outcome {
	case "buyer_wins":
		outcome = domain.DisputeOutcomeBuyerWins
		newStatus = domain.LDStatusReturnRequired
		buyerShare := 100.0
		sellerShare := 0.0
		if req.ReturnCostBuyerShare != nil {
			buyerShare = *req.ReturnCostBuyerShare
		}
		if req.ReturnCostSellerShare != nil {
			sellerShare = *req.ReturnCostSellerShare
		}
		err = s.deliveryRepo.SetReturnInfo(ctx, id, buyerShare, sellerShare)
		if err != nil {
			return err
		}
		s.sendNotification(ctx, delivery.BuyerID, "Return Required",
			"You won the dispute. Please ship the item back to the seller to receive your refund.")
		s.sendNotification(ctx, delivery.SellerID, "Dispute Resolved - Return Required",
			"Buyer won the dispute and will be returning the item. Refund will be processed after you confirm return receipt.")

	case "seller_wins":
		outcome = domain.DisputeOutcomeSellerWins
		newStatus = domain.LDStatusReleasedByAdmin
		s.releaseEscrowToSeller(ctx, delivery, escrow, adminID)
		s.closeSupportChatForDispute(ctx, *delivery.DisputeID, adminID)

	case "partial_refund":
		outcome = domain.DisputeOutcomePartialRefund
		newStatus = domain.LDStatusRefunded
		if req.RefundAmount == nil {
			return fmt.Errorf("refund amount required for partial refund")
		}
		s.processPartialRefund(ctx, delivery, escrow, *req.RefundAmount)
		s.closeSupportChatForDispute(ctx, *delivery.DisputeID, adminID)

	case "return_required":
		outcome = domain.DisputeOutcomePending
		newStatus = domain.LDStatusReturnRequired
		if req.ReturnCostBuyerShare == nil || req.ReturnCostSellerShare == nil {
			return fmt.Errorf("return cost shares required")
		}
		err = s.deliveryRepo.SetReturnInfo(ctx, id, *req.ReturnCostBuyerShare, *req.ReturnCostSellerShare)
		if err != nil {
			return err
		}

	default:
		return fmt.Errorf("invalid outcome: %s", req.Outcome)
	}
	outcomeStr := string(outcome)
	delivery.DisputeOutcome = &outcomeStr

	dispute.Resolve(adminID, outcome, req.ResolutionNotes)
	if err := s.disputeRepo.Update(ctx, dispute); err != nil {
		return err
	}

	delivery.Status = newStatus
	now := time.Now()
	if newStatus == domain.LDStatusRefunded || newStatus == domain.LDStatusReleasedByAdmin {
		delivery.ReleasedAt = &now
	}
	if err := s.deliveryRepo.Update(ctx, delivery); err != nil {
		return err
	}
	var prStatus domain.PaymentRequestStatus
	switch newStatus {
	case domain.LDStatusReturnRequired:
		prStatus = domain.PRStatusReturnRequired
	case domain.LDStatusReleasedByAdmin:
		prStatus = domain.PRStatusCompleted
	case domain.LDStatusRefunded:
		prStatus = domain.PRStatusRefunded
	}
	if prStatus != "" {
		if err := s.prRepo.UpdateStatus(ctx, delivery.PaymentRequestID, prStatus); err != nil {
			s.logger.Warn("failed to update payment request status", zap.Error(err))
		}
	}

	fromStatus := domain.LDStatusDisputed
	s.recordEvent(ctx, id, &fromStatus, newStatus, &adminID, "admin", req.ResolutionNotes, map[string]interface{}{
		"outcome": req.Outcome,
	})

	s.sendNotification(ctx, delivery.BuyerID, "Dispute Resolved", req.ResolutionNotes)
	s.sendNotification(ctx, delivery.SellerID, "Dispute Resolved", req.ResolutionNotes)

	s.logger.Info("dispute resolved",
		zap.String("delivery_id", id.String()),
		zap.String("outcome", req.Outcome),
		zap.String("admin_id", adminID.String()),
	)

	return nil
}

func (s *LogisticsDeliveryServiceImpl) ApproveRelease(ctx context.Context, id uuid.UUID, adminID uuid.UUID, req *dto.AdminApproveReleaseRequest) error {
	delivery, err := s.deliveryRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if delivery.Status != domain.LDStatusPendingAdminApproval {
		return domain.ErrInvalidStatusTransition
	}

	if !req.Approved {
		fromStatus := delivery.Status
		delivery.Status = domain.LDStatusDisputed
		if err := s.deliveryRepo.Update(ctx, delivery); err != nil {
			return err
		}
		s.recordEvent(ctx, id, &fromStatus, domain.LDStatusDisputed, &adminID, "admin", "Admin flagged for investigation", nil)
		s.sendNotification(ctx, delivery.BuyerID, "Order Under Review", "Your high-value order is under review.")
		s.sendNotification(ctx, delivery.SellerID, "Order Under Review", "Your high-value order is under review.")
		return nil
	}

	s.releaseEscrow(ctx, delivery, adminID)

	s.logger.Info("high value release approved",
		zap.String("delivery_id", id.String()),
		zap.String("admin_id", adminID.String()),
	)

	return nil
}

func (s *LogisticsDeliveryServiceImpl) ResolveDamageClaim(ctx context.Context, id uuid.UUID, adminID uuid.UUID, req *dto.AdminResolveDamageClaimRequest) error {
	delivery, err := s.deliveryRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if delivery.Status != domain.LDStatusReturnDamageDisputed {
		return domain.ErrInvalidStatusTransition
	}

	escrow, err := s.escrowRepo.GetByID(ctx, delivery.EscrowID)
	if err != nil {
		return err
	}

	fromStatus := delivery.Status
	var newStatus domain.LogisticsDeliveryStatus

	switch req.Decision {
	case "full_refund":
		newStatus = domain.LDStatusRefunded
		s.processReturnRefund(ctx, delivery, escrow)
		s.sendNotification(ctx, delivery.BuyerID, "Refund Processed",
			"Admin reviewed the damage claim and processed your full refund.")
		s.sendNotification(ctx, delivery.SellerID, "Damage Claim Resolved",
			"Admin reviewed and determined a full refund is appropriate.")

	case "partial_refund":
		if req.RefundAmount == nil {
			return fmt.Errorf("refund amount required for partial refund")
		}
		newStatus = domain.LDStatusRefunded
		s.processPartialRefund(ctx, delivery, escrow, *req.RefundAmount)
		s.sendNotification(ctx, delivery.BuyerID, "Partial Refund Processed",
			fmt.Sprintf("Admin processed a partial refund of NGN %.2f due to item damage.", *req.RefundAmount))
		s.sendNotification(ctx, delivery.SellerID, "Damage Claim Resolved",
			fmt.Sprintf("Admin processed a partial refund. You received NGN %.2f for the damage.", escrow.ItemAmount-*req.RefundAmount))

	case "release_to_seller":
		newStatus = domain.LDStatusReleasedByAdmin
		s.releaseEscrowToSeller(ctx, delivery, escrow, adminID)
		s.sendNotification(ctx, delivery.BuyerID, "Damage Claim Resolved",
			"Admin determined the damage was severe. Funds released to seller.")
		s.sendNotification(ctx, delivery.SellerID, "Funds Released",
			"Admin released funds to you due to confirmed return damage.")

	default:
		return fmt.Errorf("invalid decision: %s", req.Decision)
	}

	delivery.Status = newStatus
	now := time.Now()
	delivery.ReleasedAt = &now
	if err := s.deliveryRepo.Update(ctx, delivery); err != nil {
		return err
	}

	s.recordEvent(ctx, id, &fromStatus, newStatus, &adminID, "admin", req.ResolutionNotes, map[string]interface{}{
		"decision":      req.Decision,
		"refund_amount": req.RefundAmount,
	})

	s.logger.Info("damage claim resolved",
		zap.String("delivery_id", id.String()),
		zap.String("decision", req.Decision),
		zap.String("admin_id", adminID.String()),
	)

	return nil
}

func (s *LogisticsDeliveryServiceImpl) ApproveRefund(ctx context.Context, id uuid.UUID, adminID uuid.UUID, req *dto.AdminApproveRefundRequest) error {
	delivery, err := s.deliveryRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if delivery.Status != domain.LDStatusPendingRefundApproval {
		return domain.ErrInvalidStatusTransition
	}

	escrow, err := s.escrowRepo.GetByID(ctx, delivery.EscrowID)
	if err != nil {
		return err
	}
	s.processReturnRefund(ctx, delivery, escrow)

	notes := "Admin approved refund after return confirmation"
	if req.Notes != nil {
		notes = *req.Notes
	}

	fromStatus := delivery.Status
	s.recordEvent(ctx, id, &fromStatus, domain.LDStatusRefunded, &adminID, "admin", notes, nil)

	s.sendNotification(ctx, delivery.BuyerID, "Refund Approved", "Your refund has been processed.")
	s.sendNotification(ctx, delivery.SellerID, "Refund Processed", "The buyer's refund has been processed after return confirmation.")

	s.logger.Info("refund approved",
		zap.String("delivery_id", id.String()),
		zap.String("admin_id", adminID.String()),
	)

	return nil
}

func (s *LogisticsDeliveryServiceImpl) ListDamageClaims(ctx context.Context, limit, offset int) (*dto.LogisticsDeliveryListResponse, error) {
	if limit == 0 {
		limit = 20
	}

	status := domain.LDStatusReturnDamageDisputed
	deliveries, total, err := s.deliveryRepo.GetByStatus(ctx, status, limit, offset)
	if err != nil {
		return nil, err
	}

	responses := make([]dto.LogisticsDeliveryResponse, len(deliveries))
	for i, d := range deliveries {
		pr, _ := s.prRepo.GetByID(ctx, d.PaymentRequestID)
		escrow, _ := s.escrowRepo.GetByID(ctx, d.EscrowID)
		seller, _ := s.userRepo.GetByID(ctx, d.SellerID)
		buyer, _ := s.userRepo.GetByID(ctx, d.BuyerID)
		responses[i] = dto.ToLogisticsDeliveryResponse(d, pr, escrow, seller, buyer, uuid.Nil)
	}

	return &dto.LogisticsDeliveryListResponse{
		Deliveries: responses,
		Total:      total,
		Limit:      limit,
		Offset:     offset,
	}, nil
}

func (s *LogisticsDeliveryServiceImpl) ListPendingRefundApprovals(ctx context.Context, limit, offset int) (*dto.LogisticsDeliveryListResponse, error) {
	if limit == 0 {
		limit = 20
	}

	status := domain.LDStatusPendingRefundApproval
	deliveries, total, err := s.deliveryRepo.GetByStatus(ctx, status, limit, offset)
	if err != nil {
		return nil, err
	}

	responses := make([]dto.LogisticsDeliveryResponse, len(deliveries))
	for i, d := range deliveries {
		pr, _ := s.prRepo.GetByID(ctx, d.PaymentRequestID)
		escrow, _ := s.escrowRepo.GetByID(ctx, d.EscrowID)
		seller, _ := s.userRepo.GetByID(ctx, d.SellerID)
		buyer, _ := s.userRepo.GetByID(ctx, d.BuyerID)
		responses[i] = dto.ToLogisticsDeliveryResponse(d, pr, escrow, seller, buyer, uuid.Nil)
	}

	return &dto.LogisticsDeliveryListResponse{
		Deliveries: responses,
		Total:      total,
		Limit:      limit,
		Offset:     offset,
	}, nil
}

func (s *LogisticsDeliveryServiceImpl) GetTimeline(ctx context.Context, id uuid.UUID) (*dto.DeliveryTimelineResponse, error) {
	delivery, err := s.deliveryRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	events, err := s.eventRepo.GetByDeliveryID(ctx, id)
	if err != nil {
		return nil, err
	}

	return dto.BuildTimeline(delivery, events), nil
}

func (s *LogisticsDeliveryServiceImpl) UploadEvidence(ctx context.Context, id uuid.UUID, uploaderID uuid.UUID, uploaderType string, req *dto.UploadEvidenceRequest) error {
	delivery, err := s.deliveryRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if uploaderType == "buyer" && delivery.BuyerID != uploaderID {
		return domain.ErrUnauthorized
	}
	if uploaderType == "seller" && delivery.SellerID != uploaderID {
		return domain.ErrUnauthorized
	}

	return s.uploadEvidenceInternal(ctx, id, uploaderID, uploaderType, req.EvidenceType, req.FileURL, req.Description)
}

func (s *LogisticsDeliveryServiceImpl) uploadEvidenceInternal(ctx context.Context, deliveryID, uploaderID uuid.UUID, uploaderType, evidenceType, fileURL string, description *string) error {
	evidence := &domain.LogisticsEvidence{
		ID:                  uuid.New(),
		LogisticsDeliveryID: deliveryID,
		UploadedBy:          uploaderID,
		UploaderType:        uploaderType,
		EvidenceType:        evidenceType,
		FileURL:             fileURL,
		Description:         description,
	}

	return s.evidenceRepo.Create(ctx, evidence)
}

func (s *LogisticsDeliveryServiceImpl) ProcessExceededShipDeadlines(ctx context.Context) error {
	deliveries, err := s.deliveryRepo.GetExceededShipDeadlines(ctx)
	if err != nil {
		return err
	}

	for _, d := range deliveries {
		s.sendNotification(ctx, d.BuyerID, "Ship Deadline Exceeded",
			"The seller has not shipped your order. You may now cancel for a full refund.")
		s.sendNotification(ctx, d.SellerID, "Shipping Deadline Missed",
			"You missed the shipping deadline. The buyer can now cancel the order.")

		// Send admin alert
		if s.adminAlertSvc != nil {
			go func(delivery *domain.LogisticsDelivery) {
				seller, _ := s.userRepo.GetByID(context.Background(), delivery.SellerID)
				pr, _ := s.prRepo.GetByID(context.Background(), delivery.PaymentRequestID)

				sellerName := ""
				itemName := "Unknown Item"
				if seller != nil {
					sellerName = seller.FullName()
				}
				if pr != nil {
					itemName = pr.ItemName
				}

				daysOverdue := 1
				if !delivery.ShipDeadline.IsZero() {
					daysOverdue = int(time.Since(delivery.ShipDeadline).Hours() / 24)
					if daysOverdue < 1 {
						daysOverdue = 1
					}
				}

				s.adminAlertSvc.AlertShipDeadlineExceeded(context.Background(), delivery.ID, sellerName, itemName, daysOverdue)
			}(d)
		}
	}

	if len(deliveries) > 0 {
		s.logger.Info("processed exceeded ship deadlines", zap.Int("count", len(deliveries)))
	}

	return nil
}

func (s *LogisticsDeliveryServiceImpl) ProcessConfirmationReminders(ctx context.Context) error {
	deliveries, err := s.deliveryRepo.GetPendingConfirmationReminders(ctx, 24)
	if err != nil {
		return err
	}

	for _, d := range deliveries {
		reminderNum := d.ReminderCount + 1
		daysRemaining := ConfirmationWindowDays - reminderNum

		if daysRemaining > 0 {
			s.sendNotification(ctx, d.BuyerID, "Confirm Delivery",
				fmt.Sprintf("Please confirm delivery of your order. You have %d days remaining.", daysRemaining))
			s.deliveryRepo.UpdateReminderSent(ctx, d.ID)
		}
	}

	if len(deliveries) > 0 {
		s.logger.Info("sent confirmation reminders", zap.Int("count", len(deliveries)))
	}

	return nil
}

func (s *LogisticsDeliveryServiceImpl) ProcessAutoEscalations(ctx context.Context) error {
	deliveries, err := s.deliveryRepo.GetReadyForAutoEscalation(ctx)
	if err != nil {
		return err
	}

	for _, d := range deliveries {
		dispute := domain.NewDispute(d.EscrowID, d.SellerID, "Auto-escalated: buyer unresponsive for 2 days", nil)
		if err := s.disputeRepo.Create(ctx, dispute); err != nil {
			s.logger.Error("failed to create auto-escalation dispute", zap.Error(err), zap.String("delivery_id", d.ID.String()))
			continue
		}

		if err := s.deliveryRepo.SetDispute(ctx, d.ID, dispute.ID); err != nil {
			s.logger.Error("failed to set dispute on delivery", zap.Error(err))
			continue
		}

		s.escrowRepo.UpdateStatus(ctx, d.EscrowID, domain.EscrowStatusDisputed)

		fromStatus := d.Status
		s.recordEvent(ctx, d.ID, &fromStatus, domain.LDStatusDisputed, nil, "system", "Auto-escalated to admin: buyer unresponsive", nil)

		s.sendNotification(ctx, d.BuyerID, "Order Escalated", "Your order has been escalated to admin for review due to no response.")
		s.sendNotification(ctx, d.SellerID, "Order Escalated", "The order has been escalated to admin for review.")

		// Send admin alert
		if s.adminAlertSvc != nil {
			go func(delivery *domain.LogisticsDelivery) {
				buyer, _ := s.userRepo.GetByID(context.Background(), delivery.BuyerID)
				seller, _ := s.userRepo.GetByID(context.Background(), delivery.SellerID)
				pr, _ := s.prRepo.GetByID(context.Background(), delivery.PaymentRequestID)

				buyerName := ""
				sellerName := ""
				itemName := "Unknown Item"
				if buyer != nil {
					buyerName = buyer.FullName()
				}
				if seller != nil {
					sellerName = seller.FullName()
				}
				if pr != nil {
					itemName = pr.ItemName
				}

				s.adminAlertSvc.AlertAutoEscalatedToDispute(context.Background(), delivery.ID, buyerName, sellerName, itemName, "buyer unresponsive for 2 days")
			}(d)
		}
	}

	if len(deliveries) > 0 {
		s.logger.Info("auto-escalated deliveries", zap.Int("count", len(deliveries)))
	}

	return nil
}

func (s *LogisticsDeliveryServiceImpl) ProcessReturnAutoEscalations(ctx context.Context) error {
	deliveries, err := s.deliveryRepo.GetReadyForReturnAutoEscalation(ctx)
	if err != nil {
		return err
	}

	for _, d := range deliveries {
		dispute := domain.NewDispute(d.EscrowID, d.BuyerID, "Auto-escalated: seller unresponsive for 2 days after return delivery claimed", nil)
		if err := s.disputeRepo.Create(ctx, dispute); err != nil {
			s.logger.Error("failed to create return auto-escalation dispute", zap.Error(err), zap.String("delivery_id", d.ID.String()))
			continue
		}

		if err := s.deliveryRepo.SetDispute(ctx, d.ID, dispute.ID); err != nil {
			s.logger.Error("failed to set dispute on delivery", zap.Error(err))
			continue
		}

		s.escrowRepo.UpdateStatus(ctx, d.EscrowID, domain.EscrowStatusDisputed)

		fromStatus := d.Status
		d.Status = domain.LDStatusDisputed
		s.deliveryRepo.Update(ctx, d)
		s.recordEvent(ctx, d.ID, &fromStatus, domain.LDStatusDisputed, nil, "system", "Auto-escalated to admin: seller unresponsive after return delivery claimed", nil)

		s.sendNotification(ctx, d.SellerID, "Return Escalated", "The return has been escalated to admin for review due to no response.")
		s.sendNotification(ctx, d.BuyerID, "Return Escalated", "Your return has been escalated to admin for review.")

		if s.adminAlertSvc != nil {
			go func(delivery *domain.LogisticsDelivery) {
				buyer, _ := s.userRepo.GetByID(context.Background(), delivery.BuyerID)
				seller, _ := s.userRepo.GetByID(context.Background(), delivery.SellerID)
				pr, _ := s.prRepo.GetByID(context.Background(), delivery.PaymentRequestID)

				buyerName := ""
				sellerName := ""
				itemName := "Unknown Item"
				if buyer != nil {
					buyerName = buyer.FullName()
				}
				if seller != nil {
					sellerName = seller.FullName()
				}
				if pr != nil {
					itemName = pr.ItemName
				}

				s.adminAlertSvc.AlertAutoEscalatedToDispute(context.Background(), delivery.ID, buyerName, sellerName, itemName, "seller unresponsive for 2 days after return delivery claimed")
			}(d)
		}
	}

	if len(deliveries) > 0 {
		s.logger.Info("auto-escalated return deliveries", zap.Int("count", len(deliveries)))
	}

	return nil
}

func (s *LogisticsDeliveryServiceImpl) recordEvent(ctx context.Context, deliveryID uuid.UUID, fromStatus *domain.LogisticsDeliveryStatus, toStatus domain.LogisticsDeliveryStatus, actorID *uuid.UUID, actorType, reason string, metadata map[string]interface{}) {
	var metadataBytes []byte
	if metadata != nil {
		metadataBytes, _ = json.Marshal(metadata)
	} else {
		metadataBytes = []byte("null")
	}

	event := &domain.LogisticsDeliveryEvent{
		ID:                  uuid.New(),
		LogisticsDeliveryID: deliveryID,
		FromStatus:          fromStatus,
		ToStatus:            toStatus,
		ActorID:             actorID,
		ActorType:           actorType,
		Reason:              &reason,
		Metadata:            metadataBytes,
	}

	if err := s.eventRepo.Create(ctx, event); err != nil {
		s.logger.Error("failed to record event", zap.Error(err), zap.String("delivery_id", deliveryID.String()))
	}
}

func (s *LogisticsDeliveryServiceImpl) sendNotification(_ context.Context, userID uuid.UUID, title, message string) {
	if s.notificationService == nil {
		return
	}
	go func() {
		if err := s.notificationService.SendToUser(context.Background(), userID, title, message, nil, domain.NotificationLogisticsConfirmReminder); err != nil {
			s.logger.Debug("failed to send notification", zap.Error(err))
		}
	}()
}

func (s *LogisticsDeliveryServiceImpl) closeSupportChatForDispute(_ context.Context, disputeID, adminID uuid.UUID) {
	if s.supportChatService == nil {
		return
	}
	go func() {
		conv, err := s.supportChatService.GetByDisputeID(context.Background(), disputeID)
		if err != nil || conv == nil {
			return
		}
		if err := s.supportChatService.ResolveConversation(context.Background(), adminID, conv.ID); err != nil {
			s.logger.Warn("failed to close support chat for resolved dispute",
				zap.Error(err),
				zap.String("dispute_id", disputeID.String()),
			)
		} else {
			s.logger.Info("closed support chat for resolved dispute",
				zap.String("dispute_id", disputeID.String()),
				zap.String("conversation_id", conv.ID.String()),
			)
		}
	}()
}

func (s *LogisticsDeliveryServiceImpl) releaseEscrow(ctx context.Context, delivery *domain.LogisticsDelivery, confirmerID uuid.UUID) {
	escrow, err := s.escrowRepo.GetByID(ctx, delivery.EscrowID)
	if err != nil {
		s.logger.Error("failed to get escrow for release", zap.Error(err))
		return
	}

	s.releaseEscrowToSeller(ctx, delivery, escrow, confirmerID)
}

func (s *LogisticsDeliveryServiceImpl) releaseEscrowToSeller(ctx context.Context, delivery *domain.LogisticsDelivery, escrow *domain.Escrow, releaserID uuid.UUID) {
	idempotencyKey := fmt.Sprintf("logistics_release_%s", delivery.ID.String())
	// Deduct seller commission from item amount
	sellerFee := s.feeCalculator.CalculateSellerFee(escrow.ItemAmount)
	sellerPayout := escrow.ItemAmount - sellerFee
	_, err := s.walletService.Credit(ctx, delivery.SellerID, sellerPayout, domain.WalletTxEscrowRelease, "logistics_delivery", delivery.ID, "Payment released for delivered order", idempotencyKey)
	if err != nil {
		s.logger.Error("failed to credit seller", zap.Error(err))
		return
	}


	now := time.Now()
	escrow.SellerPayout = &sellerPayout
	escrow.ReleasedAt = &now
	escrow.Status = domain.EscrowStatusReleased
	s.escrowRepo.Update(ctx, escrow)


	delivery.Status = domain.LDStatusFundsReleased
	delivery.ReleasedAt = &now
	s.deliveryRepo.Update(ctx, delivery)
	if err := s.prRepo.UpdateStatus(ctx, delivery.PaymentRequestID, domain.PRStatusCompleted); err != nil {
		s.logger.Warn("failed to update payment request status", zap.Error(err))
	}

	fromStatus := domain.LDStatusConfirmed
	if delivery.RequiresAdminApproval {
		fromStatus = domain.LDStatusPendingAdminApproval
	}
	s.recordEvent(ctx, delivery.ID, &fromStatus, domain.LDStatusFundsReleased, &releaserID, "system", "Funds released to seller", nil)
	s.sendNotification(ctx, delivery.SellerID, "Payment Released", fmt.Sprintf("NGN %.2f has been credited to your wallet", sellerPayout))
}

func (s *LogisticsDeliveryServiceImpl) processReturnRefund(ctx context.Context, delivery *domain.LogisticsDelivery, escrow *domain.Escrow) {
	// Refund only item amount - service fee retained by platform
	refundAmount := escrow.ItemAmount

	idempotencyKey := fmt.Sprintf("return_refund_%s", delivery.ID.String())
	s.walletService.Credit(ctx, delivery.BuyerID, refundAmount, domain.WalletTxRefund, "logistics_delivery", delivery.ID, "Refund after return confirmed", idempotencyKey)
	now := time.Now()
	delivery.Status = domain.LDStatusRefunded
	delivery.ReleasedAt = &now
	s.deliveryRepo.Update(ctx, delivery)

	escrow.Status = domain.EscrowStatusRefunded
	s.escrowRepo.Update(ctx, escrow)
	if err := s.prRepo.UpdateStatus(ctx, delivery.PaymentRequestID, domain.PRStatusRefunded); err != nil {
		s.logger.Warn("failed to update payment request status", zap.Error(err))
	}

	fromStatus := domain.LDStatusReturnConfirmed
	s.recordEvent(ctx, delivery.ID, &fromStatus, domain.LDStatusRefunded, nil, "system", "Return confirmed, buyer refunded", nil)

	s.sendNotification(ctx, delivery.BuyerID, "Refund Processed", fmt.Sprintf("NGN %.2f has been refunded to your wallet", refundAmount))
}

func (s *LogisticsDeliveryServiceImpl) processPartialRefund(ctx context.Context, delivery *domain.LogisticsDelivery, escrow *domain.Escrow, refundAmount float64) {
	// Calculate seller amount from item amount, then deduct seller commission
	sellerGross := escrow.ItemAmount - refundAmount
	sellerFee := s.feeCalculator.CalculateSellerFee(sellerGross)
	sellerAmount := sellerGross - sellerFee
	idempotencyKey := fmt.Sprintf("partial_refund_buyer_%s", delivery.ID.String())
	s.walletService.Credit(ctx, delivery.BuyerID, refundAmount, domain.WalletTxRefund, "logistics_delivery", delivery.ID, "Partial refund from dispute resolution", idempotencyKey)
	if sellerAmount > 0 {
		idempotencyKey2 := fmt.Sprintf("partial_refund_seller_%s", delivery.ID.String())
		s.walletService.Credit(ctx, delivery.SellerID, sellerAmount, domain.WalletTxEscrowRelease, "logistics_delivery", delivery.ID, "Partial payment from dispute resolution", idempotencyKey2)
	}
	now := time.Now()
	escrow.SellerPayout = &sellerAmount
	escrow.ReleasedAt = &now
	escrow.Status = domain.EscrowStatusPartialRefund
	s.escrowRepo.Update(ctx, escrow)
}

var _ LogisticsDeliveryService = (*LogisticsDeliveryServiceImpl)(nil)
