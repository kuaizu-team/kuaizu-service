package service

import (
	"context"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

type TalentBatchResult struct {
	UserID   int    `json:"userId"`
	Approved bool   `json:"approved"`
	Message  string `json:"message"`
}

// BatchApproveUsers validates the complete school scope before changing anything.
// User and profile locks prevent school/status changes between validation and update.
func (s *TalentProfileService) BatchApproveUsers(ctx context.Context, ids []int, schools []int) ([]TalentBatchResult, error) {
	tx, err := s.repo.DB().BeginTxx(ctx, nil)
	if err != nil {
		return nil, ErrInternal("开始审核失败")
	}
	defer tx.Rollback()
	type row struct {
		ID        int  `db:"id"`
		SchoolID  *int `db:"school_id"`
		ProfileID *int `db:"profile_id"`
		Status    *int `db:"status"`
	}
	q, args, err := sqlx.In("SELECT u.id,u.school_id,tp.id AS profile_id,tp.status FROM `user` u LEFT JOIN talent_profile tp ON tp.user_id=u.id WHERE u.id IN (?) ORDER BY u.id FOR UPDATE", ids)
	if err != nil {
		return nil, ErrInternal("构建审核查询失败")
	}
	var rows []row
	if err = tx.SelectContext(ctx, &rows, tx.Rebind(q), args...); err != nil {
		return nil, ErrInternal("查询审核用户失败")
	}
	if len(rows) != len(ids) {
		return nil, ErrBadRequest("部分用户不存在，请刷新后重试")
	}
	for _, r := range rows {
		if schools == nil {
			continue
		}
		allowed := false
		if r.SchoolID != nil {
			for _, id := range schools {
				if id == *r.SchoolID {
					allowed = true
					break
				}
			}
		}
		if !allowed {
			return nil, ErrForbidden("包含非授权学校用户，整批未执行")
		}
	}
	results := make([]TalentBatchResult, 0, len(rows))
	for _, r := range rows {
		result := TalentBatchResult{UserID: r.ID, Message: "非待审核名片，已跳过"}
		if r.ProfileID != nil && r.Status != nil && *r.Status == models.TalentStatusReviewing {
			updated, err := tx.ExecContext(ctx, "UPDATE talent_profile SET status=?,reject_reason=NULL,updated_at=NOW() WHERE id=? AND status=?", models.TalentStatusOnline, *r.ProfileID, models.TalentStatusReviewing)
			if err != nil {
				return nil, ErrInternal("批量审核失败，整批已回滚")
			}
			count, err := updated.RowsAffected()
			if err != nil {
				return nil, ErrInternal("读取审核结果失败")
			}
			result.Approved = count == 1
			if result.Approved {
				result.Message = "审核通过"
			}
		}
		results = append(results, result)
	}
	if err = tx.Commit(); err != nil {
		return nil, ErrInternal("提交批量审核失败")
	}
	// Preserve the existing subscription notification path, only after commit.
	for _, r := range results {
		if r.Approved {
			s.notifyTalentReviewResult(ctx, r.UserID, "审核通过", "名片已上架人才库，快去看看吧！")
		}
	}
	return results, nil
}
