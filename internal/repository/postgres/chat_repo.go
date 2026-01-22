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

type ChatMessageRepo struct {
	db *sqlx.DB
}

func NewChatMessageRepo(db *sqlx.DB) repository.ChatMessageRepository {
	return &ChatMessageRepo{db: db}
}

func (r *ChatMessageRepo) Create(ctx context.Context, msg *domain.ChatMessage) error {
	query := `
		INSERT INTO chat_messages (id, job_id, sender_id, receiver_id, content, media_url, media_type, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	if msg.ID == uuid.Nil {
		msg.ID = uuid.New()
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}
	if msg.Status == "" {
		msg.Status = string(domain.MessageStatusSent)
	}
	if msg.MediaType == "" {
		msg.MediaType = string(domain.MediaTypeText)
	}

	_, err := r.db.ExecContext(ctx, query,
		msg.ID,
		msg.JobID,
		msg.SenderID,
		msg.ReceiverID,
		msg.Content,
		msg.MediaURL,
		msg.MediaType,
		msg.Status,
		msg.CreatedAt,
	)
	return err
}

func (r *ChatMessageRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.ChatMessage, error) {
	query := `SELECT * FROM chat_messages WHERE id = $1`
	var msg domain.ChatMessage
	if err := r.db.GetContext(ctx, &msg, query, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &msg, nil
}

func (r *ChatMessageRepo) GetByJobID(ctx context.Context, jobID uuid.UUID, limit int, beforeID *uuid.UUID) ([]*domain.ChatMessage, error) {
	var messages []*domain.ChatMessage
	var err error

	if beforeID != nil {
		query := `
			SELECT * FROM chat_messages 
			WHERE job_id = $1 AND created_at < (SELECT created_at FROM chat_messages WHERE id = $2)
			ORDER BY created_at DESC
			LIMIT $3
		`
		err = r.db.SelectContext(ctx, &messages, query, jobID, *beforeID, limit)
	} else {
		query := `
			SELECT * FROM chat_messages 
			WHERE job_id = $1
			ORDER BY created_at DESC
			LIMIT $2
		`
		err = r.db.SelectContext(ctx, &messages, query, jobID, limit)
	}

	return messages, err
}

func (r *ChatMessageRepo) MarkDelivered(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE chat_messages SET status = $1, delivered_at = $2 WHERE id = $3`
	_, err := r.db.ExecContext(ctx, query, domain.MessageStatusDelivered, time.Now(), id)
	return err
}

func (r *ChatMessageRepo) MarkReadByReceiver(ctx context.Context, jobID, receiverID uuid.UUID) error {
	query := `
		UPDATE chat_messages 
		SET status = $1, read_at = $2 
		WHERE job_id = $3 AND receiver_id = $4 AND status != $1
	`
	_, err := r.db.ExecContext(ctx, query, domain.MessageStatusRead, time.Now(), jobID, receiverID)
	return err
}

func (r *ChatMessageRepo) GetUnreadCount(ctx context.Context, jobID, userID uuid.UUID) (int, error) {
	query := `SELECT COUNT(*) FROM chat_messages WHERE job_id = $1 AND receiver_id = $2 AND status != $3`
	var count int
	err := r.db.GetContext(ctx, &count, query, jobID, userID, domain.MessageStatusRead)
	return count, err
}
