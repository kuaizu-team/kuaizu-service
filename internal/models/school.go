package models

import (
	"time"

	"github.com/kuaizu-team/kuaizu-service/api"
)

// School represents a school in the database
type School struct {
	ID         int       `db:"id"`
	SchoolName string    `db:"school_name"`
	SchoolCode *string   `db:"school_code"`
	CreatedAt  time.Time `db:"created_at"`
	UpdatedAt  time.Time `db:"updated_at"`
}

// ToVO converts School to API SchoolVO
func (s *School) ToVO() *api.SchoolVO {
	return &api.SchoolVO{
		Id:         &s.ID,
		SchoolName: &s.SchoolName,
		SchoolCode: s.SchoolCode,
	}
}
