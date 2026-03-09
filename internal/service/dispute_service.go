package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/domain"
	"kipa/internal/repository"
)

type DisputeService interface {
	ListForUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*DisputeWithDetails, error)
	GetByIDForUser(ctx context.Context, userID uuid.UUID, disputeID uuid.UUID) (*DisputeWithDetails, error)
}

type DisputeWithDetails struct {
	*domain.Dispute
	Role             string                 `json:"role"` // "buyer" or "seller"
	CounterPartyName *string                `json:"counter_party_name,omitempty"`
	PaymentRequest   *domain.PaymentRequest `json:"payment_request,omitempty"`
	Escrow           *domain.Escrow         `json:"escrow,omitempty"`
}

type DisputeServiceImpl struct {
	disputeRepo repository.DisputeRepository
	escrowRepo  repository.EscrowRepository
	userRepo    repository.UserRepository
	prRepo      repository.PaymentRequestRepository
	logger      *zap.Logger
}

func NewDisputeService(
	disputeRepo repository.DisputeRepository,
	escrowRepo repository.EscrowRepository,
	userRepo repository.UserRepository,
	prRepo repository.PaymentRequestRepository,
	logger *zap.Logger,
) DisputeService {
	return &DisputeServiceImpl{
		disputeRepo: disputeRepo,
		escrowRepo:  escrowRepo,
		userRepo:    userRepo,
		prRepo:      prRepo,
		logger:      logger,
	}
}

func (s *DisputeServiceImpl) ListForUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*DisputeWithDetails, error) {
	disputes, err := s.disputeRepo.GetByUserID(ctx, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get disputes: %w", err)
	}

	result := make([]*DisputeWithDetails, 0, len(disputes))
	for _, dispute := range disputes {
		details, err := s.enrichDispute(ctx, dispute, userID)
		if err != nil {
			s.logger.Warn("failed to enrich dispute",
				zap.String("dispute_id", dispute.ID.String()),
				zap.Error(err),
			)
			details = &DisputeWithDetails{Dispute: dispute}
		}
		result = append(result, details)
	}

	return result, nil
}

func (s *DisputeServiceImpl) GetByIDForUser(ctx context.Context, userID uuid.UUID, disputeID uuid.UUID) (*DisputeWithDetails, error) {
	dispute, err := s.disputeRepo.GetByID(ctx, disputeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get dispute: %w", err)
	}
	if dispute == nil {
		return nil, domain.ErrDisputeNotFound
	}
	escrow, err := s.escrowRepo.GetByID(ctx, dispute.EscrowID)
	if err != nil {
		return nil, fmt.Errorf("failed to get escrow: %w", err)
	}
	if escrow == nil {
		return nil, fmt.Errorf("escrow not found for dispute")
	}

	if escrow.BuyerID != userID && escrow.SellerID != userID {
		return nil, domain.ErrUnauthorized
	}

	return s.enrichDispute(ctx, dispute, userID)
}

func (s *DisputeServiceImpl) enrichDispute(ctx context.Context, dispute *domain.Dispute, userID uuid.UUID) (*DisputeWithDetails, error) {
	escrow, err := s.escrowRepo.GetByID(ctx, dispute.EscrowID)
	if err != nil {
		return nil, err
	}
	if escrow == nil {
		return &DisputeWithDetails{Dispute: dispute}, nil
	}

	details := &DisputeWithDetails{
		Dispute: dispute,
		Escrow:  escrow,
	}
	if escrow.BuyerID == userID {
		details.Role = "buyer"
		if seller, err := s.userRepo.GetByID(ctx, escrow.SellerID); err == nil && seller != nil {
			name := formatUserName(seller)
			details.CounterPartyName = &name
		}
	} else {
		details.Role = "seller"
		if buyer, err := s.userRepo.GetByID(ctx, escrow.BuyerID); err == nil && buyer != nil {
			name := formatUserName(buyer)
			details.CounterPartyName = &name
		}
	}
	if pr, err := s.prRepo.GetByID(ctx, escrow.PaymentRequestID); err == nil && pr != nil {
		details.PaymentRequest = pr
	}

	return details, nil
}

func formatUserName(user *domain.User) string {
	if user.FirstName != nil && user.LastName != nil {
		return *user.FirstName + " " + *user.LastName
	}
	if user.FirstName != nil {
		return *user.FirstName
	}
	return user.PhoneNumber
}

var _ DisputeService = (*DisputeServiceImpl)(nil)
