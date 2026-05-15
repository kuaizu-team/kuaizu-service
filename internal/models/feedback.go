package models

import "time"

// Feedback represents a user feedback in the database
type Feedback struct {
	ID           int       `db:"id"`
	UserID       int       `db:"user_id"`
	Content      string    `db:"content"`
	ContactImage *string   `db:"contact_image"`
	Email        *string   `db:"email"`        // 用户联系邮箱（选填）
	Status       int       `db:"status"`       // 0=pending, 1=handled
	AdminReply   *string   `db:"admin_reply"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`

	// Joined fields
	UserNickname *string `db:"nickname"`
}
