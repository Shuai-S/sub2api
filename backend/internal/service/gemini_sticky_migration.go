package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

const (
	geminiSessionMigrationLeaseTTL     = 2 * time.Minute
	geminiSessionMigrationWaitTimeout  = 5 * time.Second
	geminiSessionMigrationPollInterval = 25 * time.Millisecond
)

type geminiStickyMigrationRetryError struct {
	GroupID   int64
	AccountID int64
}

func (e *geminiStickyMigrationRetryError) Error() string {
	return "Gemini sticky binding changed while waiting for migration"
}

func (s *GatewayService) prepareGeminiStickyMigration(ctx context.Context, selection *AccountSelectionResult, groupID *int64, sessionHash string, expectedAccountID int64, deleteSourceOnFailure bool) (*AccountSelectionResult, error) {
	if selection == nil || selection.Account == nil || sessionHash == "" || selection.Account.ID == expectedAccountID {
		return selection, nil
	}
	cache, ok := s.cache.(GeminiSessionMigrationCache)
	if !ok || cache == nil {
		slog.Warn("gemini_adaptive_sticky_migration_prepare_failed",
			append(geminiStickyMigrationLogFields(ctx, derefGroupID(groupID), sessionHash, expectedAccountID, selection.Account.ID),
				"reason", "cache_unavailable")...,
		)
		if selection.ReleaseFunc != nil {
			selection.ReleaseFunc()
		}
		return nil, fmt.Errorf("%w: Gemini session migration cache is unavailable", ErrNoAvailableAccounts)
	}
	token, err := randomHexString(16)
	if err != nil {
		fields := append(geminiStickyMigrationLogFields(ctx, derefGroupID(groupID), sessionHash, expectedAccountID, selection.Account.ID), "reason", "token_generation_failed")
		fields = append(fields, geminiAdaptiveErrorLogFields(err)...)
		slog.Warn("gemini_adaptive_sticky_migration_prepare_failed", fields...)
		if selection.ReleaseFunc != nil {
			selection.ReleaseFunc()
		}
		return nil, fmt.Errorf("create Gemini session migration token: %w", err)
	}
	resolvedGroupID := derefGroupID(groupID)
	acquired, err := cache.TryAcquireSessionMigrationLease(ctx, resolvedGroupID, sessionHash, token, geminiSessionMigrationLeaseTTL)
	if err != nil {
		fields := append(geminiStickyMigrationLogFields(ctx, resolvedGroupID, sessionHash, expectedAccountID, selection.Account.ID), "reason", "lease_acquire_failed")
		fields = append(fields, geminiAdaptiveErrorLogFields(err)...)
		slog.Warn("gemini_adaptive_sticky_migration_prepare_failed", fields...)
		if selection.ReleaseFunc != nil {
			selection.ReleaseFunc()
		}
		return nil, fmt.Errorf("acquire Gemini session migration lease: %w", err)
	}
	if !acquired {
		slog.Info("gemini_adaptive_sticky_migration_lease_contended",
			geminiStickyMigrationLogFields(ctx, resolvedGroupID, sessionHash, expectedAccountID, selection.Account.ID)...,
		)
		currentAccountID, leaseAcquired, waitErr := s.waitForGeminiSessionMigration(ctx, cache, resolvedGroupID, sessionHash, expectedAccountID, token)
		if waitErr != nil {
			fields := append(geminiStickyMigrationLogFields(ctx, resolvedGroupID, sessionHash, expectedAccountID, selection.Account.ID), "reason", "lease_wait_failed")
			fields = append(fields, geminiAdaptiveErrorLogFields(waitErr)...)
			slog.Warn("gemini_adaptive_sticky_migration_prepare_failed", fields...)
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
			return nil, waitErr
		}
		if currentAccountID != expectedAccountID {
			if currentAccountID == selection.Account.ID {
				slog.Info("gemini_adaptive_sticky_migration_reused",
					append(geminiStickyMigrationLogFields(ctx, resolvedGroupID, sessionHash, expectedAccountID, selection.Account.ID),
						"current_account_id", currentAccountID)...,
				)
				selection.PreserveStickyBinding = true
				return selection, nil
			}
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
			slog.Info("gemini_adaptive_sticky_migration_retry",
				append(geminiStickyMigrationLogFields(ctx, resolvedGroupID, sessionHash, expectedAccountID, selection.Account.ID),
					"current_account_id", currentAccountID)...,
			)
			return nil, &geminiStickyMigrationRetryError{GroupID: resolvedGroupID, AccountID: currentAccountID}
		}
		if !leaseAcquired {
			slog.Warn("gemini_adaptive_sticky_migration_prepare_failed",
				append(geminiStickyMigrationLogFields(ctx, resolvedGroupID, sessionHash, expectedAccountID, selection.Account.ID),
					"reason", "lease_still_contended")...,
			)
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
			return nil, fmt.Errorf("%w: Gemini session migration is already in progress", ErrNoAvailableAccounts)
		}
	}
	currentAccountID, _ := s.cache.GetSessionAccountID(ctx, resolvedGroupID, sessionHash)
	if currentAccountID != expectedAccountID {
		_, _ = cache.ReleaseSessionMigrationLease(ctx, resolvedGroupID, sessionHash, token)
		if currentAccountID == selection.Account.ID {
			slog.Info("gemini_adaptive_sticky_migration_reused",
				append(geminiStickyMigrationLogFields(ctx, resolvedGroupID, sessionHash, expectedAccountID, selection.Account.ID),
					"current_account_id", currentAccountID)...,
			)
			selection.PreserveStickyBinding = true
			return selection, nil
		}
		if selection.ReleaseFunc != nil {
			selection.ReleaseFunc()
		}
		slog.Info("gemini_adaptive_sticky_migration_retry",
			append(geminiStickyMigrationLogFields(ctx, resolvedGroupID, sessionHash, expectedAccountID, selection.Account.ID),
				"current_account_id", currentAccountID)...,
		)
		return nil, &geminiStickyMigrationRetryError{GroupID: resolvedGroupID, AccountID: currentAccountID}
	}
	selection.PreserveStickyBinding = true
	selection.PendingGeminiMigration = &GeminiPendingStickyMigration{
		GroupID:                  resolvedGroupID,
		SessionKey:               sessionHash,
		ExpectedAccountID:        expectedAccountID,
		SignatureSourceAccountID: expectedAccountID,
		ToAccountID:              selection.Account.ID,
		LeaseToken:               token,
		DeleteSourceOnFailure:    deleteSourceOnFailure && expectedAccountID > 0,
	}
	slog.Info("gemini_adaptive_sticky_migration_prepared",
		append(geminiStickyMigrationLogFields(ctx, resolvedGroupID, sessionHash, expectedAccountID, selection.Account.ID),
			"delete_source_on_failure", selection.PendingGeminiMigration.DeleteSourceOnFailure,
			"lease_ttl_ms", geminiSessionMigrationLeaseTTL.Milliseconds())...,
	)
	return selection, nil
}

