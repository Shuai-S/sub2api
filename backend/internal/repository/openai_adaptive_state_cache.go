package repository

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

var deleteExpiredAdaptiveSchedulerStatesScript = redis.NewScript(`
local members = redis.call('ZRANGEBYSCORE', KEYS[2], '-inf', ARGV[1], 'LIMIT', 0, ARGV[2])
local removed = 0
for _, member in ipairs(members) do
  local score = redis.call('ZSCORE', KEYS[2], member)
  if score and tonumber(score) <= tonumber(ARGV[1]) then
    removed = removed + redis.call('HDEL', KEYS[1], member)
    redis.call('ZREM', KEYS[2], member)
  end
end
return removed
`)

func adaptiveSchedulerStateKeys(namespace string) (hashKey string, expiryKey string, err error) {
	switch namespace {
	case "openai", "anthropic":
		prefix := "scheduler:adaptive:" + namespace + ":v1:"
		return prefix + "states", prefix + "expires", nil
	default:
		return "", "", fmt.Errorf("unsupported adaptive scheduler namespace %q", namespace)
	}
}

func (c *gatewayCache) ScanAdaptiveSchedulerStates(
	ctx context.Context,
	namespace string,
	cursor uint64,
	count int64,
) ([]service.AdaptiveSchedulerStateCacheRecord, uint64, error) {
	if c == nil || c.rdb == nil {
		return nil, 0, fmt.Errorf("redis cache is unavailable")
	}
	hashKey, _, err := adaptiveSchedulerStateKeys(namespace)
	if err != nil {
		return nil, 0, err
	}
	if count <= 0 {
		count = 256
	}
	values, nextCursor, err := c.rdb.HScan(ctx, hashKey, cursor, "*", count).Result()
	if err != nil {
		return nil, 0, err
	}
	records := make([]service.AdaptiveSchedulerStateCacheRecord, 0, len(values)/2)
	for i := 0; i+1 < len(values); i += 2 {
		accountID, err := strconv.ParseInt(values[i], 10, 64)
		if err != nil || accountID <= 0 {
			continue
		}
		records = append(records, service.AdaptiveSchedulerStateCacheRecord{
			AccountID: accountID,
			Payload:   []byte(values[i+1]),
		})
	}
	return records, nextCursor, nil
}

func (c *gatewayCache) SaveAdaptiveSchedulerStates(
	ctx context.Context,
	namespace string,
	entries []service.AdaptiveSchedulerStateCacheEntry,
	ttl time.Duration,
) error {
	if c == nil || c.rdb == nil {
		return fmt.Errorf("redis cache is unavailable")
	}
	if len(entries) == 0 {
		return nil
	}
	hashKey, expiryKey, err := adaptiveSchedulerStateKeys(namespace)
	if err != nil {
		return err
	}
	values := make(map[string]any, len(entries))
	expires := make([]redis.Z, 0, len(entries))
	for _, entry := range entries {
		if entry.AccountID <= 0 || len(entry.Payload) == 0 || entry.ExpiresAt.IsZero() {
			continue
		}
		member := strconv.FormatInt(entry.AccountID, 10)
		values[member] = entry.Payload
		expires = append(expires, redis.Z{Score: float64(entry.ExpiresAt.Unix()), Member: member})
	}
	if len(values) == 0 {
		return nil
	}
	// Keep the snapshot and its expiry index atomic so a concurrent cleanup
	// cannot delete a freshly written hash field using the previous expiry score.
	pipe := c.rdb.TxPipeline()
	pipe.HSet(ctx, hashKey, values)
	pipe.ZAdd(ctx, expiryKey, expires...)
	if ttl > 0 {
		pipe.Expire(ctx, hashKey, ttl)
		pipe.Expire(ctx, expiryKey, ttl)
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (c *gatewayCache) DeleteExpiredAdaptiveSchedulerStates(
	ctx context.Context,
	namespace string,
	before time.Time,
	limit int64,
) (int64, error) {
	if c == nil || c.rdb == nil {
		return 0, fmt.Errorf("redis cache is unavailable")
	}
	if limit <= 0 {
		limit = 256
	}
	hashKey, expiryKey, err := adaptiveSchedulerStateKeys(namespace)
	if err != nil {
		return 0, err
	}
	return deleteExpiredAdaptiveSchedulerStatesScript.Run(
		ctx,
		c.rdb,
		[]string{hashKey, expiryKey},
		before.Unix(),
		limit,
	).Int64()
}

var _ service.AdaptiveSchedulerStateCache = (*gatewayCache)(nil)
