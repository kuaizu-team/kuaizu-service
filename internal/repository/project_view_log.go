package repository

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

// ProjectDashboardStats holds aggregated statistics for the project dashboard.
type ProjectDashboardStats struct {
	TotalViews         int
	TodayViews         int
	TotalApplicants    int
	ConversionRate     float64
	FromList           int
	FromShare          int
	Unknown            int
	AvgDurationSeconds int
	HourlyViews        []HourlyViewItem
}

// HourlyViewItem represents the view count for one hour slot.
type HourlyViewItem struct {
	Hour  time.Time `json:"hour"`
	Count int       `json:"count"`
}

// ProjectViewer is one row in the GET /projects/{id}/viewers response.
type ProjectViewer struct {
	UserID       int       `db:"user_id"        json:"user_id"`
	Nickname     *string   `db:"nickname"       json:"nickname"`
	AvatarUrl    *string   `db:"avatar_url"     json:"avatar_url"`
	LastViewedAt time.Time `db:"last_viewed_at" json:"last_viewed_at"`
}

// ProjectViewLogRepository handles project_view_log database operations.
type ProjectViewLogRepository struct {
	db *sqlx.DB
}

// NewProjectViewLogRepository creates a new ProjectViewLogRepository.
func NewProjectViewLogRepository(db *sqlx.DB) *ProjectViewLogRepository {
	return &ProjectViewLogRepository{db: db}
}

// InsertViewLog writes one view log record. Runs fire-and-forget inside a goroutine in the service layer.
// duration_ms is intentionally omitted; these rows are pure view events (duration_ms IS NULL).
func (r *ProjectViewLogRepository) InsertViewLog(ctx context.Context, log *models.ProjectViewLog) error {
	query := `
		INSERT INTO project_view_log (project_id, user_id, source)
		VALUES (:project_id, :user_id, :source)
	`
	if _, err := r.db.NamedExecContext(ctx, query, log); err != nil {
		return fmt.Errorf("insert view log: %w", err)
	}
	return nil
}

// InsertDurationLog inserts a standalone dwell-time record.
// These rows have duration_ms IS NOT NULL and are NOT counted as views.
func (r *ProjectViewLogRepository) InsertDurationLog(ctx context.Context, projectID int, userID *int, durationMs int) error {
	query := `INSERT INTO project_view_log (project_id, user_id, source, duration_ms) VALUES (?, ?, 0, ?)`
	if _, err := r.db.ExecContext(ctx, query, projectID, userID, durationMs); err != nil {
		return fmt.Errorf("insert duration log: %w", err)
	}
	return nil
}

