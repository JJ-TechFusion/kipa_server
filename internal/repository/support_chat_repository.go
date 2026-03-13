package repository

import (
	"context"

	"github.com/google/uuid"

	"kipa/internal/domain"
)

type SupportConversationRepository interface {
	Create(ctx context.Context, conversation *domain.SupportConversation) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.SupportConversation, error)
	GetByDisputeID(ctx context.Context, disputeID uuid.UUID) (*domain.SupportConversation, error)
	GetOpenByUserID(ctx context.Context, userID uuid.UUID) (*domain.SupportConversation, error)
	GetOpenGeneralByUserID(ctx context.Context, userID uuid.UUID) (*domain.SupportConversation, error)
	GetAllByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.SupportConversation, error)
	List(ctx context.Context, status *domain.SupportConversationStatus, limit, offset int) ([]*domain.SupportConversation, int, error)
	Update(ctx context.Context, conversation *domain.SupportConversation) error
	UpdateLastMessageAt(ctx context.Context, id uuid.UUID) error
}

type SupportMessageRepository interface {
	Create(ctx context.Context, message *domain.SupportMessage) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.SupportMessage, error)
	GetByConversationID(ctx context.Context, conversationID uuid.UUID, limit int, beforeID *uuid.UUID) ([]*domain.SupportMessage, error)
	MarkRead(ctx context.Context, conversationID uuid.UUID, isAdmin bool) error
	GetUnreadCount(ctx context.Context, conversationID uuid.UUID, isAdmin bool) (int, error)
}
