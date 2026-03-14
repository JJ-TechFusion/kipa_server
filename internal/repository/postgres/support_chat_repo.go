package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"kipa/internal/domain"
	"kipa/internal/repository"
)

type SupportConversationRepo struct {
	db *sqlx.DB
}

func NewSupportConversationRepo(db *sqlx.DB) repository.SupportConversationRepository {
	return &SupportConversationRepo{db: db}
}

func (r *SupportConversationRepo) Create(ctx context.Context, conversation *domain.SupportConversation) error {
	query := `
		INSERT INTO support_conversations (id, user_id, dispute_id, status, last_message_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	if conversation.ID == uuid.Nil {
		conversation.ID = uuid.New()
	}
	if conversation.CreatedAt.IsZero() {
		conversation.CreatedAt = time.Now()
	}
	if conversation.Status == "" {
		conversation.Status = domain.SupportConversationStatusOpen
	}

	_, err := r.db.ExecContext(ctx, query,
		conversation.ID,
		conversation.UserID,
		conversation.DisputeID,
		conversation.Status,
		conversation.LastMessageAt,
		conversation.CreatedAt,
	)
	return err
}

func (r *SupportConversationRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.SupportConversation, error) {
	query := `SELECT * FROM support_conversations WHERE id = $1`
	var conversation domain.SupportConversation
	if err := r.db.GetContext(ctx, &conversation, query, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &conversation, nil
}

func (r *SupportConversationRepo) GetByDisputeID(ctx context.Context, disputeID uuid.UUID) (*domain.SupportConversation, error) {
	query := `SELECT * FROM support_conversations WHERE dispute_id = $1`
	var conversation domain.SupportConversation
	if err := r.db.GetContext(ctx, &conversation, query, disputeID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &conversation, nil
}

func (r *SupportConversationRepo) GetOpenByUserID(ctx context.Context, userID uuid.UUID) (*domain.SupportConversation, error) {
	query := `SELECT * FROM support_conversations WHERE user_id = $1 AND status = 'open' ORDER BY created_at DESC LIMIT 1`
	var conversation domain.SupportConversation
	if err := r.db.GetContext(ctx, &conversation, query, userID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &conversation, nil
}

func (r *SupportConversationRepo) GetOpenGeneralByUserID(ctx context.Context, userID uuid.UUID) (*domain.SupportConversation, error) {
	query := `SELECT * FROM support_conversations WHERE user_id = $1 AND status = 'open' AND dispute_id IS NULL ORDER BY created_at DESC LIMIT 1`
	var conversation domain.SupportConversation
	if err := r.db.GetContext(ctx, &conversation, query, userID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &conversation, nil
}

func (r *SupportConversationRepo) GetAllByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.SupportConversation, error) {
	query := `SELECT * FROM support_conversations WHERE user_id = $1 AND status = 'open' ORDER BY last_message_at DESC NULLS LAST, created_at DESC`
	var conversations []*domain.SupportConversation
	if err := r.db.SelectContext(ctx, &conversations, query, userID); err != nil {
		return nil, err
	}
	return conversations, nil
}

func (r *SupportConversationRepo) List(ctx context.Context, status *domain.SupportConversationStatus, limit, offset int) ([]*domain.SupportConversation, int, error) {
	var conversations []*domain.SupportConversation
	var total int

	countQuery := `SELECT COUNT(*) FROM support_conversations`
	listQuery := `SELECT * FROM support_conversations`

	if status != nil {
		countQuery += ` WHERE status = $1`
		listQuery += ` WHERE status = $1 ORDER BY last_message_at DESC NULLS LAST, created_at DESC LIMIT $2 OFFSET $3`

		if err := r.db.GetContext(ctx, &total, countQuery, *status); err != nil {
			return nil, 0, err
		}
		if err := r.db.SelectContext(ctx, &conversations, listQuery, *status, limit, offset); err != nil {
			return nil, 0, err
		}
	} else {
		if err := r.db.GetContext(ctx, &total, countQuery); err != nil {
			return nil, 0, err
		}
		listQuery += ` ORDER BY last_message_at DESC NULLS LAST, created_at DESC LIMIT $1 OFFSET $2`
		if err := r.db.SelectContext(ctx, &conversations, listQuery, limit, offset); err != nil {
			return nil, 0, err
		}
	}

	return conversations, total, nil
}

func (r *SupportConversationRepo) Update(ctx context.Context, conversation *domain.SupportConversation) error {
	query := `
		UPDATE support_conversations
		SET status = $1, last_message_at = $2
		WHERE id = $3
	`
	_, err := r.db.ExecContext(ctx, query, conversation.Status, conversation.LastMessageAt, conversation.ID)
	return err
}

func (r *SupportConversationRepo) UpdateLastMessageAt(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE support_conversations SET last_message_at = $1 WHERE id = $2`
	_, err := r.db.ExecContext(ctx, query, time.Now(), id)
	return err
}

