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

type EscrowService interface {
	CreateAndFund(ctx context.Context, req *CreateEscrowRequest) (*domain.Escrow, error)

	GetByID(ctx context.Context, id uuid.UUID) (*domain.Escrow, error)

	GetByPaymentRequestID(ctx context.Context, prID uuid.UUID) (*domain.Escrow, error)

	GetByBuyerID(ctx context.Context, buyerID uuid.UUID, limit, offset int) ([]*domain.Escrow, error)

	GetBySellerID(ctx context.Context, sellerID uuid.UUID, limit, offset int) ([]*domain.Escrow, error)

	MarkDeliveryInProgress(ctx context.Context, escrowID uuid.UUID) error

	StartConfirmationWindow(ctx context.Context, escrowID uuid.UUID, actualDeliveryFee float64) error

	Release(ctx context.Context, escrowID uuid.UUID) error

	Dispute(ctx context.Context, escrowID uuid.UUID, reason string) error

	Refund(ctx context.Context, escrowID uuid.UUID, reason string) error

	ProcessPendingConfirmationWindows(ctx context.Context) error
}

type CreateEscrowRequest struct {
	PaymentRequestID    uuid.UUID
	BuyerID             uuid.UUID
	SellerID            uuid.UUID
	ItemAmount          float64
	DeliveryFeeEstimate float64
	BuyerFee            float64
	PaymentReference    string
	PaymentProvider     string
}

type EscrowServiceImpl struct {
	escrowRepo    repository.EscrowRepository
	walletService WalletService
	feeCalculator FeeCalculatorService
	config        *config.Config
	logger        *zap.Logger
}

func NewEscrowService(
	escrowRepo repository.EscrowRepository,
	walletService WalletService,
	feeCalculator FeeCalculatorService,
	cfg *config.Config,
	logger *zap.Logger,
) EscrowService {
	return &EscrowServiceImpl{
		escrowRepo:    escrowRepo,
		walletService: walletService,
		feeCalculator: feeCalculator,
		config:        cfg,
		logger:        logger,
	}
}

// CreateAndFund creates a new escrow and marks it as funded
func (s *EscrowServiceImpl) CreateAndFund(ctx context.Context, req *CreateEscrowRequest) (*domain.Escrow, error) {
	// Delivery fee is paid efficiently to rider on delivery
	totalLocked := req.ItemAmount + req.BuyerFee

	escrow := &domain.Escrow{
		PaymentRequestID:    req.PaymentRequestID,
		BuyerID:             req.BuyerID,
		SellerID:            req.SellerID,
		ItemAmount:          req.ItemAmount,
		DeliveryFeeEstimate: req.DeliveryFeeEstimate,
		BuyerFee:            req.BuyerFee,
		TotalLocked:         totalLocked,
		PaymentReference:    &req.PaymentReference,
		PaymentProvider:     &req.PaymentProvider,
		Status:              domain.EscrowStatusFunded,
	}

	now := time.Now()
	escrow.FundedAt = &now

	if err := s.escrowRepo.Create(ctx, escrow); err != nil {
		return nil, fmt.Errorf("failed to create escrow: %w", err)
	}

	s.logger.Info("escrow created and funded",
		zap.String("escrow_id", escrow.ID.String()),
		zap.String("pr_id", req.PaymentRequestID.String()),
		zap.Float64("total_locked", totalLocked),
	)

	return escrow, nil
}

func (s *EscrowServiceImpl) GetByID(ctx context.Context, id uuid.UUID) (*domain.Escrow, error) {
	return s.escrowRepo.GetByID(ctx, id)
}

// GetByPaymentRequestID gets an escrow by payment request ID
func (s *EscrowServiceImpl) GetByPaymentRequestID(ctx context.Context, prID uuid.UUID) (*domain.Escrow, error) {
	return s.escrowRepo.GetByPaymentRequestID(ctx, prID)
}

// GetByBuyerID gets all escrows for a buyer
func (s *EscrowServiceImpl) GetByBuyerID(ctx context.Context, buyerID uuid.UUID, limit, offset int) ([]*domain.Escrow, error) {
	return s.escrowRepo.GetByBuyerID(ctx, buyerID, limit, offset)
}

