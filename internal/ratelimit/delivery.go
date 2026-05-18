// Package ratelimit provides Redis-backed sliding window rate limiting.
package ratelimit

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// DeliveryWindow is the length of the sliding window.
	DeliveryWindow = 3600 * time.Second // 1 hour

	// DeliveryMaxCount is the maximum number of deliveries allowed per window.
	DeliveryMaxCount = 5

	// deliveryKeyPrefix is the Redis key prefix for the delivery sorted set.
	deliveryKeyPrefix = "delivery:quota:"

	// deliveryKeyTTL is the TTL set on the Redis key after each write.
	// 2× the window gives the key enough lifetime to cover the full sliding range
	// without causing premature expiry.
	deliveryKeyTTL = 2 * DeliveryWindow
)

// DeliveryLimiter implements a sliding-window rate limiter for project deliveries
// using a Redis Sorted Set.
//
// Key schema:
//
//	Key    delivery:quota:{userID}
//	Type   Sorted Set
//	Score  Unix timestamp (seconds) of the delivery
//	Member applicationID (unique within the window)
type DeliveryLimiter struct {
	rdb *redis.Client
}

// NewDeliveryLimiter creates a DeliveryLimiter backed by rdb.
func NewDeliveryLimiter(rdb *redis.Client) *DeliveryLimiter {
	return &DeliveryLimiter{rdb: rdb}
}

// QuotaInfo is the current usage state for one user.
type QuotaInfo struct {
	UsedCount int
	Limit     int
	// ResetTime is the moment the oldest in-window record expires (nil when empty).
	ResetTime *time.Time
}

func redisKey(userID int) string {
	return fmt.Sprintf("%s%d", deliveryKeyPrefix, userID)
}

// GetQuota returns the current delivery quota state for userID.
// Expired entries (older than DeliveryWindow) are cleaned up as a side effect.
// On Redis error the function returns a zeroed-out QuotaInfo and logs the error;
// callers decide whether to fail open or closed.
func (l *DeliveryLimiter) GetQuota(ctx context.Context, userID int) (QuotaInfo, error) {
	k := redisKey(userID)
	now := time.Now().Unix()
	cutoff := now - int64(DeliveryWindow/time.Second)

	pipe := l.rdb.Pipeline()
	pipe.ZRemRangeByScore(ctx, k, "0", fmt.Sprintf("%d", cutoff))
	zcardCmd := pipe.ZCard(ctx, k)
	// Fetch the oldest remaining member to calculate resetTime.
	zrangeCmd := pipe.ZRangeWithScores(ctx, k, 0, 0)

	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return QuotaInfo{Limit: DeliveryMaxCount}, fmt.Errorf("redis GetQuota: %w", err)
	}

	count := int(zcardCmd.Val())
	info := QuotaInfo{UsedCount: count, Limit: DeliveryMaxCount}

	if members := zrangeCmd.Val(); len(members) > 0 {
		t := time.Unix(int64(members[0].Score)+int64(DeliveryWindow/time.Second), 0)
		info.ResetTime = &t
	}

	return info, nil
}

// Allow checks whether userID is within the rate limit.
// Returns (true, info, nil) when the request should be allowed.
// Fails open on Redis errors (returns true) to avoid blocking users.
func (l *DeliveryLimiter) Allow(ctx context.Context, userID int) (bool, QuotaInfo) {
	info, err := l.GetQuota(ctx, userID)
	if err != nil {
		log.Printf("[DeliveryLimiter] Allow error (fail-open): %v", err)
		return true, QuotaInfo{Limit: DeliveryMaxCount}
	}
	return info.UsedCount < DeliveryMaxCount, info
}

// Record registers a successful delivery in the sliding window.
// member must be unique within the window (use the applicationID string).
// Errors are logged but not returned; a failed record only means the limiter
// may under-count, not that the user's delivery was invalid.
func (l *DeliveryLimiter) Record(ctx context.Context, userID int, member string) {
	k := redisKey(userID)
	now := float64(time.Now().Unix())

	pipe := l.rdb.Pipeline()
	pipe.ZAdd(ctx, k, redis.Z{Score: now, Member: member})
	pipe.Expire(ctx, k, deliveryKeyTTL)

	if _, err := pipe.Exec(ctx); err != nil {
		log.Printf("[DeliveryLimiter] Record error (non-fatal): %v", err)
	}
}
