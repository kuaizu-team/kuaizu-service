package models

import (
	"encoding/json"
	"time"

	"github.com/trv3wood/kuaizu-server/api"
)

// TalentProfile represents a talent profile in the database
type TalentProfile struct {
	ID                int        `db:"id"`
	UserID            int        `db:"user_id"`
	SelfEvaluation    *string    `db:"self_evaluation"`
	SkillSummary      *string    `db:"skill_summary"` // JSON array stored as string
	ProjectExperience *string    `db:"project_experience"`
	MBTI              *string    `db:"mbti"`
	Status            *int       `db:"status"` // 0: 隐私/下架, 1: 上架, 2: 审核中
	CreatedAt         *time.Time `db:"created_at"`
	UpdatedAt         *time.Time `db:"updated_at"`

	// Joined fields from user table
	Nickname  *string `db:"nickname"`
	Phone     *string `db:"phone"`
	Email     *string `db:"email"`
	AvatarUrl *string `db:"avatar_url"`
	// SchoolID/MajorID are fetched from user table and used for follow-up lookups
	SchoolID *int `db:"school_id"`
	MajorID  *int `db:"major_id"`
	// Populated after follow-up queries
	SchoolName *string `db:"-"`
	MajorName  *string `db:"-"`
}

// parseSkills parses the skill_summary JSON string into a string slice
func (t *TalentProfile) parseSkills() *[]string {
	if t.SkillSummary == nil || *t.SkillSummary == "" {
		return nil
	}
	var skills []string
	if err := json.Unmarshal([]byte(*t.SkillSummary), &skills); err != nil {
		return nil
	}
	return &skills
}

// ToVO converts TalentProfile to API TalentProfileVO (list view)
func (t *TalentProfile) ToVO() *api.TalentProfileVO {
	return &api.TalentProfileVO{
		Id:         &t.ID,
		UserId:     &t.UserID,
		Nickname:   t.Nickname,
		SchoolName: t.SchoolName,
		MajorName:  t.MajorName,
		Mbti:       t.MBTI,
		Skills:     t.parseSkills(),
		Status:     (*api.TalentStatus)(t.Status),
		AvatarUrl:  ptrFullURL(t.AvatarUrl),
	}
}

// ToDetailVO converts TalentProfile to API TalentProfileDetailVO (detail view)
func (t *TalentProfile) ToDetailVO() *api.TalentProfileDetailVO {
	vo := &api.TalentProfileDetailVO{
		Id:                &t.ID,
		UserId:            &t.UserID,
		Nickname:          t.Nickname,
		SchoolName:        t.SchoolName,
		MajorName:         t.MajorName,
		Mbti:              t.MBTI,
		Skills:            t.parseSkills(),
		SelfEvaluation:    t.SelfEvaluation,
		ProjectExperience: t.ProjectExperience,
		Status:            (*api.TalentStatus)(t.Status),
		AvatarUrl:         ptrFullURL(t.AvatarUrl),
	}

	return vo
}
