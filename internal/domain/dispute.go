package domain

import (
	"time"

	"github.com/google/uuid"
)

// DisputeStatus represents the state of a dispute
type DisputeStatus string

const (
	DisputeStatusOpen       DisputeStatus = "open"
	DisputeStatusInReview   DisputeStatus = "under_review"
	DisputeStatusResolved   DisputeStatus = "resolved"
	DisputeStatusClosed     DisputeStatus = "closed"
)

// DisputeOutcome represents the resolution outcome
type DisputeOutcome string

const (
	DisputeOutcomePending      DisputeOutcome = "pending"
	DisputeOutcomeBuyerWins    DisputeOutcome = "buyer_favor"
	DisputeOutcomeSellerWins   DisputeOutcome = "seller_favor"
	DisputeOutcomePartialRefund DisputeOutcome = "partial_refund"
	DisputeOutcomeMutual       DisputeOutcome = "mutual"
)

// Dispute represents a buyer-seller dispute
type Dispute struct {
	ID              uuid.UUID       `db:"id" json:"id"`
	EscrowID        uuid.UUID       `db:"escrow_id" json:"escrow_id"`
	OpenedBy        uuid.UUID       `db:"opened_by" json:"opened_by"`
	Reason          string          `db:"reason" json:"reason"`
	Evidence        []string        `db:"-" json:"evidence"` // Stored as JSONB
	Status          DisputeStatus   `db:"status" json:"status"`
	Outcome         DisputeOutcome  `db:"outcome" json:"outcome"`
	ResolutionNotes *string         `db:"resolution_notes" json:"resolution_notes,omitempty"`
	ResolvedBy      *uuid.UUID      `db:"resolved_by" json:"resolved_by,omitempty"`
	ResolvedAt      *time.Time      `db:"resolved_at" json:"resolved_at,omitempty"`
	CreatedAt       time.Time       `db:"created_at" json:"created_at"`
}

// NewDispute creates a new dispute
func NewDispute(escrowID, openedBy uuid.UUID, reason string, evidence []string) *Dispute {
	return &Dispute{
		ID:        uuid.New(),
		EscrowID:  escrowID,
		OpenedBy:  openedBy,
		Reason:    reason,
		Evidence:  evidence,
		Status:    DisputeStatusOpen,
		Outcome:   DisputeOutcomePending,
		CreatedAt: time.Now(),
	}
}

// CanResolve checks if the dispute can be resolved
func (d *Dispute) CanResolve() bool {
	return d.Status == DisputeStatusOpen || d.Status == DisputeStatusInReview
}

// Resolve resolves the dispute with the given outcome
func (d *Dispute) Resolve(adminID uuid.UUID, outcome DisputeOutcome, notes string) {
	now := time.Now()
	d.Status = DisputeStatusResolved
	d.Outcome = outcome
	d.ResolvedBy = &adminID
	d.ResolvedAt = &now
	d.ResolutionNotes = &notes
}

// DisputeDetails includes related entities for display
type DisputeDetails struct {
	Dispute        *Dispute        `json:"dispute"`
	Escrow         *Escrow         `json:"escrow"`
	PaymentRequest *PaymentRequest `json:"payment_request,omitempty"`
	Buyer          *User           `json:"buyer"`
	Seller         *User           `json:"seller"`
}

// RefundRequest represents a refund action
type RefundRequest struct {
	DisputeID     uuid.UUID `json:"dispute_id"`
	Amount        float64   `json:"amount"`
	RefundType    string    `json:"refund_type"` // "full" or "partial"
	Reason        string    `json:"reason"`
	ProcessedBy   uuid.UUID `json:"processed_by"`
}