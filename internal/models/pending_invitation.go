package models

import "time"

const PendingInvitationTypeSuperAdmin = "SUPER_ADMIN"

// PendingInvitation marks an invitation page that should be shown to a user.
type PendingInvitation struct {
	ID         int       `db:"id"`
	UserID     int       `db:"user_id"`
	InviteType string    `db:"invite_type"`
	ExpireAt   time.Time `db:"expire_at"`
	CreatedAt  time.Time `db:"created_at"`
	UpdatedAt  time.Time `db:"updated_at"`
}
