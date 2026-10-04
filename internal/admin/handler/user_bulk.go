package handler

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
)

const maxUserExportRows = 10000
const maxUserExportBytes = 32 * 1024 * 1024

var userExportSlots = make(chan struct{}, 2)

type userSelectionRequest struct {
	UserIDs  []int `json:"userIds"`
	Selected bool  `json:"selected"`
}

func validateUserIDs(ids []int, limit int) error {
	if len(ids) == 0 || len(ids) > limit {
		return fmt.Errorf("请选择 1 至 %d 个用户", limit)
	}
	seen := make(map[int]bool, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return fmt.Errorf("用户 ID 必须为正整数且不能重复")
		}
		seen[id] = true
	}
	return nil
}

func (s *AdminServer) BatchApproveUsers(ctx echo.Context) error {
	if !canReviewUserTalents(adminRole(ctx)) {
		return response.Forbidden(ctx, "无批量审核权限")
	}
	var req userSelectionRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "请求格式错误")
	}
	if err := validateUserIDs(req.UserIDs, 100); err != nil {
		return response.BadRequest(ctx, err.Error())
	}
	schools, err := s.adminSchoolIDs(ctx)
	if err != nil {
		return response.InternalError(ctx, "查询学校权限失败")
	}
	result, err := s.svc.TalentProfile.BatchApproveUsers(ctx.Request().Context(), req.UserIDs, schools)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return response.Success(ctx, result)
}

// Prefix dangerous spreadsheet formulas, including formulas after whitespace.
func safeCSVCell(value string) string {
	trimmed := strings.TrimLeftFunc(value, unicode.IsSpace)
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + value
	}
	if strings.HasPrefix(value, "\t") || strings.HasPrefix(value, "\r") || strings.HasPrefix(value, "\n") {
		return "'" + value
	}
	return value
}

