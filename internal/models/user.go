package models

import (
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/trv3wood/kuaizu-server/api"
	"github.com/trv3wood/kuaizu-server/internal/oss"
)

// User represents a user in the database
type User struct {
	ID                  int
	OpenID              string
	Nickname            *string
	Phone               *string
	Email               *string
	SchoolID            *int
	MajorID             *int
	Grade               *int
	OliveBranchCount    *int       // 付费橄榄枝余额
	FreeBranchUsedToday *int       // 今日已用免费次数
	LastActiveDate      *time.Time // 最后活跃日期(用于重置免费次数)
	AuthStatus          *int       // 0-未认证, 1-已认证, 2-认证失败
	AuthImgUrl          *string    // 学生证认证图
	AvatarUrl           *string    // 头像
	CoverImage          *string    // 封面图
	EmailOptOut         *bool      // 是否退订邮件推广
	CreatedAt           *time.Time

	// Joined fields (not always populated)
	SchoolName *string
	SchoolCode *string
	MajorName  *string
	ClassID    *int
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
