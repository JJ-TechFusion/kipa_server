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

// DisputeResolutionService handles dispute resolution for both intra-state and inter-state deliveries
type DisputeResolutionService interface {
	// Resolve a dispute with outcome
	ResolveDispute(ctx context.Context, disputeID uuid.UUID, adminID uuid.UUID, req *ResolveDisputeRequest) error

	// Intra-state return flow
	CreateReturnDeliveryJob(ctx context.Context, paymentRequestID uuid.UUID) (*domain.DeliveryJob, error)
	ConfirmReturnReceived(ctx context.Context, returnJobID uuid.UUID, sellerID uuid.UUID, req *ConfirmReturnRequest) error
	ResolveDamageClaim(ctx context.Context, returnJobID uuid.UUID, adminID uuid.UUID, req *ResolveDamageClaimRequest) error

	// Queries
	GetReturnJob(ctx context.Context, paymentRequestID uuid.UUID) (*domain.DeliveryJob, error)
	ListPendingDamageClaims(ctx context.Context, limit, offset int) ([]*domain.DeliveryJob, int, error)
}

type ResolveDisputeRequest struct {
	Outcome              string   // buyer_wins, seller_wins, partial_refund
	ResolutionNotes      string
	RefundAmount         *float64 // For partial refund
	ReturnCostBuyerShare *float64 // Percentage of return shipping buyer pays
}

type ConfirmReturnRequest struct {
	Condition            string   // good, damaged, incomplete
	Notes                *string
	DamageReason         *string  // Required if damaged/incomplete
	DamageEvidenceURLs   []string // Evidence photos
}

type ResolveDamageClaimRequest struct {
	Decision        string   // full_refund, partial_refund, release_to_seller
	RefundAmount    *float64 // For partial refund
	ResolutionNotes string
}

type DisputeResolutionServiceImpl struct {
	disputeRepo       repository.DisputeRepository
	escrowRepo        repository.EscrowRepository
	prRepo            repository.PaymentRequestRepository
	deliveryJobRepo   repository.DeliveryJobRepository
	userRepo          repository.UserRepository
	walletService     WalletService
	notificationService NotificationService
	feeCalculator     FeeCalculatorService
	config            *config.Config
	logger            *zap.Logger
}

func NewDisputeResolutionService(
	disputeRepo repository.DisputeRepository,
	escrowRepo repository.EscrowRepository,
	prRepo repository.PaymentRequestRepository,
	deliveryJobRepo repository.DeliveryJobRepository,
	userRepo repository.UserRepository,
	walletService WalletService,
	notificationService NotificationService,
	feeCalculator FeeCalculatorService,
	cfg *config.Config,
	logger *zap.Logger,
) DisputeResolutionService {
	return &DisputeResolutionServiceImpl{
		disputeRepo:         disputeRepo,
		escrowRepo:          escrowRepo,
		prRepo:              prRepo,
		deliveryJobRepo:     deliveryJobRepo,
		userRepo:            userRepo,
		walletService:       walletService,
		notificationService: notificationService,
		feeCalculator:       feeCalculator,
		config:              cfg,
		logger:              logger,
	}
}

func (s *DisputeResolutionServiceImpl) ResolveDispute(ctx context.Context, disputeID uuid.UUID, adminID uuid.UUID, req *ResolveDisputeRequest) error {
	dispute, err := s.disputeRepo.GetByID(ctx, disputeID)
	if err != nil {
		return err
	}

	escrow, err := s.escrowRepo.GetByID(ctx, dispute.EscrowID)
	if err != nil {
		return err
	}

	pr, err := s.prRepo.GetByID(ctx, escrow.PaymentRequestID)
	if err != nil {
		return err
	}

	// Handle based on delivery type
	switch req.Outcome {
	case "buyer_favor":
		return s.handleBuyerWins(ctx, dispute, escrow, pr, adminID, req)
	case "seller_favor":
		return s.handleSellerWins(ctx, dispute, escrow, pr, adminID, req)
	case "partial_refund":
		return s.handlePartialRefund(ctx, dispute, escrow, pr, adminID, req)
	default:
		return fmt.Errorf("invalid outcome: %s", req.Outcome)
	}
}

