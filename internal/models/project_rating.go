package models

import "time"

type ProjectMemberRating struct {
	ID             int64     `db:"id"`
	ProjectID      int       `db:"project_id"`
	RaterID        int       `db:"rater_id"`
	TargetID       int       `db:"target_id"`
	RaterMemberID  int64     `db:"rater_member_id"`
	TargetMemberID int64     `db:"target_member_id"`
	RaterRole      string    `db:"rater_role"`
	RaterWeight    float64   `db:"rater_weight"`
	Score          int       `db:"score"`
	CreatedAt      time.Time `db:"created_at"`
}

type ProjectMemberRatingStatus struct {
	MemberID        int        `json:"memberId"`
	Score           *float64   `json:"score"`
	CanRate         bool       `json:"canRate"`
	RatingFrozen    bool       `json:"ratingFrozen"`
	FreezeDays      int        `json:"freezeDays"`
	CooldownDays    int        `json:"cooldownDays"`
	LastRatedAt     *time.Time `json:"-"`
	NextRateAt      *time.Time `json:"-"`
	IsSelf          bool       `json:"isSelf"`
	RatingHint      string     `json:"ratingHint"`
	RatingCount     int        `json:"-"`
	ProjectMemberID int64      `json:"-"`
}

type ProjectMemberRatingResult struct {
	MemberID     int       `json:"memberId"`
	Score        float64   `json:"score"`
	CanRate      bool      `json:"canRate"`
	CooldownDays int       `json:"cooldownDays"`
	NextRateAt   time.Time `json:"-"`
	RatingCount  int       `json:"-"`
}
