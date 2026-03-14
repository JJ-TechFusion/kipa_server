package dto

import (
	"time"

	"github.com/google/uuid"

	"kipa/internal/domain"
)

type AddSupportStaffRequest struct {
	UserID          uuid.UUID `json:"user_id" binding:"required"`
	CanReplyChats   bool      `json:"can_reply_chats"`
	CanViewDisputes bool      `json:"can_view_disputes"`
	CanResolveChats bool      `json:"can_resolve_chats"`
	CanMessageUsers bool      `json:"can_message_users"`
}

type UpdateSupportStaffRequest struct {
	CanReplyChats   *bool `json:"can_reply_chats"`
	CanViewDisputes *bool `json:"can_view_disputes"`
	CanResolveChats *bool `json:"can_resolve_chats"`
	CanMessageUsers *bool `json:"can_message_users"`
	IsActive        *bool `json:"is_active"`
}

type SupportStaffResponse struct {
	ID              uuid.UUID                  `json:"id"`
	UserID          uuid.UUID                  `json:"user_id"`
	User            *SupportStaffUserResponse  `json:"user,omitempty"`
	AddedBy         uuid.UUID                  `json:"added_by"`
	AddedByUser     *SupportStaffUserResponse  `json:"added_by_user,omitempty"`
	CanReplyChats   bool                       `json:"can_reply_chats"`
	CanViewDisputes bool                       `json:"can_view_disputes"`
	CanResolveChats bool                       `json:"can_resolve_chats"`
	CanMessageUsers bool                       `json:"can_message_users"`
	Permissions     []string                   `json:"permissions"`
	IsActive        bool                       `json:"is_active"`
	CreatedAt       time.Time                  `json:"created_at"`
	UpdatedAt       time.Time                  `json:"updated_at"`
}

type SupportStaffUserResponse struct {
	ID          uuid.UUID `json:"id"`
	FirstName   *string   `json:"first_name,omitempty"`
	LastName    *string   `json:"last_name,omitempty"`
	PhoneNumber string    `json:"phone_number"`
}

func ToSupportStaffResponse(staff *domain.SupportStaff) *SupportStaffResponse {
	perms := staff.Permissions()
	permStrings := make([]string, len(perms))
	for i, p := range perms {
		permStrings[i] = string(p)
	}

	return &SupportStaffResponse{
		ID:              staff.ID,
		UserID:          staff.UserID,
		AddedBy:         staff.AddedBy,
		CanReplyChats:   staff.CanReplyChats,
		CanViewDisputes: staff.CanViewDisputes,
		CanResolveChats: staff.CanResolveChats,
		CanMessageUsers: staff.CanMessageUsers,
		Permissions:     permStrings,
		IsActive:        staff.IsActive,
		CreatedAt:       staff.CreatedAt,
		UpdatedAt:       staff.UpdatedAt,
	}
}

func ToSupportStaffResponseWithUsers(staff *domain.SupportStaff, user, addedByUser *domain.User) *SupportStaffResponse {
	resp := ToSupportStaffResponse(staff)

	if user != nil {
		resp.User = &SupportStaffUserResponse{
			ID:          user.ID,
			FirstName:   user.FirstName,
			LastName:    user.LastName,
			PhoneNumber: user.PhoneNumber,
		}
	}

	if addedByUser != nil {
		resp.AddedByUser = &SupportStaffUserResponse{
			ID:          addedByUser.ID,
			FirstName:   addedByUser.FirstName,
			LastName:    addedByUser.LastName,
			PhoneNumber: addedByUser.PhoneNumber,
		}
	}

	return resp
}
