package models

import "time"

// ProjectRecommendation links an approved project into the info-center recommendation surface.
type ProjectRecommendation struct {
	ID           int       `db:"id"`
	ProjectID    int       `db:"project_id"`
	DisplayOrder int       `db:"display_order"`
	IsVisible    int       `db:"is_visible"`
	IsFeatured   int       `db:"is_featured"`
	InterviewURL *string   `db:"interview_url"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`

	Project        *Project `db:"-"`
	IsFromMySchool bool     `db:"-"`
}

// ArticleRecommendation is used by podcast_recommendation and news_recommendation.
type ArticleRecommendation struct {
	ID           int       `db:"id"`
	Title        string    `db:"title"`
	Description  *string   `db:"description"`
	ArticleURL   string    `db:"article_url"`
	DisplayOrder int       `db:"display_order"`
	IsVisible    int       `db:"is_visible"`
	IsFeatured   int       `db:"is_featured"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}

func BoolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
