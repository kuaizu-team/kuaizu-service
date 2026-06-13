package models

import "time"

const (
	InvitationRecordStatusSent   = "SENT"
	InvitationRecordStatusFailed = "FAILED"
	InvitationRecordStatusJoined = "JOINED"
)

type InvitationRecord struct {
	ID            int        `db:"id"`
	Phone         string     `db:"phone"`
	ProjectID     int        `db:"project_id"`
	InviterUserID int        `db:"inviter_user_id"`
	Role          string     `db:"role"`
	Status        string     `db:"status"`
	Provider      *string    `db:"provider"`
	ProviderBizID *string    `db:"provider_biz_id"`
	ErrorMessage  *string    `db:"error_message"`
	SentAt        *time.Time `db:"sent_at"`
	RegisteredAt  *time.Time `db:"registered_at"`
	JoinedAt      *time.Time `db:"joined_at"`
	CreatedAt     time.Time  `db:"created_at"`
	UpdatedAt     time.Time  `db:"updated_at"`
}