// GetBySellerID gets all escrows for a seller
func (s *EscrowServiceImpl) GetBySellerID(ctx context.Context, sellerID uuid.UUID, limit, offset int) ([]*domain.Escrow, error) {
	return s.escrowRepo.GetBySellerID(ctx, sellerID, limit, offset)
}

// MarkDeliveryInProgress marks escrow as delivery in progress (rider picked up item)
func (s *EscrowServiceImpl) MarkDeliveryInProgress(ctx context.Context, escrowID uuid.UUID) error {
	escrow, err := s.escrowRepo.GetByID(ctx, escrowID)
	if err != nil {
		return err
	}

	if escrow.Status != domain.EscrowStatusFunded {
		return domain.ErrInvalidStatusTransition
	}

	escrow.Status = domain.EscrowStatusDeliveryInProgress

	if err := s.escrowRepo.Update(ctx, escrow); err != nil {
		return fmt.Errorf("failed to update escrow: %w", err)
	}

	s.logger.Info("escrow marked as delivery in progress",
		zap.String("escrow_id", escrowID.String()),
	)

	return nil
}

// StartConfirmationWindow marks escrow as delivered and starts the 24h confirmation window
func (s *EscrowServiceImpl) StartConfirmationWindow(ctx context.Context, escrowID uuid.UUID, actualDeliveryFee float64) error {
	escrow, err := s.escrowRepo.GetByID(ctx, escrowID)
	if err != nil {
		return err
	}

	if escrow.Status != domain.EscrowStatusFunded && escrow.Status != domain.EscrowStatusDeliveryInProgress {
		return domain.ErrInvalidStatusTransition
	}

	now := time.Now()
	confirmationWindowEnds := now.Add(24 * time.Hour)

	escrow.ActualDeliveryFee = &actualDeliveryFee
	escrow.DeliveryFeeAdjustment = s.calculateAdjustment(escrow.DeliveryFeeEstimate, actualDeliveryFee)
	escrow.DeliveryConfirmedAt = &now
	escrow.ConfirmationWindowEndsAt = &confirmationWindowEnds
	escrow.Status = domain.EscrowStatusConfirmationWindow

	// Pre-calculate payouts
	settlement := s.feeCalculator.CalculateSettlement(escrow.ItemAmount, actualDeliveryFee)
	escrow.SellerPayout = &settlement.SellerPayout
	escrow.RiderPayout = &settlement.RiderPayout
	escrow.PlatformRevenue = &settlement.PlatformRevenue

	if err := s.escrowRepo.Update(ctx, escrow); err != nil {
		return fmt.Errorf("failed to update escrow: %w", err)
	}

	s.logger.Info("escrow confirmation window started",
		zap.String("escrow_id", escrowID.String()),
		zap.Time("window_ends_at", confirmationWindowEnds),
	)

	return nil
}

// Release releases the escrow funds to seller and rider
func (s *EscrowServiceImpl) Release(ctx context.Context, escrowID uuid.UUID) error {
	escrow, err := s.escrowRepo.GetByID(ctx, escrowID)
	if err != nil {
		return err
	}

	if escrow.Status != domain.EscrowStatusConfirmationWindow {
		return domain.ErrInvalidStatusTransition
	}

	// Credit seller wallet
	if escrow.SellerPayout != nil && *escrow.SellerPayout > 0 {
		idempotencyKey := fmt.Sprintf("escrow_release_seller_%s", escrowID.String())
		_, err := s.walletService.Credit(
			ctx,
			escrow.SellerID,
			*escrow.SellerPayout,
			domain.WalletTxEscrowRelease,
			"escrow",
			escrowID,
			fmt.Sprintf("Payment for order %s", escrow.PaymentRequestID.String()),
			idempotencyKey,
		)
		if err != nil {
			s.logger.Error("failed to credit seller wallet",
				zap.Error(err),
				zap.String("escrow_id", escrowID.String()),
			)
			return fmt.Errorf("failed to credit seller: %w", err)
		}
	}

	// Update escrow status
	now := time.Now()
	escrow.ReleasedAt = &now
	escrow.Status = domain.EscrowStatusReleased

	if err := s.escrowRepo.Update(ctx, escrow); err != nil {
		return fmt.Errorf("failed to update escrow: %w", err)
	}

	s.logger.Info("escrow released",
		zap.String("escrow_id", escrowID.String()),
		zap.Float64("seller_payout", *escrow.SellerPayout),
	)

	return nil
}

