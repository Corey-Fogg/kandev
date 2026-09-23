package runtime

import (
	"errors"
	"hash/fnv"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/agent/runtimeauth"
	"github.com/kandev/kandev/internal/common/redaction"
	"github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

const (
	actionSetCriteria    = "set_criteria"
	actionVerifyCriteria = "verify_criteria"
	criteriaKey          = "criteria"
	chiefIDMetadataKey   = "orchestration_chief_id"
)

// manageCriteria handles the acceptance-criteria actions of manage_task. It
// reports false when the action is not one of them.
func (h *Handler) manageCriteria(c *gin.Context, claims *runtimeauth.AgentClaims, req models.WorkspaceTaskCommand) bool {
	if req.Action != actionSetCriteria && req.Action != actionVerifyCriteria {
		return false
	}
	// The goal is read, changed and written back whole, so concurrent
	// criteria actions on one task are serialized: two parallel
	// verify_criteria calls must not start from the same snapshot.
	unlock := h.Service.lockTaskGoal(c.Param("id"))
	defer unlock()
	task, ok := h.delegatedTask(c, claims, c.Param("id"))
	if !ok {
		return true
	}
	if req.Action == actionSetCriteria {
		h.setCriteria(c, task, req.AcceptanceCriteria)
		return true
	}
	h.verifyCriteria(c, claims, task, req.Criteria)
	return true
}

// delegatedTask loads a live task the caller created or adopted.
func (h *Handler) delegatedTask(c *gin.Context, claims *runtimeauth.AgentClaims, id string) (*taskmodels.Task, bool) {
	task, err := h.Service.Tasks.GetTask(c.Request.Context(), id)
	if err != nil || task.WorkspaceID != claims.WorkspaceID {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{errorResponseKey: "task not found"})
		return nil, false
	}
	if chief, _ := task.Metadata[chiefIDMetadataKey].(string); chief != claims.AgentProfileID {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{errorResponseKey: "task is not delegated to you"})
		return nil, false
	}
	if task.ArchivedAt != nil {
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{errorResponseKey: "task is archived"})
		return nil, false
	}
	return task, true
}

func (h *Handler) setCriteria(c *gin.Context, task *taskmodels.Task, texts []string) {
	goal, err := models.NewTaskGoal(texts, h.Service.now())
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{errorResponseKey: err.Error()})
		return
	}
	// Clearing would lift the completion gate, so it is refused while any
	// current criterion is unmet. A replacement list keeps the gate: every
	// new criterion starts unverified.
	if current := models.TaskGoalFromMetadata(task.Metadata); goal == nil && current != nil {
		if unmet := current.Unmet(); len(unmet) > 0 {
			c.AbortWithStatusJSON(http.StatusConflict, gin.H{errorResponseKey: models.ErrCriteriaUnmet.Error(),
				"detail": "acceptance criteria cannot be cleared while any is unmet", "unmet": unmetRows(unmet)})
			return
		}
	}
	if !h.writeGoal(c, task.ID, goal) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, criteriaKey: goal})
}

func (h *Handler) verifyCriteria(c *gin.Context, claims *runtimeauth.AgentClaims, task *taskmodels.Task, items []models.CriterionVerification) {
	goal := models.TaskGoalFromMetadata(task.Metadata)
	if goal == nil {
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{errorResponseKey: "task has no acceptance criteria"})
		return
	}
	redactor := redaction.NewRedactor()
	redacted := make([]models.CriterionVerification, len(items))
	for i, item := range items {
		item.Evidence = redactor.String(item.Evidence)
		redacted[i] = item
	}
	if err := goal.Verify(redacted, claims.RunID, h.Service.now()); err != nil {
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{errorResponseKey: err.Error()})
		return
	}
	if !h.writeGoal(c, task.ID, goal) {
		return
	}
	progress := goal.Progress()
	c.JSON(http.StatusOK, gin.H{"ok": true, criteriaKey: goal, "met": progress.Met, "total": progress.Total})
}

// writeGoal stores a task's goal; a nil goal clears it.
func (h *Handler) writeGoal(c *gin.Context, taskID string, goal *models.TaskGoal) bool {
	if h.Service.TaskMetadata == nil {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{errorResponseKey: errNoTaskMetadata.Error()})
		return false
	}
	changed, err := h.Service.TaskMetadata.SetTaskMetadata(c.Request.Context(), taskID, models.MetaTaskGoal, goal)
	if err != nil {
		fail(c, err)
		return false
	}
	if !changed {
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{errorResponseKey: "task is archived or unavailable"})
		return false
	}
	return true
}

// completionAllowed refuses to set a task done while any of its acceptance
// criteria is not recorded as met.
func (h *Handler) completionAllowed(c *gin.Context, claims *runtimeauth.AgentClaims, id, status string) bool {
	if status != "done" && status != stateCompleted {
		return true
	}
	task, err := h.Service.Tasks.GetTask(c.Request.Context(), id)
	if err != nil || task.WorkspaceID != claims.WorkspaceID {
		// The status update reports its own error for a missing task.
		return true
	}
	goal := models.TaskGoalFromMetadata(task.Metadata)
	if goal == nil {
		return true
	}
	unmet := goal.Unmet()
	if len(unmet) == 0 {
		return true
	}
	c.AbortWithStatusJSON(http.StatusConflict, gin.H{errorResponseKey: models.ErrCriteriaUnmet.Error(), "unmet": unmetRows(unmet)})
	return false
}

func unmetRows(unmet []models.AcceptanceCriterion) []gin.H {
	rows := make([]gin.H, 0, len(unmet))
	for _, criterion := range unmet {
		rows = append(rows, gin.H{"id": criterion.ID, "text": criterion.Text, statusKey: criterion.Status})
	}
	return rows
}

// goalLockStripes bounds the per-task goal locks.
const goalLockStripes = 64

// lockTaskGoal serializes read-modify-write of one task's goal and returns
// the unlock function. Tasks share a fixed set of stripes, so the lock set
// never grows.
func (s *Service) lockTaskGoal(taskID string) func() {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(taskID))
	lock := &s.goalLocks[hash.Sum32()%goalLockStripes]
	lock.Lock()
	return lock.Unlock
}

// criterionDigest is one acceptance criterion as a task update carries it.
type criterionDigest struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Status string `json:"status"`
}

func criteriaDigest(metadata map[string]any) []criterionDigest {
	goal := models.TaskGoalFromMetadata(metadata)
	if goal == nil {
		return nil
	}
	rows := make([]criterionDigest, 0, len(goal.Criteria))
	for _, criterion := range goal.Criteria {
		rows = append(rows, criterionDigest{ID: criterion.ID, Text: clip(criterion.Text, 120), Status: criterion.Status})
	}
	return rows
}

// errNoTaskMetadata means the metadata writer is not installed.
var errNoTaskMetadata = errors.New("task metadata is unavailable")
