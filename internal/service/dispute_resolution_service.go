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
	ConfirmReadyForReturn(ctx context.Context, paymentRequestID uuid.UUID, buyerID uuid.UUID) (*domain.DeliveryJob, error)
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
	disputeRepo         repository.DisputeRepository
	escrowRepo          repository.EscrowRepository
	prRepo              repository.PaymentRequestRepository
	deliveryJobRepo     repository.DeliveryJobRepository
	userRepo            repository.UserRepository
	walletService       WalletService
	notificationService NotificationService
	feeCalculator       FeeCalculatorService
	broadcaster         JobStatusBroadcaster
	config              *config.Config
	logger              *zap.Logger
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
	broadcaster JobStatusBroadcaster,
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
		broadcaster:         broadcaster,
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

	// For intra-state with rider delivery, require return and pre-create return job
	if pr.IsIntraStateDelivery() && pr.DeliveryJobID != nil {
		// Pre-create return delivery job in "created" state (not searching yet)
		// Buyer must confirm they're ready before search starts
		returnJob, err := s.createReturnJobInternal(ctx, pr, dispute)
		if err != nil {
			s.logger.Error("failed to create return job",
				zap.Error(err),
				zap.String("pr_id", pr.ID.String()),
			)
			// Still set status to return_required, job can be created later
			pr.Status = domain.PRStatusReturnRequired
		} else {
			// Return job created, but status is still return_required (buyer must confirm ready)
			pr.Status = domain.PRStatusReturnRequired
			pr.ReturnDeliveryJobID = &returnJob.ID
		}

		if err := s.prRepo.Update(ctx, pr); err != nil {
			return err
		}

		// Resolve dispute with pending outcome (waiting for return)
		dispute.Resolve(adminID, domain.DisputeOutcomePending, req.ResolutionNotes)
		if err := s.disputeRepo.Update(ctx, dispute); err != nil {
			return err
		}

		// Notify both parties - buyer must confirm when ready
		s.sendNotification(ctx, *pr.BuyerID, "You Won the Dispute - Return Required",
			"You won the dispute! When you're ready to return the item, tap 'Ready for Return Pickup' in the app. A rider will then be dispatched to collect it.",
			domain.NotificationDisputeResolved)
		s.sendNotification(ctx, pr.SellerID, "Dispute Resolved - Awaiting Return",
			"Buyer won the dispute. They will return the item via rider. You'll be notified when the return is on the way.",
			domain.NotificationDisputeResolved)

		s.logger.Info("dispute resolved: buyer wins, return required (intra-state)",
			zap.String("dispute_id", dispute.ID.String()),
			zap.String("pr_id", pr.ID.String()),
			zap.Bool("return_job_created", pr.ReturnDeliveryJobID != nil),
		)
		return nil
	}

	// For inter-state or no rider delivery, use logistics return flow
	// (This is handled by LogisticsDeliveryService for inter-state)
	// For pickup type or no delivery job, direct refund
	return s.processFullRefund(ctx, dispute, escrow, pr, adminID, req.ResolutionNotes)
}

