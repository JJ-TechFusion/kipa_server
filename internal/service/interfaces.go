package service

import (
	"context"

	"kipa/internal/dto"
	"kipa/internal/util"

	"github.com/google/uuid"
)

type AuthService interface {
	SendOTP(ctx context.Context, req *dto.SendOTPRequest, ipAddress, userAgent string) (*dto.SendOTPResponse, error)
	VerifyOTP(ctx context.Context, req *dto.VerifyOTPRequest, ipAddress, userAgent string) (*dto.VerifyOTPResponse, error)
	RefreshToken(ctx context.Context, req *dto.RefreshTokenRequest) (*dto.RefreshTokenResponse, error)
	Logout(ctx context.Context, userID uuid.UUID, req *dto.LogoutRequest) (*dto.LogoutResponse, error)
	ValidateAccessToken(ctx context.Context, token string) (*util.AccessTokenClaims, error)
	GetUserByID(ctx context.Context, userID uuid.UUID) (*dto.UserResponse, error)
	UpdateProfile(ctx context.Context, userID uuid.UUID, req *dto.UpdateProfileRequest) (*dto.UserResponse, error)
	GenerateDevToken(ctx context.Context, req *dto.DevTokenRequest) (*dto.VerifyOTPResponse, error)
}