func (s *DisputeResolutionServiceImpl) handleBuyerWins(ctx context.Context, dispute *domain.Dispute, escrow *domain.Escrow, pr *domain.PaymentRequest, adminID uuid.UUID, req *ResolveDisputeRequest) error {
	outcome := "buyer_favor"
	pr.DisputeOutcome = &outcome

	// For intra-state with rider delivery, require return
	if pr.IsIntraStateDelivery() && pr.DeliveryJobID != nil {
		// Buyer must return the item via rider
		pr.Status = domain.PRStatusReturnRequired
		if err := s.prRepo.Update(ctx, pr); err != nil {
			return err
		}

		// Resolve dispute with pending outcome (waiting for return)
		dispute.Resolve(adminID, domain.DisputeOutcomePending, req.ResolutionNotes)
		if err := s.disputeRepo.Update(ctx, dispute); err != nil {
			return err
		}

		// Notify both parties
		s.sendNotification(ctx, *pr.BuyerID, "Return Required",
			"You won the dispute. Please wait for a rider to pick up the item for return. Your refund will be processed after the seller confirms receipt.")
		s.sendNotification(ctx, pr.SellerID, "Dispute Resolved - Return Incoming",
			"Buyer won the dispute and will be returning the item via rider. Please confirm receipt when you receive it.")

		s.logger.Info("dispute resolved: buyer wins, return required (intra-state)",
			zap.String("dispute_id", dispute.ID.String()),
			zap.String("pr_id", pr.ID.String()),
		)
		return nil
	}

	// For inter-state or no rider delivery, use logistics return flow
	// (This is handled by LogisticsDeliveryService for inter-state)
	// For pickup type or no delivery job, direct refund
	return s.processFullRefund(ctx, dispute, escrow, pr, adminID, req.ResolutionNotes)
}

func (s *DisputeResolutionServiceImpl) handleSellerWins(ctx context.Context, dispute *domain.Dispute, escrow *domain.Escrow, pr *domain.PaymentRequest, adminID uuid.UUID, req *ResolveDisputeRequest) error {
	outcome := "seller_favor"
	pr.DisputeOutcome = &outcome

	// Release funds to seller
	idempotencyKey := fmt.Sprintf("dispute_seller_wins_%s", dispute.ID.String())
	sellerPayout := escrow.ItemAmount
	_, err := s.walletService.Credit(ctx, pr.SellerID, sellerPayout, domain.WalletTxEscrowRelease, "dispute", dispute.ID, req.ResolutionNotes, idempotencyKey)
	if err != nil {
		return err
	}

	// Update escrow
	now := time.Now()
	escrow.SellerPayout = &sellerPayout
	escrow.ReleasedAt = &now
	escrow.Status = domain.EscrowStatusReleased
	if err := s.escrowRepo.Update(ctx, escrow); err != nil {
		return err
	}

	// Update payment request
	pr.Status = domain.PRStatusCompleted
	pr.CompletedAt = &now
	if err := s.prRepo.Update(ctx, pr); err != nil {
		return err
	}

	// Resolve dispute
	dispute.Resolve(adminID, domain.DisputeOutcomeSellerWins, req.ResolutionNotes)
	if err := s.disputeRepo.Update(ctx, dispute); err != nil {
		return err
	}

	// Notify
	s.sendNotification(ctx, *pr.BuyerID, "Dispute Resolved", fmt.Sprintf("Admin ruled in favor of the seller: %s", req.ResolutionNotes))
	s.sendNotification(ctx, pr.SellerID, "Funds Released", fmt.Sprintf("You won the dispute. NGN %.2f has been credited to your wallet.", sellerPayout))

	s.logger.Info("dispute resolved: seller wins",
		zap.String("dispute_id", dispute.ID.String()),
		zap.Float64("payout", sellerPayout),
	)
	return nil
}

