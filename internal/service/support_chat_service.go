package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/domain"
	"kipa/internal/dto"
	"kipa/internal/repository"
)

type SupportChatService interface {
	GetOrCreateConversation(ctx context.Context, userID uuid.UUID) (*dto.SupportContextResponse, error)
	SendMessage(ctx context.Context, userID uuid.UUID, req *dto.SendSupportMessageRequest) (*dto.SupportMessageResponse, error)
	GetHistory(ctx context.Context, userID uuid.UUID, limit int, beforeID *uuid.UUID) ([]*dto.SupportMessageResponse, error)
	MarkAsRead(ctx context.Context, userID uuid.UUID) error

	ListConversations(ctx context.Context, status *domain.SupportConversationStatus, limit, offset int) ([]*dto.SupportConversationResponse, int, error)
	GetConversationDetails(ctx context.Context, conversationID uuid.UUID) (*dto.SupportContextResponse, error)
	AdminSendMessage(ctx context.Context, adminID, conversationID uuid.UUID, req *dto.SendSupportMessageRequest) (*dto.SupportMessageResponse, error)
	AdminGetHistory(ctx context.Context, conversationID uuid.UUID, limit int, beforeID *uuid.UUID) ([]*dto.SupportMessageResponse, error)
	AdminMarkAsRead(ctx context.Context, conversationID uuid.UUID) error
	ResolveConversation(ctx context.Context, adminID, conversationID uuid.UUID) error
}

type SupportChatServiceImpl struct {
	conversationRepo repository.SupportConversationRepository
	messageRepo      repository.SupportMessageRepository
	disputeRepo      repository.DisputeRepository
	escrowRepo       repository.EscrowRepository
	prRepo           repository.PaymentRequestRepository
	userRepo         repository.UserRepository
	notificationSvc  NotificationService
	logger           *zap.Logger
}

func NewSupportChatService(
	conversationRepo repository.SupportConversationRepository,
	messageRepo repository.SupportMessageRepository,
	disputeRepo repository.DisputeRepository,
	escrowRepo repository.EscrowRepository,
	prRepo repository.PaymentRequestRepository,
	userRepo repository.UserRepository,
	notificationSvc NotificationService,
	logger *zap.Logger,
) SupportChatService {
	return &SupportChatServiceImpl{
		conversationRepo: conversationRepo,
		messageRepo:      messageRepo,
		disputeRepo:      disputeRepo,
		escrowRepo:       escrowRepo,
		prRepo:           prRepo,
		userRepo:         userRepo,
		notificationSvc:  notificationSvc,
		logger:           logger,
	}
}

func (s *SupportChatServiceImpl) GetOrCreateConversation(ctx context.Context, userID uuid.UUID) (*dto.SupportContextResponse, error) {
	conversation, err := s.conversationRepo.GetOpenByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation: %w", err)
	}

	if conversation == nil {
		conversation = domain.NewSupportConversation(userID)
		if err := s.conversationRepo.Create(ctx, conversation); err != nil {
			return nil, fmt.Errorf("failed to create conversation: %w", err)
		}
	}

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	userName := ""
	userPhone := ""
	if user != nil {
		userName = user.FullName()
		userPhone = user.PhoneNumber
	}

	unreadCount, _ := s.messageRepo.GetUnreadCount(ctx, conversation.ID, false)

	activeDisputes, err := s.getActiveDisputesForUser(ctx, userID)
	if err != nil {
		s.logger.Warn("failed to get active disputes", zap.Error(err))
		activeDisputes = []*dto.SupportDisputeDetails{}
	}

	return &dto.SupportContextResponse{
		Conversation:   dto.ToSupportConversationResponse(conversation, userName, userPhone, unreadCount),
		ActiveDisputes: activeDisputes,
	}, nil
}

func (s *SupportChatServiceImpl) getActiveDisputesForUser(ctx context.Context, userID uuid.UUID) ([]*dto.SupportDisputeDetails, error) {
	openStatus := domain.DisputeStatusOpen
	disputes, err := s.disputeRepo.List(ctx, &repository.DisputeFilters{Status: &openStatus}, 100, 0)
	if err != nil {
		return nil, err
	}

	inReviewStatus := domain.DisputeStatusInReview
	inReviewDisputes, err := s.disputeRepo.List(ctx, &repository.DisputeFilters{Status: &inReviewStatus}, 100, 0)
	if err != nil {
		return nil, err
	}

	disputes = append(disputes, inReviewDisputes...)

	var result []*dto.SupportDisputeDetails
	for _, dispute := range disputes {
		escrow, err := s.escrowRepo.GetByID(ctx, dispute.EscrowID)
		if err != nil || escrow == nil {
			continue
		}

		if escrow.BuyerID != userID && escrow.SellerID != userID {
			continue
		}

		details := &dto.SupportDisputeDetails{
			Dispute: dto.ToSupportDisputeResponse(dispute),
			Escrow:  dto.ToSupportEscrowResponse(escrow),
		}

		pr, err := s.prRepo.GetByID(ctx, escrow.PaymentRequestID)
		if err == nil && pr != nil {
			details.PaymentRequest = dto.ToSupportPaymentRequestResponse(pr)
		}

		result = append(result, details)
	}

	return result, nil
}

