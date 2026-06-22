package models

import (
	"time"

	"github.com/kuaizu-team/kuaizu-service/api"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// Event represents a campus competition/event.
type Event struct {
	ID                   int        `db:"id"`
	Name                 string     `db:"name"`
	IsRanking            int        `db:"is_ranking"`
	RegistrationDeadline *time.Time `db:"registration_deadline"`
	ArticleURL           *string    `db:"article_url"`
	DisplayOrder         int        `db:"display_order"`
	CreatedAt            time.Time  `db:"created_at"`
	UpdatedAt            time.Time  `db:"updated_at"`
}

func (e *Event) ToVO() api.EventVO {
	isRanking := e.IsRanking == 1
	vo := api.EventVO{
		Id:           &e.ID,
		Name:         &e.Name,
		IsRanking:    &isRanking,
		ArticleUrl:   e.ArticleURL,
		DisplayOrder: &e.DisplayOrder,
		CreatedAt:    &e.CreatedAt,
		UpdatedAt:    &e.UpdatedAt,
	}
	if e.RegistrationDeadline != nil {
		date := openapi_types.Date{Time: *e.RegistrationDeadline}
		vo.RegistrationDeadline = &date
	}
	return vo
}
