package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type geminiAdaptiveOpsAccountRepo struct {
	service.AccountRepository
	accounts []service.Account
}

func (r *geminiAdaptiveOpsAccountRepo) ListOpsAccountsForStats(context.Context, string, *int64) ([]service.Account, error) {
	return append([]service.Account(nil), r.accounts...), nil
}

func TestParseOpsGeminiAdaptiveLearningFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodGet,
		"/?time_range=1h&group_id=12&model=gemini-2.5-pro&status=healthy&top_n=50&sort_by=score&sort_order=asc",
		nil,
	)

	filter, err := parseOpsGeminiAdaptiveLearningFilter(ctx)

	require.NoError(t, err)
	require.Equal(t, "1h", filter.TimeRange)
	require.Equal(t, int64(12), *filter.GroupID)
	require.Equal(t, "gemini-2.5-pro", filter.RequestedModel)
	require.Equal(t, "healthy", filter.Status)
	require.Equal(t, 50, filter.TopN)
	require.Equal(t, "score", filter.SortBy)
	require.Equal(t, "asc", filter.SortOrder)
	require.True(t, filter.StartTime.Before(filter.EndTime))
}

func TestParseOpsGeminiAdaptiveLearningFilterInvalidParams(t *testing.T) {
	queries := []string{
		"/?time_range=7d",
		"/?group_id=0",
		"/?group_id=abc",
		"/?top_n=0",
		"/?top_n=101",
		"/?top_n=10&page=1",
		"/?page=0",
		"/?page_size=101",
		"/?limit=501",
		"/?model=" + strings.Repeat("a", 257),
	}

	gin.SetMode(gin.TestMode)
	for _, query := range queries {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, query, nil)

		_, err := parseOpsGeminiAdaptiveLearningFilter(ctx)

		require.Error(t, err, "query=%s", query)
	}
}

func TestOpsGeminiAdaptiveLearningHandlerReturnsPagedSnapshot(t *testing.T) {
	cfg := &config.Config{}
	cfg.Ops.Enabled = true
	repo := &geminiAdaptiveOpsAccountRepo{accounts: []service.Account{
		{ID: 1, Name: "gemini-a", Platform: service.PlatformGemini, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, Concurrency: 8},
		{ID: 2, Name: "gemini-b", Platform: service.PlatformGemini, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, Concurrency: 4},
		{ID: 3, Name: "not-schedulable", Platform: service.PlatformGemini, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: false, Concurrency: 4},
	}}
	gateway := service.NewGatewayService(
		nil, nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	opsService := service.NewOpsService(nil, nil, cfg, repo, nil, nil, gateway, nil, nil, nil, nil)
	handler := NewOpsHandler(opsService)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/gemini-adaptive-learning", handler.GetDashboardGeminiAdaptiveLearning)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/gemini-adaptive-learning?model=gemini-2.5-pro&page=2&page_size=1&sort_by=account&sort_order=asc", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var envelope struct {
		Code int                                             `json:"code"`
		Data service.GeminiAdaptiveSchedulerLearningSnapshot `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Zero(t, envelope.Code)
	require.Equal(t, 2, envelope.Data.TotalAccounts)
	require.Equal(t, 1, envelope.Data.ReturnedAccounts)
	require.Equal(t, 2, envelope.Data.Page)
	require.Equal(t, 1, envelope.Data.PageSize)
	require.Equal(t, "gemini-2.5-pro", envelope.Data.RequestedModel)
	require.Len(t, envelope.Data.Accounts, 1)
	require.Equal(t, int64(2), envelope.Data.Accounts[0].AccountID)
}

func TestOpsGeminiAdaptiveLearningHandlerRejectsConflictingPagination(t *testing.T) {
	cfg := &config.Config{}
	cfg.Ops.Enabled = true
	opsService := service.NewOpsService(nil, nil, cfg, &geminiAdaptiveOpsAccountRepo{}, nil, nil, nil, nil, nil, nil, nil)
	handler := NewOpsHandler(opsService)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/gemini-adaptive-learning", handler.GetDashboardGeminiAdaptiveLearning)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/gemini-adaptive-learning?top_n=10&page=1", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "top_n cannot be used")
}
