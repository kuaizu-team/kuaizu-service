package models

import "time"

import "github.com/kuaizu-team/kuaizu-service/api"

type Interaction struct {
	Liked         bool  `db:"liked" json:"liked"`
	Favorited     bool  `db:"favorited" json:"favorited"`
	LikeCount     int   `db:"like_count" json:"likeCount"`
	FavoriteCount int   `db:"favorite_count" json:"favoriteCount"`
	ShareCount    int   `db:"share_count" json:"shareCount"`
	Active        *bool `db:"-" json:"active,omitempty"`
}

func (i Interaction) ToVO() *api.InteractionVO {
	return &api.InteractionVO{
		Liked: i.Liked, Favorited: i.Favorited, LikeCount: i.LikeCount,
		FavoriteCount: i.FavoriteCount, ShareCount: i.ShareCount,
	}
}

type InteractionUser struct {
	UserID          int       `db:"user_id" json:"userId"`
	TalentProfileID *int      `db:"talent_profile_id" json:"talentProfileId"`
	Nickname        *string   `db:"nickname" json:"nickname"`
	AvatarURL       *string   `db:"avatar_url" json:"avatarUrl"`
	OperatedAt      time.Time `db:"operated_at" json:"operatedAt"`
}

type FavoriteViewState struct {
	Projects       int `json:"projects"`
	TalentProfiles int `json:"talentProfiles"`
	Total          int `json:"total"`
}
