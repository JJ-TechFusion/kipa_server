package repository

import (
	"context"

	"github.com/google/uuid"

	"kipa/internal/domain"
)

type DisputeRepository interface {
	Create(ctx context.Context, dispute *domain.Dispute) error

	GetByID(ctx context.Context, id uuid.UUID) (*domain.Dispute, error)

	GetByEscrowID(ctx context.Context, escrowID uuid.UUID) (*domain.Dispute, error)

	List(ctx context.Context, filters *DisputeFilters, limit, offset int) ([]*domain.Dispute, error)

	// GetByUserID returns disputes where user is buyer or seller on the associated escrow
	GetByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.Dispute, error)

	Update(ctx context.Context, dispute *domain.Dispute) error

	GetStatistics(ctx context.Context) (*DisputeStatistics, error)
}

type DisputeFilters struct {
	Status   *domain.DisputeStatus
	Outcome  *domain.DisputeOutcome
	OpenedBy *uuid.UUID
}

type DisputeStatistics struct {
	TotalDisputes      int `json:"total_disputes"`
	OpenDisputes       int `json:"open_disputes"`
	InReviewDisputes   int `json:"in_review_disputes"`
	ResolvedDisputes   int `json:"resolved_disputes"`
	BuyerWins          int `json:"buyer_wins"`
	SellerWins         int `json:"seller_wins"`
	PartialRefunds     int `json:"partial_refunds"`
}