func (s *SupportChatServiceImpl) SendMessage(ctx context.Context, userID uuid.UUID, req *dto.SendSupportMessageRequest) (*dto.SupportMessageResponse, error) {
	conversation, err := s.conversationRepo.GetOpenByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation: %w", err)
	}
	if conversation == nil {
		return nil, fmt.Errorf("no open conversation found")
	}

	message := domain.NewSupportMessage(conversation.ID, userID, false, req.Content, req.MediaURL, req.MediaType)
	if err := s.messageRepo.Create(ctx, message); err != nil {
		return nil, fmt.Errorf("failed to create message: %w", err)
	}

	if err := s.conversationRepo.UpdateLastMessageAt(ctx, conversation.ID); err != nil {
		s.logger.Warn("failed to update last message at", zap.Error(err))
	}

	user, _ := s.userRepo.GetByID(ctx, userID)
	senderName := ""
	if user != nil {
		senderName = user.FullName()
	}

	go s.notifyAdmins(ctx, message, senderName)

	s.logger.Info("support message sent",
		zap.String("message_id", message.ID.String()),
		zap.String("conversation_id", conversation.ID.String()),
		zap.String("user_id", userID.String()),
	)

	return dto.ToSupportMessageResponse(message, senderName), nil
}

func (s *SupportChatServiceImpl) notifyAdmins(ctx context.Context, msg *domain.SupportMessage, senderName string) {
	if s.notificationSvc == nil {
		return
	}

	title := "New Support Message"
	body := msg.Content
	if len(body) > 100 {
		body = body[:97] + "..."
	}
	if msg.MediaType == string(domain.MediaTypeImage) {
		body = "📷 Image"
	}
	if senderName != "" {
		title = fmt.Sprintf("Support: %s", senderName)
	}

	data := map[string]string{
		"conversation_id": msg.ConversationID.String(),
		"message_id":      msg.ID.String(),
		"sender_id":       msg.SenderID.String(),
		"type":            "support_message",
	}

	if err := s.notificationSvc.SendToAdmins(ctx, title, body, data); err != nil {
		s.logger.Warn("failed to send admin notification",
			zap.Error(err),
		)
	}
}

func (s *SupportChatServiceImpl) GetHistory(ctx context.Context, userID uuid.UUID, limit int, beforeID *uuid.UUID) ([]*dto.SupportMessageResponse, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	conversation, err := s.conversationRepo.GetOpenByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation: %w", err)
	}
	if conversation == nil {
		return []*dto.SupportMessageResponse{}, nil
	}

	messages, err := s.messageRepo.GetByConversationID(ctx, conversation.ID, limit, beforeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}

	userNames := make(map[uuid.UUID]string)

	result := make([]*dto.SupportMessageResponse, len(messages))
	for i, msg := range messages {
		senderName := ""
		if name, ok := userNames[msg.SenderID]; ok {
			senderName = name
		} else {
			user, err := s.userRepo.GetByID(ctx, msg.SenderID)
			if err == nil && user != nil {
				senderName = user.FullName()
				userNames[msg.SenderID] = senderName
			}
		}

		if msg.IsAdmin && senderName == "" {
			senderName = "Support"
		}

		result[i] = dto.ToSupportMessageResponse(msg, senderName)
	}

	return result, nil
}

func (s *SupportChatServiceImpl) MarkAsRead(ctx context.Context, userID uuid.UUID) error {
	conversation, err := s.conversationRepo.GetOpenByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get conversation: %w", err)
	}
	if conversation == nil {
		return nil
	}

	return s.messageRepo.MarkRead(ctx, conversation.ID, false)
}


func (s *SupportChatServiceImpl) ListConversations(ctx context.Context, status *domain.SupportConversationStatus, limit, offset int) ([]*dto.SupportConversationResponse, int, error) {
	conversations, total, err := s.conversationRepo.List(ctx, status, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list conversations: %w", err)
	}

	result := make([]*dto.SupportConversationResponse, len(conversations))
	for i, conv := range conversations {
		user, _ := s.userRepo.GetByID(ctx, conv.UserID)
		userName := ""
		userPhone := ""
		if user != nil {
			userName = user.FullName()
			userPhone = user.PhoneNumber
		}

		unreadCount, _ := s.messageRepo.GetUnreadCount(ctx, conv.ID, true)

		result[i] = dto.ToSupportConversationResponse(conv, userName, userPhone, unreadCount)
	}

	return result, total, nil
}

