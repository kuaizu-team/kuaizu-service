package models

import (
	"time"

	"github.com/kuaizu-team/kuaizu-service/api"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

var eventRegistrationLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// Event represents a campus competition/event.
type Event struct {
	ID                   int        `db:"id"`
	Name                 string     `db:"name"`
	IsRanking            int        `db:"is_ranking"`
	RegistrationDeadline *time.Time `db:"registration_deadline"`
	ArticleURL           *string    `db:"article_url"`
	Level                *string    `db:"level"`
	Summary              *string    `db:"summary"`
	SchoolID             *int       `db:"school_id"`
	SchoolName           *string    `db:"school_name"`
	AdminID              *int       `db:"admin_id"`
	CreatorID            *int       `db:"creator_id"`
	ManagerUsername      *string    `db:"manager_username"`
	ManagerNickname      *string    `db:"manager_nickname"`
	DisplayOrder         int        `db:"display_order"`
	ProjectCount         int        `db:"project_count"`
	CreatedAt            time.Time  `db:"created_at"`
	UpdatedAt            time.Time  `db:"updated_at"`
}

func (e *Event) ToVO() api.EventVO {
	isRanking := e.IsRanking == 1
	isOpen := IsEventRegistrationOpen(e.RegistrationDeadline, time.Now())
	isExpired := !isOpen
	vo := api.EventVO{
		Id:           &e.ID,
		Name:         &e.Name,
		IsRanking:    &isRanking,
		IsOpen:       &isOpen,
		IsExpired:    &isExpired,
		ArticleUrl:   e.ArticleURL,
		Summary:      e.Summary,
		SchoolName:   e.SchoolName,
		DisplayOrder: &e.DisplayOrder,
		CreatedAt:    &e.CreatedAt,
		UpdatedAt:    &e.UpdatedAt,
	}
	if e.Level != nil {
		level := api.EventVOLevel(*e.Level)
		vo.Level = &level
	}
	if e.RegistrationDeadline != nil {
		date := openapi_types.Date{Time: *e.RegistrationDeadline}
		vo.RegistrationDeadline = &date
	}
	return vo
}

// IsEventRegistrationOpen reports whether registration is still open at now.
// A DATE deadline is treated as an exclusive boundary at midnight of the next
// day, so the full deadline date remains available for registration.
func IsEventRegistrationOpen(deadline *time.Time, now time.Time) bool {
	if deadline == nil {
		return true
	}

	year, month, day := deadline.Date()
	expiresAt := time.Date(year, month, day+1, 0, 0, 0, 0, eventRegistrationLocation)
	return now.Before(expiresAt)
}
