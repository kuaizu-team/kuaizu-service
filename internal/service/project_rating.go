package service

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

type ratingMemberRow struct {
	ID     int64  `db:"id"`
	UserID int    `db:"user_id"`
	Role   string `db:"role"`
}

type weightedRatingRow struct {
	Score  int     `db:"score"`
	Weight float64 `db:"rater_weight"`
}

type ratingStatusRow struct {
	ProjectMemberID int64      `db:"project_member_id"`
	MemberID        int        `db:"member_id"`
	Score           *float64   `db:"score"`
	LastRatedAt     *time.Time `db:"last_rated_at"`
	RatingCount     int        `db:"rating_count"`
}

func projectRoleRatingWeight(role string) float64 {
	switch role {
	case models.ProjectRoleTeamLeader:
		return models.ProjectRatingWeightLeader
	case models.ProjectRoleTeamMember:
		return models.ProjectRatingWeightMember
	case models.ProjectRoleLearningMember:
		return models.ProjectRatingWeightLearning
	default:
		return models.ProjectRatingWeightManager
	}
}

func weightedProjectRatingScore(rows []weightedRatingRow) (float64, int) {
	var weightedSum, weightSum float64
	for _, row := range rows {
		if row.Weight <= 0 {
			continue
		}
		weightedSum += float64(row.Score) * row.Weight
		weightSum += row.Weight
	}
	if weightSum == 0 {
		return 0, 0
	}
	return math.Round(weightedSum/weightSum*100) / 100, len(rows)
}

func projectRatingCooldown(lastRatedAt *time.Time, now time.Time) (bool, int, *time.Time) {
	if lastRatedAt == nil {
		return true, 0, nil
	}
	next := lastRatedAt.Add(models.ProjectRatingCooldown)
	if !next.After(now) {
		return true, 0, &next
	}
	days := int(math.Ceil(next.Sub(now).Hours() / 24))
	if days < 1 {
		days = 1
	}
	return false, days, &next
}

func (s *ProjectService) RateProjectMember(ctx context.Context, projectID, raterID, targetUserID, score int) (*models.ProjectMemberRatingResult, error) {
	if projectID <= 0 || targetUserID <= 0 {
		return nil, ErrBadRequest("invalid project or target user id")
	}
	if score < 0 || score > 100 {
		return nil, ErrBadRequest("score must be between 0 and 100")
	}
	if raterID == targetUserID {
		return nil, ErrBadRequest("不能给自己评分")
	}

	tx, err := s.repo.DB().BeginTxx(ctx, nil)
	if err != nil {
		return nil, ErrInternal("开启评分事务失败")
	}
	defer tx.Rollback()

	var members []ratingMemberRow
	query, args, err := sqlx.In(`SELECT id,user_id,role FROM project_members
		WHERE project_id=? AND user_id IN (?,?) ORDER BY id FOR UPDATE`, projectID, raterID, targetUserID)
	if err != nil {
		return nil, ErrInternal("构建成员查询失败")
	}
	if err := tx.SelectContext(ctx, &members, tx.Rebind(query), args...); err != nil {
		return nil, ErrInternal("获取项目成员失败")
	}
	if len(members) != 2 {
		return nil, ErrForbidden("评分人与被评分人必须都是当前项目成员")
	}

	var rater, target ratingMemberRow
	for _, member := range members {
		if member.UserID == raterID {
			rater = member
		}
		if member.UserID == targetUserID {
			target = member
		}
	}
	if rater.ID == 0 || target.ID == 0 {
		return nil, ErrForbidden("评分人与被评分人必须都是当前项目成员")
	}

	var lastRatedAt time.Time
	err = tx.GetContext(ctx, &lastRatedAt, `SELECT created_at FROM project_member_rating
		WHERE project_id=? AND rater_id=? AND target_member_id=?
		ORDER BY created_at DESC,id DESC LIMIT 1`, projectID, raterID, target.ID)
	if err != nil && err != sql.ErrNoRows {
		return nil, ErrInternal("获取评分冷却状态失败")
	}
	now := time.Now()
	if err == nil {
		canRate, days, _ := projectRatingCooldown(&lastRatedAt, now)
		if !canRate {
			return nil, ErrBadRequest(fmt.Sprintf("评分冷却中，剩余 %d 天可评分", days))
		}
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO project_member_rating(
		project_id,rater_id,target_id,rater_member_id,target_member_id,rater_role,rater_weight,score,created_at
	) VALUES(?,?,?,?,?,?,?,?,?)`, projectID, raterID, targetUserID, rater.ID, target.ID, rater.Role, projectRoleRatingWeight(rater.Role), score, now); err != nil {
		return nil, ErrInternal("提交评分失败")
	}

	var effectiveRows []weightedRatingRow
	if err := tx.SelectContext(ctx, &effectiveRows, `SELECT r.score,r.rater_weight
		FROM project_member_rating r
		INNER JOIN (
			SELECT rater_id,MAX(id) AS latest_id
			FROM project_member_rating
			WHERE target_member_id=?
			GROUP BY rater_id
		) latest ON latest.latest_id=r.id
		ORDER BY r.id`, target.ID); err != nil {
		return nil, ErrInternal("计算当前评分失败")
	}
	currentScore, ratingCount := weightedProjectRatingScore(effectiveRows)
	if ratingCount == 0 {
		return nil, ErrInternal("计算当前评分失败")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO project_member_score(
		project_id,project_member_id,member_id,score,updated_at
	) VALUES(?,?,?,?,?)
	ON DUPLICATE KEY UPDATE score=VALUES(score),updated_at=VALUES(updated_at)`,
		projectID, target.ID, targetUserID, currentScore, now); err != nil {
		return nil, ErrInternal("更新项目成员评分失败")
	}
	if err := tx.Commit(); err != nil {
		return nil, ErrInternal("提交评分事务失败")
	}

	nextRateAt := now.Add(models.ProjectRatingCooldown)
	return &models.ProjectMemberRatingResult{
		MemberID: targetUserID, Score: currentScore, CanRate: false,
		CooldownDays: 30, NextRateAt: nextRateAt, RatingCount: ratingCount,
	}, nil
}

