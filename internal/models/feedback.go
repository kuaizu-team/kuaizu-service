package models

import "time"

// Feedback represents a user feedback in the database
type Feedback struct {
	ID         int       `db:"id"`
	UserID     int       `db:"user_id"`
	Content    string    `db:"content"`
	Email      *string   `db:"email"`       // 用户联系邮箱（选填）
	Status     int       `db:"status"`      // 0=pending, 1=handled
	AdminReply *string   `db:"admin_reply"`
	CreatedAt  time.Time `db:"created_at"`
	UpdatedAt  time.Time `db:"updated_at"`

	// Joined fields
	UserNickname *string `db:"nickname"`
	UserSchoolID *int    `db:"user_school_id"` // 发起人学校ID（admin权限校验用）
}