// createReturnJobInternal creates a return job in "created" state (not searching yet)
// Buyer must confirm they're ready before search starts
func (s *DisputeResolutionServiceImpl) createReturnJobInternal(ctx context.Context, pr *domain.PaymentRequest, dispute *domain.Dispute) (*domain.DeliveryJob, error) {
	if pr.DeliveryJobID == nil {
		return nil, fmt.Errorf("no original delivery job found")
	}

	originalJob, err := s.deliveryJobRepo.GetByID(ctx, *pr.DeliveryJobID)
	if err != nil {
		return nil, err
	}

	// Get coordinates from original job
	coords, err := s.deliveryJobRepo.GetJobCoordinates(ctx, *pr.DeliveryJobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get original job coordinates: %w", err)
	}

	var disputeID *uuid.UUID
	if dispute != nil {
		disputeID = &dispute.ID
	}

	// Calculate return delivery fee (same route, reversed)
	// For return: pickup is buyer's location (was dropoff), dropoff is seller's location (was pickup)
	var deliveryFee float64
	if s.feeCalculator != nil {
		feeEstimate, err := s.feeCalculator.CalculateDeliveryFeeWithVehicle(
			ctx,
			coords.DropoffLat, coords.DropoffLng, // Return pickup (buyer's location)
			coords.PickupLat, coords.PickupLng,   // Return dropoff (seller's location)
			originalJob.RequiredVehicleType,
		)
		if err != nil {
			s.logger.Warn("failed to calculate return delivery fee, using original fee",
				zap.Error(err),
				zap.String("pr_id", pr.ID.String()),
			)
			// Fallback to original delivery fee
			deliveryFee = originalJob.DeliveryFee
		} else {
			deliveryFee = feeEstimate.TotalFee
		}
	} else {
		// No fee calculator, use original fee
		deliveryFee = originalJob.DeliveryFee
	}

	// Create return job in "created" state - NOT searching yet
	// Buyer must confirm ready before search starts
	returnJob := &domain.DeliveryJob{
		ID:                  uuid.New(),
		PaymentRequestID:    pr.ID,
		IsReturn:            true,
		OriginalJobID:       pr.DeliveryJobID,
		DisputeID:           disputeID,
		PickupAddress:       originalJob.DropoffAddress, // Buyer's location (was dropoff)
		PickupLat:           coords.DropoffLat,
		PickupLng:           coords.DropoffLng,
		DropoffAddress:      originalJob.PickupAddress, // Seller's location (was pickup)
		DropoffLat:          coords.PickupLat,
		DropoffLng:          coords.PickupLng,
		RequiredVehicleType: originalJob.RequiredVehicleType,
		DeliveryFee:         deliveryFee,
		SearchRadiusKM:      5,
		Status:              domain.DJStatusCreated, // Waiting for buyer to confirm ready
	}

	if err := s.deliveryJobRepo.Create(ctx, returnJob); err != nil {
		return nil, err
	}

	s.logger.Info("created return delivery job (awaiting buyer ready)",
		zap.String("job_id", returnJob.ID.String()),
		zap.String("pr_id", pr.ID.String()),
		zap.String("original_job_id", originalJob.ID.String()),
	)

	return returnJob, nil
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
	s.sendNotification(ctx, *pr.BuyerID, "Dispute Resolved", fmt.Sprintf("Admin ruled in favor of the seller: %s", req.ResolutionNotes), domain.NotificationDisputeResolved)
	s.sendNotification(ctx, pr.SellerID, "Funds Released", fmt.Sprintf("You won the dispute. NGN %.2f has been credited to your wallet.", sellerPayout), domain.NotificationDisputeResolved)

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
	s.sendNotification(ctx, *pr.BuyerID, "Partial Refund Processed", fmt.Sprintf("NGN %.2f has been refunded to your wallet.", refundAmount), domain.NotificationRefundProcessed)
	s.sendNotification(ctx, pr.SellerID, "Partial Payment Released", fmt.Sprintf("NGN %.2f has been credited to your wallet.", sellerAmount), domain.NotificationDisputeResolved)

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
	s.sendNotification(ctx, *pr.BuyerID, "Refund Processed", fmt.Sprintf("NGN %.2f has been refunded to your wallet.", escrow.TotalLocked), domain.NotificationRefundProcessed)
	s.sendNotification(ctx, pr.SellerID, "Dispute Resolved", notes, domain.NotificationDisputeResolved)

	return nil
}

