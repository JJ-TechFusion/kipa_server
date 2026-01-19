package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/domain"
	"kipa/internal/repository"
)

// RiderCommissionService handles rider commission operations
type RiderCommissionService interface {
	// RecordCommission creates a commission record when delivery is completed
	RecordCommission(ctx context.Context, riderProfileID, deliveryJobID uuid.UUID, deliveryFee float64) error

	// GetSummary returns the rider's commission summary
	GetSummary(ctx context.Context, userID uuid.UUID) (*CommissionSummary, error)

	// GetPendingCommissions returns all pending commission records for a rider
	GetPendingCommissions(ctx context.Context, userID uuid.UUID) ([]*domain.RiderCommission, error)

	// PayAllFromWallet pays all pending commissions from rider's wallet
	PayAllFromWallet(ctx context.Context, userID uuid.UUID) (*CommissionPaymentResult, error)

	// CheckAndUpdateBlockStatus checks if rider should be blocked and updates status
	CheckAndUpdateBlockStatus(ctx context.Context, riderProfileID uuid.UUID) error

	// WaiveCommissions waives all pending commissions (admin action)
	WaiveCommissions(ctx context.Context, riderProfileID uuid.UUID, reason string) error

	// GetFeeBreakdown calculates and returns the fee breakdown for a delivery
	GetFeeBreakdown(deliveryFee float64) *domain.RiderFeeBreakdown
}

// CommissionSummary represents a rider's commission status
type CommissionSummary struct {
	TotalOwed         float64 `json:"total_owed"`
	UnpaidRideCount   int     `json:"unpaid_ride_count"`
	IsBlocked         bool    `json:"is_blocked"`
	RidesUntilBlocked int     `json:"rides_until_blocked"`
	GracePeriodRides  int     `json:"grace_period_rides"`
	CommissionRate    float64 `json:"commission_rate"`
}

// CommissionPaymentResult represents the result of paying commissions
type CommissionPaymentResult struct {
	AmountPaid       float64 `json:"amount_paid"`
	CommissionsPaid  int     `json:"commissions_paid"`
	NewWalletBalance float64 `json:"new_wallet_balance"`
	IsNowUnblocked   bool    `json:"is_now_unblocked"`
}

type RiderCommissionServiceImpl struct {
	commissionRepo repository.RiderCommissionRepository
	paymentRepo    repository.CommissionPaymentRepository
	riderRepo      repository.RiderProfileRepository
	walletService  WalletService
	logger         *zap.Logger
}

func NewRiderCommissionService(
	commissionRepo repository.RiderCommissionRepository,
	paymentRepo repository.CommissionPaymentRepository,
	riderRepo repository.RiderProfileRepository,
	walletService WalletService,
	logger *zap.Logger,
) RiderCommissionService {
	return &RiderCommissionServiceImpl{
		commissionRepo: commissionRepo,
		paymentRepo:    paymentRepo,
		riderRepo:      riderRepo,
		walletService:  walletService,
		logger:         logger,
	}
}

// RecordCommission creates a commission record when delivery is completed
func (s *RiderCommissionServiceImpl) RecordCommission(ctx context.Context, riderProfileID, deliveryJobID uuid.UUID, deliveryFee float64) error {
	// Create commission record
	commission := domain.NewRiderCommission(riderProfileID, deliveryJobID, deliveryFee)

	if err := s.commissionRepo.Create(ctx, commission); err != nil {
		return fmt.Errorf("failed to create commission record: %w", err)
	}

	// Update rider's accumulated commission stats
	totalOwed, count, err := s.commissionRepo.GetPendingTotals(ctx, riderProfileID)
	if err != nil {
		s.logger.Error("failed to get pending totals", zap.Error(err))
		// Don't fail the operation, commission was recorded
	} else {
		if err := s.riderRepo.UpdateCommissionStats(ctx, riderProfileID, totalOwed, count); err != nil {
			s.logger.Error("failed to update commission stats", zap.Error(err))
		}
	}

	s.logger.Info("rider commission recorded",
		zap.String("rider_profile_id", riderProfileID.String()),
		zap.String("delivery_job_id", deliveryJobID.String()),
		zap.Float64("delivery_fee", deliveryFee),
		zap.Float64("commission_amount", commission.CommissionAmount),
		zap.Int("total_unpaid_count", count),
	)

	return nil
}

// GetSummary returns the rider's commission summary
func (s *RiderCommissionServiceImpl) GetSummary(ctx context.Context, userID uuid.UUID) (*CommissionSummary, error) {
	profile, err := s.riderRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	ridesUntilBlocked := profile.RidesUntilBlocked()

	return &CommissionSummary{
		TotalOwed:         profile.UnpaidCommission,
		UnpaidRideCount:   profile.UnpaidDeliveryCount,
		IsBlocked:         profile.IsCommissionBlocked,
		RidesUntilBlocked: ridesUntilBlocked,
		GracePeriodRides:  domain.MaxUnpaidRidesBeforeBlock,
		CommissionRate:    domain.RiderCommissionRate,
	}, nil
}

// GetPendingCommissions returns all pending commission records for a rider
func (s *RiderCommissionServiceImpl) GetPendingCommissions(ctx context.Context, userID uuid.UUID) ([]*domain.RiderCommission, error) {
	profile, err := s.riderRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return s.commissionRepo.GetPendingByRiderID(ctx, profile.ID)
}