func (s *ProjectService) ListProjectMemberRatings(ctx context.Context, projectID, userID int) ([]models.ProjectMemberRatingStatus, error) {
	if projectID <= 0 {
		return nil, ErrBadRequest("invalid project id")
	}
	var raterMemberID int64
	if err := s.repo.DB().GetContext(ctx, &raterMemberID,
		`SELECT id FROM project_members WHERE project_id=? AND user_id=?`, projectID, userID); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrForbidden("只有当前项目成员可以查看评分状态")
		}
		return nil, ErrInternal("获取当前成员信息失败")
	}

	var rows []ratingStatusRow
	if err := s.repo.DB().SelectContext(ctx, &rows, `SELECT
			pm.id AS project_member_id,pm.user_id AS member_id,pms.score,
			(
				SELECT r.created_at FROM project_member_rating r
				WHERE r.rater_id=? AND r.target_member_id=pm.id
				ORDER BY r.created_at DESC,r.id DESC LIMIT 1
			) AS last_rated_at,
			(
				SELECT COUNT(DISTINCT r2.rater_id)
				FROM project_member_rating r2
				WHERE r2.target_member_id=pm.id
			) AS rating_count
		FROM project_members pm
		LEFT JOIN project_member_score pms ON pms.project_member_id=pm.id
		WHERE pm.project_id=?
		ORDER BY pm.id`, userID, projectID); err != nil {
		return nil, ErrInternal("获取项目评分状态失败")
	}

	now := time.Now()
	result := make([]models.ProjectMemberRatingStatus, 0, len(rows))
	for _, row := range rows {
		status := models.ProjectMemberRatingStatus{
			MemberID: row.MemberID, Score: row.Score, RatingCount: row.RatingCount,
			IsSelf: row.MemberID == userID, ProjectMemberID: row.ProjectMemberID,
		}
		if status.IsSelf {
			status.RatingHint = "不能给自己评分"
		} else {
			status.CanRate, status.CooldownDays, status.NextRateAt = projectRatingCooldown(row.LastRatedAt, now)
			status.LastRatedAt = row.LastRatedAt
			if status.CanRate {
				status.RatingHint = "每30天可评分一次"
			} else {
				status.RatingHint = fmt.Sprintf("剩余 %d 天可评分", status.CooldownDays)
			}
		}
		result = append(result, status)
	}
	return result, nil
}