func (s *DisputeResolutionServiceImpl) handlePartialRefund(ctx context.Context, dispute *domain.Dispute, escrow *domain.Escrow, pr *domain.PaymentRequest, adminID uuid.UUID, req *ResolveDisputeRequest) error {
	if req.RefundAmount == nil {
		return fmt.Errorf("refund amount required for partial refund")
	}

	outcome := "partial_refund"
	pr.DisputeOutcome = &outcome
	refundAmount := *req.RefundAmount
	sellerAmount := escrow.TotalLocked - refundAmount

	// Refund buyer
	idempotencyKey := fmt.Sprintf("dispute_partial_buyer_%s", dispute.ID.String())
	_, err := s.walletService.Credit(ctx, *pr.BuyerID, refundAmount, domain.WalletTxRefund, "dispute", dispute.ID, req.ResolutionNotes, idempotencyKey)
	if err != nil {
		return err
	}

	// Credit seller remaining
	if sellerAmount > 0 {
		idempotencyKey2 := fmt.Sprintf("dispute_partial_seller_%s", dispute.ID.String())
		_, err = s.walletService.Credit(ctx, pr.SellerID, sellerAmount, domain.WalletTxEscrowRelease, "dispute", dispute.ID, req.ResolutionNotes, idempotencyKey2)
		if err != nil {
			return err
		}
	}

	// Update escrow
	now := time.Now()
	escrow.SellerPayout = &sellerAmount
	escrow.ReleasedAt = &now
	escrow.Status = domain.EscrowStatusPartialRefund
	if err := s.escrowRepo.Update(ctx, escrow); err != nil {
		return err
	}

	// Update payment request
	pr.Status = domain.PRStatusRefunded
	if err := s.prRepo.Update(ctx, pr); err != nil {
		return err
	}

	// Resolve dispute
	dispute.Resolve(adminID, domain.DisputeOutcomePartialRefund, req.ResolutionNotes)
	if err := s.disputeRepo.Update(ctx, dispute); err != nil {
		return err
	}

	// Notify
	s.sendNotification(ctx, *pr.BuyerID, "Partial Refund Processed", fmt.Sprintf("NGN %.2f has been refunded to your wallet.", refundAmount))
	s.sendNotification(ctx, pr.SellerID, "Partial Payment Released", fmt.Sprintf("NGN %.2f has been credited to your wallet.", sellerAmount))

	s.logger.Info("dispute resolved: partial refund",
		zap.String("dispute_id", dispute.ID.String()),
		zap.Float64("refund", refundAmount),
		zap.Float64("seller", sellerAmount),
	)
	return nil
}

func (s *DisputeResolutionServiceImpl) processFullRefund(ctx context.Context, dispute *domain.Dispute, escrow *domain.Escrow, pr *domain.PaymentRequest, adminID uuid.UUID, notes string) error {
	// Full refund to buyer
	idempotencyKey := fmt.Sprintf("dispute_refund_%s", dispute.ID.String())
	_, err := s.walletService.Credit(ctx, *pr.BuyerID, escrow.TotalLocked, domain.WalletTxRefund, "dispute", dispute.ID, notes, idempotencyKey)
	if err != nil {
		return err
	}

	// Update escrow
	now := time.Now()
	escrow.ReleasedAt = &now
	escrow.Status = domain.EscrowStatusRefunded
	if err := s.escrowRepo.Update(ctx, escrow); err != nil {
		return err
	}

	// Update payment request
	pr.Status = domain.PRStatusRefunded
	if err := s.prRepo.Update(ctx, pr); err != nil {
		return err
	}

	// Resolve dispute
	dispute.Resolve(adminID, domain.DisputeOutcomeBuyerWins, notes)
	if err := s.disputeRepo.Update(ctx, dispute); err != nil {
		return err
	}

	// Notify
	s.sendNotification(ctx, *pr.BuyerID, "Refund Processed", fmt.Sprintf("NGN %.2f has been refunded to your wallet.", escrow.TotalLocked))
	s.sendNotification(ctx, pr.SellerID, "Dispute Resolved", notes)

	return nil
}

