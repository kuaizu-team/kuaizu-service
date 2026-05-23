package models

import "time"

// TalentViewLog is a single row in the talent_view_log table.
type TalentViewLog struct {
	ID         int64     `db:"id"`
	TalentID   int       `db:"talent_id"`
	UserID     *int      `db:"user_id"`
	Source     int       `db:"source"`
	DurationMs *int      `db:"duration_ms"`
	ViewedAt   time.Time `db:"viewed_at"`
}
