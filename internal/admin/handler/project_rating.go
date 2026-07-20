package handler

import (
	"database/sql"
	"math"
	"strconv"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
)

type adminProjectRatingItem struct {
	ID             int64     `db:"id" json:"id"`
	ProjectID      int       `db:"project_id" json:"projectId"`
	ProjectName    string    `db:"project_name" json:"projectName"`
	RaterID        int       `db:"rater_id" json:"raterId"`
	RaterNickname  *string   `db:"rater_nickname" json:"raterNickname"`
	TargetID       int       `db:"target_id" json:"targetId"`
	TargetMemberID int64     `db:"target_member_id" json:"targetMemberId"`
	RaterRole      string    `db:"rater_role" json:"raterRole"`
	RaterWeight    float64   `db:"rater_weight" json:"raterWeight"`
	Score          int       `db:"score" json:"score"`
	CreatedAt      time.Time `db:"created_at" json:"createdAt"`
	IsEffective    bool      `db:"is_effective" json:"isEffective"`
	ProjectScore   *float64  `db:"project_score" json:"projectScore"`
}

type updateProjectRatingRequest struct {
	Score int `json:"score"`
}

type projectRatingRecord struct {
	ID             int64 `db:"id"`
	ProjectID      int   `db:"project_id"`
	TargetID       int   `db:"target_id"`
	TargetMemberID int64 `db:"target_member_id"`
}

type projectRatingAccessRecord struct {
	TargetID       int   `db:"target_id"`
	TargetMemberID int64 `db:"target_member_id"`
}

type memberRemovalMatch struct {
	ID        int64     `db:"id"`
	JoinedAt  time.Time `db:"joined_at"`
	RemovedAt time.Time `db:"removed_at"`
}

func canAdjustProjectRating(role int) bool {
	return role == models.AdminRoleSuperAdmin || role == models.AdminRoleSchoolSuperAdmin || role == models.AdminRoleSchoolAdmin
}

