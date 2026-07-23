package admin

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// GetDashboardAnthropicAdaptiveLearning returns the Anthropic adaptive scheduler learning snapshot.
// GET /api/v1/admin/ops/dashboard/anthropic-adaptive-learning
func (h *OpsHandler) GetDashboardAnthropicAdaptiveLearning(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	filter, err := parseOpsAnthropicAdaptiveLearningFilter(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	snapshot, err := h.opsService.GetAnthropicAdaptiveSchedulerLearningSnapshot(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, snapshot)
}

func parseOpsAnthropicAdaptiveLearningFilter(c *gin.Context) (*service.AnthropicAdaptiveSchedulerLearningFilter, error) {
	if c == nil {
		return nil, fmt.Errorf("invalid request")
	}
	filter := &service.AnthropicAdaptiveSchedulerLearningFilter{
		RequestedModel: strings.TrimSpace(c.Query("model")),
		Status:         strings.TrimSpace(c.Query("status")),
		SortBy:         strings.TrimSpace(c.Query("sort_by")),
		SortOrder:      strings.TrimSpace(c.Query("sort_order")),
	}
	if len(filter.RequestedModel) > 256 {
		return nil, fmt.Errorf("invalid model")
	}
	if timeRange := strings.TrimSpace(c.Query("time_range")); timeRange != "" {
		duration, ok := parseOpsOpenAITokenStatsDuration(timeRange)
		if !ok {
			return nil, fmt.Errorf("invalid time_range")
		}
		end := time.Now().UTC()
		filter.TimeRange = timeRange
		filter.StartTime = end.Add(-duration)
		filter.EndTime = end
	}

	if value := strings.TrimSpace(c.Query("group_id")); value != "" {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("invalid group_id")
		}
		filter.GroupID = &id
	}

	topNRaw := strings.TrimSpace(c.Query("top_n"))
	pageRaw := strings.TrimSpace(c.Query("page"))
	pageSizeRaw := strings.TrimSpace(c.Query("page_size"))
	limitRaw := strings.TrimSpace(c.Query("limit"))
	if topNRaw != "" && (pageRaw != "" || pageSizeRaw != "" || limitRaw != "") {
		return nil, fmt.Errorf("invalid query: top_n cannot be used with page/page_size/limit")
	}
	if topNRaw != "" {
		topN, err := strconv.Atoi(topNRaw)
		if err != nil || topN < 1 || topN > 100 {
			return nil, fmt.Errorf("invalid top_n")
		}
		filter.TopN = topN
		return filter, nil
	}

	filter.Page = 1
	filter.PageSize = 20
	if limitRaw != "" {
		limit, err := strconv.Atoi(limitRaw)
		if err != nil || limit < 1 || limit > 500 {
			return nil, fmt.Errorf("invalid limit")
		}
		filter.TopN = limit
		return filter, nil
	}
	if pageRaw != "" {
		page, err := strconv.Atoi(pageRaw)
		if err != nil || page < 1 {
			return nil, fmt.Errorf("invalid page")
		}
		filter.Page = page
	}
	if pageSizeRaw != "" {
		pageSize, err := strconv.Atoi(pageSizeRaw)
		if err != nil || pageSize < 1 || pageSize > 100 {
			return nil, fmt.Errorf("invalid page_size")
		}
		filter.PageSize = pageSize
	}
	return filter, nil
}