func (s *GatewayService) waitForGeminiSessionMigration(ctx context.Context, cache GeminiSessionMigrationCache, groupID int64, sessionHash string, expectedAccountID int64, token string) (currentAccountID int64, acquired bool, err error) {
	if s == nil || s.cache == nil || cache == nil {
		return expectedAccountID, false, fmt.Errorf("%w: Gemini session migration cache is unavailable", ErrNoAvailableAccounts)
	}
	timer := time.NewTimer(geminiSessionMigrationWaitTimeout)
	defer timer.Stop()
	ticker := time.NewTicker(geminiSessionMigrationPollInterval)
	defer ticker.Stop()

	for {
		currentAccountID, _ = s.cache.GetSessionAccountID(ctx, groupID, sessionHash)
		if currentAccountID != expectedAccountID {
			return currentAccountID, false, nil
		}
		acquired, err = cache.TryAcquireSessionMigrationLease(ctx, groupID, sessionHash, token, geminiSessionMigrationLeaseTTL)
		if err != nil {
			return currentAccountID, false, fmt.Errorf("acquire Gemini session migration lease after wait: %w", err)
		}
		if acquired {
			currentAccountID, _ = s.cache.GetSessionAccountID(ctx, groupID, sessionHash)
			if currentAccountID != expectedAccountID {
				_, _ = cache.ReleaseSessionMigrationLease(ctx, groupID, sessionHash, token)
				return currentAccountID, false, nil
			}
			return currentAccountID, true, nil
		}

		select {
		case <-ctx.Done():
			return currentAccountID, false, ctx.Err()
		case <-timer.C:
			return currentAccountID, false, fmt.Errorf("%w: timed out waiting for Gemini session migration", ErrNoAvailableAccounts)
		case <-ticker.C:
		}
	}
}

