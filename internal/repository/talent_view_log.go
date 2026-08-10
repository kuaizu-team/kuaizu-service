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
	HourlyViews        []HourlyViewItem
	OwnerUserID        int
	VisitUnread        int
}

// TalentViewer is one row in the GET /talent-profiles/{id}/viewers response.
type TalentViewer struct {
	UserID       int       `db:"user_id"        json:"user_id"`
	Nickname     *string   `db:"nickname"       json:"nickname"`
	AvatarUrl    *string   `db:"avatar_url"     json:"avatar_url"`
	LastViewedAt time.Time `db:"last_viewed_at" json:"last_viewed_at"`
}

// TopTalentViewer is one row in the GET /talent-profiles/{id}/top-viewers response.
type TopTalentViewer struct {
	UserID    int     `db:"user_id"    json:"user_id"`
	Nickname  *string `db:"nickname"   json:"nickname"`
	AvatarUrl *string `db:"avatar_url" json:"avatar_url"`
	ViewCount int     `db:"view_count" json:"view_count"`
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

// RecordView atomically stores the view event and updates the denormalized counter.
func (r *TalentViewLogRepository) RecordView(ctx context.Context, log *models.TalentViewLog) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin talent view transaction: %w", err)
	}
	defer tx.Rollback()

	query := `
		INSERT INTO talent_view_log (talent_id, user_id, source)
		VALUES (:talent_id, :user_id, :source)
	`
	if _, err := tx.NamedExecContext(ctx, query, log); err != nil {
		return fmt.Errorf("insert talent view log: %w", err)
	}
	result, err := tx.ExecContext(ctx, `UPDATE talent_profile SET view_count = view_count + 1 WHERE id = ?`, log.TalentID)
	if err != nil {
		return fmt.Errorf("increment talent view count: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("increment talent view count: talent profile not found")
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit talent view transaction: %w", err)
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

func (r *TalentViewLogRepository) NotifyProgress(ctx context.Context, talentID, viewerUserID, ownerUserID int) (InteractionNotifyProgress, error) {
	var progress InteractionNotifyProgress
	err := r.db.QueryRowxContext(ctx, `
		SELECT
			COUNT(DISTINCT CASE WHEN user_id IS NOT NULL AND user_id<>? THEN user_id END) distinct_user_count,
			NOT EXISTS (
				SELECT 1
				FROM talent_view_log prev
				WHERE prev.talent_id = ?
				  AND prev.user_id = ?
				  AND prev.duration_ms IS NULL
				  AND prev.id < (
					SELECT MAX(cur.id)
					FROM talent_view_log cur
					WHERE cur.talent_id = ?
					  AND cur.user_id = ?
					  AND cur.duration_ms IS NULL
				  )
				  AND prev.viewed_at >= DATE_SUB((
					SELECT MAX(cur.viewed_at)
					FROM talent_view_log cur
					WHERE cur.talent_id = ?
					  AND cur.user_id = ?
					  AND cur.duration_ms IS NULL
				  ), INTERVAL 30 DAY)
			) is_new_user
		FROM talent_view_log
		WHERE talent_id = ?
		  AND duration_ms IS NULL
		  AND viewed_at >= DATE_SUB(NOW(), INTERVAL 30 DAY)
	`, ownerUserID, talentID, viewerUserID, talentID, viewerUserID, talentID, viewerUserID, talentID).Scan(&progress.DistinctUserCount, &progress.IsNewUser)
	if err != nil {
		return InteractionNotifyProgress{}, fmt.Errorf("get talent visit notify progress: %w", err)
	}
	return progress, nil
}

// GetDashboardStats returns aggregated dashboard data for a single talent profile.
func (r *TalentViewLogRepository) GetDashboardStats(ctx context.Context, talentID int) (*TalentDashboardStats, error) {
	var totalViews int
	var ownerUserID int
	if err := r.db.QueryRowxContext(ctx,
		`SELECT view_count, user_id FROM talent_profile WHERE id = ?`, talentID,
	).Scan(&totalViews, &ownerUserID); err != nil {
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

	var nowEpoch int64
	if err := r.db.QueryRowxContext(ctx,
		`SELECT CAST(FLOOR(UNIX_TIMESTAMP(NOW()) / 3600) * 3600 AS SIGNED)`,
	).Scan(&nowEpoch); err != nil {
		return nil, fmt.Errorf("get talent current hour: %w", err)
	}
	startEpoch := nowEpoch - int64(23*time.Hour/time.Second)

	type hourlyRow struct {
		HourEpoch int64 `db:"hour_epoch"`
		Count     int   `db:"cnt"`
	}
	var rawHourly []hourlyRow
	if err := r.db.SelectContext(ctx, &rawHourly,
		`SELECT CAST(FLOOR(UNIX_TIMESTAMP(viewed_at) / 3600) * 3600 AS SIGNED) AS hour_epoch, COUNT(*) AS cnt
		 FROM talent_view_log
		 WHERE talent_id = ? AND viewed_at >= FROM_UNIXTIME(?) AND viewed_at < FROM_UNIXTIME(?) AND duration_ms IS NULL
		 GROUP BY hour_epoch ORDER BY hour_epoch ASC`,
		talentID, startEpoch, nowEpoch+3600,
	); err != nil {
		return nil, fmt.Errorf("get talent hourly_views: %w", err)
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

	stats := &TalentDashboardStats{
		TotalViews:         totalViews,
		TodayViews:         todayViews,
		AvgDurationSeconds: int(avgDurationMs / 1000),
		HourlyViews:        hourlyViews,
		OwnerUserID:        ownerUserID,
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
func (r *TalentViewLogRepository) GetViewers(ctx context.Context, talentID, page, limit int) ([]TalentViewer, int, error) {
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
		 ORDER BY last_viewed_at DESC, vl.user_id DESC
		 LIMIT ? OFFSET ?`,
		talentID, limit, (page-1)*limit,
	); err != nil {
		return nil, 0, fmt.Errorf("get talent viewers: %w", err)
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

// GetTopViewersToday returns the users with the most view events today.
func (r *TalentViewLogRepository) GetTopViewersToday(ctx context.Context, talentID, limit int) ([]TopTalentViewer, error) {
	viewers := make([]TopTalentViewer, 0)
	if err := r.db.SelectContext(ctx, &viewers,
		`SELECT vl.user_id, u.nickname, u.avatar_url, COUNT(*) AS view_count
		 FROM talent_view_log vl
		 JOIN `+"`user`"+` u ON u.id = vl.user_id
		 WHERE vl.talent_id = ? AND vl.user_id IS NOT NULL
		   AND vl.viewed_at >= CURDATE() AND vl.duration_ms IS NULL
		 GROUP BY vl.user_id, u.nickname, u.avatar_url
		 ORDER BY view_count DESC, MAX(vl.viewed_at) DESC
		 LIMIT ?`,
		talentID, limit,
	); err != nil {
		return nil, fmt.Errorf("get talent top viewers today: %w", err)
	}
	for i := range viewers {
		viewers[i].Nickname = models.DisplayNickname(viewers[i].Nickname)
		if viewers[i].AvatarUrl != nil && *viewers[i].AvatarUrl != "" {
			fullURL := oss.FullURL(*viewers[i].AvatarUrl)
			viewers[i].AvatarUrl = &fullURL
		}
	}
	return viewers, nil
}

func (r *TalentViewLogRepository) CountUnreadVisits(ctx context.Context, talentID, ownerUserID int) (int, error) {
	var count int
	err := r.db.QueryRowxContext(ctx, `
		SELECT COUNT(DISTINCT vl.user_id)
		FROM talent_view_log vl
		WHERE vl.talent_id = ?
		  AND vl.user_id IS NOT NULL
		  AND vl.user_id <> ?
		  AND vl.duration_ms IS NULL
		  AND vl.viewed_at > COALESCE(
			(SELECT s.last_viewed_at
			 FROM interaction_dashboard_view_state s
			 WHERE s.user_id = ? AND s.target_type = 'talent-profiles'
			   AND s.target_id = ? AND s.interaction_type = 'visit'),
			'1970-01-01 00:00:01'
		  )
		  AND NOT EXISTS (
			SELECT 1
			FROM talent_view_log prev
			WHERE prev.talent_id = vl.talent_id
			  AND prev.user_id = vl.user_id
			  AND prev.duration_ms IS NULL
			  AND (prev.viewed_at < vl.viewed_at OR (prev.viewed_at = vl.viewed_at AND prev.id < vl.id))
			  AND prev.viewed_at >= DATE_SUB(vl.viewed_at, INTERVAL 30 DAY)
		  )
	`, talentID, ownerUserID, ownerUserID, talentID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count unread talent visits: %w", err)
	}
	return count, nil
}
