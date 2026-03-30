package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/domain"
	"kipa/internal/dto"
)

type supportChatHandlerTestService struct {
	getConversationDetails func(ctx context.Context, conversationID uuid.UUID) (*dto.SupportContextResponse, error)
}

func (s *supportChatHandlerTestService) GetOrCreateConversation(ctx context.Context, userID uuid.UUID) (*dto.SupportContextResponse, error) {
	return nil, nil
}
func (s *supportChatHandlerTestService) SendMessage(ctx context.Context, userID uuid.UUID, req *dto.SendSupportMessageRequest) (*dto.SupportMessageResponse, error) {
	return nil, nil
}
func (s *supportChatHandlerTestService) GetHistory(ctx context.Context, userID uuid.UUID, limit int, beforeID *uuid.UUID) ([]*dto.SupportMessageResponse, error) {
	return nil, nil
}
func (s *supportChatHandlerTestService) MarkAsRead(ctx context.Context, userID uuid.UUID) error {
	return nil
}
func (s *supportChatHandlerTestService) CreateDisputeConversation(ctx context.Context, userID, disputeID uuid.UUID) (*dto.SupportConversationResponse, error) {
	return nil, nil
}
func (s *supportChatHandlerTestService) ListUserConversations(ctx context.Context, userID uuid.UUID) ([]*dto.SupportConversationResponse, error) {
	return nil, nil
}
func (s *supportChatHandlerTestService) GetUserConversation(ctx context.Context, userID, conversationID uuid.UUID) (*dto.SupportContextResponse, error) {
	return nil, nil
}
func (s *supportChatHandlerTestService) SendMessageToConversation(ctx context.Context, userID, conversationID uuid.UUID, req *dto.SendSupportMessageRequest) (*dto.SupportMessageResponse, error) {
	return nil, nil
}
func (s *supportChatHandlerTestService) GetConversationHistory(ctx context.Context, userID, conversationID uuid.UUID, limit int, beforeID *uuid.UUID) ([]*dto.SupportMessageResponse, error) {
	return nil, nil
}
func (s *supportChatHandlerTestService) MarkConversationAsRead(ctx context.Context, userID, conversationID uuid.UUID) error {
	return nil
}
func (s *supportChatHandlerTestService) GetByDisputeID(ctx context.Context, disputeID uuid.UUID) (*domain.SupportConversation, error) {
	return nil, nil
}
func (s *supportChatHandlerTestService) ListConversations(ctx context.Context, status *domain.SupportConversationStatus, limit, offset int) ([]*dto.SupportConversationResponse, int, error) {
	return nil, 0, nil
}
func (s *supportChatHandlerTestService) GetConversationDetails(ctx context.Context, conversationID uuid.UUID) (*dto.SupportContextResponse, error) {
	if s.getConversationDetails != nil {
		return s.getConversationDetails(ctx, conversationID)
	}
	return nil, nil
}
func (s *supportChatHandlerTestService) AdminSendMessage(ctx context.Context, adminID, conversationID uuid.UUID, req *dto.SendSupportMessageRequest) (*dto.SupportMessageResponse, error) {
	return nil, nil
}
func (s *supportChatHandlerTestService) AdminSendMessageToUser(ctx context.Context, adminID, userID uuid.UUID, req *dto.SendSupportMessageRequest) (*dto.SupportMessageResponse, error) {
	return nil, nil
}
func (s *supportChatHandlerTestService) AdminGetHistory(ctx context.Context, conversationID uuid.UUID, limit int, beforeID *uuid.UUID) ([]*dto.SupportMessageResponse, error) {
	return nil, nil
}
func (s *supportChatHandlerTestService) AdminMarkAsRead(ctx context.Context, conversationID uuid.UUID) error {
	return nil
}
func (s *supportChatHandlerTestService) ResolveConversation(ctx context.Context, adminID, conversationID uuid.UUID) error {
	return nil
}

func TestGetConversationDetailsReturnsLinkedDisputeOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)

	conversationID := uuid.New()
	disputeID := uuid.New()
	userID := uuid.New()
	escrowID := uuid.New()
	now := time.Now().UTC()

	handler := NewSupportChatHandler(&supportChatHandlerTestService{
		getConversationDetails: func(ctx context.Context, gotConversationID uuid.UUID) (*dto.SupportContextResponse, error) {
			if gotConversationID != conversationID {
				t.Fatalf("expected conversation ID %s, got %s", conversationID, gotConversationID)
			}

			return &dto.SupportContextResponse{
				Conversation: &dto.SupportConversationResponse{
					ID:        conversationID,
					UserID:    userID,
					UserName:  "Test User",
					UserPhone: "+2348000000000",
					DisputeID: &disputeID,
					Dispute: &dto.SupportDisputeDetails{
						Dispute: &dto.SupportDisputeResponse{
							ID:        disputeID,
							EscrowID:  escrowID,
							OpenedBy:  userID,
							Reason:    "Item not as described",
							Evidence:  []string{"https://example.com/evidence.png"},
							Status:    string(domain.DisputeStatusOpen),
							Outcome:   string(domain.DisputeOutcomePending),
							CreatedAt: now,
						},
					},
					Status:      string(domain.SupportConversationStatusOpen),
					UnreadCount: 2,
					CreatedAt:   now,
				},
			}, nil
		},
	}, zap.NewNop())

	recorder := httptest.NewRecorder()
	c, router := gin.CreateTestContext(recorder)
	router.GET("/admin/support/conversations/:id", handler.GetConversationDetails)
	req := httptest.NewRequest(http.MethodGet, "/admin/support/conversations/"+conversationID.String(), nil)
	c.Request = req
	router.HandleContext(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}

	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if _, ok := response["active_disputes"]; ok {
		t.Fatalf("expected response to omit active_disputes, got %v", response["active_disputes"])
	}

	conversationValue, ok := response["conversation"].(map[string]any)
	if !ok {
		t.Fatalf("expected conversation object in response")
	}

	disputeValue, ok := conversationValue["dispute"].(map[string]any)
	if !ok {
		t.Fatalf("expected conversation.dispute object in response")
	}

	if disputeValue["dispute"] == nil {
		t.Fatalf("expected linked dispute details to be present")
	}
}