func (s *AdminServer) ExportUsers(ctx echo.Context) error {
	if !canExportUserList(adminRole(ctx)) {
		return response.Forbidden(ctx, "无导出权限")
	}
	var req userSelectionRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "请求格式错误")
	}
	if req.Selected {
		if err := validateUserIDs(req.UserIDs, maxUserExportRows); err != nil {
			return response.BadRequest(ctx, err.Error())
		}
	} else if len(req.UserIDs) > 0 {
		return response.BadRequest(ctx, "部分导出必须指定 selected")
	}
	params, valid, err := s.parseUserListParams(ctx)
	if !valid {
		return err
	}
	select {
	case userExportSlots <- struct{}{}:
		defer func() { <-userExportSlots }()
	default:
		return ctx.JSON(http.StatusTooManyRequests, map[string]interface{}{"code": 429, "message": "导出任务较多，请稍后重试"})
	}
	work, cancel := context.WithTimeout(ctx.Request().Context(), 30*time.Second)
	defer cancel()
	// Validate every explicitly selected ID independently of filters. Never silently omit an unauthorized ID.
	if req.Selected {
		q, args, err := sqlx.In("SELECT id,school_id FROM `user` WHERE id IN (?)", req.UserIDs)
		if err != nil {
			return response.BadRequest(ctx, "用户选择无效")
		}
		type schoolRow struct {
			ID       int  `db:"id"`
			SchoolID *int `db:"school_id"`
		}
		var rows []schoolRow
		if err = s.repo.DB().SelectContext(work, &rows, s.repo.DB().Rebind(q), args...); err != nil {
			return response.InternalError(ctx, "校验导出范围失败")
		}
		if len(rows) != len(req.UserIDs) {
			return response.BadRequest(ctx, "部分用户不存在，请刷新后重试")
		}
		for _, r := range rows {
			if adminRole(ctx) == models.AdminRoleSchoolSuperAdmin && !schoolIDInScope(r.SchoolID, params.SchoolIDs) {
				return response.Forbidden(ctx, "包含非授权学校用户，导出已拒绝")
			}
		}
	}
	where, args, err := repository.UserFilterSQL(params)
	if err != nil {
		return response.InternalError(ctx, "构建导出筛选失败")
	}
	filterWhere, filterArgs := where, append([]interface{}{}, args...)
	if req.Selected {
		q, a, err := sqlx.In("u.id IN (?)", req.UserIDs)
		if err != nil {
			return response.BadRequest(ctx, "用户选择无效")
		}
		where += " AND " + q
		args = append(args, a...)
	}
	var ids []int
	if err = s.repo.DB().SelectContext(work, &ids, s.repo.DB().Rebind("SELECT u.id FROM `user` u WHERE "+where+" ORDER BY u.id LIMIT 10001"), args...); err != nil {
		return response.InternalError(ctx, "查询导出用户失败")
	}
	if len(ids) > maxUserExportRows {
		return response.BadRequest(ctx, "单次最多导出 10000 人，请缩小筛选范围")
	}
	// Stage on disk: query/file errors are returned before any attachment bytes are sent.
	file, err := os.CreateTemp("", "kuaizu-users-*.csv")
	if err != nil {
		return response.InternalError(ctx, "创建导出文件失败")
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err = file.WriteString("\xef\xbb\xbf"); err != nil {
		return response.InternalError(ctx, "写入导出文件失败")
	}
	writer := csv.NewWriter(file)
	if err = writer.Write([]string{"昵称", "MBTI", "学校", "专业", "入学年份", "自我介绍", "项目经历", "协作等级", "协作具体分数值", "是否通过学生认证", "是否入驻人才库", "电话", "微信号", "邮箱号", "账号状态"}); err != nil {
		return response.InternalError(ctx, "写入表头失败")
	}
	for start := 0; start < len(ids); start += 200 {
		end := start + 200
		if end > len(ids) {
			end = len(ids)
		}
		in, inArgs, _ := sqlx.In("u.id IN (?)", ids[start:end])
		// Reapply filters/school scope so changes since the ID query cannot leak data.
		batchArgs := append(append([]interface{}{}, filterArgs...), inArgs...)
		query := "SELECT COALESCE(u.nickname,''),COALESCE(tp.mbti,''),COALESCE(s.school_name,''),COALESCE(m.major_name,''),COALESCE(CAST(u.grade AS CHAR),''),COALESCE(tp.self_evaluation,''),COALESCE(tp.project_experience,''),u.collaboration_score,COALESCE(u.auth_status,0),COALESCE(tp.status,0),COALESCE(u.phone,''),COALESCE(u.wechat_id,''),COALESCE(u.email,''),u.user_status FROM `user` u LEFT JOIN talent_profile tp ON tp.user_id=u.id LEFT JOIN school s ON s.id=u.school_id LEFT JOIN major m ON m.id=u.major_id WHERE " + filterWhere + " AND " + in + " ORDER BY u.id"
		rows, err := s.repo.DB().QueryContext(work, s.repo.DB().Rebind(query), batchArgs...)
		if err != nil {
			return response.InternalError(ctx, "读取导出字段失败")
		}
		for rows.Next() {
			var nickname, mbti, school, major, grade, intro, experience, phone, wechat, email string
			var score *float64
			var auth, talent, status int
			if err = rows.Scan(&nickname, &mbti, &school, &major, &grade, &intro, &experience, &score, &auth, &talent, &phone, &wechat, &email, &status); err != nil {
				rows.Close()
				return response.InternalError(ctx, "读取导出记录失败")
			}
			level, scoreText := "", ""
			if score != nil {
				level = models.CollaborationLevel(*score)
				scoreText = strconv.FormatFloat(*score, 'f', 2, 64)
			}
			authText, talentText, statusText := "否", "未入驻人才库", "正常"
			if auth == 1 {
				authText = "是"
			}
			if talent == 1 {
				talentText = "已入驻人才库"
			}
			if status == 1 {
				statusText = "封禁"
			} else if status == 2 {
				statusText = "已毕业"
			}
			record := []string{nickname, mbti, school, major, grade, intro, experience, level, scoreText, authText, talentText, phone, wechat, email, statusText}
			for i := range record {
				record[i] = safeCSVCell(record[i])
			}
			if err = writer.Write(record); err != nil {
				rows.Close()
				return response.InternalError(ctx, "写入导出记录失败")
			}
			writer.Flush()
			if writer.Error() != nil {
				rows.Close()
				return response.InternalError(ctx, "写入导出文件失败")
			}
			size, err := file.Seek(0, 1)
			if err != nil || size > maxUserExportBytes {
				rows.Close()
				return response.BadRequest(ctx, "导出文件超过 32MB，请缩小筛选范围")
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return response.InternalError(ctx, "导出查询中断，请重试")
		}
	}
	writer.Flush()
	if writer.Error() != nil {
		return response.InternalError(ctx, "完成导出文件失败")
	}
	if err = work.Err(); err != nil {
		return response.InternalError(ctx, "导出超时，请缩小筛选范围")
	}
	if _, err = file.Seek(0, 0); err != nil {
		return response.InternalError(ctx, "读取导出文件失败")
	}
	label := "当前筛选"
	if req.Selected {
		label = "已勾选"
	}
	name := "用户名单_" + time.Now().UTC().Add(8*time.Hour).Format("2006-01-02") + "_" + label + ".csv"
	ctx.Response().Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(name))
	ctx.Response().Header().Set("Cache-Control", "no-store")
	return ctx.Stream(http.StatusOK, "text/csv; charset=utf-8", file)
}

func canReviewUserTalents(role int) bool {
	return role == models.AdminRoleSuperAdmin || role == models.AdminRoleSchoolSuperAdmin || role == models.AdminRoleSchoolAdmin
}

func canExportUserList(role int) bool {
	return role == models.AdminRoleSuperAdmin || role == models.AdminRoleSchoolSuperAdmin
}
