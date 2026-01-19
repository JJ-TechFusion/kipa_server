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
	smsProvider     integration.SMSProvider
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
	smsProvider integration.SMSProvider,
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
		smsProvider:     smsProvider,
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

	// Generate OTP
	plainOTP, err := s.otpGenerator.Generate(s.config.OTP.Length)
	if err != nil {
		return nil, fmt.Errorf("failed to generate OTP: %w", err)
	}

	// Hash OTP
	hashedOTP, err := s.hasher.Hash(plainOTP)
	if err != nil {
		return nil, fmt.Errorf("failed to hash OTP: %w", err)
	}

	verificationID := s.otpGenerator.GenerateVerificationID()
	expiresAt := time.Now().Add(s.config.OTP.ExpiryDuration)

	// Store OTP
	otpRecord := &domain.OTPVerification{
		PhoneNumber:    req.PhoneNumber,
		VerificationID: verificationID,
		CodeHash:       hashedOTP,
		Purpose:        domain.OTPPurposeLogin,
		ExpiresAt:      expiresAt,
		Attempts:       0,
	}

	if err := s.otpRepo.Create(ctx, otpRecord); err != nil {
		return nil, fmt.Errorf("failed to store OTP: %w", err)
	}

	// Send OTP via SMS (with voice fallback)
	// In development mode, log OTP and skip SMS sending if it fails
	isDevelopment := s.config.Server.Environment == "development"
	
	if isDevelopment {
		s.logger.Info("🔐 DEVELOPMENT MODE - OTP Generated",
			zap.String("phone", req.PhoneNumber),
			zap.String("otp_code", plainOTP),
			zap.String("verification_id", verificationID.String()),
		)
	}
	
	result, err := s.smsProvider.SendWithFallback(ctx, req.PhoneNumber, plainOTP)
	if err != nil || !result.Success {
		if isDevelopment {
			// In development, log the error but don't fail the request
			s.logger.Warn("SMS delivery failed (continuing in dev mode)",
				zap.String("phone", req.PhoneNumber),
				zap.Error(err),
			)
		} else {
			// In production, fail the request if SMS fails
			s.logger.Error("failed to send OTP",
				zap.String("phone", req.PhoneNumber),
				zap.Error(err),
			)
			return nil, domain.ErrSMSDeliveryFailed
		}
	}

	// Increment rate limit
	if err := s.sessionRepo.IncrementRateLimitCount(ctx, req.PhoneNumber, s.config.OTP.RateLimitWindow); err != nil {
		s.logger.Error("failed to increment rate limit", zap.Error(err))
	}

	s.logger.Info("OTP sent successfully",
		zap.String("phone", req.PhoneNumber),
		zap.String("verification_id", verificationID.String()),
		zap.String("provider", func() string {
			if result != nil {
				return result.Provider
			}
			return "dev-mode"
		}()),
	)

	return &dto.SendOTPResponse{
		VerificationID: verificationID.String(),
		ExpiresAt:      expiresAt,
		RetryAfter:     60,
	}, nil
}

func (s *AuthServiceImpl) VerifyOTP(ctx context.Context, req *dto.VerifyOTPRequest, ipAddress, userAgent string) (*dto.VerifyOTPResponse, error) {
	// Check idempotency key first
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

	// Parse verification ID
	verificationID, err := uuid.Parse(req.VerificationID)
	if err != nil {
		return nil, domain.ErrInvalidOTP
	}

	// Get OTP record
	otpRecord, err := s.otpRepo.GetByVerificationID(ctx, req.PhoneNumber, verificationID)
	if err != nil {
		if errors.Is(err, domain.ErrOTPNotFound) {
			return nil, domain.ErrInvalidOTP
		}
		return nil, fmt.Errorf("failed to get OTP: %w", err)
	}

	// Check if already verified
	if otpRecord.VerifiedAt != nil {
		return nil, domain.ErrOTPAlreadyVerified
	}

	// Check expiry
	if time.Now().After(otpRecord.ExpiresAt) {
		return nil, domain.ErrOTPExpired
	}

	// Check attempts
	if otpRecord.Attempts >= s.config.OTP.MaxAttempts {
		return nil, domain.ErrMaxOTPAttemptsExceeded
	}

	// Increment attempts
	if err := s.otpRepo.IncrementAttempts(ctx, otpRecord.ID); err != nil {
		s.logger.Error("failed to increment OTP attempts", zap.Error(err))
	}

	// Verify OTP
	if !s.hasher.Compare(req.OTPCode, otpRecord.CodeHash) {
		remainingAttempts := s.config.OTP.MaxAttempts - otpRecord.Attempts - 1
		s.logger.Warn("invalid OTP attempt",
			zap.String("phone", req.PhoneNumber),
			zap.Int("remaining_attempts", remainingAttempts),
		)
		return nil, domain.ErrInvalidOTP
	}

	// Mark OTP as verified
	if err := s.otpRepo.MarkVerified(ctx, otpRecord.ID); err != nil {
		s.logger.Error("failed to mark OTP as verified", zap.Error(err))
	}

	// Get or create user
	user, isNewUser, err := s.getOrCreateUser(ctx, req.PhoneNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get or create user: %w", err)
	}

	// Check if user is suspended
	if user.IsSuspended {
		return nil, domain.ErrUserSuspended
	}

	// Create idempotency key
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

	// Revoke existing sessions for this device
	if err := s.sessionRepo.Delete(ctx, user.ID, req.DeviceID); err != nil {
		if !errors.Is(err, domain.ErrSessionNotFound) {
			s.logger.Error("failed to revoke existing session", zap.Error(err))
		}
	}

	// Generate tokens
	accessToken, accessExpiresAt, err := s.jwtManager.GenerateAccessToken(user.ID, req.DeviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken, refreshExpiresAt, err := s.jwtManager.GenerateRefreshToken(user.ID, req.DeviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	// Store session
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

	// Update last login
	if err := s.userRepo.UpdateLastLogin(ctx, user.ID); err != nil {
		s.logger.Error("failed to update last login", zap.Error(err))
	}

	// Set phone as verified if new user
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
	// Validate refresh token
	claims, err := s.jwtManager.ValidateRefreshToken(req.RefreshToken)
	if err != nil {
		if errors.Is(err, util.ErrExpiredToken) {
			return nil, domain.ErrTokenExpired
		}
		return nil, domain.ErrInvalidRefreshToken
	}

	// Validate session exists
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

	// Generate new tokens (rotation)
	accessToken, accessExpiresAt, err := s.jwtManager.GenerateAccessToken(claims.UserID, req.DeviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken, refreshExpiresAt, err := s.jwtManager.GenerateRefreshToken(claims.UserID, req.DeviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	// Update session with new refresh token
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

	// Update fields if provided
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

	// Create new user
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

func generateRequestHash(phoneNumber, verificationID, otpCode string) string {
	data := fmt.Sprintf("%s:%s:%s", phoneNumber, verificationID, otpCode)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// Ensure AuthServiceImpl implements AuthService
var _ AuthService = (*AuthServiceImpl)(nil)