func (s *SupportChatServiceImpl) GetConversationDetails(ctx context.Context, conversationID uuid.UUID) (*dto.SupportContextResponse, error) {
	conversation, err := s.conversationRepo.GetByID(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation: %w", err)
	}
	if conversation == nil {
		return nil, fmt.Errorf("conversation not found")
	}

	user, err := s.userRepo.GetByID(ctx, conversation.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	userName := ""
	userPhone := ""
	if user != nil {
		userName = user.FullName()
		userPhone = user.PhoneNumber
	}

	unreadCount, _ := s.messageRepo.GetUnreadCount(ctx, conversationID, true)

	activeDisputes, err := s.getActiveDisputesForUser(ctx, conversation.UserID)
	if err != nil {
		s.logger.Warn("failed to get active disputes", zap.Error(err))
		activeDisputes = []*dto.SupportDisputeDetails{}
	}

	return &dto.SupportContextResponse{
		Conversation:   dto.ToSupportConversationResponse(conversation, userName, userPhone, unreadCount),
		ActiveDisputes: activeDisputes,
	}, nil
}

func (s *SupportChatServiceImpl) AdminSendMessage(ctx context.Context, adminID, conversationID uuid.UUID, req *dto.SendSupportMessageRequest) (*dto.SupportMessageResponse, error) {
	conversation, err := s.conversationRepo.GetByID(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation: %w", err)
	}
	if conversation == nil {
		return nil, fmt.Errorf("conversation not found")
	}

	message := domain.NewSupportMessage(conversationID, adminID, true, req.Content, req.MediaURL, req.MediaType)
	if err := s.messageRepo.Create(ctx, message); err != nil {
		return nil, fmt.Errorf("failed to create message: %w", err)
	}

	if err := s.conversationRepo.UpdateLastMessageAt(ctx, conversationID); err != nil {
		s.logger.Warn("failed to update last message at", zap.Error(err))
	}

	admin, _ := s.userRepo.GetByID(ctx, adminID)
	senderName := "Support"
	if admin != nil && admin.FullName() != "" {
		senderName = admin.FullName()
	}

	go s.notifyUser(ctx, conversation.UserID, message, senderName)

	s.logger.Info("admin support message sent",
		zap.String("message_id", message.ID.String()),
		zap.String("conversation_id", conversationID.String()),
		zap.String("admin_id", adminID.String()),
	)

	return dto.ToSupportMessageResponse(message, senderName), nil
}

func (s *SupportChatServiceImpl) notifyUser(ctx context.Context, userID uuid.UUID, msg *domain.SupportMessage, _ string) {
	if s.notificationSvc == nil {
		return
	}

	title := "Support Response"
	body := msg.Content
	if len(body) > 100 {
		body = body[:97] + "..."
	}
	if msg.MediaType == string(domain.MediaTypeImage) {
		body = "📷 Image"
	}

	data := map[string]string{
		"conversation_id": msg.ConversationID.String(),
		"message_id":      msg.ID.String(),
		"type":            "support_response",
	}

	if err := s.notificationSvc.SendToUser(ctx, userID, title, body, data, domain.NotificationSupportMessage); err != nil {
		s.logger.Warn("failed to send user notification",
			zap.Error(err),
			zap.String("user_id", userID.String()),
		)
	}
}

func (s *SupportChatServiceImpl) AdminGetHistory(ctx context.Context, conversationID uuid.UUID, limit int, beforeID *uuid.UUID) ([]*dto.SupportMessageResponse, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	messages, err := s.messageRepo.GetByConversationID(ctx, conversationID, limit, beforeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}

	userNames := make(map[uuid.UUID]string)

	result := make([]*dto.SupportMessageResponse, len(messages))
	for i, msg := range messages {
		senderName := ""
		if name, ok := userNames[msg.SenderID]; ok {
			senderName = name
		} else {
			user, err := s.userRepo.GetByID(ctx, msg.SenderID)
			if err == nil && user != nil {
				senderName = user.FullName()
				userNames[msg.SenderID] = senderName
			}
		}

		if msg.IsAdmin && senderName == "" {
			senderName = "Support"
		}

		result[i] = dto.ToSupportMessageResponse(msg, senderName)
	}

	return result, nil
}

func (s *SupportChatServiceImpl) AdminMarkAsRead(ctx context.Context, conversationID uuid.UUID) error {
	return s.messageRepo.MarkRead(ctx, conversationID, true)
}

func (s *SupportChatServiceImpl) ResolveConversation(ctx context.Context, adminID, conversationID uuid.UUID) error {
	conversation, err := s.conversationRepo.GetByID(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("failed to get conversation: %w", err)
	}
	if conversation == nil {
		return fmt.Errorf("conversation not found")
	}

	conversation.Resolve()
	if err := s.conversationRepo.Update(ctx, conversation); err != nil {
		return fmt.Errorf("failed to update conversation: %w", err)
	}

	s.logger.Info("support conversation resolved",
		zap.String("conversation_id", conversationID.String()),
		zap.String("admin_id", adminID.String()),
	)

	return nil
}

var _ SupportChatService = (*SupportChatServiceImpl)(nil)
