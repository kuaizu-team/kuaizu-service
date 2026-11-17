package service

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

type orderOrigin struct {
	SchoolID  *int   `db:"origin_school_id"`
	Promotion string `db:"promotion_status"`
	Status    string `db:"status"`
}

func (s *CampusCooperationService) saveSchoolOrder(ctx context.Context, tx *sqlx.Tx, scope repository.CooperationAdminScope, adminID int, id int64, in CommercialOrderInput) (int64, error) {
	if id != 0 {
		var origin orderOrigin
		if err := tx.GetContext(ctx, &origin, `SELECT origin_school_id,promotion_status,status FROM commercial_order WHERE id=? FOR UPDATE`, id); err != nil {
			return 0, cooperationError(err)
		}
		if origin.SchoolID == nil || !scope.CanAccess(*origin.SchoolID, true) {
			return 0, ErrForbidden("只能编辑本校自建商单")
		}
		if origin.Promotion == "national" {
			return 0, ErrForbidden("已全国发布的商单由平台维护；仍可编辑本校发布内容")
		}
		if origin.Status == "ended" && in.Status != "ended" {
			return 0, ErrBadRequest("已结束的商单不可重新开放")
		}
	} else if !scope.CanAccess(in.SchoolID, true) {
		return 0, ErrForbidden("请选择当前负责的学校")
	}
	if in.FeishuWikiNodeToken != nil && *in.FeishuWikiNodeToken != "" {
		return 0, ErrForbidden("飞书商单文档仅由平台绑定")
	}
	var template any
	if len(in.Template) > 0 && string(in.Template) != "null" {
		template = string(in.Template)
	}
	if id == 0 {
		result, err := tx.ExecContext(ctx, `INSERT INTO commercial_order(name,company_name,description,status,content_template,created_by,updated_by,origin_school_id,promotion_status) VALUES(?,?,?,?,?,?,?,?,'local')`, in.Name, in.CompanyName, in.Description, in.Status, template, adminID, adminID, in.SchoolID)
		if err != nil {
			return 0, err
		}
		id, err = result.LastInsertId()
		if err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO campus_cooperation(commercial_order_id,school_id,claimed_by) VALUES(?,?,?)`, id, in.SchoolID, adminID); err != nil {
			return 0, err
		}
	} else {
		// Editing a submitted proposal invalidates that version's consent.
		if _, err := tx.ExecContext(ctx, `UPDATE commercial_order SET name=?,company_name=?,description=?,status=?,content_template=IF(?,content_template,?),updated_by=?,promotion_status='local',promotion_consent_by=NULL,promotion_consent_at=NULL WHERE id=?`, in.Name, in.CompanyName, in.Description, in.Status, len(in.Template) == 0, template, adminID, id); err != nil {
			return 0, err
		}
	}
	if in.Status == "ended" {
		if _, err := tx.ExecContext(ctx, `UPDATE campus_cooperation SET published=0,updated_by=? WHERE commercial_order_id=?`, adminID, id); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit()
}

func (s *CampusCooperationService) changeOrderPromotion(ctx context.Context, adminID int, id int64, action string) (int64, error) {
	if id <= 0 {
		return 0, ErrBadRequest("请先保存商单")
	}
	tx, err := s.repo.DB().BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	scope, err := s.adminScope(ctx, tx, adminID, true)
	if err != nil {
		return 0, err
	}
	var origin orderOrigin
	if err = tx.GetContext(ctx, &origin, `SELECT origin_school_id,promotion_status,status FROM commercial_order WHERE id=? FOR UPDATE`, id); err != nil {
		return 0, cooperationError(err)
	}
	next, err := promotionTransition(scope, origin, action)
	if err != nil {
		return 0, err
	}
	if next == "submitted" {
		_, err = tx.ExecContext(ctx, `UPDATE commercial_order SET promotion_status='submitted',promotion_consent_by=?,promotion_consent_at=NOW(),updated_by=? WHERE id=?`, adminID, adminID, id)
	} else if next == "local" {
		_, err = tx.ExecContext(ctx, `UPDATE commercial_order SET promotion_status='local',promotion_consent_by=NULL,promotion_consent_at=NULL,updated_by=? WHERE id=?`, adminID, id)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE commercial_order SET promotion_status='national',updated_by=? WHERE id=?`, adminID, id)
	}
	if err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func promotionTransition(scope repository.CooperationAdminScope, origin orderOrigin, action string) (string, error) {
	if origin.SchoolID == nil {
		return "", ErrBadRequest("平台商单无需向上传递")
	}
	if action == "promote" {
		if scope.Role != models.AdminRoleSuperAdmin {
			return "", ErrForbidden("只有平台超管可以发布到全国")
		}
		if origin.Promotion != "submitted" || origin.Status == "ended" {
			return "", ErrBadRequest("须由来源校区同意向上传递，且商单未结束")
		}
		return "national", nil
	}
	if scope.Role != models.AdminRoleSchoolSuperAdmin || !scope.CanAccess(*origin.SchoolID, true) {
		return "", ErrForbidden("只有来源校区当前负责人可以同意或撤回向上传递")
	}
	if action == "submit" && origin.Promotion == "local" && origin.Status != "ended" {
		return "submitted", nil
	}
	if action == "withdraw" && origin.Promotion == "submitted" {
		return "local", nil
	}
	return "", ErrBadRequest("当前状态无法执行该操作；全国发布后不可撤回")
}
