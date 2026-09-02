package models

import (
	"time"

	"github.com/kuaizu-team/kuaizu-service/api"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

var eventRegistrationLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// ParseEventDate parses a DATE value in the event registration business zone.
func ParseEventDate(value string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02", value, eventRegistrationLocation)
}

// Event represents a campus competition/event.
type Event struct {
	ID                   int        `db:"id"`
	Name                 string     `db:"name"`
	IsRanking            int        `db:"is_ranking"`
	RegistrationDeadline *time.Time `db:"registration_deadline"`
	ArticleURL           *string    `db:"article_url"`
	Level                *string    `db:"level"`
	Summary              *string    `db:"summary"`
	OrganizerName        *string    `db:"organizer_name"`
	Description          *string    `db:"description"`
	ResourceURL          *string    `db:"resource_url"`
	QQGroup              *string    `db:"qq_group"`
	AllowCrossSchool     int        `db:"allow_cross_school"`
	AllowCrossMajor      int        `db:"allow_cross_major"`
	CrossSchoolMajorRule *string    `db:"cross_school_major_rule"`
	ParticipationMode    *string    `db:"participation_mode"`
	TeamMinMembers       *int       `db:"team_min_members"`
	TeamMaxMembers       *int       `db:"team_max_members"`
	ViewCount            int64      `db:"view_count"`
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
		Id:               &e.ID,
		Name:             &e.Name,
		IsRanking:        &isRanking,
		IsOpen:           &isOpen,
		IsExpired:        &isExpired,
		ArticleUrl:       e.ArticleURL,
		Summary:          e.Summary,
		OrganizerName:    e.OrganizerName,
		Description:      e.Description,
		ResourceUrl:      e.ResourceURL,
		QqGroup:          e.QQGroup,
		AllowCrossSchool: boolPtr(e.AllowCrossSchool == 1),
		AllowCrossMajor:  boolPtr(e.AllowCrossMajor == 1),
		TeamMinMembers:   e.TeamMinMembers,
		TeamMaxMembers:   e.TeamMaxMembers,
		ViewCount:        &e.ViewCount,
		SchoolName:       e.SchoolName,
		DisplayOrder:     &e.DisplayOrder,
		CreatedAt:        &e.CreatedAt,
		UpdatedAt:        &e.UpdatedAt,
	}
	if e.Level != nil {
		level := api.EventVOLevel(*e.Level)
		vo.Level = &level
	}
	if e.CrossSchoolMajorRule != nil {
		rule := api.EventVOCrossSchoolMajorRule(*e.CrossSchoolMajorRule)
		vo.CrossSchoolMajorRule = &rule
	}
	if e.ParticipationMode != nil {
		mode := api.EventVOParticipationMode(*e.ParticipationMode)
		vo.ParticipationMode = &mode
	}
	if e.RegistrationDeadline != nil {
		date := openapi_types.Date{Time: *e.RegistrationDeadline}
		vo.RegistrationDeadline = &date
	}
	return vo
}

func boolPtr(value bool) *bool { return &value }

// EventTimelineNode is one administrator-defined milestone in an event timeline.
type EventTimelineNode struct {
	ID          int64     `db:"id" json:"id"`
	EventID     int       `db:"event_id" json:"eventId"`
	Title       string    `db:"title" json:"title"`
	NodeTime    time.Time `db:"node_time" json:"nodeTime"`
	Description *string   `db:"description" json:"description"`
	SortOrder   int       `db:"sort_order" json:"sortOrder"`
	CreatedAt   time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt   time.Time `db:"updated_at" json:"updatedAt"`
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
