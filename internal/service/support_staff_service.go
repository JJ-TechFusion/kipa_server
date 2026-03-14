package service

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/domain"
	"kipa/internal/dto"
	"kipa/internal/repository"
)

type SupportStaffService interface {
	AddStaff(ctx context.Context, adminID uuid.UUID, req *dto.AddSupportStaffRequest) (*dto.SupportStaffResponse, error)
	UpdateStaff(ctx context.Context, adminID, userID uuid.UUID, req *dto.UpdateSupportStaffRequest) (*dto.SupportStaffResponse, error)
	RemoveStaff(ctx context.Context, adminID, userID uuid.UUID) error
	ListStaff(ctx context.Context, activeOnly bool, limit, offset int) ([]*dto.SupportStaffResponse, int, error)
	GetStaff(ctx context.Context, userID uuid.UUID) (*dto.SupportStaffResponse, error)
}

type SupportStaffServiceImpl struct {
	staffRepo repository.SupportStaffRepository
	userRepo  repository.UserRepository
	logger    *zap.Logger
}

func NewSupportStaffService(
	staffRepo repository.SupportStaffRepository,
	userRepo repository.UserRepository,
	logger *zap.Logger,
) SupportStaffService {
	return &SupportStaffServiceImpl{
		staffRepo: staffRepo,
		userRepo:  userRepo,
		logger:    logger,
	}
}

func (s *SupportStaffServiceImpl) AddStaff(ctx context.Context, adminID uuid.UUID, req *dto.AddSupportStaffRequest) (*dto.SupportStaffResponse, error) {
	// Verify target user exists
	user, err := s.userRepo.GetByID(ctx, req.UserID)
	if err != nil {
		return nil, err
	}

	// Check if already support staff
	exists, err := s.staffRepo.ExistsByUserID(ctx, req.UserID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, domain.ErrSupportStaffExists
	}

	staff := &domain.SupportStaff{
		UserID:          req.UserID,
		AddedBy:         adminID,
		CanReplyChats:   req.CanReplyChats,
		CanViewDisputes: req.CanViewDisputes,
		CanResolveChats: req.CanResolveChats,
		CanMessageUsers: req.CanMessageUsers,
		IsActive:        true,
	}

	if err := s.staffRepo.Create(ctx, staff); err != nil {
		s.logger.Error("failed to create support staff", zap.Error(err))
		return nil, err
	}

	// Get admin user for response
	adminUser, _ := s.userRepo.GetByID(ctx, adminID)

	s.logger.Info("support staff added",
		zap.String("user_id", req.UserID.String()),
		zap.String("added_by", adminID.String()),
	)

	return dto.ToSupportStaffResponseWithUsers(staff, user, adminUser), nil
}

func (s *SupportStaffServiceImpl) UpdateStaff(ctx context.Context, adminID, userID uuid.UUID, req *dto.UpdateSupportStaffRequest) (*dto.SupportStaffResponse, error) {
	staff, err := s.staffRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if req.CanReplyChats != nil {
		staff.CanReplyChats = *req.CanReplyChats
	}
	if req.CanViewDisputes != nil {
		staff.CanViewDisputes = *req.CanViewDisputes
	}
	if req.CanResolveChats != nil {
		staff.CanResolveChats = *req.CanResolveChats
	}
	if req.CanMessageUsers != nil {
		staff.CanMessageUsers = *req.CanMessageUsers
	}
	if req.IsActive != nil {
		staff.IsActive = *req.IsActive
	}

	if err := s.staffRepo.Update(ctx, staff); err != nil {
		s.logger.Error("failed to update support staff", zap.Error(err))
		return nil, err
	}

	// Fetch updated staff with users
	staff, _ = s.staffRepo.GetByUserID(ctx, userID)
	user, _ := s.userRepo.GetByID(ctx, userID)
	addedByUser, _ := s.userRepo.GetByID(ctx, staff.AddedBy)

	s.logger.Info("support staff updated",
		zap.String("user_id", userID.String()),
		zap.String("updated_by", adminID.String()),
	)

	return dto.ToSupportStaffResponseWithUsers(staff, user, addedByUser), nil
}

func (s *SupportStaffServiceImpl) RemoveStaff(ctx context.Context, adminID, userID uuid.UUID) error {
	if err := s.staffRepo.Delete(ctx, userID); err != nil {
		s.logger.Error("failed to remove support staff", zap.Error(err))
		return err
	}

	s.logger.Info("support staff removed",
		zap.String("user_id", userID.String()),
		zap.String("removed_by", adminID.String()),
	)

	return nil
}

func (s *SupportStaffServiceImpl) ListStaff(ctx context.Context, activeOnly bool, limit, offset int) ([]*dto.SupportStaffResponse, int, error) {
	staffList, total, err := s.staffRepo.List(ctx, activeOnly, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]*dto.SupportStaffResponse, len(staffList))
	for i, staff := range staffList {
		user, _ := s.userRepo.GetByID(ctx, staff.UserID)
		addedByUser, _ := s.userRepo.GetByID(ctx, staff.AddedBy)
		responses[i] = dto.ToSupportStaffResponseWithUsers(staff, user, addedByUser)
	}

	return responses, total, nil
}

func (s *SupportStaffServiceImpl) GetStaff(ctx context.Context, userID uuid.UUID) (*dto.SupportStaffResponse, error) {
	staff, err := s.staffRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	user, _ := s.userRepo.GetByID(ctx, userID)
	addedByUser, _ := s.userRepo.GetByID(ctx, staff.AddedBy)

	return dto.ToSupportStaffResponseWithUsers(staff, user, addedByUser), nil
}
