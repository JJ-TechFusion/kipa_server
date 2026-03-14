package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type SupportStaff struct {
	ID              uuid.UUID `db:"id" json:"id"`
	UserID          uuid.UUID `db:"user_id" json:"user_id"`
	AddedBy         uuid.UUID `db:"added_by" json:"added_by"`
	CanReplyChats   bool      `db:"can_reply_chats" json:"can_reply_chats"`
	CanViewDisputes bool      `db:"can_view_disputes" json:"can_view_disputes"`
	CanResolveChats bool      `db:"can_resolve_chats" json:"can_resolve_chats"`
	CanMessageUsers bool      `db:"can_message_users" json:"can_message_users"`
	IsActive        bool      `db:"is_active" json:"is_active"`
	CreatedAt       time.Time `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time `db:"updated_at" json:"updated_at"`
}

type SupportPermission string

const (
	PermReplyChats   SupportPermission = "reply_chats"
	PermViewDisputes SupportPermission = "view_disputes"
	PermResolveChats SupportPermission = "resolve_chats"
	PermMessageUsers SupportPermission = "message_users"
)

func (s *SupportStaff) HasPermission(perm SupportPermission) bool {
	if !s.IsActive {
		return false
	}
	switch perm {
	case PermReplyChats:
		return s.CanReplyChats
	case PermViewDisputes:
		return s.CanViewDisputes
	case PermResolveChats:
		return s.CanResolveChats
	case PermMessageUsers:
		return s.CanMessageUsers
	default:
		return false
	}
}

func (s *SupportStaff) Permissions() []SupportPermission {
	var perms []SupportPermission
	if s.CanReplyChats {
		perms = append(perms, PermReplyChats)
	}
	if s.CanViewDisputes {
		perms = append(perms, PermViewDisputes)
	}
	if s.CanResolveChats {
		perms = append(perms, PermResolveChats)
	}
	if s.CanMessageUsers {
		perms = append(perms, PermMessageUsers)
	}
	return perms
}

var ErrSupportStaffNotFound = errors.New("support staff not found")
var ErrSupportStaffExists = errors.New("user is already support staff")
var ErrSupportInsufficientPermission = errors.New("insufficient permission for this action")
