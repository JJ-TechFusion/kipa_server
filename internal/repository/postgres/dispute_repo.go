package postgres

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"kipa/internal/domain"
	"kipa/internal/repository"
)

type DisputeRepo struct {
	db *sqlx.DB
}

func NewDisputeRepo(db *sqlx.DB) *DisputeRepo {
	return &DisputeRepo{db: db}
}

func (r *DisputeRepo) Create(ctx context.Context, dispute *domain.Dispute) error {
	evidenceJSON, err := json.Marshal(dispute.Evidence)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO disputes (id, escrow_id, opened_by, reason, evidence, status, outcome, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err = r.db.ExecContext(ctx, query,
		dispute.ID,
		dispute.EscrowID,
		dispute.OpenedBy,
		dispute.Reason,
		evidenceJSON,
		dispute.Status,
		dispute.Outcome,
		dispute.CreatedAt,
	)

	return err
}

func (r *DisputeRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Dispute, error) {
	query := `
		SELECT id, escrow_id, opened_by, reason, evidence, status, outcome, 
		       resolution_notes, resolved_by, resolved_at, created_at
		FROM disputes
		WHERE id = $1
	`

	var dispute domain.Dispute
	var evidenceJSON []byte
	var resolutionNotes sql.NullString
	var resolvedBy sql.NullString
	var resolvedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&dispute.ID,
		&dispute.EscrowID,
		&dispute.OpenedBy,
		&dispute.Reason,
		&evidenceJSON,
		&dispute.Status,
		&dispute.Outcome,
		&resolutionNotes,
		&resolvedBy,
		&resolvedAt,
		&dispute.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if len(evidenceJSON) > 0 {
		// Try to unmarshal as []string first
		var evidence []string
		if err := json.Unmarshal(evidenceJSON, &evidence); err != nil {
			// If that fails, evidence might be stored differently or empty object
			// Just set to empty slice to avoid blocking
			dispute.Evidence = []string{}
		} else {
			dispute.Evidence = evidence
		}
	}

	if resolutionNotes.Valid {
		dispute.ResolutionNotes = &resolutionNotes.String
	}
	if resolvedBy.Valid {
		id, _ := uuid.Parse(resolvedBy.String)
		dispute.ResolvedBy = &id
	}
	if resolvedAt.Valid {
		dispute.ResolvedAt = &resolvedAt.Time
	}

	return &dispute, nil
}

func (r *DisputeRepo) GetByEscrowID(ctx context.Context, escrowID uuid.UUID) (*domain.Dispute, error) {
	query := `
		SELECT id, escrow_id, opened_by, reason, evidence, status, outcome, 
		       resolution_notes, resolved_by, resolved_at, created_at
		FROM disputes
		WHERE escrow_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`

	var dispute domain.Dispute
	var evidenceJSON []byte
	var resolutionNotes sql.NullString
	var resolvedBy sql.NullString
	var resolvedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, query, escrowID).Scan(
		&dispute.ID,
		&dispute.EscrowID,
		&dispute.OpenedBy,
		&dispute.Reason,
		&evidenceJSON,
		&dispute.Status,
		&dispute.Outcome,
		&resolutionNotes,
		&resolvedBy,
		&resolvedAt,
		&dispute.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if len(evidenceJSON) > 0 {
		var evidence []string
		if err := json.Unmarshal(evidenceJSON, &evidence); err == nil {
			dispute.Evidence = evidence
		} else {
			dispute.Evidence = []string{}
		}
	}

	if resolutionNotes.Valid {
		dispute.ResolutionNotes = &resolutionNotes.String
	}
	if resolvedBy.Valid {
		id, _ := uuid.Parse(resolvedBy.String)
		dispute.ResolvedBy = &id
	}
	if resolvedAt.Valid {
		dispute.ResolvedAt = &resolvedAt.Time
	}

	return &dispute, nil
}

func (r *DisputeRepo) List(ctx context.Context, filters *repository.DisputeFilters, limit, offset int) ([]*domain.Dispute, error) {
	query := `
		SELECT id, escrow_id, opened_by, reason, evidence, status, outcome,
		       resolution_notes, resolved_by, resolved_at, created_at
		FROM disputes
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var disputes []*domain.Dispute
	for rows.Next() {
		var dispute domain.Dispute
		var evidenceJSON []byte
		var resolutionNotes sql.NullString
		var resolvedBy sql.NullString
		var resolvedAt sql.NullTime

		err := rows.Scan(
			&dispute.ID,
			&dispute.EscrowID,
			&dispute.OpenedBy,
			&dispute.Reason,
			&evidenceJSON,
			&dispute.Status,
			&dispute.Outcome,
			&resolutionNotes,
			&resolvedBy,
			&resolvedAt,
			&dispute.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		if len(evidenceJSON) > 0 {
			var evidence []string
			if err := json.Unmarshal(evidenceJSON, &evidence); err == nil {
				dispute.Evidence = evidence
			} else {
				dispute.Evidence = []string{}
			}
		}

		if resolutionNotes.Valid {
			dispute.ResolutionNotes = &resolutionNotes.String
		}
		if resolvedBy.Valid {
			id, _ := uuid.Parse(resolvedBy.String)
			dispute.ResolvedBy = &id
		}
		if resolvedAt.Valid {
			dispute.ResolvedAt = &resolvedAt.Time
		}

		disputes = append(disputes, &dispute)
	}

	return disputes, rows.Err()
}

func (r *DisputeRepo) GetByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.Dispute, error) {
	query := `
		SELECT d.id, d.escrow_id, d.opened_by, d.reason, d.evidence, d.status, d.outcome,
		       d.resolution_notes, d.resolved_by, d.resolved_at, d.created_at
		FROM disputes d
		JOIN escrows e ON d.escrow_id = e.id
		WHERE e.buyer_id = $1 OR e.seller_id = $1
		ORDER BY d.created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var disputes []*domain.Dispute
	for rows.Next() {
		var dispute domain.Dispute
		var evidenceJSON []byte
		var resolutionNotes sql.NullString
		var resolvedBy sql.NullString
		var resolvedAt sql.NullTime

		err := rows.Scan(
			&dispute.ID,
			&dispute.EscrowID,
			&dispute.OpenedBy,
			&dispute.Reason,
			&evidenceJSON,
			&dispute.Status,
			&dispute.Outcome,
			&resolutionNotes,
			&resolvedBy,
			&resolvedAt,
			&dispute.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		if len(evidenceJSON) > 0 {
			var evidence []string
			if err := json.Unmarshal(evidenceJSON, &evidence); err == nil {
				dispute.Evidence = evidence
			} else {
				dispute.Evidence = []string{}
			}
		}

		if resolutionNotes.Valid {
			dispute.ResolutionNotes = &resolutionNotes.String
		}
		if resolvedBy.Valid {
			id, _ := uuid.Parse(resolvedBy.String)
			dispute.ResolvedBy = &id
		}
		if resolvedAt.Valid {
			dispute.ResolvedAt = &resolvedAt.Time
		}

		disputes = append(disputes, &dispute)
	}

	return disputes, rows.Err()
}

func (r *DisputeRepo) Update(ctx context.Context, dispute *domain.Dispute) error {
	evidenceJSON, err := json.Marshal(dispute.Evidence)
	if err != nil {
		return err
	}

	query := `
		UPDATE disputes
		SET status = $2, outcome = $3, resolution_notes = $4, resolved_by = $5, resolved_at = $6, evidence = $7
		WHERE id = $1
	`

	_, err = r.db.ExecContext(ctx, query,
		dispute.ID,
		dispute.Status,
		dispute.Outcome,
		dispute.ResolutionNotes,
		dispute.ResolvedBy,
		dispute.ResolvedAt,
		evidenceJSON,
	)

	return err
}

func (r *DisputeRepo) GetStatistics(ctx context.Context) (*repository.DisputeStatistics, error) {
	query := `
		SELECT 
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE status = 'open') as open_disputes,
			COUNT(*) FILTER (WHERE status = 'under_review') as in_review,
			COUNT(*) FILTER (WHERE status = 'resolved') as resolved,
			COUNT(*) FILTER (WHERE outcome = 'buyer_favor') as buyer_wins,
			COUNT(*) FILTER (WHERE outcome = 'seller_favor') as seller_wins,
			COUNT(*) FILTER (WHERE outcome = 'partial_refund') as partial_refunds
		FROM disputes
	`

	var stats repository.DisputeStatistics
	err := r.db.QueryRowContext(ctx, query).Scan(
		&stats.TotalDisputes,
		&stats.OpenDisputes,
		&stats.InReviewDisputes,
		&stats.ResolvedDisputes,
		&stats.BuyerWins,
		&stats.SellerWins,
		&stats.PartialRefunds,
	)

	if err != nil {
		return nil, err
	}

	return &stats, nil
}

var _ repository.DisputeRepository = (*DisputeRepo)(nil)