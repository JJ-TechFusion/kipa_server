package service

import (
	"context"

	"firebase.google.com/go/v4/messaging"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/domain"
	"kipa/internal/integration/fcm"
	"kipa/internal/repository"
)

type NotificationService interface {
	SendToUser(ctx context.Context, userID uuid.UUID, title, body string, data map[string]string, notifType domain.NotificationType) error

	SendToUsers(ctx context.Context, userIDs []uuid.UUID, title, body string, data map[string]string, notifType domain.NotificationType) error
	SendBroadcast(ctx context.Context, topic, title, body string, data map[string]string) error
	SendToAdmins(ctx context.Context, title, body string, data map[string]string) error

	RegisterToken(ctx context.Context, userID uuid.UUID, token string, platform domain.DevicePlatform) error

	RemoveToken(ctx context.Context, token string) error
}

type NotificationServiceImpl struct {
	fcmClient   *fcm.Client
	tokenRepo   repository.DeviceTokenRepository
	logRepo     repository.NotificationLogRepository
	logger      *zap.Logger
}

func NewNotificationService(
	fcmClient *fcm.Client,
	tokenRepo repository.DeviceTokenRepository,
	logRepo repository.NotificationLogRepository,
	logger *zap.Logger,
) NotificationService {
	return &NotificationServiceImpl{
		fcmClient: fcmClient,
		tokenRepo: tokenRepo,
		logRepo:   logRepo,
		logger:    logger,
	}
}

func (s *NotificationServiceImpl) SendToUser(ctx context.Context, userID uuid.UUID, title, body string, data map[string]string, notifType domain.NotificationType) error {
	tokens, err := s.tokenRepo.GetByUserID(ctx, userID)
	if err != nil {
		s.logger.Error("failed to get user tokens", zap.Error(err))
		return err
	}

	if len(tokens) == 0 {
		s.logger.Debug("no device tokens for user", zap.String("user_id", userID.String()))
		return nil
	}

	tokenStrings := make([]string, len(tokens))
	for i, t := range tokens {
		tokenStrings[i] = t.Token
	}
	if data == nil {
		data = make(map[string]string)
	}
	data["type"] = string(notifType)

	if s.fcmClient != nil {
		response, err := s.fcmClient.SendToTokens(ctx, tokenStrings, title, body, data)
		if err != nil {
			return err
		}

		if response != nil {
			for i, res := range response.Responses {
				if !res.Success && messaging.IsUnregistered(res.Error) {
					_ = s.tokenRepo.Delete(ctx, tokenStrings[i])
				}
			}
		}
	}

	log := &domain.NotificationLog{
		UserID:           &userID,
		Title:            title,
		Body:             body,
		NotificationType: notifType,
		Delivered:        true,
	}
	_ = s.logRepo.Create(ctx, log)

	return nil
}

func (s *NotificationServiceImpl) SendToUsers(ctx context.Context, userIDs []uuid.UUID, title, body string, data map[string]string, notifType domain.NotificationType) error {
	for _, userID := range userIDs {
		if err := s.SendToUser(ctx, userID, title, body, data, notifType); err != nil {
			s.logger.Warn("failed to send notification to user",
				zap.String("user_id", userID.String()),
				zap.Error(err),
			)
		}
	}
	return nil
}

func (s *NotificationServiceImpl) SendBroadcast(ctx context.Context, topic, title, body string, data map[string]string) error {
	if s.fcmClient == nil {
		s.logger.Warn("FCM client not configured, skipping broadcast")
		return nil
	}

	return s.fcmClient.SendToTopic(ctx, topic, title, body, data)
}

func (s *NotificationServiceImpl) SendToAdmins(ctx context.Context, title, body string, data map[string]string) error {
	return s.SendBroadcast(ctx, "admins", title, body, data)
}

func (s *NotificationServiceImpl) RegisterToken(ctx context.Context, userID uuid.UUID, token string, platform domain.DevicePlatform) error {
	deviceToken := &domain.DeviceToken{
		UserID:   userID,
		Token:    token,
		Platform: platform,
	}
	return s.tokenRepo.Save(ctx, deviceToken)
}

func (s *NotificationServiceImpl) RemoveToken(ctx context.Context, token string) error {
	return s.tokenRepo.Delete(ctx, token)
}

var _ NotificationService = (*NotificationServiceImpl)(nil)
