package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"kipa/internal/config"
	"kipa/internal/domain"
	"kipa/internal/dto"
	"kipa/internal/integration"
	"kipa/internal/repository"
	"kipa/internal/util"
)

type AuthServiceImpl struct {
	userRepo        repository.UserRepository
	otpRepo         repository.OTPRepository
	idempotencyRepo repository.IdempotencyRepository
	sessionRepo     repository.SessionRepository
	verifyProvider  integration.VerifyProvider
	jwtManager      util.JWTManager
	hasher          util.Hasher
	otpGenerator    util.OTPGenerator
	config          *config.Config
	logger          *zap.Logger
}

func NewAuthService(
	userRepo repository.UserRepository,
	otpRepo repository.OTPRepository,
	idempotencyRepo repository.IdempotencyRepository,
	sessionRepo repository.SessionRepository,
	verifyProvider integration.VerifyProvider,
	jwtManager util.JWTManager,
	hasher util.Hasher,
	otpGenerator util.OTPGenerator,
	cfg *config.Config,
	logger *zap.Logger,
) AuthService {
	return &AuthServiceImpl{
		userRepo:        userRepo,
		otpRepo:         otpRepo,
		idempotencyRepo: idempotencyRepo,
		sessionRepo:     sessionRepo,
		verifyProvider:  verifyProvider,
		jwtManager:      jwtManager,
		hasher:          hasher,
		otpGenerator:    otpGenerator,
		config:          cfg,
		logger:          logger,
	}
}

func (s *AuthServiceImpl) SendOTP(ctx context.Context, req *dto.SendOTPRequest, ipAddress, userAgent string) (*dto.SendOTPResponse, error) {
	// Check rate limit
	count, err := s.sessionRepo.GetRateLimitCount(ctx, req.PhoneNumber)
	if err != nil {
		s.logger.Error("failed to check rate limit", zap.Error(err))
		return nil, fmt.Errorf("failed to check rate limit: %w", err)
	}

	if count >= s.config.OTP.RateLimitCount {
		s.logger.Warn("OTP rate limit exceeded",
			zap.String("phone", req.PhoneNumber),
			zap.Int("count", count),
		)
		return nil, domain.ErrOTPRateLimitExceeded
	}

	// Send verification via Twilio Verify
	result, err := s.verifyProvider.SendVerification(ctx, req.PhoneNumber, "sms")
	if err != nil || !result.Success {
		s.logger.Error("failed to send verification",
			zap.String("phone", req.PhoneNumber),
			zap.Error(err),
		)
		return nil, domain.ErrSMSDeliveryFailed
	}

	// Increment rate limit
	if err := s.sessionRepo.IncrementRateLimitCount(ctx, req.PhoneNumber, s.config.OTP.RateLimitWindow); err != nil {
		s.logger.Error("failed to increment rate limit", zap.Error(err))
	}

	s.logger.Info("verification sent successfully",
		zap.String("phone", req.PhoneNumber),
		zap.String("status", result.Status),
	)

	// Use phone number as verification ID since Twilio manages the verification
	return &dto.SendOTPResponse{
		VerificationID: req.PhoneNumber,
		ExpiresAt:      time.Now().Add(s.config.OTP.ExpiryDuration),
		RetryAfter:     60,
	}, nil
}

