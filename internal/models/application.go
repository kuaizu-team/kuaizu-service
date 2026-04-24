package models

import (
	"time"

	"github.com/kuaizu-team/kuaizu-service/api"
)

// ProjectApplication represents a project application in the database
type ProjectApplication struct {
	ID        int       `db:"id"`
	ProjectID int       `db:"project_id"`
	UserID    int       `db:"user_id"`
	Status    int       `db:"status"` // 0-待审核, 1-已通过, 2-已拒绝
	AppliedAt time.Time `db:"applied_at"`
	UpdatedAt time.Time `db:"updated_at"`

	// Joined fields
	ProjectName   *string        `db:"project_name"`
	Applicant     *User          `db:"-"`
	TalentProfile *TalentProfile `db:"-"`
}

// ToVO converts ProjectApplication to API ProjectApplicationVO
func (a *ProjectApplication) ToVO() *api.ProjectApplicationVO {

	vo := &api.ProjectApplicationVO{
		Id:          &a.ID,
		ProjectId:   &a.ProjectID,
		ProjectName: a.ProjectName,
		Status:      (*api.ApplicationStatus)(&a.Status),
		AppliedAt:   &a.AppliedAt,
	}

	if a.Applicant != nil {
		vo.Applicant = a.Applicant.ToVO()
	}

	if a.TalentProfile != nil {
		vo.TalentProfile = a.TalentProfile.ToVO()
	}

	return vo
}
