package runtime

import (
	"context"
	"errors"
	"math"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/orchestration/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// metricsTaskLimit bounds the delegated tasks one metrics read covers.
const metricsTaskLimit = 1000

// errInvalidMetricsDays rejects a metrics window other than 7 or 30 days.
var errInvalidMetricsDays = errors.New("days must be 7 or 30")

// subcentsPerDollar converts usage cost subcents to US dollars.
const subcentsPerDollar = 10000

// metrics reports the calling coordinator's delegated outcomes over the last
// 7 or 30 days. Cycle time runs from creation to the first merged pull
// request, or to the last update of a completed task without one. Cost
// includes the coordinator's own conversation.
func (h *Handler) metrics(c *gin.Context) {
	claims, ok := h.caller(c)
	if !ok {
		return
	}
	days := 7
	switch c.Query("days") {
	case "", "7":
	case "30":
		days = 30
	default:
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{errorResponseKey: errInvalidMetricsDays.Error()})
		return
	}
	result, err := h.Service.coordinatorMetrics(c.Request.Context(), claims.WorkspaceID, claims.AgentProfileID, claims.TaskID, days, time.Now().UTC())
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// CoordinatorMetrics reports an orchestrator's delegated outcomes for a
// person reading its workspace. It never creates the conversation.
func (s *Service) CoordinatorMetrics(ctx context.Context, workspaceID, agentID string, days int) (models.CoordinatorMetrics, error) {
	if days != 7 && days != 30 {
		return models.CoordinatorMetrics{}, errInvalidMetricsDays
	}
	if err := s.scopeOrchestrator(ctx, workspaceID, agentID); err != nil {
		return models.CoordinatorMetrics{}, err
	}
	conversationID, err := s.Repo.ConversationTaskID(ctx, agentID)
	if err != nil {
		return models.CoordinatorMetrics{}, err
	}
	return s.coordinatorMetrics(ctx, workspaceID, agentID, conversationID, days, s.now())
}

func (s *Service) coordinatorMetrics(ctx context.Context, workspaceID, agentID, conversationID string, days int, now time.Time) (models.CoordinatorMetrics, error) {
	since := now.Add(-time.Duration(days) * 24 * time.Hour)
	result := models.CoordinatorMetrics{Days: days, Since: since.Format(time.RFC3339)}
	tasks, err := s.Repo.DelegatedTasksSince(ctx, workspaceID, agentID, since, metricsTaskLimit)
	if err != nil {
		return result, err
	}
	result.Delegated, result.Truncated = len(tasks), len(tasks) == metricsTaskLimit
	ids := make([]string, 0, len(tasks)+1)
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	merged, mergesKnown, err := s.Repo.FirstMergedAt(ctx, ids)
	if err != nil {
		return result, err
	}
	var cycles []float64
	for _, task := range tasks {
		completed := task.State == string(v1.TaskStateCompleted)
		if completed {
			result.Completed++
		} else if task.State == string(v1.TaskStateFailed) {
			result.Failed++
		}
		if at, ok := merged[task.ID]; ok {
			cycles = append(cycles, at.Sub(task.CreatedAt).Hours())
		} else if completed {
			cycles = append(cycles, task.UpdatedAt.Sub(task.CreatedAt).Hours())
		}
	}
	if result.Completed+result.Failed > 0 {
		result.SuccessRate = rounded(float64(result.Completed) / float64(result.Completed+result.Failed))
	}
	result.CycleTimeSamples = len(cycles)
	result.CycleTimeMedianHours, result.CycleTimeP90Hours = percentile(cycles, 0.5), percentile(cycles, 0.9)
	if conversationID != "" {
		ids = append(ids, conversationID)
	}
	cost, unpriced, err := s.Repo.UsageCost(ctx, ids, since)
	if err != nil {
		return result, err
	}
	result.CostUSD, result.UnpricedEvents = *rounded(float64(cost) / subcentsPerDollar), unpriced
	if mergesKnown {
		count := len(merged)
		result.MergedPullRequests = &count
		if count > 0 {
			result.CostPerMergedPRUSD = rounded(result.CostUSD / float64(count))
		}
	}
	return result, nil
}

// percentile is the nearest-rank percentile of values, or nil when empty.
func percentile(values []float64, p float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	rank := int(math.Ceil(p*float64(len(sorted)))) - 1
	return rounded(sorted[max(rank, 0)])
}

func rounded(value float64) *float64 {
	value = math.Round(value*100) / 100
	return &value
}
