//go:build unit

package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestParseOpsAnthropicAdaptiveLearningFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodGet,
		"/?time_range=1h&group_id=12&model=claude-sonnet-4&status=healthy&top_n=50&sort_by=score&sort_order=asc",
		nil,
	)

	filter, err := parseOpsAnthropicAdaptiveLearningFilter(ctx)

	require.NoError(t, err)
	require.Equal(t, "1h", filter.TimeRange)
	require.Equal(t, int64(12), *filter.GroupID)
	require.Equal(t, "claude-sonnet-4", filter.RequestedModel)
	require.Equal(t, "healthy", filter.Status)
	require.Equal(t, 50, filter.TopN)
	require.Equal(t, "score", filter.SortBy)
	require.Equal(t, "asc", filter.SortOrder)
	require.True(t, filter.StartTime.Before(filter.EndTime))
}

func TestParseOpsAnthropicAdaptiveLearningFilterInvalidParams(t *testing.T) {
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

		_, err := parseOpsAnthropicAdaptiveLearningFilter(ctx)

		require.Error(t, err, "query=%s", query)
	}
}
