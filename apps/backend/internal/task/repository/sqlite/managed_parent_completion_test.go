package sqlite

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/pkg/api/v1"
)

func TestManagedParentCannotCompleteWithUnfinishedChildren(t *testing.T) {
	repo := newRepoForHealTests(t)
	ctx := context.Background()
	insertTask(t, repo.db, "managed-parent")
	insertTask(t, repo.db, "unfinished-child")
	if _, err := repo.db.Exec(`UPDATE tasks SET metadata = '{"orchestration_managed":true}' WHERE id = ?`, "managed-parent"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.Exec(`UPDATE tasks SET parent_id = ? WHERE id = ?`, "managed-parent", "unfinished-child"); err != nil {
		t.Fatal(err)
	}

	err := repo.UpdateTaskState(ctx, "managed-parent", v1.TaskStateCompleted)
	if err == nil || !strings.Contains(err.Error(), "unfinished-child") {
		t.Fatalf("UpdateTaskState error = %v, want unfinished child guard", err)
	}
	task, err := repo.GetTask(ctx, "managed-parent")
	if err != nil {
		t.Fatal(err)
	}
	if task.State == v1.TaskStateCompleted {
		t.Fatal("managed parent completed while a child was unfinished")
	}

	if err := repo.UpdateTaskState(ctx, "unfinished-child", v1.TaskStateCompleted); err != nil {
		t.Fatalf("complete child: %v", err)
	}
	if err := repo.UpdateTaskState(ctx, "managed-parent", v1.TaskStateCompleted); err != nil {
		t.Fatalf("complete parent after child: %v", err)
	}
}

func TestUnmanagedParentCompletionIsUnchanged(t *testing.T) {
	repo := newRepoForHealTests(t)
	ctx := context.Background()
	insertTask(t, repo.db, "ordinary-parent")
	insertTask(t, repo.db, "ordinary-child")
	if _, err := repo.db.Exec(`UPDATE tasks SET parent_id = ? WHERE id = ?`, "ordinary-parent", "ordinary-child"); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateTaskState(ctx, "ordinary-parent", v1.TaskStateCompleted); err != nil {
		t.Fatalf("ordinary parent completion: %v", err)
	}
}
