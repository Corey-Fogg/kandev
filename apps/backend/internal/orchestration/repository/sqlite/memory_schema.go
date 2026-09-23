package sqlite

import (
	"fmt"
	"github.com/kandev/kandev/internal/db"
)

const (
	requiredTextColumn = "TEXT NOT NULL DEFAULT ''"
)

func (r *Repository) migrateMemoryContext() error {
	tx, err := r.db.Beginx()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	columns := []struct{ name, definition string }{
		{"owner_user_id", requiredTextColumn}, {"scope", "TEXT NOT NULL DEFAULT 'workspace'"},
		{"scope_id", requiredTextColumn}, {"source_comment_id", requiredTextColumn},
		{"revision", "INTEGER NOT NULL DEFAULT 1"}, {"confirmed", "INTEGER NOT NULL DEFAULT 0"},
		{"priority", "INTEGER NOT NULL DEFAULT 0"}, {"expires_at", "TIMESTAMP"}, {"forgotten_at", "TIMESTAMP"},
	}
	for _, c := range columns {
		exists, err := db.ColumnExists(tx, "orchestration_memory", c.name)
		if err != nil {
			return err
		}
		if !exists {
			if _, err := tx.Exec(renderSchema(tx.DriverName(), fmt.Sprintf("ALTER TABLE orchestration_memory ADD COLUMN %s %s", c.name, c.definition))); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