func (s *AuthServiceImpl) VerifyOTP(ctx context.Context, req *dto.VerifyOTPRequest, ipAddress, userAgent string) (*dto.VerifyOTPResponse, error) {
	// Check idempotency
	exists, err := s.idempotencyRepo.Exists(ctx, req.IdempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("failed to check idempotency: %w", err)
	}
	if exists {
		s.logger.Warn("duplicate OTP verification request",
			zap.String("idempotency_key", req.IdempotencyKey),
		)
		return nil, domain.ErrIdempotencyKeyExists
	}

	// Verify code with Twilio Verify
	result, err := s.verifyProvider.CheckVerification(ctx, req.PhoneNumber, req.OTPCode)
	if err != nil {
		s.logger.Error("verification check failed",
			zap.String("phone", req.PhoneNumber),
			zap.Error(err),
		)
		return nil, domain.ErrInvalidOTP
	}

	if !result.Success {
		s.logger.Warn("invalid OTP attempt",
			zap.String("phone", req.PhoneNumber),
			zap.String("status", result.Status),
		)
		return nil, domain.ErrInvalidOTP
	}


	user, isNewUser, err := s.getOrCreateUser(ctx, req.PhoneNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get or create user: %w", err)
	}

	if user.IsSuspended {
		return nil, domain.ErrUserSuspended
	}


	idempotencyKey := &domain.IdempotencyKey{
		Key:         req.IdempotencyKey,
		Operation:   "verify_otp",
		RequestHash: generateRequestHash(req.PhoneNumber, req.VerificationID, req.OTPCode),
	}
	if err := s.idempotencyRepo.Create(ctx, idempotencyKey); err != nil {
		if !errors.Is(err, domain.ErrIdempotencyKeyExists) {
			s.logger.Error("failed to create idempotency key", zap.Error(err))
		}
	}

	
	if err := s.sessionRepo.Delete(ctx, user.ID, req.DeviceID); err != nil {
		if !errors.Is(err, domain.ErrSessionNotFound) {
			s.logger.Error("failed to revoke existing session", zap.Error(err))
		}
	}

	accessToken, accessExpiresAt, err := s.jwtManager.GenerateAccessToken(user.ID, req.DeviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken, refreshExpiresAt, err := s.jwtManager.GenerateRefreshToken(user.ID, req.DeviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}


	session := &domain.Session{
		UserID:       user.ID,
		DeviceID:     req.DeviceID,
		RefreshToken: refreshToken,
		UserAgent:    userAgent,
		IPAddress:    ipAddress,
		CreatedAt:    time.Now(),
		ExpiresAt:    refreshExpiresAt,
	}
	if err := s.sessionRepo.Store(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to store session: %w", err)
	}


	if err := s.userRepo.UpdateLastLogin(ctx, user.ID); err != nil {
		s.logger.Error("failed to update last login", zap.Error(err))
	}


	if isNewUser {
		if err := s.userRepo.SetPhoneVerified(ctx, user.ID); err != nil {
			s.logger.Error("failed to set phone verified", zap.Error(err))
		}
	}

	s.logger.Info("OTP verified successfully",
		zap.String("user_id", user.ID.String()),
		zap.Bool("is_new_user", isNewUser),
	)

	return &dto.VerifyOTPResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int(time.Until(accessExpiresAt).Seconds()),
		TokenType:    "Bearer",
		User:         userToResponse(user),
		IsNewUser:    isNewUser,
	}, nil
}

func (s *AuthServiceImpl) RefreshToken(ctx context.Context, req *dto.RefreshTokenRequest) (*dto.RefreshTokenResponse, error) {

	claims, err := s.jwtManager.ValidateRefreshToken(req.RefreshToken)
	if err != nil {
		if errors.Is(err, util.ErrExpiredToken) {
			return nil, domain.ErrTokenExpired
		}
		return nil, domain.ErrInvalidRefreshToken
	}


	valid, err := s.sessionRepo.ValidateRefreshToken(ctx, claims.UserID, req.DeviceID, req.RefreshToken)
	if err != nil {
		if errors.Is(err, domain.ErrSessionNotFound) {
			return nil, domain.ErrSessionRevoked
		}
		return nil, fmt.Errorf("failed to validate session: %w", err)
	}
	if !valid {
		return nil, domain.ErrInvalidRefreshToken
	}


	accessToken, accessExpiresAt, err := s.jwtManager.GenerateAccessToken(claims.UserID, req.DeviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken, refreshExpiresAt, err := s.jwtManager.GenerateRefreshToken(claims.UserID, req.DeviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}


	session := &domain.Session{
		UserID:       claims.UserID,
		DeviceID:     req.DeviceID,
		RefreshToken: refreshToken,
		CreatedAt:    time.Now(),
		ExpiresAt:    refreshExpiresAt,
	}
	if err := s.sessionRepo.Store(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to update session: %w", err)
	}

	return &dto.RefreshTokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int(time.Until(accessExpiresAt).Seconds()),
		TokenType:    "Bearer",
	}, nil
}

func (s *AuthServiceImpl) Logout(ctx context.Context, userID uuid.UUID, req *dto.LogoutRequest) (*dto.LogoutResponse, error) {
	if req.AllDevices {
		if err := s.sessionRepo.DeleteAllForUser(ctx, userID); err != nil {
			return nil, fmt.Errorf("failed to logout from all devices: %w", err)
		}
		return &dto.LogoutResponse{Message: "Successfully logged out from all devices"}, nil
	}

	if err := s.sessionRepo.Delete(ctx, userID, req.DeviceID); err != nil {
		if !errors.Is(err, domain.ErrSessionNotFound) {
			return nil, fmt.Errorf("failed to logout: %w", err)
		}
	}

	return &dto.LogoutResponse{Message: "Successfully logged out"}, nil
}

func (s *AuthServiceImpl) ValidateAccessToken(ctx context.Context, token string) (*util.AccessTokenClaims, error) {
	claims, err := s.jwtManager.ValidateAccessToken(token)
	if err != nil {
		if errors.Is(err, util.ErrExpiredToken) {
			return nil, domain.ErrTokenExpired
		}
		return nil, domain.ErrInvalidToken
	}
	return claims, nil
}

func (s *AuthServiceImpl) GetUserByID(ctx context.Context, userID uuid.UUID) (*dto.UserResponse, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if user.IsSuspended {
		return nil, domain.ErrUserSuspended
	}

	response := userToResponse(user)
	return &response, nil
}

