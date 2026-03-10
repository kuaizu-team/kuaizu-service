package models

import (
	"encoding/json"
	"time"

	"github.com/trv3wood/kuaizu-server/api"
)

// TalentProfile represents a talent profile in the database
type TalentProfile struct {
	ID                int       `db:"id"`
	UserID            int       `db:"user_id"`
	SelfEvaluation    *string   `db:"self_evaluation"`
	SkillSummary      *string   `db:"skill_summary"` // JSON array stored as string
	ProjectExperience *string   `db:"project_experience"`
	MBTI              *string   `db:"mbti"`
	Status            int       `db:"status"` // 0: 下架, 1: 上架
	IsPublicContact   bool      `db:"is_public_contact"`
	CreatedAt         time.Time `db:"created_at"`
	UpdatedAt         time.Time `db:"updated_at"`

	// Joined fields from user table
	Nickname   *string `db:"nickname"`
	SchoolName *string `db:"school_name"`
	MajorName  *string `db:"major_name"`
	Phone      *string `db:"phone"`
	Email      *string `db:"email"`
	AvatarUrl  *string `db:"avatar_url"`
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
	status := api.TalentStatus(t.Status)
	return &api.TalentProfileVO{
		Id:              &t.ID,
		UserId:          &t.UserID,
		Nickname:        t.Nickname,
		SchoolName:      t.SchoolName,
		MajorName:       t.MajorName,
		Mbti:            t.MBTI,
		Skills:          t.parseSkills(),
		IsPublicContact: &t.IsPublicContact,
		Status:          &status,
		AvatarUrl:       ptrFullURL(t.AvatarUrl),
	}
}

// ToDetailVO converts TalentProfile to API TalentProfileDetailVO (detail view)
func (t *TalentProfile) ToDetailVO(showContact bool) *api.TalentProfileDetailVO {
	status := api.TalentStatus(t.Status)
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
		IsPublicContact:   &t.IsPublicContact,
		Status:            &status,
		AvatarUrl:         ptrFullURL(t.AvatarUrl),
	}

	// Only show contact info if allowed
	if showContact && t.IsPublicContact {
		if t.Phone != nil {
			vo.Contact = t.Phone
		} else if t.Email != nil {
			vo.Contact = t.Email
		}
	}

	return vo
}
