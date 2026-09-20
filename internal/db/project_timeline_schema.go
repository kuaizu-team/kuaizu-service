package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// CheckProjectTimelineSchema verifies the release's required columns without
// reading business rows or applying migrations.
func CheckProjectTimelineSchema(ctx context.Context, pool *sqlx.DB) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	for _, check := range []struct {
		query     string
		migration string
	}{
		{"SELECT default_timeline_hidden FROM project LIMIT 0", "sql/migration_project_default_timeline_hidden.sql"},
		{"SELECT id, project_id, user_id, title, detail, related_members, created_at, updated_at FROM project_member_timeline LIMIT 0", "sql/migration_project_member_timeline.sql"},
	} {
		rows, err := pool.QueryContext(ctx, check.query)
		if err != nil {
			return fmt.Errorf("project timeline schema unavailable; verify %s before deployment: %w", check.migration, err)
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("project timeline schema check failed: %w", err)
		}
	}
	return nil
}
