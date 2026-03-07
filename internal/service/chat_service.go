package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/domain"
	"kipa/internal/repository"
)

type ChatService interface {
	SendMessage(ctx context.Context, senderID, jobID uuid.UUID, content string, mediaURL *string, mediaType string) (*domain.ChatMessage, error)
	GetHistory(ctx context.Context, jobID uuid.UUID, limit int, beforeID *uuid.UUID) ([]*ChatMessageResponse, error)
	MarkAsRead(ctx context.Context, jobID, userID uuid.UUID) error
	ValidateParticipant(ctx context.Context, userID, jobID uuid.UUID) (receiverID uuid.UUID, err error)
	GetUnreadCount(ctx context.Context, jobID, userID uuid.UUID) (int, error)
}

type ChatMessageResponse struct {
	*domain.ChatMessage
	SenderType string `json:"sender_type"`
	SenderName string `json:"sender_name"`
}

type ChatServiceImpl struct {
	chatRepo        repository.ChatMessageRepository
	deliveryJobRepo repository.DeliveryJobRepository
	prRepo          repository.PaymentRequestRepository
	riderRepo       repository.RiderProfileRepository
	userRepo        repository.UserRepository
	notificationSvc NotificationService
	logger          *zap.Logger
}

func NewChatService(
	chatRepo repository.ChatMessageRepository,
	deliveryJobRepo repository.DeliveryJobRepository,
	prRepo repository.PaymentRequestRepository,
	riderRepo repository.RiderProfileRepository,
	userRepo repository.UserRepository,
	notificationSvc NotificationService,
	logger *zap.Logger,
) ChatService {
	return &ChatServiceImpl{
		chatRepo:        chatRepo,
		deliveryJobRepo: deliveryJobRepo,
		prRepo:          prRepo,
		riderRepo:       riderRepo,
		userRepo:        userRepo,
		notificationSvc: notificationSvc,
		logger:          logger,
	}
}

func (s *ChatServiceImpl) ValidateParticipant(ctx context.Context, userID, jobID uuid.UUID) (uuid.UUID, error) {
	job, err := s.deliveryJobRepo.GetByID(ctx, jobID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to get delivery job: %w", err)
	}
	if job == nil {
		return uuid.Nil, fmt.Errorf("delivery job not found")
	}

	pr, err := s.prRepo.GetByID(ctx, job.PaymentRequestID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to get payment request: %w", err)
	}

	var receiverID uuid.UUID


	resolveRiderUserID := func(riderProfileID uuid.UUID) (uuid.UUID, error) {
		profile, err := s.riderRepo.GetByID(ctx, riderProfileID)
		if err != nil {
			return uuid.Nil, err
		}
		if profile == nil {
			return uuid.Nil, fmt.Errorf("rider profile not found")
		}
		return profile.UserID, nil
	}


	if job.RiderID != nil {
		riderUserID, err := resolveRiderUserID(*job.RiderID)
		if err == nil && riderUserID == userID {
		
			if pr.BuyerID != nil {
				receiverID = *pr.BuyerID
			} else {
				receiverID = pr.SellerID
			}
			return receiverID, nil
		}
	}
	if (pr.BuyerID != nil && *pr.BuyerID == userID) || pr.SellerID == userID {
		if job.RiderID == nil {
			return uuid.Nil, fmt.Errorf("no rider assigned to job yet")
		}
		id, err := resolveRiderUserID(*job.RiderID)
		if err != nil {
			return uuid.Nil, fmt.Errorf("failed to resolve rider user id: %w", err)
		}
		receiverID = id
		return receiverID, nil
	}

	return uuid.Nil, fmt.Errorf("user is not a participant in this job")
}