// CreateReturnDeliveryJob creates a return delivery job for intra-state returns
func (s *DisputeResolutionServiceImpl) CreateReturnDeliveryJob(ctx context.Context, paymentRequestID uuid.UUID) (*domain.DeliveryJob, error) {
	pr, err := s.prRepo.GetByID(ctx, paymentRequestID)
	if err != nil {
		return nil, err
	}

	if pr.Status != domain.PRStatusReturnRequired {
		return nil, fmt.Errorf("payment request not in return_required status")
	}

	if pr.ReturnDeliveryJobID != nil {
		// Already has a return job
		return s.deliveryJobRepo.GetByID(ctx, *pr.ReturnDeliveryJobID)
	}

	// Get original delivery job
	if pr.DeliveryJobID == nil {
		return nil, fmt.Errorf("no original delivery job found")
	}
	originalJob, err := s.deliveryJobRepo.GetByID(ctx, *pr.DeliveryJobID)
	if err != nil {
		return nil, err
	}

	// Get coordinates from original job for Google Maps routing
	coords, err := s.deliveryJobRepo.GetJobCoordinates(ctx, *pr.DeliveryJobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get original job coordinates: %w", err)
	}

	// Get escrow for dispute reference
	var disputeID *uuid.UUID
	if pr.EscrowID != nil {
		escrow, _ := s.escrowRepo.GetByID(ctx, *pr.EscrowID)
		if escrow != nil {
			dispute, _ := s.disputeRepo.GetByEscrowID(ctx, escrow.ID)
			if dispute != nil {
				disputeID = &dispute.ID
			}
		}
	}

	// Create return job with reversed addresses and coordinates
	// For return: original dropoff (buyer) becomes pickup, original pickup (seller) becomes dropoff
	now := time.Now()
	returnJob := &domain.DeliveryJob{
		ID:                  uuid.New(),
		PaymentRequestID:    paymentRequestID,
		IsReturn:            true,
		OriginalJobID:       pr.DeliveryJobID,
		DisputeID:           disputeID,
		PickupAddress:       originalJob.DropoffAddress, // Buyer's location (was dropoff)
		PickupLat:           coords.DropoffLat,          // Swap: original dropoff lat
		PickupLng:           coords.DropoffLng,          // Swap: original dropoff lng
		DropoffAddress:      originalJob.PickupAddress,  // Seller's location (was pickup)
		DropoffLat:          coords.PickupLat,           // Swap: original pickup lat
		DropoffLng:          coords.PickupLng,           // Swap: original pickup lng
		RequiredVehicleType: originalJob.RequiredVehicleType,
		DeliveryFee:         0, // Return delivery fee handled separately (dispute resolution decides)
		SearchRadiusKM:      5,
		SearchStartedAt:     &now,
		Status:              domain.DJStatusSearching,
	}

	if err := s.deliveryJobRepo.Create(ctx, returnJob); err != nil {
		return nil, err
	}

	// Update payment request
	pr.ReturnDeliveryJobID = &returnJob.ID
	pr.Status = domain.PRStatusReturnSearchingRider
	if err := s.prRepo.Update(ctx, pr); err != nil {
		return nil, err
	}

	// Notify buyer
	s.sendNotification(ctx, *pr.BuyerID, "Return Pickup Scheduled",
		"A rider will be dispatched to pick up the item for return. Please have it ready.")

	s.logger.Info("return delivery job created",
		zap.String("job_id", returnJob.ID.String()),
		zap.String("pr_id", paymentRequestID.String()),
		zap.String("original_job_id", originalJob.ID.String()),
	)

	return returnJob, nil
}

// ConfirmReturnReceived handles seller confirming return receipt
func (s *DisputeResolutionServiceImpl) ConfirmReturnReceived(ctx context.Context, returnJobID uuid.UUID, sellerID uuid.UUID, req *ConfirmReturnRequest) error {
	job, err := s.deliveryJobRepo.GetByID(ctx, returnJobID)
	if err != nil {
		return err
	}

	if !job.IsReturn {
		return fmt.Errorf("not a return job")
	}

	// Verify seller owns this
	pr, err := s.prRepo.GetByID(ctx, job.PaymentRequestID)
	if err != nil {
		return err
	}
	if pr.SellerID != sellerID {
		return domain.ErrUnauthorized
	}

	if job.Status != domain.DJStatusReturnDelivered {
		return domain.ErrInvalidStatusTransition
	}

	now := time.Now()
	job.ReturnConfirmedAt = &now
	job.ReturnCondition = &req.Condition

	switch req.Condition {
	case "good":
		// Good condition → process refund
		job.Status = domain.DJStatusReturnConfirmed
		if err := s.deliveryJobRepo.Update(ctx, job); err != nil {
			return err
		}

		// Update payment request
		pr.Status = domain.PRStatusReturnConfirmed
		if err := s.prRepo.Update(ctx, pr); err != nil {
			return err
		}

		// Process refund
		return s.processReturnRefund(ctx, job, pr)

	case "damaged", "incomplete":
		// Seller disputes return condition
		if req.DamageReason == nil || *req.DamageReason == "" {
			return fmt.Errorf("damage reason required when condition is damaged or incomplete")
		}
		job.DamageClaimReason = req.DamageReason
		job.Status = domain.DJStatusReturnDamageDisputed
		if err := s.deliveryJobRepo.Update(ctx, job); err != nil {
			return err
		}

		// Update payment request
		pr.Status = domain.PRStatusReturnDamageDisputed
		if err := s.prRepo.Update(ctx, pr); err != nil {
			return err
		}

		// Notify
		s.sendNotification(ctx, *pr.BuyerID, "Return Condition Disputed",
			fmt.Sprintf("Seller claims the returned item is %s. Admin will review and decide.", req.Condition))
		s.sendNotification(ctx, sellerID, "Damage Claim Submitted",
			"Your damage claim has been submitted. Admin will review and make a decision.")

		s.logger.Info("return damage claimed",
			zap.String("job_id", returnJobID.String()),
			zap.String("condition", req.Condition),
		)
		return nil

	default:
		return fmt.Errorf("invalid condition: %s", req.Condition)
	}
}

