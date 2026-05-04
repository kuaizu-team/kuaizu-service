package models

import (
	"time"

	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/oss"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// User represents a user in the database
type User struct {
	ID                  int        `db:"id"`
	OpenID              string     `db:"openid"`
	Nickname            *string    `db:"nickname"`
	Phone               *string    `db:"phone"`
	Email               *string    `db:"email"`
	SchoolID            *int       `db:"school_id"`
	MajorID             *int       `db:"major_id"`
	Grade               *int       `db:"grade"`
	OliveBranchCount    *int       `db:"olive_branch_count"`     // 付费橄榄枝余额
	FreeBranchUsedToday *int       `db:"free_branch_used_today"` // 今日已用免费次数
	LastActiveDate      *time.Time `db:"last_active_date"`       // 最后活跃日期(用于重置免费次数)
	AuthStatus          *int       `db:"auth_status"`            // 0-未认证, 1-已认证, 2-认证失败
	AuthImgUrl          *string    `db:"auth_img_url"`           // 学生证认证图
	AvatarUrl           *string    `db:"avatar_url"`             // 头像
	CoverImage          *string    `db:"cover_image"`            // 封面图
	EmailOptOut         *bool      `db:"email_opt_out"`          // 是否退订邮件推广
	WechatID            *string    `db:"wechat_id"`              // 微信号
	CreatedAt           *time.Time `db:"created_at"`

	// Joined fields (not always populated)
	SchoolName *string  `db:"school_name"`
	SchoolCode *string  `db:"school_code"`
	MajorName  *string  `db:"major_name"`
	ClassID    *int     `db:"class_id"`
	Skills     []string `db:"-"`
}

// ToVO converts User to API UserVO
func (u *User) ToVO() *api.UserVO {
	vo := &api.UserVO{
		Id:                  &u.ID,
		Nickname:            u.Nickname,
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

// ptrFullURL takes a nullable relative OSS path and returns a pointer to the full URL.
// Returns nil when the input is nil.
func ptrFullURL(rel *string) *string {
	if rel == nil {
		return nil
	}
	v := oss.FullURL(*rel)
	return &v
}
