package models

import (
	"time"

	"github.com/trv3wood/kuaizu-server/api"
)

// ProjectApplication represents a project application in the database
type ProjectApplication struct {
	ID        int
	ProjectID int
	UserID    int
	Contact   *string
	Status    int // 0-待审核, 1-已通过, 2-已拒绝
	AppliedAt time.Time
	UpdatedAt time.Time

	// Joined fields
	ProjectName *string
	Applicant   *User
}

// ToVO converts ProjectApplication to API ProjectApplicationVO
func (a *ProjectApplication) ToVO() *api.ProjectApplicationVO {
	status := api.ApplicationStatus(a.Status)

	vo := &api.ProjectApplicationVO{
		Id:          &a.ID,
		ProjectId:   &a.ProjectID,
		ProjectName: a.ProjectName,
		Contact:     a.Contact,
		Status:      &status,
		AppliedAt:   &a.AppliedAt,
	}

	if a.Applicant != nil {
		vo.Applicant = a.Applicant.ToVO()
	}

	return vo
}