func (s *DisputeResolutionServiceImpl) processReturnRefund(ctx context.Context, job *domain.DeliveryJob, pr *domain.PaymentRequest) error {
	if pr.EscrowID == nil {
		return fmt.Errorf("no escrow found")
	}

	escrow, err := s.escrowRepo.GetByID(ctx, *pr.EscrowID)
	if err != nil {
		return err
	}

	// Full refund
	idempotencyKey := fmt.Sprintf("return_refund_%s", job.ID.String())
	_, err = s.walletService.Credit(ctx, *pr.BuyerID, escrow.TotalLocked, domain.WalletTxRefund, "delivery_job", job.ID, "Refund after return confirmed", idempotencyKey)
	if err != nil {
		return err
	}

	// Update job status
	job.Status = domain.DJStatusReturnRefunded
	if err := s.deliveryJobRepo.Update(ctx, job); err != nil {
		return err
	}

	// Update escrow
	now := time.Now()
	escrow.ReleasedAt = &now
	escrow.Status = domain.EscrowStatusRefunded
	if err := s.escrowRepo.Update(ctx, escrow); err != nil {
		return err
	}

	// Update payment request
	pr.Status = domain.PRStatusRefunded
	if err := s.prRepo.Update(ctx, pr); err != nil {
		return err
	}

	// Notify
	s.sendNotification(ctx, *pr.BuyerID, "Refund Processed",
		fmt.Sprintf("NGN %.2f has been refunded to your wallet.", escrow.TotalLocked))
	s.sendNotification(ctx, pr.SellerID, "Return Complete",
		"The return has been completed. Buyer has been refunded.")

	s.logger.Info("return refund processed",
		zap.String("job_id", job.ID.String()),
		zap.Float64("amount", escrow.TotalLocked),
	)

	return nil
}

// ResolveDamageClaim handles admin resolving a return damage claim
func (s *DisputeResolutionServiceImpl) ResolveDamageClaim(ctx context.Context, returnJobID uuid.UUID, adminID uuid.UUID, req *ResolveDamageClaimRequest) error {
	job, err := s.deliveryJobRepo.GetByID(ctx, returnJobID)
	if err != nil {
		return err
	}

	if !job.IsReturn || job.Status != domain.DJStatusReturnDamageDisputed {
		return domain.ErrInvalidStatusTransition
	}

	pr, err := s.prRepo.GetByID(ctx, job.PaymentRequestID)
	if err != nil {
		return err
	}

	if pr.EscrowID == nil {
		return fmt.Errorf("no escrow found")
	}

	escrow, err := s.escrowRepo.GetByID(ctx, *pr.EscrowID)
	if err != nil {
		return err
	}

	switch req.Decision {
	case "full_refund":
		// Admin decides damage claim invalid → full refund
		return s.processReturnRefund(ctx, job, pr)

	case "partial_refund":
		if req.RefundAmount == nil {
			return fmt.Errorf("refund amount required for partial refund")
		}
		return s.processPartialReturnRefund(ctx, job, pr, escrow, *req.RefundAmount, req.ResolutionNotes)

	case "release_to_seller":
		// Severe damage → release to seller
		return s.releaseToSeller(ctx, job, pr, escrow, req.ResolutionNotes)

	default:
		return fmt.Errorf("invalid decision: %s", req.Decision)
	}
}

