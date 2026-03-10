package models

import (
	"time"

	"github.com/trv3wood/kuaizu-server/api"
)

// Project represents a project in the database
type Project struct {
	ID                   int
	CreatorID            int
	Name                 string
	Description          *string
	SchoolID             *int
	Direction            *int
	MemberCount          *int
	Status               int        // 0-待审核, 1-已通过, 2-已驳回, 3-已关闭
	PromotionStatus      int        // 0-无, 1-推广中, 2-已结束
	PromotionExpireTime  *time.Time // 推广结束时间
	ViewCount            int        // 浏览量
	CreatedAt            time.Time
	UpdatedAt            time.Time
	IsCrossSchool        *int
	EducationRequirement *int
	SkillRequirement     *string

	// Joined fields
	SchoolName *string
	Creator    *User
}

// ToVO converts Project to API ProjectVO
func (p *Project) ToVO() *api.ProjectVO {
	status := api.ProjectStatus(p.Status)

	vo := &api.ProjectVO{
		Id:                  &p.ID,
		Name:                &p.Name,
		Description:         p.Description,
		Direction:           (*api.Direction)(p.Direction),
		SchoolId:            p.SchoolID,
		SchoolName:          p.SchoolName,
		MemberCount:         p.MemberCount,
		Status:              &status,
		PromotionStatus:     &p.PromotionStatus,
		ViewCount:           &p.ViewCount,
		CreatedAt:           &p.CreatedAt,
		PromotionExpireTime: p.PromotionExpireTime,
	}

	return vo
}

// ToDetailVO converts Project to API ProjectDetailVO
func (p *Project) ToDetailVO() *api.ProjectDetailVO {
	status := api.ProjectStatus(p.Status)

	vo := &api.ProjectDetailVO{
		Id:                   &p.ID,
		Name:                 &p.Name,
		Description:          p.Description,
		Direction:            (*api.Direction)(p.Direction),
		SchoolId:             p.SchoolID,
		SchoolName:           p.SchoolName,
		MemberCount:          p.MemberCount,
		Status:               &status,
		PromotionStatus:      &p.PromotionStatus,
		ViewCount:            &p.ViewCount,
		CreatedAt:            &p.CreatedAt,
		IsCrossSchool:        p.IsCrossSchool,
		EducationRequirement: p.EducationRequirement,
		SkillRequirement:     p.SkillRequirement,
		PromotionExpireTime:  p.PromotionExpireTime,
	}

	if p.Creator != nil {
		vo.Creator = p.Creator.ToVO()
	}

	return vo
}
