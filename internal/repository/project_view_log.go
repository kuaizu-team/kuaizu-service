package repository

import (
	"context"
	"fmt"
	"math"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

// ProjectDashboardStats holds aggregated statistics for the project dashboard.
type ProjectDashboardStats struct {
	TotalViews      int
	TodayViews      int
	TotalApplicants int
	ConversionRate  float64
	FromList        int
	FromShare       int
	Unknown         int
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

// GetDashboardStats returns aggregated dashboard data for a single project.
func (r *ProjectViewLogRepository) GetDashboardStats(ctx context.Context, projectID int) (*ProjectDashboardStats, error) {
	// 1. Total views from the denormalised counter on the project row (fast, indexed).
	var totalViews int
	if err := r.db.QueryRowxContext(ctx,
		`SELECT view_count FROM project WHERE id = ?`, projectID,
	).Scan(&totalViews); err != nil {
		return nil, fmt.Errorf("get total_views: %w", err)
	}

	// 2. Today's views from the log table (supports time-windowed queries for phase-2/3).
	var todayViews int
	if err := r.db.QueryRowxContext(ctx,
		`SELECT COUNT(*) FROM project_view_log WHERE project_id = ? AND viewed_at >= CURDATE()`, projectID,
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

	// 4. Source breakdown in a single aggregation query.
	type sourceRow struct {
		Source int `db:"source"`
		Count  int `db:"cnt"`
	}
	var srcRows []sourceRow
	if err := r.db.SelectContext(ctx, &srcRows,
		`SELECT source, COUNT(*) AS cnt FROM project_view_log WHERE project_id = ? GROUP BY source`, projectID,
	); err != nil {
		return nil, fmt.Errorf("get source_stats: %w", err)
	}

	stats := &ProjectDashboardStats{
		TotalViews:      totalViews,
		TodayViews:      todayViews,
		TotalApplicants: totalApplicants,
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
