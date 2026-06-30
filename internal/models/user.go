package models

import (
	"strings"
	"time"

	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/oss"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

const DefaultUserNickname = "快组儿"

// DisplayNickname keeps persisted data unchanged while providing one branded
// fallback for every API and notification representation.
func DisplayNickname(nickname *string) *string {
	if nickname == nil {
		value := DefaultUserNickname
		return &value
	}
	value := strings.TrimSpace(*nickname)
	if value == "" || value == "匿名用户" {
		value = DefaultUserNickname
	}
	return &value
}

// User represents a user in the database
type User struct {
	ID                       int        `db:"id"`
	OpenID                   string     `db:"openid"`
	Nickname                 *string    `db:"nickname"`
	Phone                    *string    `db:"phone"`
	Email                    *string    `db:"email"`
	SchoolID                 *int       `db:"school_id"`
	MajorID                  *int       `db:"major_id"`
	Grade                    *int       `db:"grade"`
	OliveBranchCount         *int       `db:"olive_branch_count"`          // 付费橄榄枝余额
	FreeBranchUsedToday      *int       `db:"free_branch_used_today"`      // 今日已用免费次数
	LastActiveDate           *time.Time `db:"last_active_date"`            // 最后活跃日期(用于重置免费次数)
	AuthStatus               *int       `db:"auth_status"`                 // 0-未认证, 1-已认证, 2-认证失败
	AuthImgUrl               *string    `db:"auth_img_url"`                // 学生证认证图
	AvatarUrl                *string    `db:"avatar_url"`                  // 头像
	CoverImage               *string    `db:"cover_image"`                 // 封面图
	EmailOptOut              *bool      `db:"email_opt_out"`               // 是否退订邮件推广
	WechatID                 *string    `db:"wechat_id"`                   // 微信号
	SentOliveViewedAt        *time.Time `db:"sent_olive_viewed_at"`        // 最后查看已发送橄榄枝的时间
	ApplicationsLastViewedAt *time.Time `db:"applications_last_viewed_at"` // 最后查看投递管理页的时间
	LastViewedMyProjectsAt   *time.Time `db:"last_viewed_my_projects_at"`  // 最后查看我的项目页的时间
	UserStatus               int        `db:"user_status"`                 // 0=正常, 1=封禁, 2=已毕业
	BanReason                *string    `db:"ban_reason"`                  // 封禁原因（仅 user_status=1 时有意义）
	CollaborationScore       *float64   `db:"collaboration_score"`
	CreatedAt                *time.Time `db:"created_at"`

	// Joined fields (not always populated)
	SchoolName      *string  `db:"school_name"`
	SchoolCode      *string  `db:"school_code"`
	MajorName       *string  `db:"major_name"`
	ClassID         *int     `db:"class_id"`
	TalentProfileID *int     `db:"talent_profile_id"`
	Skills          []string `db:"-"`
	PendingCount    int      `db:"pending_count"` // 管理后台用：待审核投递数+待处理橄榄枝数（仅管理后台列表填充）
}

// ToVO converts User to API UserVO
func (u *User) ToVO() *api.UserVO {
	vo := &api.UserVO{
		Id:                  &u.ID,
		Nickname:            DisplayNickname(u.Nickname),
		Phone:               u.Phone,
		Email:               u.Email,
		Grade:               u.Grade,
		OliveBranchCount:    u.OliveBranchCount,
		FreeBranchUsedToday: u.FreeBranchUsedToday,
		Wechat:              u.WechatID,
		AuthImgUrl:          ptrFullURL(u.AuthImgUrl),
		AvatarUrl:           ptrFullURL(u.AvatarUrl),
		CoverImage:          ptrFullURL(u.CoverImage),
		CreatedAt:           u.CreatedAt,
		TalentProfileId:     u.TalentProfileID,
		SchoolName:          u.SchoolName,
		MajorName:           u.MajorName,
		CollaborationScore:  u.CollaborationScore,
	}
	if u.CollaborationScore != nil {
		level := CollaborationLevel(*u.CollaborationScore)
		vo.CollaborationLevel = &level
	}

	if u.AuthStatus != nil {
		authStatus := api.AuthStatus(*u.AuthStatus)
		vo.AuthStatus = &authStatus
	}

	// Add LastActiveDate if available
	if u.LastActiveDate != nil {
		date := openapi_types.Date{Time: *u.LastActiveDate}
		vo.LastActiveDate = &date
	}

	// Populate school if available
	if u.SchoolID != nil && u.SchoolName != nil {
		vo.School = &api.SchoolVO{
			Id:         u.SchoolID,
			SchoolName: u.SchoolName,
			SchoolCode: u.SchoolCode,
		}
	}

	// Populate major if available
	if u.MajorID != nil && u.MajorName != nil {
		vo.Major = &api.MajorVO{
			Id:        u.MajorID,
			MajorName: u.MajorName,
			ClassId:   u.ClassID,
		}
	}

	if len(u.Skills) > 0 {
		skills := append([]string(nil), u.Skills...)
		vo.Skills = &skills
	}

	return vo
}

func CollaborationLevel(score float64) string {
	switch {
	case score >= 95:
		return "极好"
	case score >= 90:
		return "优秀"
	case score >= 85:
		return "良好"
	case score >= 50:
		return "中等"
	default:
		return "较差"
	}
}

// ptrFullURL takes a nullable relative OSS path and returns a pointer to the full URL.
// Returns nil when the input is nil.
func ptrFullURL(rel *string) *string {
	if rel == nil {
		return nil
	}
	v := oss.FullURL(*rel)
	return &v
}
