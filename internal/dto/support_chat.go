package dto

import (
	"time"

	"github.com/google/uuid"

	"kipa/internal/domain"
)

type SendSupportMessageRequest struct {
	Content   string  `json:"content" binding:"required,min=1,max=2000"`
	MediaURL  *string `json:"media_url,omitempty"`
	MediaType string  `json:"media_type,omitempty"`
}

type SupportMessageResponse struct {
	ID             uuid.UUID  `json:"id"`
	ConversationID uuid.UUID  `json:"conversation_id"`
	SenderID       uuid.UUID  `json:"sender_id"`
	SenderName     string     `json:"sender_name"`
	IsAdmin        bool       `json:"is_admin"`
	Content        string     `json:"content"`
	MediaURL       *string    `json:"media_url,omitempty"`
	MediaType      string     `json:"media_type"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	ReadAt         *time.Time `json:"read_at,omitempty"`
}

type SupportConversationResponse struct {
	ID            uuid.UUID              `json:"id"`
	UserID        uuid.UUID              `json:"user_id"`
	UserName      string                 `json:"user_name"`
	UserPhone     string                 `json:"user_phone"`
	DisputeID     *uuid.UUID             `json:"dispute_id,omitempty"`
	Dispute       *SupportDisputeDetails `json:"dispute,omitempty"`
	Status        string                 `json:"status"`
	LastMessageAt *time.Time             `json:"last_message_at,omitempty"`
	UnreadCount   int                    `json:"unread_count"`
	CreatedAt     time.Time              `json:"created_at"`
}

type SupportContextResponse struct {
	Conversation *SupportConversationResponse `json:"conversation"`
}

type SupportDisputeDetails struct {
	Dispute        *SupportDisputeResponse        `json:"dispute"`
	Escrow         *SupportEscrowResponse         `json:"escrow,omitempty"`
	PaymentRequest *SupportPaymentRequestResponse `json:"payment_request,omitempty"`
}

type SupportDisputeResponse struct {
	ID              uuid.UUID  `json:"id"`
	EscrowID        uuid.UUID  `json:"escrow_id"`
	OpenedBy        uuid.UUID  `json:"opened_by"`
	Reason          string     `json:"reason"`
	Evidence        []string   `json:"evidence"`
	Status          string     `json:"status"`
	Outcome         string     `json:"outcome"`
	ResolutionNotes *string    `json:"resolution_notes,omitempty"`
	ResolvedBy      *uuid.UUID `json:"resolved_by,omitempty"`
	ResolvedAt      *time.Time `json:"resolved_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

type SupportEscrowResponse struct {
	ID                       uuid.UUID  `json:"id"`
	PaymentRequestID         uuid.UUID  `json:"payment_request_id"`
	BuyerID                  uuid.UUID  `json:"buyer_id"`
	SellerID                 uuid.UUID  `json:"seller_id"`
	ItemAmount               float64    `json:"item_amount"`
	BuyerFee                 float64    `json:"buyer_fee"`
	DeliveryFeeEstimate      float64    `json:"delivery_fee_estimate"`
	TotalLocked              float64    `json:"total_locked"`
	Status                   string     `json:"status"`
	FundedAt                 *time.Time `json:"funded_at,omitempty"`
	ConfirmationWindowEndsAt *time.Time `json:"confirmation_window_ends_at,omitempty"`
	CreatedAt                time.Time  `json:"created_at"`
}

type SupportPaymentRequestResponse struct {
	ID              uuid.UUID  `json:"id"`
	SellerID        uuid.UUID  `json:"seller_id"`
	BuyerID         *uuid.UUID `json:"buyer_id,omitempty"`
	PaymentCode     *string    `json:"payment_code,omitempty"`
	ItemName        string     `json:"item_name"`
	ItemDescription *string    `json:"item_description,omitempty"`
	ItemPrice       float64    `json:"item_price"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
}

func ToSupportDisputeResponse(d *domain.Dispute) *SupportDisputeResponse {
	return &SupportDisputeResponse{
		ID:              d.ID,
		EscrowID:        d.EscrowID,
		OpenedBy:        d.OpenedBy,
		Reason:          d.Reason,
		Evidence:        d.Evidence,
		Status:          string(d.Status),
		Outcome:         string(d.Outcome),
		ResolutionNotes: d.ResolutionNotes,
		ResolvedBy:      d.ResolvedBy,
		ResolvedAt:      d.ResolvedAt,
		CreatedAt:       d.CreatedAt,
	}
}

func ToSupportEscrowResponse(e *domain.Escrow) *SupportEscrowResponse {
	return &SupportEscrowResponse{
		ID:                       e.ID,
		PaymentRequestID:         e.PaymentRequestID,
		BuyerID:                  e.BuyerID,
		SellerID:                 e.SellerID,
		ItemAmount:               e.ItemAmount,
		BuyerFee:                 e.BuyerFee,
		DeliveryFeeEstimate:      e.DeliveryFeeEstimate,
		TotalLocked:              e.TotalLocked,
		Status:                   string(e.Status),
		FundedAt:                 e.FundedAt,
		ConfirmationWindowEndsAt: e.ConfirmationWindowEndsAt,
		CreatedAt:                e.CreatedAt,
	}
}

func ToSupportPaymentRequestResponse(pr *domain.PaymentRequest) *SupportPaymentRequestResponse {
	return &SupportPaymentRequestResponse{
		ID:              pr.ID,
		SellerID:        pr.SellerID,
		BuyerID:         pr.BuyerID,
		PaymentCode:     pr.PaymentCode,
		ItemName:        pr.ItemName,
		ItemDescription: pr.ItemDescription,
		ItemPrice:       pr.ItemPrice,
		Status:          string(pr.Status),
		CreatedAt:       pr.CreatedAt,
	}
}

func ToSupportMessageResponse(m *domain.SupportMessage, senderName string) *SupportMessageResponse {
	return &SupportMessageResponse{
		ID:             m.ID,
		ConversationID: m.ConversationID,
		SenderID:       m.SenderID,
		SenderName:     senderName,
		IsAdmin:        m.IsAdmin,
		Content:        m.Content,
		MediaURL:       m.MediaURL,
		MediaType:      m.MediaType,
		Status:         string(m.Status),
		CreatedAt:      m.CreatedAt,
		ReadAt:         m.ReadAt,
	}
}

func ToSupportConversationResponse(c *domain.SupportConversation, userName, userPhone string, unreadCount int) *SupportConversationResponse {
	return &SupportConversationResponse{
		ID:            c.ID,
		UserID:        c.UserID,
		UserName:      userName,
		UserPhone:     userPhone,
		DisputeID:     c.DisputeID,
		Status:        string(c.Status),
		LastMessageAt: c.LastMessageAt,
		UnreadCount:   unreadCount,
		CreatedAt:     c.CreatedAt,
	}
}

func ToSupportConversationResponseWithDispute(c *domain.SupportConversation, userName, userPhone string, unreadCount int, dispute *SupportDisputeDetails) *SupportConversationResponse {
	resp := ToSupportConversationResponse(c, userName, userPhone, unreadCount)
	resp.Dispute = dispute
	return resp
}