// GetDashboardStats returns aggregated dashboard data for a single project.
// Rows with duration_ms IS NOT NULL are dwell-time-only records and are excluded from view counts.
func (r *ProjectViewLogRepository) GetDashboardStats(ctx context.Context, projectID int) (*ProjectDashboardStats, error) {
	// 1. Total views from the denormalised counter on the project row (fast, indexed).
	var totalViews int
	if err := r.db.QueryRowxContext(ctx,
		`SELECT view_count FROM project WHERE id = ?`, projectID,
	).Scan(&totalViews); err != nil {
		return nil, fmt.Errorf("get total_views: %w", err)
	}

	// 2. Today's views — exclude duration-only rows.
	var todayViews int
	if err := r.db.QueryRowxContext(ctx,
		`SELECT COUNT(*) FROM project_view_log WHERE project_id = ? AND viewed_at >= CURDATE() AND duration_ms IS NULL`, projectID,
	).Scan(&todayViews); err != nil {
		return nil, fmt.Errorf("get today_views: %w", err)
	}

	// 3. Distinct applicants (all statuses).
	var totalApplicants int
	if err := r.db.QueryRowxContext(ctx,
		`SELECT COUNT(DISTINCT user_id) FROM project_application WHERE project_id = ?`, projectID,
	).Scan(&totalApplicants); err != nil {
		return nil, fmt.Errorf("get total_applicants: %w", err)
	}

	// 4. Source breakdown — exclude duration-only rows.
	type sourceRow struct {
		Source int `db:"source"`
		Count  int `db:"cnt"`
	}
	var srcRows []sourceRow
	if err := r.db.SelectContext(ctx, &srcRows,
		`SELECT source, COUNT(*) AS cnt FROM project_view_log WHERE project_id = ? AND duration_ms IS NULL GROUP BY source`, projectID,
	); err != nil {
		return nil, fmt.Errorf("get source_stats: %w", err)
	}

	// 5. Average dwell time in seconds.
	var avgDurationMs float64
	if err := r.db.QueryRowxContext(ctx,
		`SELECT COALESCE(AVG(duration_ms), 0) FROM project_view_log WHERE project_id = ? AND duration_ms IS NOT NULL`, projectID,
	).Scan(&avgDurationMs); err != nil {
		return nil, fmt.Errorf("get avg_duration: %w", err)
	}

	// 6. Hourly view counts for the last 24 hours. Generate 24 slots in Go and
	//    merge with the DB results so missing hours are filled with 0.
	now := time.Now().UTC()
	currentHour := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC)
	startHour := currentHour.Add(-23 * time.Hour)

	type hourlyRow struct {
		HourStr string `db:"hour_str"`
		Count   int    `db:"cnt"`
	}
	var rawHourly []hourlyRow
	if err := r.db.SelectContext(ctx, &rawHourly,
		`SELECT DATE_FORMAT(viewed_at, '%Y-%m-%d %H:00:00') AS hour_str, COUNT(*) AS cnt
		 FROM project_view_log
		 WHERE project_id = ? AND viewed_at >= ? AND duration_ms IS NULL
		 GROUP BY hour_str ORDER BY hour_str ASC`,
		projectID, startHour,
	); err != nil {
		return nil, fmt.Errorf("get hourly_views: %w", err)
	}
	hourMap := make(map[string]int, len(rawHourly))
	for _, row := range rawHourly {
		hourMap[row.HourStr] = row.Count
	}
	hourlyViews := make([]HourlyViewItem, 24)
	for i := 0; i < 24; i++ {
		slot := startHour.Add(time.Duration(i) * time.Hour)
		key := slot.Format("2006-01-02 15:04:05")
		hourlyViews[i] = HourlyViewItem{Hour: slot, Count: hourMap[key]}
	}

	stats := &ProjectDashboardStats{
		TotalViews:         totalViews,
		TodayViews:         todayViews,
		TotalApplicants:    totalApplicants,
		AvgDurationSeconds: int(math.Round(avgDurationMs / 1000)),
		HourlyViews:        hourlyViews,
	}

	for _, row := range srcRows {
		switch row.Source {
		case models.ViewSourceList:
			stats.FromList = row.Count
		case models.ViewSourceShare:
			stats.FromShare = row.Count
		default:
			stats.Unknown += row.Count
		}
	}

	if totalViews > 0 {
		raw := float64(totalApplicants) / float64(totalViews) * 100
		stats.ConversionRate = math.Round(raw*100) / 100
	}

	return stats, nil
}

// GetViewers returns authenticated users who viewed the project in the last 24 hours
// (duration-only rows excluded). Results are de-duplicated by user and sorted by
// last viewed time descending.
func (r *ProjectViewLogRepository) GetViewers(ctx context.Context, projectID, limit int) ([]ProjectViewer, int, error) {
	var total int
	if err := r.db.QueryRowxContext(ctx,
		`SELECT COUNT(DISTINCT vl.user_id)
		 FROM project_view_log vl
		 WHERE vl.project_id = ? AND vl.user_id IS NOT NULL
		   AND vl.viewed_at >= NOW() - INTERVAL 24 HOUR AND vl.duration_ms IS NULL`,
		projectID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count viewers: %w", err)
	}

	viewers := make([]ProjectViewer, 0)
	if err := r.db.SelectContext(ctx, &viewers,
		`SELECT vl.user_id, u.nickname, u.avatar_url, MAX(vl.viewed_at) AS last_viewed_at
		 FROM project_view_log vl
		 JOIN `+"`user`"+` u ON u.id = vl.user_id
		 WHERE vl.project_id = ? AND vl.user_id IS NOT NULL
		   AND vl.viewed_at >= NOW() - INTERVAL 24 HOUR AND vl.duration_ms IS NULL
		 GROUP BY vl.user_id, u.nickname, u.avatar_url
		 ORDER BY last_viewed_at DESC
		 LIMIT ?`,
		projectID, limit,
	); err != nil {
		return nil, 0, fmt.Errorf("get viewers: %w", err)
	}
	return viewers, total, nil
}