func (s *GatewayService) CommitGeminiStickyMigration(ctx context.Context, migration *GeminiPendingStickyMigration) error {
	if migration == nil || migration.completed.Swap(true) {
		return nil
	}
	cache, ok := s.cache.(GeminiSessionMigrationCache)
	if !ok || cache == nil {
		slog.Warn("gemini_adaptive_sticky_migration_commit_failed",
			append(geminiStickyMigrationLogFields(ctx, migration.GroupID, migration.SessionKey, migration.ExpectedAccountID, migration.ToAccountID),
				"reason", "cache_unavailable")...,
		)
		return fmt.Errorf("gemini session migration cache is unavailable")
	}
	cacheCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	defer func() {
		_, releaseErr := cache.ReleaseSessionMigrationLease(cacheCtx, migration.GroupID, migration.SessionKey, migration.LeaseToken)
		if releaseErr != nil {
			fields := append(geminiStickyMigrationLogFields(ctx, migration.GroupID, migration.SessionKey, migration.ExpectedAccountID, migration.ToAccountID), "reason", "lease_release_failed")
			fields = append(fields, geminiAdaptiveErrorLogFields(releaseErr)...)
			slog.Warn("gemini_adaptive_sticky_migration_commit_cleanup_failed", fields...)
		}
	}()
	swapped, err := cache.CompareAndSwapSessionAccountID(cacheCtx, migration.GroupID, migration.SessionKey, migration.ExpectedAccountID, migration.ToAccountID, migration.LeaseToken, stickySessionTTL)
	if err != nil {
		fields := append(geminiStickyMigrationLogFields(ctx, migration.GroupID, migration.SessionKey, migration.ExpectedAccountID, migration.ToAccountID), "reason", "cas_failed")
		fields = append(fields, geminiAdaptiveErrorLogFields(err)...)
		slog.Warn("gemini_adaptive_sticky_migration_commit_failed", fields...)
		return fmt.Errorf("commit Gemini sticky migration: %w", err)
	}
	if !swapped {
		slog.Warn("gemini_adaptive_sticky_migration_commit_failed",
			append(geminiStickyMigrationLogFields(ctx, migration.GroupID, migration.SessionKey, migration.ExpectedAccountID, migration.ToAccountID),
				"reason", "binding_or_lease_changed")...,
		)
		return fmt.Errorf("commit Gemini sticky migration: binding or lease changed")
	}
	if s.geminiAdaptiveScheduler != nil {
		s.geminiAdaptiveScheduler.stickyMigrateTotal.Add(1)
	}
	slog.Info("gemini_adaptive_sticky_migration_committed",
		geminiStickyMigrationLogFields(ctx, migration.GroupID, migration.SessionKey, migration.ExpectedAccountID, migration.ToAccountID)...,
	)
	return nil
}

func (s *GatewayService) AbortGeminiStickyMigration(ctx context.Context, migration *GeminiPendingStickyMigration) {
	if migration == nil || migration.completed.Swap(true) {
		return
	}
	cache, ok := s.cache.(GeminiSessionMigrationCache)
	if !ok || cache == nil {
		slog.Warn("gemini_adaptive_sticky_migration_abort_failed",
			append(geminiStickyMigrationLogFields(ctx, migration.GroupID, migration.SessionKey, migration.ExpectedAccountID, migration.ToAccountID),
				"reason", "cache_unavailable")...,
		)
		return
	}
	cacheCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	deleted := false
	if migration.DeleteSourceOnFailure {
		var deleteErr error
		deleted, deleteErr = cache.CompareAndDeleteSessionAccountID(cacheCtx, migration.GroupID, migration.SessionKey, migration.ExpectedAccountID, migration.LeaseToken)
		if deleteErr != nil {
			fields := append(geminiStickyMigrationLogFields(ctx, migration.GroupID, migration.SessionKey, migration.ExpectedAccountID, migration.ToAccountID), "reason", "source_delete_failed")
			fields = append(fields, geminiAdaptiveErrorLogFields(deleteErr)...)
			slog.Warn("gemini_adaptive_sticky_migration_abort_failed", fields...)
		}
	}
	released, releaseErr := cache.ReleaseSessionMigrationLease(cacheCtx, migration.GroupID, migration.SessionKey, migration.LeaseToken)
	if releaseErr != nil {
		fields := append(geminiStickyMigrationLogFields(ctx, migration.GroupID, migration.SessionKey, migration.ExpectedAccountID, migration.ToAccountID), "reason", "lease_release_failed")
		fields = append(fields, geminiAdaptiveErrorLogFields(releaseErr)...)
		slog.Warn("gemini_adaptive_sticky_migration_abort_failed", fields...)
	}
	slog.Info("gemini_adaptive_sticky_migration_aborted",
		append(geminiStickyMigrationLogFields(ctx, migration.GroupID, migration.SessionKey, migration.ExpectedAccountID, migration.ToAccountID),
			"delete_source_on_failure", migration.DeleteSourceOnFailure,
			"source_deleted", deleted,
			"lease_released", released)...,
	)
}

func geminiStickyMigrationLogFields(ctx context.Context, groupID int64, sessionHash string, fromAccountID, toAccountID int64) []any {
	return []any{
		"request_id", contextStringValue(ctx, ctxkey.RequestID),
		"client_request_id", contextStringValue(ctx, ctxkey.ClientRequestID),
		"group_id", groupID,
		"session", shortSessionHash(sessionHash),
		"from_account_id", fromAccountID,
		"to_account_id", toAccountID,
	}
}
