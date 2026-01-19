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
type DistributedLock struct {
	client *redis.Client
}
func NewDistributedLock(client *redis.Client) *DistributedLock {
	return &DistributedLock{
		client: client,
	}
}
func (l *DistributedLock) Lock(ctx context.Context, name string, ttl time.Duration) (release func(), acquired bool) {
	key := lockKey(name)
	lockValue := uuid.New().String() // Unique value to ensure only the owner can release
	ok, err := l.client.SetNX(ctx, key, lockValue, ttl).Result()
	if err != nil || !ok {
		return nil, false
	}
	release = func() {
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
func (l *DistributedLock) TryLockWithRetry(ctx context.Context, name string, ttl time.Duration, maxRetries int, retryDelay time.Duration) (release func(), acquired bool) {
	for i := 0; i <= maxRetries; i++ {
		release, acquired = l.Lock(ctx, name, ttl)
		if acquired {
			return release, true
		}
		select {
		case <-ctx.Done():
			return nil, false
		case <-time.After(retryDelay):
		}
	}

	return nil, false
}
func (l *DistributedLock) Extend(ctx context.Context, name string, ttl time.Duration) bool {
	key := lockKey(name)
	ok, err := l.client.Expire(ctx, key, ttl).Result()
	return err == nil && ok
}
func (l *DistributedLock) IsLocked(ctx context.Context, name string) bool {
	key := lockKey(name)
	exists, _ := l.client.Exists(ctx, key).Result()
	return exists > 0
}

func lockKey(name string) string {
	return fmt.Sprintf("%s%s", lockKeyPrefix, name)
}
