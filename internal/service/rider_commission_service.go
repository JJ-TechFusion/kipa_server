package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/domain"
	"kipa/internal/repository"
)
type RiderCommissionService interface {
	RecordCommission(ctx context.Context, riderProfileID, deliveryJobID uuid.UUID, deliveryFee float64) error
	GetSummary(ctx context.Context, userID uuid.UUID) (*CommissionSummary, error)
	GetPendingCommissions(ctx context.Context, userID uuid.UUID) ([]*domain.RiderCommission, error)
	PayAllFromWallet(ctx context.Context, userID uuid.UUID) (*CommissionPaymentResult, error)
	CheckAndUpdateBlockStatus(ctx context.Context, riderProfileID uuid.UUID) error
	WaiveCommissions(ctx context.Context, riderProfileID uuid.UUID, reason string) error
	GetFeeBreakdown(ctx context.Context, deliveryFee float64) *domain.RiderFeeBreakdown
}
type CommissionSummary struct {
	TotalOwed         float64 `json:"total_owed"`
	UnpaidRideCount   int     `json:"unpaid_ride_count"`
	IsBlocked         bool    `json:"is_blocked"`
	RidesUntilBlocked int     `json:"rides_until_blocked"`
	GracePeriodRides  int     `json:"grace_period_rides"`
	CommissionRate    float64 `json:"commission_rate"`
}
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
	settingsRepo   repository.PlatformSettingsRepository
	walletService  WalletService
	logger         *zap.Logger
}

func NewRiderCommissionService(
	commissionRepo repository.RiderCommissionRepository,
	paymentRepo repository.CommissionPaymentRepository,
	riderRepo repository.RiderProfileRepository,
	settingsRepo repository.PlatformSettingsRepository,
	walletService WalletService,
	logger *zap.Logger,
) RiderCommissionService {
	return &RiderCommissionServiceImpl{
		commissionRepo: commissionRepo,
		paymentRepo:    paymentRepo,
		riderRepo:      riderRepo,
		settingsRepo:   settingsRepo,
		walletService:  walletService,
		logger:         logger,
	}
}

func (s *RiderCommissionServiceImpl) getCommissionSettings(ctx context.Context) *domain.CommissionSettings {
	// Default settings
	defaults := &domain.CommissionSettings{
		MaxUnpaidRides: domain.MaxUnpaidRidesBeforeBlock,
		CommissionRate: domain.RiderCommissionRate,
	}

	if s.settingsRepo == nil {
		return defaults
	}

	data, err := s.settingsRepo.Get(ctx, "commission_settings")
	if err != nil || data == nil {
		return defaults
	}

	var settings domain.CommissionSettings
	if err := json.Unmarshal(data.SettingValue, &settings); err != nil {
		s.logger.Warn("failed to parse commission settings, using defaults", zap.Error(err))
		return defaults
	}

	return &settings
}