type SupportMessageRepo struct {
	db *sqlx.DB
}

func NewSupportMessageRepo(db *sqlx.DB) repository.SupportMessageRepository {
	return &SupportMessageRepo{db: db}
}

func (r *SupportMessageRepo) Create(ctx context.Context, message *domain.SupportMessage) error {
	query := `
		INSERT INTO support_messages (id, conversation_id, sender_id, is_admin, content, media_url, media_type, status, created_at, read_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	if message.ID == uuid.Nil {
		message.ID = uuid.New()
	}
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now()
	}
	if message.Status == "" {
		message.Status = domain.MessageStatusSent
	}
	if message.MediaType == "" {
		message.MediaType = string(domain.MediaTypeText)
	}

	_, err := r.db.ExecContext(ctx, query,
		message.ID,
		message.ConversationID,
		message.SenderID,
		message.IsAdmin,
		message.Content,
		message.MediaURL,
		message.MediaType,
		message.Status,
		message.CreatedAt,
		message.ReadAt,
	)
	return err
}

func (r *SupportMessageRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.SupportMessage, error) {
	query := `SELECT * FROM support_messages WHERE id = $1`
	var message domain.SupportMessage
	if err := r.db.GetContext(ctx, &message, query, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &message, nil
}

func (r *SupportMessageRepo) GetByConversationID(ctx context.Context, conversationID uuid.UUID, limit int, beforeID *uuid.UUID) ([]*domain.SupportMessage, error) {
	var messages []*domain.SupportMessage
	var err error

	if beforeID != nil {
		query := `
			SELECT * FROM support_messages
			WHERE conversation_id = $1 AND created_at < (SELECT created_at FROM support_messages WHERE id = $2)
			ORDER BY created_at DESC
			LIMIT $3
		`
		err = r.db.SelectContext(ctx, &messages, query, conversationID, *beforeID, limit)
	} else {
		query := `
			SELECT * FROM support_messages
			WHERE conversation_id = $1
			ORDER BY created_at DESC
			LIMIT $2
		`
		err = r.db.SelectContext(ctx, &messages, query, conversationID, limit)
	}

	return messages, err
}

func (r *SupportMessageRepo) MarkRead(ctx context.Context, conversationID uuid.UUID, isAdmin bool) error {
	query := `
		UPDATE support_messages
		SET status = $1, read_at = $2
		WHERE conversation_id = $3 AND is_admin = $4 AND status != $1
	`
	_, err := r.db.ExecContext(ctx, query, domain.MessageStatusRead, time.Now(), conversationID, !isAdmin)
	return err
}

func (r *SupportMessageRepo) GetUnreadCount(ctx context.Context, conversationID uuid.UUID, isAdmin bool) (int, error) {
	query := `SELECT COUNT(*) FROM support_messages WHERE conversation_id = $1 AND is_admin = $2 AND status != $3`
	var count int
	err := r.db.GetContext(ctx, &count, query, conversationID, !isAdmin, domain.MessageStatusRead)
	return count, err
}
