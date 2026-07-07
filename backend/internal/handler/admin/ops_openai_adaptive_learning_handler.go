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

// GetDashboardOpenAIAdaptiveLearning returns the OpenAI adaptive scheduler learning snapshot.
// GET /api/v1/admin/ops/dashboard/openai-adaptive-learning
func (h *OpsHandler) GetDashboardOpenAIAdaptiveLearning(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	filter, err := parseOpsOpenAIAdaptiveLearningFilter(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	snapshot, err := h.opsService.GetOpenAIAdaptiveSchedulerLearningSnapshot(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, snapshot)
}

func parseOpsOpenAIAdaptiveLearningFilter(c *gin.Context) (*service.OpenAIAdaptiveSchedulerLearningFilter, error) {
	if c == nil {
		return nil, fmt.Errorf("invalid request")
	}
	filter := &service.OpenAIAdaptiveSchedulerLearningFilter{
		Status:    strings.TrimSpace(c.Query("status")),
		SortBy:    strings.TrimSpace(c.Query("sort_by")),
		SortOrder: strings.TrimSpace(c.Query("sort_order")),
	}
	if timeRange := strings.TrimSpace(c.Query("time_range")); timeRange != "" {
		dur, ok := parseOpsOpenAITokenStatsDuration(timeRange)
		if !ok {
			return nil, fmt.Errorf("invalid time_range")
		}
		end := time.Now().UTC()
		filter.TimeRange = timeRange
		filter.StartTime = end.Add(-dur)
		filter.EndTime = end
	}

	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
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
