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
	escrowRepo      repository.EscrowRepository
	deliveryJobRepo repository.DeliveryJobRepository
	walletRepo      repository.WalletRepository
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
	escrowRepo repository.EscrowRepository,
	deliveryJobRepo repository.DeliveryJobRepository,
	walletRepo repository.WalletRepository,
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
		escrowRepo:      escrowRepo,
		deliveryJobRepo: deliveryJobRepo,
		walletRepo:      walletRepo,
		verifyProvider:  verifyProvider,
		jwtManager:      jwtManager,
		hasher:          hasher,
		otpGenerator:    otpGenerator,
		config:          cfg,
		logger:          logger,
	}
}

func (s *AuthServiceImpl) SendOTP(ctx context.Context, req *dto.SendOTPRequest, ipAddress, userAgent string) (*dto.SendOTPResponse, error) {
	req.PhoneNumber = util.NormalizePhoneNumber(req.PhoneNumber)

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

	result, err := s.verifyProvider.SendVerification(ctx, req.PhoneNumber, "sms")
	if err != nil || !result.Success {
		s.logger.Error("failed to send verification",
			zap.String("phone", req.PhoneNumber),
			zap.Error(err),
		)
		return nil, domain.ErrSMSDeliveryFailed
	}

	if err := s.sessionRepo.IncrementRateLimitCount(ctx, req.PhoneNumber, s.config.OTP.RateLimitWindow); err != nil {
		s.logger.Error("failed to increment rate limit", zap.Error(err))
	}

	if err := s.sessionRepo.SetLastOTPSentTime(ctx, req.PhoneNumber, s.config.OTP.ResendCooldown); err != nil {
		s.logger.Error("failed to set last OTP sent time", zap.Error(err))
	}

	s.logger.Info("verification sent successfully",
		zap.String("phone", req.PhoneNumber),
		zap.String("status", result.Status),
	)

	// Generate idempotency key for the client to use during verification
	idempotencyKey := uuid.New().String() + uuid.New().String()[:8]

	// Use phone number as verification ID since Twilio manages the verification
	return &dto.SendOTPResponse{
		VerificationID: req.PhoneNumber,
		IdempotencyKey: idempotencyKey,
		ExpiresAt:      time.Now().Add(s.config.OTP.ExpiryDuration),
		RetryAfter:     int(s.config.OTP.ResendCooldown.Seconds()),
	}, nil
}

func (s *AuthServiceImpl) ResendOTP(ctx context.Context, req *dto.ResendOTPRequest, ipAddress, userAgent string) (*dto.SendOTPResponse, error) {
	// Normalize phone number to handle variations like +23408... → +2348...
	req.PhoneNumber = util.NormalizePhoneNumber(req.PhoneNumber)

	// Check resend cooldown
	lastSent, err := s.sessionRepo.GetLastOTPSentTime(ctx, req.PhoneNumber)
	if err != nil {
		s.logger.Error("failed to check last OTP sent time", zap.Error(err))
		return nil, fmt.Errorf("failed to check cooldown: %w", err)
	}

	if !lastSent.IsZero() {
		elapsed := time.Since(lastSent)
		if elapsed < s.config.OTP.ResendCooldown {
			remaining := s.config.OTP.ResendCooldown - elapsed
			s.logger.Warn("OTP resend cooldown not elapsed",
				zap.String("phone", req.PhoneNumber),
				zap.Duration("remaining", remaining),
			)
			return nil, domain.ErrOTPResendCooldown
		}
	}

	// Check overall rate limit
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
		s.logger.Error("failed to resend verification",
			zap.String("phone", req.PhoneNumber),
			zap.Error(err),
		)
		return nil, domain.ErrSMSDeliveryFailed
	}

	// Increment rate limit
	if err := s.sessionRepo.IncrementRateLimitCount(ctx, req.PhoneNumber, s.config.OTP.RateLimitWindow); err != nil {
		s.logger.Error("failed to increment rate limit", zap.Error(err))
	}

	// Update last OTP sent time
	if err := s.sessionRepo.SetLastOTPSentTime(ctx, req.PhoneNumber, s.config.OTP.ResendCooldown); err != nil {
		s.logger.Error("failed to set last OTP sent time", zap.Error(err))
	}

	s.logger.Info("verification resent successfully",
		zap.String("phone", req.PhoneNumber),
		zap.String("status", result.Status),
	)

	// Generate new idempotency key for the resend
	idempotencyKey := uuid.New().String() + uuid.New().String()[:8]

	return &dto.SendOTPResponse{
		VerificationID: req.PhoneNumber,
		IdempotencyKey: idempotencyKey,
		ExpiresAt:      time.Now().Add(s.config.OTP.ExpiryDuration),
		RetryAfter:     int(s.config.OTP.ResendCooldown.Seconds()),
	}, nil
}

func (s *AuthServiceImpl) VerifyOTP(ctx context.Context, req *dto.VerifyOTPRequest, ipAddress, userAgent string) (*dto.VerifyOTPResponse, error) {
	// Normalize phone number to handle variations like +23408... → +2348...
	req.PhoneNumber = util.NormalizePhoneNumber(req.PhoneNumber)

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

func (s *AuthServiceImpl) DeleteAccount(ctx context.Context, userID uuid.UUID) error {
	// Check for active escrows
	hasActiveEscrows, err := s.escrowRepo.HasActiveByUserID(ctx, userID)
	if err != nil {
		s.logger.Error("failed to check active escrows", zap.Error(err))
		return fmt.Errorf("failed to check active escrows: %w", err)
	}
	if hasActiveEscrows {
		return domain.ErrActiveEscrowsExist
	}

	// Check for active delivery jobs (as rider)
	hasActiveDeliveries, err := s.deliveryJobRepo.HasActiveByRiderUserID(ctx, userID)
	if err != nil {
		s.logger.Error("failed to check active deliveries", zap.Error(err))
		return fmt.Errorf("failed to check active deliveries: %w", err)
	}
	if hasActiveDeliveries {
		return domain.ErrActiveDeliveriesExist
	}

	// Check wallet balance
	wallet, err := s.walletRepo.GetByUserID(ctx, userID)
	if err != nil && !errors.Is(err, domain.ErrWalletNotFound) {
		s.logger.Error("failed to get wallet", zap.Error(err))
		return fmt.Errorf("failed to check wallet: %w", err)
	}
	if wallet != nil && (wallet.AvailableBalance > 0 || wallet.LockedBalance > 0) {
		return domain.ErrWalletBalanceNotZero
	}

	// Soft delete user (anonymize PII, suspend account)
	if err := s.userRepo.SoftDelete(ctx, userID); err != nil {
		s.logger.Error("failed to soft delete user", zap.Error(err))
		return fmt.Errorf("failed to delete account: %w", err)
	}

	// Invalidate all sessions
	if err := s.sessionRepo.DeleteAllForUser(ctx, userID); err != nil {
		s.logger.Error("failed to delete sessions", zap.Error(err))
		// Don't fail the deletion for this
	}

	s.logger.Info("user account deleted",
		zap.String("user_id", userID.String()),
	)

	return nil
}

var _ AuthService = (*AuthServiceImpl)(nil)
