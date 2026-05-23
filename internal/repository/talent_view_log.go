package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/oss"
)

// TalentDashboardStats holds aggregated statistics for the talent dashboard.
type TalentDashboardStats struct {
	TotalViews         int
	TodayViews         int
	FromList           int
	FromShare          int
	Unknown            int
	AvgDurationSeconds int
}

// TalentViewer is one row in the GET /talent-profiles/{id}/viewers response.
type TalentViewer struct {
	UserID       int       `db:"user_id"        json:"user_id"`
	Nickname     *string   `db:"nickname"       json:"nickname"`
	AvatarUrl    *string   `db:"avatar_url"     json:"avatar_url"`
	LastViewedAt time.Time `db:"last_viewed_at" json:"last_viewed_at"`
}

// TalentViewLogRepository handles talent_view_log database operations.
type TalentViewLogRepository struct {
	db *sqlx.DB
}

// NewTalentViewLogRepository creates a new TalentViewLogRepository.
func NewTalentViewLogRepository(db *sqlx.DB) *TalentViewLogRepository {
	return &TalentViewLogRepository{db: db}
}

// InsertViewLog writes one pure view event. Duration-only rows are inserted separately.
func (r *TalentViewLogRepository) InsertViewLog(ctx context.Context, log *models.TalentViewLog) error {
	query := `
		INSERT INTO talent_view_log (talent_id, user_id, source)
		VALUES (:talent_id, :user_id, :source)
	`
	if _, err := r.db.NamedExecContext(ctx, query, log); err != nil {
		return fmt.Errorf("insert talent view log: %w", err)
	}
	return nil
}

// InsertDurationLog inserts a standalone dwell-time record and does not count it as a view.
func (r *TalentViewLogRepository) InsertDurationLog(ctx context.Context, talentID int, userID *int, durationMs int) error {
	query := `INSERT INTO talent_view_log (talent_id, user_id, source, duration_ms) VALUES (?, ?, 0, ?)`
	if _, err := r.db.ExecContext(ctx, query, talentID, userID, durationMs); err != nil {
		return fmt.Errorf("insert talent duration log: %w", err)
	}
	return nil
}

// GetDashboardStats returns aggregated dashboard data for a single talent profile.
func (r *TalentViewLogRepository) GetDashboardStats(ctx context.Context, talentID int) (*TalentDashboardStats, error) {
	var totalViews int
	if err := r.db.QueryRowxContext(ctx,
		`SELECT view_count FROM talent_profile WHERE id = ?`, talentID,
	).Scan(&totalViews); err != nil {
		return nil, fmt.Errorf("get talent total_views: %w", err)
	}

	var todayViews int
	if err := r.db.QueryRowxContext(ctx,
		`SELECT COUNT(*) FROM talent_view_log WHERE talent_id = ? AND viewed_at >= CURDATE() AND duration_ms IS NULL`, talentID,
	).Scan(&todayViews); err != nil {
		return nil, fmt.Errorf("get talent today_views: %w", err)
	}

	type sourceRow struct {
		Source int `db:"source"`
		Count  int `db:"cnt"`
	}
	var srcRows []sourceRow
	if err := r.db.SelectContext(ctx, &srcRows,
		`SELECT source, COUNT(*) AS cnt FROM talent_view_log WHERE talent_id = ? AND duration_ms IS NULL GROUP BY source`, talentID,
	); err != nil {
		return nil, fmt.Errorf("get talent source_stats: %w", err)
	}

	var avgDurationMs float64
	if err := r.db.QueryRowxContext(ctx,
		`SELECT COALESCE(AVG(duration_ms), 0) FROM talent_view_log WHERE talent_id = ? AND duration_ms BETWEEN 1 AND 3600000`, talentID,
	).Scan(&avgDurationMs); err != nil {
		return nil, fmt.Errorf("get talent avg_duration: %w", err)
	}

	stats := &TalentDashboardStats{
		TotalViews:         totalViews,
		TodayViews:         todayViews,
		AvgDurationSeconds: int(avgDurationMs / 1000),
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

	return stats, nil
}

// GetViewers returns authenticated users who viewed the talent profile in the last 24 hours.
func (r *TalentViewLogRepository) GetViewers(ctx context.Context, talentID, limit int) ([]TalentViewer, int, error) {
	var total int
	if err := r.db.QueryRowxContext(ctx,
		`SELECT COUNT(DISTINCT vl.user_id)
		 FROM talent_view_log vl
		 WHERE vl.talent_id = ? AND vl.user_id IS NOT NULL
		   AND vl.viewed_at >= NOW() - INTERVAL 24 HOUR AND vl.duration_ms IS NULL`,
		talentID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count talent viewers: %w", err)
	}

	viewers := make([]TalentViewer, 0)
	if err := r.db.SelectContext(ctx, &viewers,
		`SELECT vl.user_id, u.nickname, u.avatar_url, MAX(vl.viewed_at) AS last_viewed_at
		 FROM talent_view_log vl
		 JOIN `+"`user`"+` u ON u.id = vl.user_id
		 WHERE vl.talent_id = ? AND vl.user_id IS NOT NULL
		   AND vl.viewed_at >= NOW() - INTERVAL 24 HOUR AND vl.duration_ms IS NULL
		 GROUP BY vl.user_id, u.nickname, u.avatar_url
		 ORDER BY last_viewed_at DESC
		 LIMIT ?`,
		talentID, limit,
	); err != nil {
		return nil, 0, fmt.Errorf("get talent viewers: %w", err)
	}
	for i := range viewers {
		if viewers[i].AvatarUrl != nil && *viewers[i].AvatarUrl != "" {
			fullURL := oss.FullURL(*viewers[i].AvatarUrl)
			viewers[i].AvatarUrl = &fullURL
		}
	}
	return viewers, total, nil
}
