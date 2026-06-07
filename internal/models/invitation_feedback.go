package models

import "time"

const (
	InvitationFeedbackStatusPending       = "pending"
	InvitationFeedbackStatusInterested    = "interested"
	InvitationFeedbackStatusNotInterested = "not_interested"
)

const (
	InvitationConversationStatusInProgress = "in_progress"
	InvitationConversationStatusAccepted   = "accepted"
	InvitationConversationStatusRejected   = "rejected"
)

// InvitationFeedback tracks a user's super-admin invitation feedback.
type InvitationFeedback struct {
	ID                 int        `db:"id"`
	UserID             int        `db:"user_id"`
	Status             string     `db:"status"`
	IntentionText      *string    `db:"intention_text"`
	ConversationStatus *string    `db:"conversation_status"`
	CreatedAt          *time.Time `db:"created_at"`
	UpdatedAt          *time.Time `db:"updated_at"`
}
