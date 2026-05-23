package models

import "time"

// View source constants for project_view_log.source
const (
	ViewSourceUnknown = 0 // direct or untracked
	ViewSourceList    = 1 // from the project list page
	ViewSourceShare   = 2 // from a share card
)

// ProjectViewLog is a single row in the project_view_log table.
type ProjectViewLog struct {
	ID         int64     `db:"id"`
	ProjectID  int       `db:"project_id"`
	UserID     *int      `db:"user_id"`   // NULL for unauthenticated viewers
	Source     int       `db:"source"`    // 0/1/2 — see ViewSource* constants
	DurationMs *int      `db:"duration_ms"` // reserved for phase-3 dwell-time
	ViewedAt   time.Time `db:"viewed_at"`
}