// PayAllFromWallet pays all pending commissions from rider's wallet
func (s *RiderCommissionServiceImpl) PayAllFromWallet(ctx context.Context, userID uuid.UUID) (*CommissionPaymentResult, error) {
	profile, err := s.riderRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Get total owed
	totalOwed, count, err := s.commissionRepo.GetPendingTotals(ctx, profile.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending totals: %w", err)
	}

	if count == 0 {
		return nil, domain.ErrNoCommissionOwed
	}

	// Check wallet balance
	wallet, err := s.walletService.GetBalance(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet balance: %w", err)
	}

	if wallet.AvailableBalance < totalOwed {
		return nil, domain.ErrInsufficientBalance
	}

	// Debit wallet
	idempotencyKey := fmt.Sprintf("commission_payment_%s_%d", profile.ID.String(), count)
	_, err = s.walletService.Debit(
		ctx,
		userID,
		totalOwed,
		domain.WalletTxPlatformFee,
		"commission_payment",
		profile.ID,
		fmt.Sprintf("Commission payment for %d deliveries", count),
		idempotencyKey,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to debit wallet: %w", err)
	}

	// Mark all commissions as paid
	paidCount, err := s.commissionRepo.MarkAllPendingAsPaid(ctx, profile.ID, "wallet")
	if err != nil {
		s.logger.Error("failed to mark commissions as paid", zap.Error(err))
		// Wallet was debited, but commission status update failed - needs manual intervention
	}

	// Clear commission debt on rider profile
	if err := s.riderRepo.ClearCommissionDebt(ctx, profile.ID); err != nil {
		s.logger.Error("failed to clear commission debt", zap.Error(err))
	}

	// Record payment
	payment := &domain.CommissionPayment{
		RiderProfileID:  profile.ID,
		Amount:          totalOwed,
		CommissionsPaid: paidCount,
		PaymentMethod:   "wallet",
	}
	if err := s.paymentRepo.Create(ctx, payment); err != nil {
		s.logger.Error("failed to record commission payment", zap.Error(err))
	}

	// Get new wallet balance
	newWallet, _ := s.walletService.GetBalance(ctx, userID)
	newBalance := 0.0
	if newWallet != nil {
		newBalance = newWallet.AvailableBalance
	}

	s.logger.Info("rider paid commission",
		zap.String("user_id", userID.String()),
		zap.Float64("amount_paid", totalOwed),
		zap.Int("commissions_paid", paidCount),
	)

	return &CommissionPaymentResult{
		AmountPaid:       totalOwed,
		CommissionsPaid:  paidCount,
		NewWalletBalance: newBalance,
		IsNowUnblocked:   true,
	}, nil
}

// CheckAndUpdateBlockStatus checks if rider should be blocked and updates status
func (s *RiderCommissionServiceImpl) CheckAndUpdateBlockStatus(ctx context.Context, riderProfileID uuid.UUID) error {
	profile, err := s.riderRepo.GetByID(ctx, riderProfileID)
	if err != nil {
		return err
	}

	shouldBlock := profile.ShouldBlockForCommission()

	// Only update if status is changing
	if shouldBlock && !profile.IsCommissionBlocked {
		if err := s.riderRepo.SetCommissionBlocked(ctx, riderProfileID, true); err != nil {
			return fmt.Errorf("failed to block rider: %w", err)
		}
		s.logger.Warn("rider blocked for unpaid commission",
			zap.String("rider_profile_id", riderProfileID.String()),
			zap.Float64("unpaid_commission", profile.UnpaidCommission),
			zap.Int("unpaid_count", profile.UnpaidDeliveryCount),
		)
	} else if !shouldBlock && profile.IsCommissionBlocked {
		if err := s.riderRepo.SetCommissionBlocked(ctx, riderProfileID, false); err != nil {
			return fmt.Errorf("failed to unblock rider: %w", err)
		}
		s.logger.Info("rider unblocked after commission payment",
			zap.String("rider_profile_id", riderProfileID.String()),
		)
	}

	return nil
}

// WaiveCommissions waives all pending commissions (admin action)
func (s *RiderCommissionServiceImpl) WaiveCommissions(ctx context.Context, riderProfileID uuid.UUID, reason string) error {
	paidVia := fmt.Sprintf("admin_waive: %s", reason)

	_, err := s.commissionRepo.MarkAsWaived(ctx, riderProfileID, paidVia)
	if err != nil {
		return fmt.Errorf("failed to waive commissions: %w", err)
	}

	// Clear commission debt
	if err := s.riderRepo.ClearCommissionDebt(ctx, riderProfileID); err != nil {
		return fmt.Errorf("failed to clear commission debt: %w", err)
	}

	s.logger.Info("rider commissions waived",
		zap.String("rider_profile_id", riderProfileID.String()),
		zap.String("reason", reason),
	)

	return nil
}

// GetFeeBreakdown calculates and returns the fee breakdown for a delivery
func (s *RiderCommissionServiceImpl) GetFeeBreakdown(deliveryFee float64) *domain.RiderFeeBreakdown {
	return domain.NewRiderFeeBreakdown(deliveryFee)
}

// Ensure RiderCommissionServiceImpl implements RiderCommissionService
var _ RiderCommissionService = (*RiderCommissionServiceImpl)(nil)
