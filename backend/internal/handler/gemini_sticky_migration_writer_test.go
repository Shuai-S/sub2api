package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type geminiMigrationWriterCache struct {
	service.GatewayCache
	swapOK       bool
	swapErr      error
	swapCalls    int
	releaseCalls int
}

func (c *geminiMigrationWriterCache) TryAcquireSessionMigrationLease(context.Context, int64, string, string, time.Duration) (bool, error) {
	return true, nil
}

func (c *geminiMigrationWriterCache) CompareAndSwapSessionAccountID(context.Context, int64, string, int64, int64, string, time.Duration) (bool, error) {
	c.swapCalls++
	return c.swapOK, c.swapErr
}

func (c *geminiMigrationWriterCache) CompareAndDeleteSessionAccountID(context.Context, int64, string, int64, string) (bool, error) {
	return true, nil
}

func (c *geminiMigrationWriterCache) ReleaseSessionMigrationLease(context.Context, int64, string, string) (bool, error) {
	c.releaseCalls++
	return true, nil
}

func newGeminiMigrationWriterGateway(cache service.GatewayCache) *service.GatewayService {
	return service.NewGatewayService(
		nil, nil, nil, nil, nil, nil, nil, cache, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
}

func newGeminiMigrationWriterContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	return ctx, recorder
}

func newGeminiMigrationWriterPendingMigration() *service.GeminiPendingStickyMigration {
	return &service.GeminiPendingStickyMigration{
		GroupID:           11,
		SessionKey:        "session-hash",
		ExpectedAccountID: 101,
		ToAccountID:       202,
		LeaseToken:        "lease-token",
	}
}

func TestGeminiStickyMigrationWriterStopsBodyAfterCommitFailure(t *testing.T) {
	cache := &geminiMigrationWriterCache{swapOK: false}
	ctx, recorder := newGeminiMigrationWriterContext()
	writer, restore := installGeminiStickyMigrationWriter(
		ctx,
		newGeminiMigrationWriterGateway(cache),
		newGeminiMigrationWriterPendingMigration(),
	)
	defer restore()

	writer.WriteHeader(http.StatusOK)
	n, err := writer.WriteString(`{"id":"upstream-success"}`)

	require.ErrorContains(t, err, "binding or lease changed")
	require.Zero(t, n)
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Contains(t, recorder.Body.String(), "Gemini session migration failed")
	require.NotContains(t, recorder.Body.String(), "upstream-success")
	require.Equal(t, 1, cache.swapCalls)
	require.Equal(t, 1, cache.releaseCalls)
}

func TestGeminiStickyMigrationWriterCommitsOnceBeforeSuccessBody(t *testing.T) {
	cache := &geminiMigrationWriterCache{swapOK: true}
	ctx, recorder := newGeminiMigrationWriterContext()
	writer, restore := installGeminiStickyMigrationWriter(
		ctx,
		newGeminiMigrationWriterGateway(cache),
		newGeminiMigrationWriterPendingMigration(),
	)
	defer restore()

	writer.WriteHeader(http.StatusOK)
	n, err := writer.WriteString("first")
	require.NoError(t, err)
	require.Equal(t, len("first"), n)
	n, err = writer.Write([]byte("-second"))
	require.NoError(t, err)
	require.Equal(t, len("-second"), n)

	require.True(t, writer.Committed())
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "first-second", recorder.Body.String())
	require.Equal(t, 1, cache.swapCalls)
	require.Equal(t, 1, cache.releaseCalls)
}

func TestGeminiStickyMigrationWriterDoesNotCommitErrorResponse(t *testing.T) {
	cache := &geminiMigrationWriterCache{swapOK: true, swapErr: errors.New("must not be used")}
	ctx, recorder := newGeminiMigrationWriterContext()
	writer, restore := installGeminiStickyMigrationWriter(
		ctx,
		newGeminiMigrationWriterGateway(cache),
		newGeminiMigrationWriterPendingMigration(),
	)
	defer restore()

	writer.WriteHeader(http.StatusBadGateway)
	n, err := writer.WriteString("upstream failed")

	require.NoError(t, err)
	require.Equal(t, len("upstream failed"), n)
	require.Equal(t, http.StatusBadGateway, recorder.Code)
	require.Equal(t, "upstream failed", recorder.Body.String())
	require.Zero(t, cache.swapCalls)
	require.Zero(t, cache.releaseCalls)
}