// ConfirmReadyForReturn is called when buyer confirms they're ready for return pickup
// This starts the rider search
func (s *DisputeResolutionServiceImpl) ConfirmReadyForReturn(ctx context.Context, paymentRequestID uuid.UUID, buyerID uuid.UUID) (*domain.DeliveryJob, error) {
	pr, err := s.prRepo.GetByID(ctx, paymentRequestID)
	if err != nil {
		return nil, err
	}

	// Verify buyer owns this
	if pr.BuyerID == nil || *pr.BuyerID != buyerID {
		return nil, domain.ErrUnauthorized
	}

	// Must be in return_required status
	if pr.Status != domain.PRStatusReturnRequired {
		return nil, fmt.Errorf("cannot confirm ready: status is %s, expected return_required", pr.Status)
	}

	var returnJob *domain.DeliveryJob

	// Check if return job already exists
	if pr.ReturnDeliveryJobID != nil {
		returnJob, err = s.deliveryJobRepo.GetByID(ctx, *pr.ReturnDeliveryJobID)
		if err != nil {
			return nil, fmt.Errorf("failed to get return job: %w", err)
		}

		// Job must be in created status
		if returnJob.Status != domain.DJStatusCreated {
			return nil, fmt.Errorf("return job already in progress: %s", returnJob.Status)
		}
	} else {
		// Create return job if it doesn't exist
		returnJob, err = s.createReturnJobInternal(ctx, pr, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create return job: %w", err)
		}
		pr.ReturnDeliveryJobID = &returnJob.ID
	}

	// Start the search
	now := time.Now()
	returnJob.Status = domain.DJStatusSearching
	returnJob.SearchStartedAt = &now

	if err := s.deliveryJobRepo.Update(ctx, returnJob); err != nil {
		return nil, fmt.Errorf("failed to update return job: %w", err)
	}

	if s.broadcaster != nil {
		s.broadcaster.BroadcastJobStatus(returnJob.ID.String(), string(domain.DJStatusSearching))
	}

	// Update PR status
	pr.Status = domain.PRStatusReturnSearchingRider
	if err := s.prRepo.Update(ctx, pr); err != nil {
		return nil, fmt.Errorf("failed to update payment request: %w", err)
	}

	// Notify buyer
	s.sendNotification(ctx, buyerID, "Return Pickup Scheduled",
		"We're searching for a rider to pick up your item. Please have it ready.",
		domain.NotificationItemReturning)
	s.sendNotification(ctx, pr.SellerID, "Return Pickup Started",
		"The buyer is ready for return. A rider is being dispatched to pick up the item.",
		domain.NotificationItemReturning)

	s.logger.Info("buyer confirmed ready for return, search started",
		zap.String("job_id", returnJob.ID.String()),
		zap.String("pr_id", paymentRequestID.String()),
		zap.String("buyer_id", buyerID.String()),
	)

	return returnJob, nil
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
		// Already has a return job - get it and start search if needed
		existingJob, err := s.deliveryJobRepo.GetByID(ctx, *pr.ReturnDeliveryJobID)
		if err != nil {
			return nil, err
		}

		// If job is in created status, start the search
		if existingJob.Status == domain.DJStatusCreated {
			now := time.Now()
			existingJob.Status = domain.DJStatusSearching
			existingJob.SearchStartedAt = &now

			if err := s.deliveryJobRepo.Update(ctx, existingJob); err != nil {
				return nil, fmt.Errorf("failed to update return job: %w", err)
			}

			if s.broadcaster != nil {
				s.broadcaster.BroadcastJobStatus(existingJob.ID.String(), string(domain.DJStatusSearching))
			}

			// Update PR status
			pr.Status = domain.PRStatusReturnSearchingRider
			if err := s.prRepo.Update(ctx, pr); err != nil {
				return nil, fmt.Errorf("failed to update payment request: %w", err)
			}
		}

		return existingJob, nil
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

	// Calculate return delivery fee
	var deliveryFee float64
	if s.feeCalculator != nil {
		feeEstimate, err := s.feeCalculator.CalculateDeliveryFeeWithVehicle(
			ctx,
			coords.DropoffLat, coords.DropoffLng, // Return pickup (buyer's location)
			coords.PickupLat, coords.PickupLng,   // Return dropoff (seller's location)
			originalJob.RequiredVehicleType,
		)
		if err != nil {
			s.logger.Warn("failed to calculate return delivery fee, using original fee",
				zap.Error(err),
				zap.String("pr_id", pr.ID.String()),
			)
			deliveryFee = originalJob.DeliveryFee
		} else {
			deliveryFee = feeEstimate.TotalFee
		}
	} else {
		deliveryFee = originalJob.DeliveryFee
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
		DeliveryFee:         deliveryFee,
		SearchRadiusKM:      5,
		SearchStartedAt:     &now,
		Status:              domain.DJStatusSearching,
	}

	if err := s.deliveryJobRepo.Create(ctx, returnJob); err != nil {
		return nil, err
	}

	if s.broadcaster != nil {
		s.broadcaster.BroadcastJobStatus(returnJob.ID.String(), string(domain.DJStatusSearching))
	}

	// Update payment request
	pr.ReturnDeliveryJobID = &returnJob.ID
	pr.Status = domain.PRStatusReturnSearchingRider
	if err := s.prRepo.Update(ctx, pr); err != nil {
		return nil, err
	}

	// Notify buyer
	s.sendNotification(ctx, *pr.BuyerID, "Return Pickup Scheduled",
		"A rider will be dispatched to pick up the item for return. Please have it ready.",
		domain.NotificationItemReturning)

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

		if s.broadcaster != nil {
			s.broadcaster.BroadcastJobStatus(returnJobID.String(), string(domain.DJStatusReturnConfirmed))
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

		if s.broadcaster != nil {
			s.broadcaster.BroadcastJobStatus(returnJobID.String(), string(domain.DJStatusReturnDamageDisputed))
		}

		// Update payment request
		pr.Status = domain.PRStatusReturnDamageDisputed
		if err := s.prRepo.Update(ctx, pr); err != nil {
			return err
		}

		// Notify
		s.sendNotification(ctx, *pr.BuyerID, "Return Condition Disputed",
			fmt.Sprintf("Seller claims the returned item is %s. Admin will review and decide.", req.Condition),
			domain.NotificationDisputeOpened)
		s.sendNotification(ctx, sellerID, "Damage Claim Submitted",
			"Your damage claim has been submitted. Admin will review and make a decision.",
			domain.NotificationDisputeOpened)

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

	if s.broadcaster != nil {
		s.broadcaster.BroadcastJobStatus(job.ID.String(), string(domain.DJStatusReturnRefunded))
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
		fmt.Sprintf("NGN %.2f has been refunded to your wallet.", escrow.TotalLocked),
		domain.NotificationRefundProcessed)
	s.sendNotification(ctx, pr.SellerID, "Return Complete",
		"The return has been completed. Buyer has been refunded.",
		domain.NotificationItemReturned)

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

	if s.broadcaster != nil {
		s.broadcaster.BroadcastJobStatus(job.ID.String(), string(domain.DJStatusReturnRefunded))
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
		fmt.Sprintf("NGN %.2f has been refunded due to item damage.", refundAmount),
		domain.NotificationRefundProcessed)
	s.sendNotification(ctx, pr.SellerID, "Damage Claim Resolved",
		fmt.Sprintf("NGN %.2f has been credited for the damage compensation.", sellerAmount),
		domain.NotificationDisputeResolved)

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

	if s.broadcaster != nil {
		s.broadcaster.BroadcastJobStatus(job.ID.String(), string(domain.DJStatusReturnConfirmed))
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
		"Admin determined the return damage was severe. Funds released to seller.",
		domain.NotificationDisputeResolved)
	s.sendNotification(ctx, pr.SellerID, "Funds Released",
		fmt.Sprintf("NGN %.2f has been credited due to confirmed return damage.", escrow.TotalLocked),
		domain.NotificationDisputeResolved)

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

func (s *DisputeResolutionServiceImpl) sendNotification(_ context.Context, userID uuid.UUID, title, message string, notifType domain.NotificationType) {
	if s.notificationService == nil {
		return
	}
	go func() {
		if err := s.notificationService.SendToUser(context.Background(), userID, title, message, nil, notifType); err != nil {
			s.logger.Debug("failed to send notification", zap.Error(err))
		}
	}()
}

var _ DisputeResolutionService = (*DisputeResolutionServiceImpl)(nil)