func (s *RiderCommissionServiceImpl) RecordCommission(ctx context.Context, riderProfileID, deliveryJobID uuid.UUID, deliveryFee float64) error {
	settings := s.getCommissionSettings(ctx)
	commission := domain.NewRiderCommission(riderProfileID, deliveryJobID, deliveryFee, settings.CommissionRate)

	if err := s.commissionRepo.Create(ctx, commission); err != nil {
		return fmt.Errorf("failed to create commission record: %w", err)
	}
	totalOwed, count, err := s.commissionRepo.GetPendingTotals(ctx, riderProfileID)
	if err != nil {
		s.logger.Error("failed to get pending totals", zap.Error(err))
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
func (s *RiderCommissionServiceImpl) GetSummary(ctx context.Context, userID uuid.UUID) (*CommissionSummary, error) {
	profile, err := s.riderRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	settings := s.getCommissionSettings(ctx)

	ridesUntilBlocked := settings.MaxUnpaidRides - profile.UnpaidDeliveryCount
	if ridesUntilBlocked < 0 {
		ridesUntilBlocked = 0
	}

	return &CommissionSummary{
		TotalOwed:         profile.UnpaidCommission,
		UnpaidRideCount:   profile.UnpaidDeliveryCount,
		IsBlocked:         profile.IsCommissionBlocked,
		RidesUntilBlocked: ridesUntilBlocked,
		GracePeriodRides:  settings.MaxUnpaidRides,
		CommissionRate:    settings.CommissionRate,
	}, nil
}
func (s *RiderCommissionServiceImpl) GetPendingCommissions(ctx context.Context, userID uuid.UUID) ([]*domain.RiderCommission, error) {
	profile, err := s.riderRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return s.commissionRepo.GetPendingByRiderID(ctx, profile.ID)
}
func (s *RiderCommissionServiceImpl) PayAllFromWallet(ctx context.Context, userID uuid.UUID) (*CommissionPaymentResult, error) {
	profile, err := s.riderRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	totalOwed, count, err := s.commissionRepo.GetPendingTotals(ctx, profile.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending totals: %w", err)
	}

	if count == 0 {
		return nil, domain.ErrNoCommissionOwed
	}
	wallet, err := s.walletService.GetBalance(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet balance: %w", err)
	}

	if wallet.AvailableBalance < totalOwed {
		return nil, domain.ErrInsufficientBalance
	}
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
	paidCount, err := s.commissionRepo.MarkAllPendingAsPaid(ctx, profile.ID, "wallet")
	if err != nil {
		s.logger.Error("failed to mark commissions as paid", zap.Error(err))
	}
	if err := s.riderRepo.ClearCommissionDebt(ctx, profile.ID); err != nil {
		s.logger.Error("failed to clear commission debt", zap.Error(err))
	}
	payment := &domain.CommissionPayment{
		RiderProfileID:  profile.ID,
		Amount:          totalOwed,
		CommissionsPaid: paidCount,
		PaymentMethod:   "wallet",
	}
	if err := s.paymentRepo.Create(ctx, payment); err != nil {
		s.logger.Error("failed to record commission payment", zap.Error(err))
	}
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
func (s *RiderCommissionServiceImpl) CheckAndUpdateBlockStatus(ctx context.Context, riderProfileID uuid.UUID) error {
	profile, err := s.riderRepo.GetByID(ctx, riderProfileID)
	if err != nil {
		return err
	}

	settings := s.getCommissionSettings(ctx)
	shouldBlock := profile.UnpaidDeliveryCount >= settings.MaxUnpaidRides

	if shouldBlock && !profile.IsCommissionBlocked {
		if err := s.riderRepo.SetCommissionBlocked(ctx, riderProfileID, true); err != nil {
			return fmt.Errorf("failed to block rider: %w", err)
		}
		s.logger.Warn("rider blocked for unpaid commission",
			zap.String("rider_profile_id", riderProfileID.String()),
			zap.Float64("unpaid_commission", profile.UnpaidCommission),
			zap.Int("unpaid_count", profile.UnpaidDeliveryCount),
			zap.Int("max_unpaid_rides", settings.MaxUnpaidRides),
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
func (s *RiderCommissionServiceImpl) WaiveCommissions(ctx context.Context, riderProfileID uuid.UUID, reason string) error {
	paidVia := fmt.Sprintf("admin_waive: %s", reason)

	_, err := s.commissionRepo.MarkAsWaived(ctx, riderProfileID, paidVia)
	if err != nil {
		return fmt.Errorf("failed to waive commissions: %w", err)
	}
	if err := s.riderRepo.ClearCommissionDebt(ctx, riderProfileID); err != nil {
		return fmt.Errorf("failed to clear commission debt: %w", err)
	}

	s.logger.Info("rider commissions waived",
		zap.String("rider_profile_id", riderProfileID.String()),
		zap.String("reason", reason),
	)

	return nil
}
func (s *RiderCommissionServiceImpl) GetFeeBreakdown(ctx context.Context, deliveryFee float64) *domain.RiderFeeBreakdown {
	settings := s.getCommissionSettings(ctx)
	return domain.NewRiderFeeBreakdown(deliveryFee, settings.CommissionRate)
}
var _ RiderCommissionService = (*RiderCommissionServiceImpl)(nil)
