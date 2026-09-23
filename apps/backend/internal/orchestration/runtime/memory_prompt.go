package runtime

import (
	"sort"

	"github.com/kandev/kandev/internal/common/redaction"
	"github.com/kandev/kandev/internal/orchestration/models"
)

// promptMemoryBudgetBytes bounds the memory injected into one coordinator prompt.
const promptMemoryBudgetBytes = 12 * 1024

// promptMemory selects the most recently updated entries that fit the prompt
// budget and reports how many were left out.
func promptMemory(rows []*models.AgentMemory) ([]*models.AgentMemory, int) {
	ordered := append([]*models.AgentMemory(nil), rows...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if !ordered[i].UpdatedAt.Equal(ordered[j].UpdatedAt) {
			return ordered[i].UpdatedAt.After(ordered[j].UpdatedAt)
		}
		return ordered[i].ID < ordered[j].ID
	})
	selected := make([]*models.AgentMemory, 0, len(ordered))
	used, omitted := 0, 0
	for _, row := range ordered {
		entry := *row
		entry.Content = clip(redaction.NewRedactor().String(entry.Content), maxMemoryContentBytes)
		size := len(entry.Key) + len(entry.Content)
		if used+size > promptMemoryBudgetBytes {
			omitted++
			continue
		}
		used += size
		selected = append(selected, &entry)
	}
	return selected, omitted
}