func (s *DisputeResolutionServiceImpl) processPartialReturnRefund(ctx context.Context, job *domain.DeliveryJob, pr *domain.PaymentRequest, escrow *domain.Escrow, refundAmount float64, notes string) error {
	sellerAmount := escrow.TotalLocked - refundAmount

	// Refund buyer
	idempotencyKey := fmt.Sprintf("return_partial_buyer_%s", job.ID.String())
	_, err := s.walletService.Credit(ctx, *pr.BuyerID, refundAmount, domain.WalletTxRefund, "delivery_job", job.ID, notes, idempotencyKey)
	if err != nil {
		return err
	}

	// Credit seller
	if sellerAmount > 0 {
		idempotencyKey2 := fmt.Sprintf("return_partial_seller_%s", job.ID.String())
		_, err = s.walletService.Credit(ctx, pr.SellerID, sellerAmount, domain.WalletTxEscrowRelease, "delivery_job", job.ID, notes, idempotencyKey2)
		if err != nil {
			return err
		}
	}

	// Update job
	job.Status = domain.DJStatusReturnRefunded
	if err := s.deliveryJobRepo.Update(ctx, job); err != nil {
		return err
	}

	// Update escrow
	now := time.Now()
	escrow.SellerPayout = &sellerAmount
	escrow.ReleasedAt = &now
	escrow.Status = domain.EscrowStatusPartialRefund
	if err := s.escrowRepo.Update(ctx, escrow); err != nil {
		return err
	}

	// Update payment request
	pr.Status = domain.PRStatusRefunded
	if err := s.prRepo.Update(ctx, pr); err != nil {
		return err
	}

	// Notify
	s.sendNotification(ctx, *pr.BuyerID, "Partial Refund Processed",
		fmt.Sprintf("NGN %.2f has been refunded due to item damage.", refundAmount))
	s.sendNotification(ctx, pr.SellerID, "Damage Claim Resolved",
		fmt.Sprintf("NGN %.2f has been credited for the damage compensation.", sellerAmount))

	return nil
}

func (s *DisputeResolutionServiceImpl) releaseToSeller(ctx context.Context, job *domain.DeliveryJob, pr *domain.PaymentRequest, escrow *domain.Escrow, notes string) error {
	// Release full amount to seller
	idempotencyKey := fmt.Sprintf("return_damage_seller_%s", job.ID.String())
	_, err := s.walletService.Credit(ctx, pr.SellerID, escrow.TotalLocked, domain.WalletTxEscrowRelease, "delivery_job", job.ID, notes, idempotencyKey)
	if err != nil {
		return err
	}

	// Update job
	job.Status = domain.DJStatusReturnConfirmed
	if err := s.deliveryJobRepo.Update(ctx, job); err != nil {
		return err
	}

	// Update escrow
	now := time.Now()
	escrow.SellerPayout = &escrow.TotalLocked
	escrow.ReleasedAt = &now
	escrow.Status = domain.EscrowStatusReleased
	if err := s.escrowRepo.Update(ctx, escrow); err != nil {
		return err
	}

	// Update payment request
	pr.Status = domain.PRStatusCompleted
	pr.CompletedAt = &now
	if err := s.prRepo.Update(ctx, pr); err != nil {
		return err
	}

	// Notify
	s.sendNotification(ctx, *pr.BuyerID, "Damage Claim Resolved",
		"Admin determined the return damage was severe. Funds released to seller.")
	s.sendNotification(ctx, pr.SellerID, "Funds Released",
		fmt.Sprintf("NGN %.2f has been credited due to confirmed return damage.", escrow.TotalLocked))

	return nil
}

// GetReturnJob retrieves return job for a payment request
func (s *DisputeResolutionServiceImpl) GetReturnJob(ctx context.Context, paymentRequestID uuid.UUID) (*domain.DeliveryJob, error) {
	pr, err := s.prRepo.GetByID(ctx, paymentRequestID)
	if err != nil {
		return nil, err
	}

	if pr.ReturnDeliveryJobID == nil {
		return nil, fmt.Errorf("no return job found")
	}

	return s.deliveryJobRepo.GetByID(ctx, *pr.ReturnDeliveryJobID)
}

// ListPendingDamageClaims returns delivery jobs with damage claims pending admin review
func (s *DisputeResolutionServiceImpl) ListPendingDamageClaims(ctx context.Context, limit, offset int) ([]*domain.DeliveryJob, int, error) {
	return s.deliveryJobRepo.GetByStatus(ctx, domain.DJStatusReturnDamageDisputed, limit, offset)
}

func (s *DisputeResolutionServiceImpl) sendNotification(_ context.Context, userID uuid.UUID, title, message string) {
	if s.notificationService == nil {
		return
	}
	go func() {
		if err := s.notificationService.SendToUser(context.Background(), userID, title, message, nil, domain.NotificationLogisticsConfirmReminder); err != nil {
			s.logger.Debug("failed to send notification", zap.Error(err))
		}
	}()
}

var _ DisputeResolutionService = (*DisputeResolutionServiceImpl)(nil)