// Dispute marks the escrow as disputed
func (s *EscrowServiceImpl) Dispute(ctx context.Context, escrowID uuid.UUID, reason string) error {
	escrow, err := s.escrowRepo.GetByID(ctx, escrowID)
	if err != nil {
		return err
	}

	// Can only dispute during confirmation window or delivery phase
	if escrow.Status != domain.EscrowStatusConfirmationWindow &&
		escrow.Status != domain.EscrowStatusDelivered &&
		escrow.Status != domain.EscrowStatusDeliveryInProgress {
		return domain.ErrCannotDispute
	}

	escrow.Status = domain.EscrowStatusDisputed

	if err := s.escrowRepo.Update(ctx, escrow); err != nil {
		return fmt.Errorf("failed to update escrow: %w", err)
	}

	s.logger.Info("escrow disputed",
		zap.String("escrow_id", escrowID.String()),
		zap.String("reason", reason),
	)

	return nil
}

// Refund refunds the escrow to the buyer
func (s *EscrowServiceImpl) Refund(ctx context.Context, escrowID uuid.UUID, reason string) error {
	escrow, err := s.escrowRepo.GetByID(ctx, escrowID)
	if err != nil {
		return err
	}

	// Can only refund from certain states
	validStates := []domain.EscrowStatus{
		domain.EscrowStatusFunded,
		domain.EscrowStatusDisputed,
	}

	validState := false
	for _, s := range validStates {
		if escrow.Status == s {
			validState = true
			break
		}
	}
	if !validState {
		return domain.ErrInvalidStatusTransition
	}

	// Credit buyer wallet with refund
	idempotencyKey := fmt.Sprintf("escrow_refund_%s", escrowID.String())
	_, err = s.walletService.Credit(
		ctx,
		escrow.BuyerID,
		escrow.TotalLocked,
		domain.WalletTxRefund,
		"escrow",
		escrowID,
		fmt.Sprintf("Refund for order: %s", reason),
		idempotencyKey,
	)
	if err != nil {
		return fmt.Errorf("failed to refund buyer: %w", err)
	}

	escrow.Status = domain.EscrowStatusRefunded

	if err := s.escrowRepo.Update(ctx, escrow); err != nil {
		return fmt.Errorf("failed to update escrow: %w", err)
	}

	s.logger.Info("escrow refunded",
		zap.String("escrow_id", escrowID.String()),
		zap.String("reason", reason),
		zap.Float64("refund_amount", escrow.TotalLocked),
	)

	return nil
}

// ProcessPendingConfirmationWindows processes all escrows past their confirmation window
func (s *EscrowServiceImpl) ProcessPendingConfirmationWindows(ctx context.Context) error {
	escrows, err := s.escrowRepo.GetPendingConfirmationWindow(ctx)
	if err != nil {
		return fmt.Errorf("failed to get pending escrows: %w", err)
	}

	for _, escrow := range escrows {
		if err := s.Release(ctx, escrow.ID); err != nil {
			s.logger.Error("failed to auto-release escrow",
				zap.Error(err),
				zap.String("escrow_id", escrow.ID.String()),
			)
			continue
		}
	}

	if len(escrows) > 0 {
		s.logger.Info("processed pending confirmation windows",
			zap.Int("count", len(escrows)),
		)
	}

	return nil
}

// calculateAdjustment calculates the difference between estimated and actual delivery fee
func (s *EscrowServiceImpl) calculateAdjustment(estimated, actual float64) *float64 {
	adjustment := actual - estimated
	return &adjustment
}

// Ensure EscrowServiceImpl implements EscrowService
var _ EscrowService = (*EscrowServiceImpl)(nil)
