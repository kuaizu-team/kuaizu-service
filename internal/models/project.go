package models

import (
	"time"

	"github.com/kuaizu-team/kuaizu-service/api"
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
	PublisherRole        *string    `db:"publisher_role"`
	InitiatingSchoolID   *int       `db:"initiating_school_id"`

	// Joined fields
	SchoolName                 *string      `db:"school_name"`
	PublisherRoleName          *string      `db:"publisher_role_name"`
	InitiatingSchoolName       *string      `db:"initiating_school_name"`
	Tags                       []ProjectTag `db:"-"`
	Interaction                Interaction  `db:"-"`
	Creator                    *User        `db:"-"`
	CreatorTalentProfileStatus *int         `db:"-"`                         // 创建者名片状态（仅详情接口填充）
	PendingApplicationCount    int          `db:"pending_application_count"` // 待处理申请数（仅列表接口填充）
	PendingCount               int          `db:"pending_count"`             // 管理后台用：投递+橄榄枝待处理总数（仅管理后台列表填充）
}

type ProjectTag struct {
	ID   int    `db:"id" json:"id"`
	Name string `db:"name" json:"name"`
}

// ToVO converts Project to API ProjectVO
func (p *Project) ToVO() *api.ProjectVO {
	status := api.ProjectStatus(p.Status)

	vo := &api.ProjectVO{
		Id:                      &p.ID,
		Name:                    &p.Name,
		Description:             p.Description,
		Direction:               (*api.Direction)(p.Direction),
		SchoolId:                p.SchoolID,
		SchoolName:              p.SchoolName,
		MemberCount:             p.MemberCount,
		Status:                  &status,
		PromotionStatus:         &p.PromotionStatus,
		IsCrossSchool:           p.IsCrossSchool,
		ViewCount:               &p.ViewCount,
		PendingApplicationCount: &p.PendingApplicationCount,
		UpdatedAt:               &p.UpdatedAt,
		PublisherRole:           p.PublisherRole,
		PublisherRoleName:       p.PublisherRoleName,
		InitiatingSchoolId:      p.InitiatingSchoolID,
		InitiatingSchoolName:    p.InitiatingSchoolName,
		Interaction:             p.Interaction.ToVO(),
	}
	if len(p.Tags) > 0 {
		tags := make([]api.ProjectTagVO, len(p.Tags))
		for i := range p.Tags {
			tags[i] = api.ProjectTagVO{Id: p.Tags[i].ID, Name: p.Tags[i].Name}
		}
		vo.Tags = &tags
	}
	if p.Creator != nil {
		vo.Creator = p.Creator.ToVO()
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
		PublisherRole:        p.PublisherRole,
		PublisherRoleName:    p.PublisherRoleName,
		InitiatingSchoolId:   p.InitiatingSchoolID,
		InitiatingSchoolName: p.InitiatingSchoolName,
		Interaction:          p.Interaction.ToVO(),
	}
	if len(p.Tags) > 0 {
		tags := make([]api.ProjectTagVO, len(p.Tags))
		for i := range p.Tags {
			tags[i] = api.ProjectTagVO{Id: p.Tags[i].ID, Name: p.Tags[i].Name}
		}
		vo.Tags = &tags
	}

	if p.Creator != nil {
		vo.Creator = p.Creator.ToVO()
	}

	return vo
}
