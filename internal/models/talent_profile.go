package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kuaizu-team/kuaizu-service/api"
)

type JSONStringArray struct {
	Items []string
	Valid bool
}

func (a *JSONStringArray) Scan(value interface{}) error {
	if value == nil {
		a.Items = nil
		a.Valid = false
		return nil
	}

	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("scan JSONStringArray from %T", value)
	}

	var items []string
	if err := json.Unmarshal(data, &items); err != nil {
		return fmt.Errorf("unmarshal JSONStringArray: %w", err)
	}

	a.Items = items
	a.Valid = true
	return nil
}

func (a JSONStringArray) Value() (driver.Value, error) {
	if !a.Valid {
		return nil, nil
	}

	data, err := json.Marshal(a.Items)
	if err != nil {
		return nil, fmt.Errorf("marshal JSONStringArray: %w", err)
	}

	return data, nil
}

// TalentProfile represents a talent profile in the database
type TalentProfile struct {
	ID                int             `db:"id"`
	UserID            int             `db:"user_id"`
	SelfEvaluation    *string         `db:"self_evaluation"`
	SkillSummary      JSONStringArray `db:"skill_summary"`
	ProjectExperience *string         `db:"project_experience"`
	MBTI              *string         `db:"mbti"`
	Status            *int            `db:"status"` // 0: 隐私/下架, 1: 上架, 2: 审核中
	CreatedAt         *time.Time      `db:"created_at"`
	UpdatedAt         *time.Time      `db:"updated_at"`

	// Joined fields from user table
	Nickname   *string `db:"nickname"`
	Phone      *string `db:"phone"`
	Email      *string `db:"email"`
	WechatID   *string `db:"wechat_id"`
	AvatarUrl  *string `db:"avatar_url"`
	Grade      *int    `db:"grade"`
	AuthStatus *int    `db:"auth_status"`
	// SchoolID/MajorID are fetched from user table and used for follow-up lookups
	SchoolID *int `db:"school_id"`
	MajorID  *int `db:"major_id"`
	// Populated after follow-up queries
	SchoolName *string `db:"-"`
	MajorName  *string `db:"-"`
}

func (t *TalentProfile) skills() *[]string {
	if !t.SkillSummary.Valid {
		return nil
	}

	skills := append([]string(nil), t.SkillSummary.Items...)
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
		Skills:     t.skills(),
		Status:     (*api.TalentStatus)(t.Status),
		AvatarUrl:  ptrFullURL(t.AvatarUrl),
		AuthStatus: t.AuthStatus,
		Grade:      t.Grade,
	}
}

// ptrStr returns a pointer to s. Used to ensure optional string fields are
// always present in the JSON response (as "" rather than absent).
func ptrStr(s string) *string { return &s }

// ToDetailVO converts TalentProfile to API TalentProfileDetailVO (detail view).
// All required display fields are guaranteed to be non-nil so the frontend
// always receives them (even as zero values) without relying on omitempty absence.
func (t *TalentProfile) ToDetailVO() *api.TalentProfileDetailVO {
	// skills: always return an array, never omit the key
	skills := t.skills()
	if skills == nil {
		empty := []string{}
		skills = &empty
	}

	// string fields: return "" instead of absent when user hasn't filled them in
	mbti := t.MBTI
	if mbti == nil {
		mbti = ptrStr("")
	}
	selfEval := t.SelfEvaluation
	if selfEval == nil {
		selfEval = ptrStr("")
	}
	projExp := t.ProjectExperience
	if projExp == nil {
		projExp = ptrStr("")
	}

	return &api.TalentProfileDetailVO{
		Id:                &t.ID,
		UserId:            &t.UserID,
		Nickname:          t.Nickname,
		SchoolName:        t.SchoolName,
		MajorName:         t.MajorName,
		Mbti:              mbti,
		Skills:            skills,
		SelfEvaluation:    selfEval,
		ProjectExperience: projExp,
		Status:            (*api.TalentStatus)(t.Status),
		AvatarUrl:         ptrFullURL(t.AvatarUrl),
		Email:             t.Email,
		Phone:             t.Phone,
		Wechat:            t.WechatID,
		Grade:             t.Grade,
		AuthStatus:        t.AuthStatus,
	}
}
