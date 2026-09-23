package sqlite

import "github.com/kandev/kandev/internal/db/dialect"

// renderSchema expands dialect tokens. Like the other dialect helpers, any
// driver that is not PostgreSQL is SQLite, including wrapped SQLite drivers
// registered under their own name.
func renderSchema(driver, schema string) string {
	if !dialect.IsPostgres(driver) {
		driver = dialect.SQLite3
	}
	return dialect.MustRenderSchema(driver, schema)
}
