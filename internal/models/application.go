package models

import (
	"time"

	"github.com/kuaizu-team/kuaizu-service/api"
)

// ProjectApplication represents a project application in the database
type ProjectApplication struct {
	ID           int       `db:"id"`
	ProjectID    int       `db:"project_id"`
	UserID       int       `db:"user_id"`
	Status       int       `db:"status"`  // 0-待审核, 1-正在互相了解, 2-已拒绝, 3-已加入团队
	IsRead       bool      `db:"is_read"` // 项目方是否已读
	ReviewerID   *int      `db:"reviewer_id"`
	ReviewerRole *string   `db:"reviewer_role"`
	AssignedRole *string   `db:"assigned_role"`
	AppliedAt    time.Time `db:"applied_at"`
	UpdatedAt    time.Time `db:"updated_at"`

	// Joined fields
	ProjectName      *string        `db:"project_name"`
	ReviewerRoleName *string        `db:"reviewer_role_name"`
	AssignedRoleName *string        `db:"assigned_role_name"`
	IsCurrentMember  *bool          `db:"is_current_member"`
	CanReview        *bool          `db:"-"`
	Applicant        *User          `db:"-"`
	TalentProfile    *TalentProfile `db:"-"`
}

// ToVO converts ProjectApplication to API ProjectApplicationVO
func (a *ProjectApplication) ToVO() *api.ProjectApplicationVO {

	vo := &api.ProjectApplicationVO{
		Id:               &a.ID,
		ProjectId:        &a.ProjectID,
		ProjectName:      a.ProjectName,
		Status:           (*api.ApplicationStatus)(&a.Status),
		IsRead:           &a.IsRead,
		ReviewerId:       a.ReviewerID,
		ReviewerRole:     a.ReviewerRole,
		ReviewerRoleName: a.ReviewerRoleName,
		AssignedRole:     a.AssignedRole,
		AssignedRoleName: a.AssignedRoleName,
		IsCurrentMember:  a.IsCurrentMember,
		CanReview:        a.CanReview,
		AppliedAt:        &a.AppliedAt,
	}

	if a.Applicant != nil {
		vo.Applicant = a.Applicant.ToVO()
	}

	if a.TalentProfile != nil {
		vo.TalentProfile = a.TalentProfile.ToVO()
	}

	return vo
}
