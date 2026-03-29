package models

import (
	"time"

	"github.com/trv3wood/kuaizu-server/api"
)

// Project represents a project in the database
type Project struct {
	ID                   int        `db:"id"`
	CreatorID            int        `db:"creator_id"`
	Name                 string     `db:"name"`
	Description          *string    `db:"description"`
	SchoolID             *int       `db:"school_id"`
	Direction            *int       `db:"direction"`
	MemberCount          *int       `db:"member_count"`
	Status               int        `db:"status"`                // 0-待审核, 1-已通过, 2-已驳回, 3-已关闭
	PromotionStatus      int        `db:"promotion_status"`      // 0-无, 1-推广中, 2-已结束
	PromotionExpireTime  *time.Time `db:"promotion_expire_time"` // 推广结束时间
	ViewCount            int        `db:"view_count"`            // 浏览量
	CreatedAt            time.Time  `db:"created_at"`
	UpdatedAt            time.Time  `db:"updated_at"`
	IsCrossSchool        *int       `db:"is_cross_school"`
	EducationRequirement *int       `db:"education_requirement"`
	SkillRequirement     *string    `db:"skill_requirement"`

	// Joined fields
	SchoolName *string `db:"school_name"`
	Creator    *User   `db:"-"`
}

// ToVO converts Project to API ProjectVO
func (p *Project) ToVO() *api.ProjectVO {
	status := api.ProjectStatus(p.Status)

	return &api.ProjectVO{
		Id:              &p.ID,
		Name:            &p.Name,
		Description:     p.Description,
		Direction:       (*api.Direction)(p.Direction),
		SchoolId:        p.SchoolID,
		SchoolName:      p.SchoolName,
		MemberCount:     p.MemberCount,
		Status:          &status,
		PromotionStatus: &p.PromotionStatus,
		IsCrossSchool:   p.IsCrossSchool,
		ViewCount:       &p.ViewCount,
	}
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
