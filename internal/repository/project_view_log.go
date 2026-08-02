package repository

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/oss"
)

// ProjectDashboardStats holds aggregated statistics for the project dashboard.
type ProjectDashboardStats struct {
	TotalViews          int
	TodayViews          int
	UniqueVisitors      int
	TotalApplicants     int
	ProcessedApplicants int
	ConversionRate      float64
	FromList            int
	FromShare           int
	Unknown             int
	AvgDurationSeconds  int
	HourlyViews         []HourlyViewItem
	PromotionStats      ProjectPromotionDashboardStats
	VisitUnread         int
}

// ProjectPromotionDashboardStats contains compact email promotion metrics.
type ProjectPromotionDashboardStats struct {
	TotalRecipients      int     `json:"total_recipients"`
	CompletedRecipients  int     `json:"completed_recipients"`
	Recent7DayRecipients int     `json:"recent_7_day_recipients"`
	ReachRate            float64 `json:"reach_rate"`
}

// HourlyViewItem represents the view count for one hour slot.
type HourlyViewItem struct {
	Hour  time.Time `json:"hour"`
	Count int       `json:"count"`
}

// ProjectViewer is one row in the GET /projects/{id}/viewers response.
type ProjectViewer struct {
	UserID             int       `db:"user_id"           json:"user_id"`
	TalentProfileID    *int      `db:"talent_profile_id" json:"talent_profile_id"`
	Nickname           *string   `db:"nickname"          json:"nickname"`
	AvatarUrl          *string   `db:"avatar_url"        json:"avatar_url"`
	AuthStatus         int       `db:"auth_status"       json:"auth_status"`
	CollaborationScore *float64  `db:"collaboration_score" json:"collaboration_score,omitempty"`
	LastViewedAt       time.Time `db:"last_viewed_at"    json:"last_viewed_at"`
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

func (r *ProjectViewLogRepository) NotifyProgress(ctx context.Context, projectID, viewerUserID, ownerUserID int) (InteractionNotifyProgress, error) {
	var progress InteractionNotifyProgress
	err := r.db.QueryRowxContext(ctx, `
		SELECT
			COUNT(DISTINCT CASE WHEN user_id IS NOT NULL AND user_id<>? THEN user_id END) distinct_user_count,
			NOT EXISTS (
				SELECT 1
				FROM project_view_log prev
				WHERE prev.project_id = ?
				  AND prev.user_id = ?
				  AND prev.duration_ms IS NULL
				  AND prev.id < (
					SELECT MAX(cur.id)
					FROM project_view_log cur
					WHERE cur.project_id = ?
					  AND cur.user_id = ?
					  AND cur.duration_ms IS NULL
				  )
				  AND prev.viewed_at >= DATE_SUB((
					SELECT MAX(cur.viewed_at)
					FROM project_view_log cur
					WHERE cur.project_id = ?
					  AND cur.user_id = ?
					  AND cur.duration_ms IS NULL
				  ), INTERVAL 30 DAY)
			) is_new_user
		FROM project_view_log
		WHERE project_id = ?
		  AND duration_ms IS NULL
		  AND viewed_at >= DATE_SUB(NOW(), INTERVAL 30 DAY)
	`, ownerUserID, projectID, viewerUserID, projectID, viewerUserID, projectID, viewerUserID, projectID).Scan(&progress.DistinctUserCount, &progress.IsNewUser)
	if err != nil {
		return InteractionNotifyProgress{}, fmt.Errorf("get project visit notify progress: %w", err)
	}
	return progress, nil
}

// GetDashboardStats returns aggregated dashboard data for a single project.
// Rows with duration_ms IS NOT NULL are dwell-time-only records and are excluded from view counts.
func (r *ProjectViewLogRepository) GetDashboardStats(ctx context.Context, projectID int) (*ProjectDashboardStats, error) {
	// Load independent scalar metrics in one database round-trip. Duration-only
	// rows remain excluded from every view counter.
	var totalViews, todayViews, totalApplicants, uniqueVisitors, processedApplicants int
	var avgDurationMs float64
	if err := r.db.QueryRowxContext(ctx, `SELECT
		p.view_count,
		(SELECT COUNT(*) FROM project_view_log vl WHERE vl.project_id=p.id AND vl.viewed_at>=CURDATE() AND vl.duration_ms IS NULL),
		(SELECT COUNT(DISTINCT vl.user_id) FROM project_view_log vl WHERE vl.project_id=p.id AND vl.user_id IS NOT NULL AND vl.duration_ms IS NULL),
		(SELECT COUNT(DISTINCT pa.user_id) FROM project_application pa WHERE pa.project_id=p.id),
		(SELECT COUNT(*) FROM project_application pa WHERE pa.project_id=p.id AND pa.status<>?),
		(SELECT COALESCE(AVG(vl.duration_ms),0) FROM project_view_log vl WHERE vl.project_id=p.id AND vl.duration_ms IS NOT NULL)
		FROM project p WHERE p.id=?`, models.ApplicationStatusPending, projectID).Scan(
		&totalViews, &todayViews, &uniqueVisitors, &totalApplicants, &processedApplicants, &avgDurationMs,
	); err != nil {
		return nil, fmt.Errorf("get dashboard scalar metrics: %w", err)
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

	// Hourly view counts for the last 24 hours. Bucket by Unix epoch so the
	//    generated UTC slots stay aligned with MySQL TIMESTAMP conversion.
	var nowEpoch int64
	if err := r.db.QueryRowxContext(ctx,
		`SELECT CAST(FLOOR(UNIX_TIMESTAMP(NOW()) / 3600) * 3600 AS SIGNED)`,
	).Scan(&nowEpoch); err != nil {
		return nil, fmt.Errorf("get current hour: %w", err)
	}
	startEpoch := nowEpoch - int64(23*time.Hour/time.Second)

	type hourlyRow struct {
		HourEpoch int64 `db:"hour_epoch"`
		Count     int   `db:"cnt"`
	}
	var rawHourly []hourlyRow
	if err := r.db.SelectContext(ctx, &rawHourly,
		`SELECT CAST(FLOOR(UNIX_TIMESTAMP(viewed_at) / 3600) * 3600 AS SIGNED) AS hour_epoch, COUNT(*) AS cnt
		 FROM project_view_log
		 WHERE project_id = ? AND viewed_at >= FROM_UNIXTIME(?) AND viewed_at < FROM_UNIXTIME(?) AND duration_ms IS NULL
		 GROUP BY hour_epoch ORDER BY hour_epoch ASC`,
		projectID, startEpoch, nowEpoch+3600,
	); err != nil {
		return nil, fmt.Errorf("get hourly_views: %w", err)
	}
	hourMap := make(map[int64]int, len(rawHourly))
	for _, row := range rawHourly {
		hourMap[row.HourEpoch] = row.Count
	}
	hourlyViews := make([]HourlyViewItem, 24)
	for i := 0; i < 24; i++ {
		slotEpoch := startEpoch + int64(i*3600)
		slot := time.Unix(slotEpoch, 0).UTC()
		hourlyViews[i] = HourlyViewItem{Hour: slot, Count: hourMap[slotEpoch]}
	}

	stats := &ProjectDashboardStats{
		TotalViews:          totalViews,
		TodayViews:          todayViews,
		UniqueVisitors:      uniqueVisitors,
		TotalApplicants:     totalApplicants,
		ProcessedApplicants: processedApplicants,
		AvgDurationSeconds:  int(math.Round(avgDurationMs / 1000)),
		HourlyViews:         hourlyViews,
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

	if uniqueVisitors > 0 {
		raw := float64(totalApplicants) / float64(uniqueVisitors) * 100
		stats.ConversionRate = math.Round(raw*100) / 100
	}

	if err := r.db.QueryRowxContext(ctx, `
		SELECT
			COALESCE(SUM(max_recipients), 0) AS total_recipients,
			COALESCE(SUM(total_sent), 0) AS completed_recipients,
			COALESCE(SUM(CASE WHEN created_at >= DATE_SUB(NOW(), INTERVAL 7 DAY) THEN max_recipients ELSE 0 END), 0) AS recent_7_day_recipients
		FROM email_promotion
		WHERE project_id = ?
	`, projectID).Scan(
		&stats.PromotionStats.TotalRecipients,
		&stats.PromotionStats.CompletedRecipients,
		&stats.PromotionStats.Recent7DayRecipients,
	); err != nil {
		return nil, fmt.Errorf("get promotion dashboard stats: %w", err)
	}
	if stats.PromotionStats.TotalRecipients > 0 {
		raw := float64(stats.PromotionStats.CompletedRecipients) / float64(stats.PromotionStats.TotalRecipients) * 100
		stats.PromotionStats.ReachRate = math.Round(raw*100) / 100
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
		`SELECT vl.user_id, tp.id AS talent_profile_id, u.nickname, u.avatar_url, COALESCE(u.auth_status, 0) AS auth_status,
		        u.collaboration_score,
		        MAX(vl.viewed_at) AS last_viewed_at
		 FROM project_view_log vl
		 JOIN `+"`user`"+` u ON u.id = vl.user_id
		 LEFT JOIN talent_profile tp ON tp.user_id = vl.user_id
		 WHERE vl.project_id = ? AND vl.user_id IS NOT NULL
		   AND vl.viewed_at >= NOW() - INTERVAL 24 HOUR AND vl.duration_ms IS NULL
		 GROUP BY vl.user_id, tp.id, u.nickname, u.avatar_url, u.auth_status, u.collaboration_score
		 ORDER BY last_viewed_at DESC
		 LIMIT ?`,
		projectID, limit,
	); err != nil {
		return nil, 0, fmt.Errorf("get viewers: %w", err)
	}
	for i := range viewers {
		viewers[i].Nickname = models.DisplayNickname(viewers[i].Nickname)
		if viewers[i].AvatarUrl != nil && *viewers[i].AvatarUrl != "" {
			fullURL := oss.FullURL(*viewers[i].AvatarUrl)
			viewers[i].AvatarUrl = &fullURL
		}
	}
	return viewers, total, nil
}

func (r *ProjectViewLogRepository) CountUnreadVisits(ctx context.Context, projectID, ownerUserID int) (int, error) {
	var count int
	err := r.db.QueryRowxContext(ctx, `
		SELECT COUNT(DISTINCT vl.user_id)
		FROM project_view_log vl
		WHERE vl.project_id = ?
		  AND vl.user_id IS NOT NULL
		  AND vl.user_id <> ?
		  AND vl.duration_ms IS NULL
		  AND vl.viewed_at > COALESCE(
			(SELECT s.last_viewed_at
			 FROM interaction_dashboard_view_state s
			 WHERE s.user_id = ? AND s.target_type = 'projects'
			   AND s.target_id = ? AND s.interaction_type = 'visit'),
			'1970-01-01 00:00:01'
		  )
		  AND NOT EXISTS (
			SELECT 1
			FROM project_view_log prev
			WHERE prev.project_id = vl.project_id
			  AND prev.user_id = vl.user_id
			  AND prev.duration_ms IS NULL
			  AND (prev.viewed_at < vl.viewed_at OR (prev.viewed_at = vl.viewed_at AND prev.id < vl.id))
			  AND prev.viewed_at >= DATE_SUB(vl.viewed_at, INTERVAL 30 DAY)
		  )
	`, projectID, ownerUserID, ownerUserID, projectID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count unread project visits: %w", err)
	}
	return count, nil
}
