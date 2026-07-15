package handler

import (
	"strconv"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
)

// StatPoint is one time-series bucket returned by the stats endpoints.
type StatPoint struct {
	Date  string `json:"date"  db:"date"`
	Count int64  `json:"count" db:"count"`
}

// parseStatsRange validates the `range` query parameter.
// Returns (days, isHourly, errMsg).
//
//	1d  → isHourly=true,  days=1   — last 24 hours, bucketed by hour
//	Nd  → isHourly=false, days=N   — last N calendar days, bucketed by day
func parseStatsRange(r string) (days int, isHourly bool, errMsg string) {
	switch r {
	case "1d":
		return 1, true, ""
	case "7d":
		return 7, false, ""
	case "30d":
		return 30, false, ""
	case "180d":
		return 180, false, ""
	case "365d":
		return 365, false, ""
	default:
		return 0, false, "range 必须为 1d / 7d / 30d / 180d / 365d"
	}
}

// resolveSchoolFilter returns the school_id to use for filtering:
//   - campus admins (role=2/3): always forced to their own school
//   - super admin (role=1):     honours the optional `schoolId` query param
func (s *AdminServer) resolveSchoolFilter(ctx echo.Context) (*int, []int, error) {
	if adminRole(ctx) == models.AdminRoleSchoolSuperAdmin {
		ids, err := s.adminSchoolIDs(ctx)
		return nil, ids, err
	}
	// Campus admin — JWT-bound school takes priority; ignore query param.
	if sid := adminSchoolID(ctx); sid != nil {
		return sid, nil, nil
	}
	// Super admin — optional filter from query param.
	if v := ctx.QueryParam("schoolId"); v != "" {
		if id, err := strconv.Atoi(v); err == nil && id > 0 {
			return &id, nil, nil
		}
	}
	return nil, nil, nil
}

func appendStatsSchoolFilter(query string, args []interface{}, sid *int, schoolIDs []int) (string, []interface{}, error) {
	if len(schoolIDs) > 0 {
		condition, inArgs, err := sqlx.In(" AND school_id IN (?)", schoolIDs)
		if err != nil {
			return query, args, err
		}
		return query + condition, append(args, inArgs...), nil
	}
	if schoolIDs != nil {
		return query + " AND 1=0", args, nil
	}
	if sid != nil {
		return query + " AND school_id = ?", append(args, *sid), nil
	}
	return query, args, nil
}

// GetRegistrationStats handles GET /admin/stats/registrations
//
// Counts new users grouped by hour (range=1d) or by day (all other ranges),
// based on user.created_at.
func (s *AdminServer) GetRegistrationStats(ctx echo.Context) error {
	days, isHourly, errMsg := parseStatsRange(ctx.QueryParam("range"))
	if errMsg != "" {
		return response.BadRequest(ctx, errMsg)
	}

	sid, schoolIDs, scopeErr := s.resolveSchoolFilter(ctx)
	if scopeErr != nil {
		return response.InternalError(ctx, "查询学校权限失败")
	}
	db := s.repo.DB()
	rctx := ctx.Request().Context()

	var (
		query string
		args  []interface{}
	)

	if isHourly {
		// Last 24 hours, bucketed by hour.
		// DATE_FORMAT produces e.g. "2026-05-18T14:00".
		query = `
			SELECT DATE_FORMAT(created_at, '%Y-%m-%dT%H:00') AS date,
			       COUNT(*) AS count
			FROM ` + "`user`" + `
			WHERE created_at >= NOW() - INTERVAL 24 HOUR`
		query, args, _ = appendStatsSchoolFilter(query, args, sid, schoolIDs)
		query += " GROUP BY date ORDER BY date ASC"
	} else {
		// Last N calendar days (inclusive of today), bucketed by day.
		query = `
			SELECT DATE_FORMAT(created_at, '%Y-%m-%d') AS date,
			       COUNT(*) AS count
			FROM ` + "`user`" + `
			WHERE DATE(created_at) >= CURDATE() - INTERVAL ? DAY`
		args = append(args, days-1)
		query, args, _ = appendStatsSchoolFilter(query, args, sid, schoolIDs)
		query += " GROUP BY date ORDER BY date ASC"
	}

	var points []StatPoint
	if err := db.SelectContext(rctx, &points, query, args...); err != nil {
		return response.InternalError(ctx, "查询注册趋势失败")
	}
	if points == nil {
		points = []StatPoint{}
	}

	return response.Success(ctx, points)
}

// GetActivationStats handles GET /admin/stats/activations
//
// Counts active users grouped by day, based on user.last_active_date.
//
// Because last_active_date is a DATE column (not DATETIME), the 1d range cannot
// be broken into per-hour buckets. Instead, a single data point for today is
// returned (empty array when no users were active today).
func (s *AdminServer) GetActivationStats(ctx echo.Context) error {
	days, isHourly, errMsg := parseStatsRange(ctx.QueryParam("range"))
	if errMsg != "" {
		return response.BadRequest(ctx, errMsg)
	}

	sid, schoolIDs, scopeErr := s.resolveSchoolFilter(ctx)
	if scopeErr != nil {
		return response.InternalError(ctx, "查询学校权限失败")
	}
	db := s.repo.DB()
	rctx := ctx.Request().Context()

	var (
		query string
		args  []interface{}
	)

	if isHourly {
		// last_active_date is DATE-only — hourly breakdown is not available.
		// Degrade to a single data point for today.
		// HAVING COUNT(*) > 0 ensures an empty array is returned when no users
		// were active today (avoids returning {"date":"...","count":0}).
		query = `
			SELECT DATE_FORMAT(CURDATE(), '%Y-%m-%d') AS date,
			       COUNT(*) AS count
			FROM ` + "`user`" + `
			WHERE last_active_date = CURDATE()`
		query, args, _ = appendStatsSchoolFilter(query, args, sid, schoolIDs)
		query += " HAVING COUNT(*) > 0"
	} else {
		// Last N calendar days, bucketed by day.
		query = `
			SELECT DATE_FORMAT(last_active_date, '%Y-%m-%d') AS date,
			       COUNT(*) AS count
			FROM ` + "`user`" + `
			WHERE last_active_date >= CURDATE() - INTERVAL ? DAY`
		args = append(args, days-1)
		query, args, _ = appendStatsSchoolFilter(query, args, sid, schoolIDs)
		query += " GROUP BY date ORDER BY date ASC"
	}

	var points []StatPoint
	if err := db.SelectContext(rctx, &points, query, args...); err != nil {
		return response.InternalError(ctx, "查询活跃趋势失败")
	}
	if points == nil {
		points = []StatPoint{}
	}

	return response.Success(ctx, points)
}
