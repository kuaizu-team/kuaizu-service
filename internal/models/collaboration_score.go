package models

import "time"

type CollaborationScore struct {
	ID             int64      `db:"id" json:"id"`
	UserID         int        `db:"user_id" json:"userId"`
	ProjectID      int        `db:"project_id" json:"projectId"`
	ScorerID       int        `db:"scorer_id" json:"scorerId"`
	Score          float64    `db:"score" json:"score"`
	RatingCount    int        `db:"rating_count" json:"ratingCount"`
	CreatedAt      *time.Time `db:"created_at" json:"createdAt"`
	ProjectName    *string    `db:"project_name" json:"projectName,omitempty"`
	ScorerNickname *string    `db:"scorer_nickname" json:"scorerNickname,omitempty"`
}