func (s *AuthServiceImpl) UpdateProfile(ctx context.Context, userID uuid.UUID, req *dto.UpdateProfileRequest) (*dto.UserResponse, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if user.IsSuspended {
		return nil, domain.ErrUserSuspended
	}

	if req.FirstName != nil {
		user.FirstName = req.FirstName
	}
	if req.LastName != nil {
		user.LastName = req.LastName
	}
	if req.Email != nil {
		user.Email = req.Email
	}
	if req.ProfilePhotoURL != nil {
		user.ProfilePhotoURL = req.ProfilePhotoURL
	}

	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to update profile: %w", err)
	}

	s.logger.Info("user profile updated",
		zap.String("user_id", userID.String()),
	)

	response := userToResponse(user)
	return &response, nil
}

func (s *AuthServiceImpl) getOrCreateUser(ctx context.Context, phoneNumber string) (*domain.User, bool, error) {
	user, err := s.userRepo.GetByPhoneNumber(ctx, phoneNumber)
	if err == nil {
		return user, false, nil
	}

	if !errors.Is(err, domain.ErrUserNotFound) {
		return nil, false, err
	}

	newUser := &domain.User{
		PhoneNumber:   phoneNumber,
		PhoneVerified: true,
		IsSeller:      false,
		IsBuyer:       false,
		IsRider:       false,
	}

	if err := s.userRepo.Create(ctx, newUser); err != nil {
		return nil, false, fmt.Errorf("failed to create user: %w", err)
	}

	return newUser, true, nil
}

func userToResponse(user *domain.User) dto.UserResponse {
	return dto.UserResponse{
		ID:              user.ID,
		PhoneNumber:     user.PhoneNumber,
		PhoneVerified:   user.PhoneVerified,
		Email:           user.Email,
		FirstName:       user.FirstName,
		LastName:        user.LastName,
		IsSeller:        user.IsSeller,
		IsBuyer:         user.IsBuyer,
		IsRider:         user.IsRider,
		SellerVerified:  user.SellerVerified,
		ProfilePhotoURL: user.ProfilePhotoURL,
		CreatedAt:       user.CreatedAt,
	}
}

func (s *AuthServiceImpl) GenerateDevToken(ctx context.Context, req *dto.DevTokenRequest) (*dto.VerifyOTPResponse, error) {
	// Only allow in development environment
	if s.config.Server.Environment != "development" {
		return nil, domain.ErrUnauthorized
	}

	// Check if user exists
	existingUser, err := s.userRepo.GetByPhoneNumber(ctx, req.PhoneNumber)
	if err != nil && !errors.Is(err, domain.ErrUserNotFound) {
		return nil, err
	}

	var user *domain.User
	if existingUser != nil {
		// User exists, update their role if needed
		user = existingUser
		switch req.Role {
		case "buyer":
			user.IsBuyer = true
		case "seller":
			user.IsSeller = true
		case "rider":
			user.IsRider = true
		case "admin":
			user.IsAdmin = true
		}
		
		// Update first and last name
		user.FirstName = &req.FirstName
		user.LastName = &req.LastName
		
		if err := s.userRepo.Update(ctx, user); err != nil {
			return nil, err
		}
	} else {
		// Create new user
		user = &domain.User{
			ID:            uuid.New(),
			PhoneNumber:   req.PhoneNumber,
			FirstName:     &req.FirstName,
			LastName:      &req.LastName,
			PhoneVerified: true, // Auto-verify for dev
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}

		// Set role
		switch req.Role {
		case "buyer":
			user.IsBuyer = true
		case "seller":
			user.IsSeller = true
		case "rider":
			user.IsRider = true
		case "admin":
			user.IsAdmin = true
		}

		if err := s.userRepo.Create(ctx, user); err != nil {
			return nil, err
		}
	}

	// Generate tokens
	accessToken, accessExpiresAt, err := s.jwtManager.GenerateAccessToken(user.ID, user.PhoneNumber)
	if err != nil {
		return nil, err
	}

	refreshToken, refreshExpiresAt, err := s.jwtManager.GenerateRefreshToken(user.ID, user.PhoneNumber)
	if err != nil {
		return nil, err
	}

	// Store refresh token
	session := &domain.Session{
		UserID:       user.ID,
		DeviceID:     "dev-session", // Default device ID for dev
		RefreshToken: refreshToken,
		UserAgent:    "dev-client",
		IPAddress:    "127.0.0.1",
		CreatedAt:    time.Now(),
		ExpiresAt:    refreshExpiresAt,
	}
	if err := s.sessionRepo.Store(ctx, session); err != nil {
		return nil, err
	}

	userResponse := userToResponse(user)
	return &dto.VerifyOTPResponse{
		User:         userResponse,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int(time.Until(accessExpiresAt).Seconds()),
		TokenType:    "Bearer",
		IsNewUser:    existingUser == nil,
	}, nil
}

func generateRequestHash(phoneNumber, verificationID, otpCode string) string {
	data := fmt.Sprintf("%s:%s:%s", phoneNumber, verificationID, otpCode)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}
var _ AuthService = (*AuthServiceImpl)(nil)
