package models

import (
	"time"

	"github.com/kuaizu-team/kuaizu-service/api"
)

const (
	InformationCategoryCampusEvent     = "campus_event"
	InformationCategoryCampusProject   = "campus_project"
	InformationCategoryKuaizuTalking   = "kuaizu_talking"
	InformationCategoryDeveloperWeekly = "developer_weekly"
)

// InformationContent represents a published information-center item.
type InformationContent struct {
	ID           int       `db:"id"`
	Title        string    `db:"title"`
	URL          string    `db:"url"`
	Content      string    `db:"content"`
	Category     string    `db:"category"`
	DisplayOrder int       `db:"display_order"`
	IsPublished  int       `db:"is_published"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}

func IsValidInformationCategory(category string) bool {
	switch category {
	case InformationCategoryCampusEvent,
		InformationCategoryCampusProject,
		InformationCategoryKuaizuTalking,
		InformationCategoryDeveloperWeekly:
		return true
	default:
		return false
	}
}

// ToVO converts InformationContent to API InformationContentVO.
func (i *InformationContent) ToVO() api.InformationContentVO {
	return api.InformationContentVO{
		Id:        &i.ID,
		Title:     &i.Title,
		Url:       &i.URL,
		Content:   &i.Content,
		CreatedAt: &i.CreatedAt,
	}
}
