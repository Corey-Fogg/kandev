// Package sqlite owns workspace orchestrator registrations and role templates.
// It depends on core profile/workspace tables, not the Office feature.
package sqlite

import "github.com/jmoiron/sqlx"

type Repository struct {
	db, ro *sqlx.DB
	// registered caches RegisteredProfileIDs, which Office and the run
	// dispatcher read on hot paths.
	registered registeredCache
}

func New(db, ro *sqlx.DB) *Repository {
	if ro == nil {
		ro = db
	}
	return &Repository{db: db, ro: ro}
}
