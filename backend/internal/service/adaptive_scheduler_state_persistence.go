package service

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"os"
	"strconv"
	"sync"
	"time"
)

func adaptiveStateInstanceID() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "unknown"
	}
	return hostname + "-" + strconv.Itoa(os.Getpid())
}

const (
	adaptiveStateFlushInterval        = 5 * time.Minute
	adaptiveStateFlushJitter          = 30 * time.Second
	adaptiveStateRetention            = 12 * time.Hour
	adaptiveStateHashTTL              = 24 * time.Hour
	adaptiveStateRestoreTimeout       = 5 * time.Second
	adaptiveStateRedisTimeout         = time.Second
	adaptiveStateShutdownFlushTimeout = time.Second
	adaptiveStateCleanupBatchSize     = 256
)

const (
	adaptiveSchedulerStateNamespaceOpenAI    = "openai"
	adaptiveSchedulerStateNamespaceAnthropic = "anthropic"
)

// AdaptiveSchedulerStateCacheRecord is one raw account snapshot read from Redis.
type AdaptiveSchedulerStateCacheRecord struct {
	AccountID int64
	Payload   []byte
}

// AdaptiveSchedulerStateCacheEntry is one account snapshot written to Redis.
type AdaptiveSchedulerStateCacheEntry struct {
	AccountID int64
	Payload   []byte
	ExpiresAt time.Time
}

// AdaptiveSchedulerStateCache is an optional GatewayCache capability. Runtime
// scheduling never reads it; it is used for one startup restore and periodic
// best-effort checkpoints only.
type AdaptiveSchedulerStateCache interface {
	ScanAdaptiveSchedulerStates(ctx context.Context, namespace string, cursor uint64, count int64) ([]AdaptiveSchedulerStateCacheRecord, uint64, error)
	SaveAdaptiveSchedulerStates(ctx context.Context, namespace string, entries []AdaptiveSchedulerStateCacheEntry, ttl time.Duration) error
	DeleteExpiredAdaptiveSchedulerStates(ctx context.Context, namespace string, before time.Time, limit int64) (int64, error)
}

type adaptiveStateRestoreFunc func(context.Context) (restored, stale, invalid int, err error)
type adaptiveStateFlushFunc func(context.Context) error

type adaptiveStatePersistenceWorker struct {
	cache     AdaptiveSchedulerStateCache
	namespace string
	restore   adaptiveStateRestoreFunc
	flush     adaptiveStateFlushFunc
	now       func() time.Time

	startOnce sync.Once
	stopOnce  sync.Once
	done      chan struct{}
	cancel    context.CancelFunc

	flushInterval  time.Duration
	flushJitter    time.Duration
	restoreTimeout time.Duration
	redisTimeout   time.Duration
}

func newAdaptiveStatePersistenceWorker(
	cache AdaptiveSchedulerStateCache,
	namespace string,
	restore adaptiveStateRestoreFunc,
	flush adaptiveStateFlushFunc,
	now func() time.Time,
) *adaptiveStatePersistenceWorker {
	return &adaptiveStatePersistenceWorker{
		cache:          cache,
		namespace:      namespace,
		restore:        restore,
		flush:          flush,
		now:            now,
		done:           make(chan struct{}),
		flushInterval:  adaptiveStateFlushInterval,
		flushJitter:    adaptiveStateFlushJitter,
		restoreTimeout: adaptiveStateRestoreTimeout,
		redisTimeout:   adaptiveStateRedisTimeout,
	}
}

func (w *adaptiveStatePersistenceWorker) Start() {
	if w == nil || w.cache == nil || w.restore == nil || w.flush == nil {
		return
	}
	w.startOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		w.cancel = cancel
		go w.run(ctx)
	})
}

func (w *adaptiveStatePersistenceWorker) Stop(ctx context.Context) error {
	if w == nil || w.cancel == nil {
		return nil
	}
	w.stopOnce.Do(w.cancel)
	select {
	case <-w.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *adaptiveStatePersistenceWorker) run(ctx context.Context) {
	defer close(w.done)

	restoreCtx, cancelRestore := context.WithTimeout(ctx, w.restoreTimeout)
	restored, stale, invalid, err := w.restore(restoreCtx)
	cancelRestore()
	if err != nil {
		slog.Warn(w.namespace+"_adaptive_state_restore_failed", "error", err)
	} else {
		slog.Info(w.namespace+"_adaptive_state_restore_completed",
			"restored_accounts", restored,
			"stale_accounts", stale,
			"invalid_accounts", invalid,
		)
	}

	timer := time.NewTimer(w.nextFlushDelay())
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			flushCtx, cancel := context.WithTimeout(context.Background(), adaptiveStateShutdownFlushTimeout)
			if err := w.flush(flushCtx); err != nil {
				slog.Warn(w.namespace+"_adaptive_state_shutdown_flush_failed", "error", err)
			}
			cancel()
			return
		case <-timer.C:
			w.flushAndCleanup(ctx)
			timer.Reset(w.nextFlushDelay())
		}
	}
}

func (w *adaptiveStatePersistenceWorker) nextFlushDelay() time.Duration {
	delay := w.flushInterval
	if w.flushJitter <= 0 {
		return delay
	}
	width := int64(2*w.flushJitter) + 1
	delay += time.Duration(rand.Int64N(width)) - w.flushJitter
	if delay <= 0 {
		return time.Second
	}
	return delay
}

func (w *adaptiveStatePersistenceWorker) flushAndCleanup(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, w.redisTimeout)
	err := w.flush(ctx)
	cancel()
	if err != nil {
		slog.Warn(w.namespace+"_adaptive_state_flush_failed", "error", err)
		return
	}

	ctx, cancel = context.WithTimeout(parent, w.redisTimeout)
	_, err = w.cache.DeleteExpiredAdaptiveSchedulerStates(ctx, w.namespace, w.now(), adaptiveStateCleanupBatchSize)
	cancel()
	if err != nil {
		slog.Warn(w.namespace+"_adaptive_state_cleanup_failed", "error", err)
	}
}
