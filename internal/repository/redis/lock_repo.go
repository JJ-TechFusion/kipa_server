package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

const (
	lockKeyPrefix = "lock:"
)

// DistributedLock provides distributed locking via Redis
type DistributedLock struct {
	client *redis.Client
}

// NewDistributedLock creates a new distributed lock manager
func NewDistributedLock(client *redis.Client) *DistributedLock {
	return &DistributedLock{
		client: client,
	}
}

// Lock attempts to acquire a lock with the given name.
// Returns a release function that should be called when done, and a boolean indicating success.
// The lock will automatically expire after the TTL even if Release is not called.
func (l *DistributedLock) Lock(ctx context.Context, name string, ttl time.Duration) (release func(), acquired bool) {
	key := lockKey(name)
	lockValue := uuid.New().String() // Unique value to ensure only the owner can release

	// Use SET NX (only set if not exists) with expiry
	ok, err := l.client.SetNX(ctx, key, lockValue, ttl).Result()
	if err != nil || !ok {
		return nil, false
	}

	// Return release function
	release = func() {
		// Only delete if we still own the lock (compare and delete)
		// Using Lua script for atomicity
		script := redis.NewScript(`
			if redis.call("GET", KEYS[1]) == ARGV[1] then
				return redis.call("DEL", KEYS[1])
			else
				return 0
			end
		`)
		script.Run(ctx, l.client, []string{key}, lockValue)
	}

	return release, true
}

// TryLockWithRetry attempts to acquire a lock with retries
func (l *DistributedLock) TryLockWithRetry(ctx context.Context, name string, ttl time.Duration, maxRetries int, retryDelay time.Duration) (release func(), acquired bool) {
	for i := 0; i <= maxRetries; i++ {
		release, acquired = l.Lock(ctx, name, ttl)
		if acquired {
			return release, true
		}

		// Check if context is cancelled before retrying
		select {
		case <-ctx.Done():
			return nil, false
		case <-time.After(retryDelay):
			// Continue to next retry
		}
	}

	return nil, false
}

// Extend extends the TTL of an existing lock (for long-running operations)
func (l *DistributedLock) Extend(ctx context.Context, name string, ttl time.Duration) bool {
	key := lockKey(name)
	ok, err := l.client.Expire(ctx, key, ttl).Result()
	return err == nil && ok
}

// IsLocked checks if a lock is currently held
func (l *DistributedLock) IsLocked(ctx context.Context, name string) bool {
	key := lockKey(name)
	exists, _ := l.client.Exists(ctx, key).Result()
	return exists > 0
}

func lockKey(name string) string {
	return fmt.Sprintf("%s%s", lockKeyPrefix, name)
}