// ListUserProjectRatings returns all raw ratings received by one user. This
// endpoint is admin-only; client APIs deliberately return aggregate scores only.
func (s *AdminServer) ListUserProjectRatings(ctx echo.Context) error {
	if adminRole(ctx) == models.AdminRoleEventManager {
		return response.Forbidden(ctx, "权限不足")
	}
	userID, err := strconv.Atoi(ctx.Param("id"))
	if err != nil || userID <= 0 {
		return response.BadRequest(ctx, "invalid user id")
	}
	if err := s.requireUserSchoolAccess(ctx, userID); err != nil {
		return err
	}

	page, _ := strconv.Atoi(ctx.QueryParam("page"))
	size, _ := strconv.Atoi(ctx.QueryParam("size"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}

	var total int64
	if err := s.repo.DB().GetContext(ctx.Request().Context(), &total,
		"SELECT COUNT(*) FROM project_member_rating WHERE target_id=?", userID); err != nil {
		return response.InternalError(ctx, "获取评分记录总数失败")
	}

	list := make([]adminProjectRatingItem, 0)
	if err := s.repo.DB().SelectContext(ctx.Request().Context(), &list, `
		SELECT r.id,r.project_id,p.name AS project_name,r.rater_id,u.nickname AS rater_nickname,
			r.target_id,r.target_member_id,r.rater_role,r.rater_weight,r.score,r.created_at,
			(latest.latest_id=r.id) AS is_effective,pms.score AS project_score
		FROM project_member_rating r
		INNER JOIN project p ON p.id=r.project_id
		LEFT JOIN `+"`user`"+` u ON u.id=r.rater_id
		INNER JOIN (
			SELECT target_member_id,rater_id,MAX(id) AS latest_id
			FROM project_member_rating
			WHERE target_id=?
			GROUP BY target_member_id,rater_id
		) latest ON latest.target_member_id=r.target_member_id AND latest.rater_id=r.rater_id
		LEFT JOIN project_member_score pms ON pms.project_member_id=r.target_member_id
		WHERE r.target_id=?
		ORDER BY r.created_at DESC,r.id DESC
		LIMIT ? OFFSET ?`, userID, userID, size, (page-1)*size); err != nil {
		return response.InternalError(ctx, "获取评分记录失败")
	}

	totalPages := int((total + int64(size) - 1) / int64(size))
	return response.Success(ctx, map[string]interface{}{
		"list": list, "total": total, "page": page, "size": size, "totalPages": totalPages,
	})
}

// UpdateProjectRating handles PUT /admin/ratings/:id. It updates one raw rating
// and atomically refreshes the target membership cycle's weighted score cache.
func (s *AdminServer) UpdateProjectRating(ctx echo.Context) error {
	if !canAdjustProjectRating(adminRole(ctx)) {
		return response.Forbidden(ctx, "权限不足")
	}
	ratingID, err := strconv.ParseInt(ctx.Param("id"), 10, 64)
	if err != nil || ratingID <= 0 {
		return response.BadRequest(ctx, "invalid rating id")
	}
	var req updateProjectRatingRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	if req.Score < 0 || req.Score > 100 {
		return response.BadRequest(ctx, "score must be between 0 and 100")
	}

	var accessRecord projectRatingAccessRecord
	if err := s.repo.DB().GetContext(ctx.Request().Context(), &accessRecord,
		"SELECT target_id,target_member_id FROM project_member_rating WHERE id=?", ratingID); err != nil {
		if err == sql.ErrNoRows {
			return response.NotFound(ctx, "评分记录不存在")
		}
		return response.InternalError(ctx, "获取评分记录失败")
	}
	if err := s.requireUserSchoolAccess(ctx, accessRecord.TargetID); err != nil {
		return err
	}

	tx, err := s.repo.DB().BeginTxx(ctx.Request().Context(), nil)
	if err != nil {
		return response.InternalError(ctx, "开启事务失败")
	}
	defer tx.Rollback()

	// Client ratings and member removal both lock the active membership row
	// before calculating or freezing a score. Use the same lock to serialize an
	// admin adjustment with those flows. Removed cycles have no active row and
	// cannot receive new client ratings, so sql.ErrNoRows is expected for them.
	var activeMemberID int64
	if err := tx.GetContext(ctx.Request().Context(), &activeMemberID,
		"SELECT id FROM project_members WHERE id=? FOR UPDATE", accessRecord.TargetMemberID); err != nil && err != sql.ErrNoRows {
		return response.InternalError(ctx, "lock rating membership cycle failed")
	}

	var record projectRatingRecord
	if err := tx.GetContext(ctx.Request().Context(), &record, `
		SELECT id,project_id,target_id,target_member_id
		FROM project_member_rating WHERE id=? FOR UPDATE`, ratingID); err != nil {
		if err == sql.ErrNoRows {
			return response.NotFound(ctx, "评分记录不存在")
		}
		return response.InternalError(ctx, "锁定评分记录失败")
	}
	if _, err := tx.ExecContext(ctx.Request().Context(),
		"UPDATE project_member_rating SET score=? WHERE id=?", req.Score, ratingID); err != nil {
		return response.InternalError(ctx, "更新评分失败")
	}

	var projectScore sql.NullFloat64
	var ratingCount int
	if err := tx.QueryRowxContext(ctx.Request().Context(), `
		SELECT ROUND(SUM(r.score*r.rater_weight)/NULLIF(SUM(r.rater_weight),0),2),COUNT(*)
		FROM project_member_rating r
		INNER JOIN (
			SELECT rater_id,MAX(id) AS latest_id
			FROM project_member_rating
			WHERE target_member_id=?
			GROUP BY rater_id
		) latest ON latest.latest_id=r.id`, record.TargetMemberID).Scan(&projectScore, &ratingCount); err != nil {
		return response.InternalError(ctx, "重新计算项目综合分失败")
	}
	if !projectScore.Valid || ratingCount == 0 {
		return response.InternalError(ctx, "重新计算项目综合分失败")
	}
	projectScore.Float64 = math.Round(projectScore.Float64*100) / 100
	if _, err := tx.ExecContext(ctx.Request().Context(), `
		INSERT INTO project_member_score(project_id,project_member_id,member_id,score,updated_at)
		VALUES(?,?,?,?,NOW())
		ON DUPLICATE KEY UPDATE score=VALUES(score),updated_at=VALUES(updated_at)`,
		record.ProjectID, record.TargetMemberID, record.TargetID, projectScore.Float64); err != nil {
		return response.InternalError(ctx, "更新项目综合分失败")
	}
	historicalScoreUpdated := false
	var removal memberRemovalMatch
	removalErr := tx.GetContext(ctx.Request().Context(), &removal, `
		SELECT id,joined_at,removed_at
		FROM project_member_removal
		WHERE user_id=? AND project_id=?
			AND joined_at<=(SELECT MIN(created_at) FROM project_member_rating WHERE target_member_id=?)
			AND removed_at>=(SELECT MAX(created_at) FROM project_member_rating WHERE target_member_id=?)
		ORDER BY removed_at DESC,id DESC LIMIT 1 FOR UPDATE`,
		record.TargetID, record.ProjectID, record.TargetMemberID, record.TargetMemberID)
	if removalErr != nil && removalErr != sql.ErrNoRows {
		return response.InternalError(ctx, "匹配成员移除评分周期失败")
	}
	if removalErr == nil {
		if _, err := tx.ExecContext(ctx.Request().Context(),
			"UPDATE project_member_removal SET score=? WHERE id=?", projectScore.Float64, removal.ID); err != nil {
			return response.InternalError(ctx, "同步成员移除评分失败")
		}
		var frozenScoreID int64
		frozenErr := tx.GetContext(ctx.Request().Context(), &frozenScoreID, `
			SELECT id FROM collaboration_score
			WHERE user_id=? AND project_id=?
				AND created_at BETWEEN ? AND DATE_ADD(?,INTERVAL 5 SECOND)
			ORDER BY ABS(TIMESTAMPDIFF(SECOND,created_at,?)),id DESC LIMIT 1 FOR UPDATE`,
			record.TargetID, record.ProjectID, removal.JoinedAt, removal.RemovedAt, removal.RemovedAt)
		if frozenErr != nil && frozenErr != sql.ErrNoRows {
			return response.InternalError(ctx, "匹配历史协作评分失败")
		}
		if frozenErr == nil {
			if _, err := tx.ExecContext(ctx.Request().Context(),
				"UPDATE collaboration_score SET score=?,rating_count=? WHERE id=?",
				projectScore.Float64, ratingCount, frozenScoreID); err != nil {
				return response.InternalError(ctx, "同步历史协作评分失败")
			}
			if _, err := tx.ExecContext(ctx.Request().Context(), `UPDATE `+"`user`"+` SET collaboration_score=(
				SELECT avg_score FROM (SELECT COALESCE(AVG(score),100) AS avg_score FROM collaboration_score WHERE user_id=?) t
			) WHERE id=?`, record.TargetID, record.TargetID); err != nil {
				return response.InternalError(ctx, "更新用户协作指数失败")
			}
			historicalScoreUpdated = true
		}
	}

	if err := tx.Commit(); err != nil {
		return response.InternalError(ctx, "提交事务失败")
	}

	return response.Success(ctx, map[string]interface{}{
		"id": record.ID, "score": req.Score, "targetId": record.TargetID,
		"targetMemberId": record.TargetMemberID, "projectScore": projectScore.Float64,
		"ratingCount": ratingCount, "historicalScoreUpdated": historicalScoreUpdated,
	})
}