func (s *ChatServiceImpl) SendMessage(ctx context.Context, senderID, jobID uuid.UUID, content string, mediaURL *string, mediaType string) (*domain.ChatMessage, error) {
	receiverID, err := s.ValidateParticipant(ctx, senderID, jobID)
	if err != nil {
		return nil, err
	}

	if mediaType == "" {
		mediaType = string(domain.MediaTypeText)
	}

	msg := &domain.ChatMessage{
		ID:         uuid.New(),
		JobID:      jobID,
		SenderID:   senderID,
		ReceiverID: receiverID,
		Content:    content,
		MediaURL:   mediaURL,
		MediaType:  mediaType,
		Status:     string(domain.MessageStatusSent),
	}

	if err := s.chatRepo.Create(ctx, msg); err != nil {
		s.logger.Error("failed to create chat message",
			zap.Error(err),
			zap.String("job_id", jobID.String()),
		)
		return nil, fmt.Errorf("failed to save message: %w", err)
	}

	go s.sendPushNotification(ctx, msg)

	s.logger.Info("chat message sent",
		zap.String("message_id", msg.ID.String()),
		zap.String("job_id", jobID.String()),
		zap.String("sender_id", senderID.String()),
		zap.String("receiver_id", receiverID.String()),
	)

	return msg, nil
}

func (s *ChatServiceImpl) sendPushNotification(ctx context.Context, msg *domain.ChatMessage) {
	if s.notificationSvc == nil {
		return
	}

	title := "New Message"
	body := msg.Content
	if len(body) > 100 {
		body = body[:97] + "..."
	}
	if msg.MediaType == string(domain.MediaTypeImage) {
		body = "📷 Image"
	}

	data := map[string]string{
		"job_id":     msg.JobID.String(),
		"message_id": msg.ID.String(),
		"sender_id":  msg.SenderID.String(),
		"type":       "chat_message",
	}

	if err := s.notificationSvc.SendToUser(ctx, msg.ReceiverID, title, body, data, domain.NotificationChatMessage); err != nil {
		s.logger.Warn("failed to send chat push notification",
			zap.Error(err),
			zap.String("receiver_id", msg.ReceiverID.String()),
		)
	}
}

func (s *ChatServiceImpl) GetHistory(ctx context.Context, jobID uuid.UUID, limit int, beforeID *uuid.UUID) ([]*ChatMessageResponse, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	messages, err := s.chatRepo.GetByJobID(ctx, jobID, limit, beforeID)
	if err != nil {
		return nil, err
	}

	// Get job and payment request to determine sender types
	job, err := s.deliveryJobRepo.GetByID(ctx, jobID)
	if err != nil {
		return nil, err
	}

	var buyerID, sellerID, riderUserID uuid.UUID

	if job.PaymentRequestID != uuid.Nil {
		pr, err := s.prRepo.GetByID(ctx, job.PaymentRequestID)
		if err == nil && pr != nil {
			sellerID = pr.SellerID
			if pr.BuyerID != nil {
				buyerID = *pr.BuyerID
			}
		}
	}

	if job.RiderID != nil {
		profile, err := s.riderRepo.GetByID(ctx, *job.RiderID)
		if err == nil && profile != nil {
			riderUserID = profile.UserID
		}
	}

	// Cache user names to avoid repeated lookups
	userNames := make(map[uuid.UUID]string)

	result := make([]*ChatMessageResponse, len(messages))
	for i, msg := range messages {
		senderType := "unknown"
		senderName := ""

		// Determine sender type
		if msg.SenderID == riderUserID {
			senderType = "rider"
		} else if msg.SenderID == buyerID {
			senderType = "buyer"
		} else if msg.SenderID == sellerID {
			senderType = "seller"
		}

		// Get sender name (with caching)
		if name, ok := userNames[msg.SenderID]; ok {
			senderName = name
		} else {
			user, err := s.userRepo.GetByID(ctx, msg.SenderID)
			if err == nil && user != nil {
				senderName = user.FullName()
				userNames[msg.SenderID] = senderName
			}
		}

		result[i] = &ChatMessageResponse{
			ChatMessage: msg,
			SenderType:  senderType,
			SenderName:  senderName,
		}
	}

	return result, nil
}

func (s *ChatServiceImpl) MarkAsRead(ctx context.Context, jobID, userID uuid.UUID) error {
	return s.chatRepo.MarkReadByReceiver(ctx, jobID, userID)
}

func (s *ChatServiceImpl) GetUnreadCount(ctx context.Context, jobID, userID uuid.UUID) (int, error) {
	return s.chatRepo.GetUnreadCount(ctx, jobID, userID)
}
