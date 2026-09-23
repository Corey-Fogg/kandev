package runtime

import (
	"strings"
	"unicode/utf8"

	taskservice "github.com/kandev/kandev/internal/task/service"
)

const titleTruncatedKey = "title_truncated"

// fitTaskTitle shortens a coordinator-supplied title to the task title limit,
// cutting at a word boundary when one falls in the second half of the limit.
// It reports whether the title was shortened.
func fitTaskTitle(title string) (string, bool) {
	title = strings.TrimSpace(title)
	if taskservice.ValidateTaskTitle(title) == nil {
		return title, false
	}
	const ellipsis = "…"
	limit := taskservice.TaskTitleMaxLength - utf8.RuneCountInString(ellipsis)
	cut := string([]rune(title)[:limit])
	if space := strings.LastIndexAny(cut, " \t"); space > 0 && utf8.RuneCountInString(cut[:space]) >= limit/2 {
		cut = cut[:space]
	}
	return strings.TrimRight(cut, " \t,;:.-") + ellipsis, true
}
