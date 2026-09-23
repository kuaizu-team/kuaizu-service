package handler

import (
	"database/sql"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/labstack/echo/v4"
)

type collaborationRange struct {
	Level        string  `json:"level"`
	Min          float64 `json:"min"`
	Max          float64 `json:"max"`
	MaxInclusive bool    `json:"maxInclusive"`
}

func collaborationRanges() []collaborationRange {
	bounds := []float64{0, 50, 85, 90, 95, 100}
	ranges := make([]collaborationRange, 0, 5)
	for i := 0; i < 5; i++ {
		ranges = append(ranges, collaborationRange{Level: models.CollaborationLevel(bounds[i]), Min: bounds[i], Max: bounds[i+1], MaxInclusive: i == 4})
	}
	return ranges
}

type collaborationProjectDetail struct {
	ProjectID   int      `db:"project_id" json:"projectId"`
	ProjectName string   `db:"project_name" json:"projectName"`
	Score       *float64 `db:"score" json:"score"`
}

// GetUserCollaborationDetail exposes only aggregates to authenticated viewers.
// The original self-history endpoint and all score calculations remain unchanged.
func (s *Server) GetUserCollaborationDetail(ctx echo.Context, id int) error {
	if GetOptionalUserID(ctx) <= 0 {
		return Unauthorized(ctx, "请先登录后查看协作指数详情")
	}
	if id <= 0 {
		return BadRequest(ctx, "用户 ID 无效")
	}
	var score float64
	if err := s.repo.DB().GetContext(ctx.Request().Context(), &score, "SELECT COALESCE(collaboration_score,90) FROM `user` WHERE id=?", id); err != nil {
		if err == sql.ErrNoRows {
			return NotFound(ctx, "用户不存在")
		}
		return InternalError(ctx, "获取协作指数失败")
	}
	projects := make([]collaborationProjectDetail, 0)
	// Same aggregate and fallback precedence as GetMyCollaborationHistory. Add
	// current/removed membership and ownership for published, unrated projects.
	err := s.repo.DB().SelectContext(ctx.Request().Context(), &projects, `
 WITH scored AS (
  SELECT pms.project_id,ROUND(AVG(pms.score),2) AS score
  FROM project_member_score pms WHERE pms.member_id=? AND pms.score IS NOT NULL GROUP BY pms.project_id
  UNION ALL
  SELECT cs.project_id,ROUND(AVG(cs.score),2) AS score
  FROM collaboration_score cs WHERE cs.user_id=? AND NOT EXISTS (
   SELECT 1 FROM project_member_score pms
   WHERE pms.member_id=cs.user_id AND pms.project_id=cs.project_id AND pms.score IS NOT NULL
  ) GROUP BY cs.project_id
 ), participated AS (
  SELECT project_id FROM scored WHERE project_id IS NOT NULL
  UNION
  SELECT pm.project_id FROM project_members pm JOIN project p ON p.id=pm.project_id
  WHERE pm.user_id=? AND p.status IN (1,3,5) AND p.deleted_at IS NULL
  UNION
  SELECT pr.project_id FROM project_member_removal pr JOIN project p ON p.id=pr.project_id
  WHERE pr.user_id=? AND p.status IN (1,3,5) AND p.deleted_at IS NULL
  UNION
  SELECT p.id FROM project p WHERE p.creator_id=? AND p.status IN (1,3,5) AND p.deleted_at IS NULL
 )
 SELECT participated.project_id,COALESCE(p.name,'已结束的项目') AS project_name,scored.score
 FROM participated LEFT JOIN project p ON p.id=participated.project_id
 LEFT JOIN scored ON scored.project_id=participated.project_id
 ORDER BY p.created_at DESC,participated.project_id DESC`, id, id, id, id, id)
	if err != nil {
		return InternalError(ctx, "获取参与项目评分失败")
	}
	return Success(ctx, struct {
		UserID   int                          `json:"userId"`
		Score    float64                      `json:"score"`
		Level    string                       `json:"level"`
		Ranges   []collaborationRange         `json:"ranges"`
		Projects []collaborationProjectDetail `json:"projects"`
	}{id, score, models.CollaborationLevel(score), collaborationRanges(), projects})
}
