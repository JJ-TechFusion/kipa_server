package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"kipa/internal/domain"
	"kipa/internal/repository"
)

const (
	sessionKeyPrefix   = "session:"
	userSessionsPrefix = "user_sessions:"
	rateLimitPrefix    = "otp_rate:"
)

type SessionRepo struct {
	client *redis.Client
	ttl    time.Duration
}

func NewSessionRepo(client *redis.Client, ttl time.Duration) repository.SessionRepository {
	return &SessionRepo{
		client: client,
		ttl:    ttl,
	}
}

func (r *SessionRepo) Store(ctx context.Context, session *domain.Session) error {
	key := sessionKey(session.UserID, session.DeviceID)

	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	pipe := r.client.Pipeline()
	pipe.Set(ctx, key, data, r.ttl)
	pipe.SAdd(ctx, userSessionsKey(session.UserID), session.DeviceID)
	pipe.Expire(ctx, userSessionsKey(session.UserID), r.ttl)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to store session: %w", err)
	}

	return nil
}

func (r *SessionRepo) Get(ctx context.Context, userID uuid.UUID, deviceID string) (*domain.Session, error) {
	key := sessionKey(userID, deviceID)

	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, domain.ErrSessionNotFound
		}
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	var session domain.Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return &session, nil
}

func (r *SessionRepo) Delete(ctx context.Context, userID uuid.UUID, deviceID string) error {
	key := sessionKey(userID, deviceID)

	pipe := r.client.Pipeline()
	pipe.Del(ctx, key)
	pipe.SRem(ctx, userSessionsKey(userID), deviceID)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	return nil
}

func (r *SessionRepo) DeleteAllForUser(ctx context.Context, userID uuid.UUID) error {
	userKey := userSessionsKey(userID)

	deviceIDs, err := r.client.SMembers(ctx, userKey).Result()
	if err != nil {
		return fmt.Errorf("failed to get user sessions: %w", err)
	}

	if len(deviceIDs) == 0 {
		return nil
	}

	pipe := r.client.Pipeline()
	for _, deviceID := range deviceIDs {
		pipe.Del(ctx, sessionKey(userID, deviceID))
	}
	pipe.Del(ctx, userKey)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete user sessions: %w", err)
	}

	return nil
}

func (r *SessionRepo) ValidateRefreshToken(ctx context.Context, userID uuid.UUID, deviceID, refreshToken string) (bool, error) {
	session, err := r.Get(ctx, userID, deviceID)
	if err != nil {
		return false, err
	}

	return session.RefreshToken == refreshToken, nil
}

func (r *SessionRepo) GetRateLimitCount(ctx context.Context, phoneNumber string) (int, error) {
	key := rateLimitKey(phoneNumber)

	count, err := r.client.Get(ctx, key).Int()
	if err != nil {
		if err == redis.Nil {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to get rate limit count: %w", err)
	}

	return count, nil
}

func (r *SessionRepo) IncrementRateLimitCount(ctx context.Context, phoneNumber string, window time.Duration) error {
	key := rateLimitKey(phoneNumber)

	pipe := r.client.Pipeline()
	pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, window)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to increment rate limit: %w", err)
	}

	return nil
}

func sessionKey(userID uuid.UUID, deviceID string) string {
	return fmt.Sprintf("%s%s:%s", sessionKeyPrefix, userID.String(), deviceID)
}

func userSessionsKey(userID uuid.UUID) string {
	return fmt.Sprintf("%s%s", userSessionsPrefix, userID.String())
}

func rateLimitKey(phoneNumber string) string {
	return fmt.Sprintf("%s%s", rateLimitPrefix, phoneNumber)
}
