package models

import "time"

const (
	StatusNotificationApplicationAccepted = "application-accepted"
	StatusNotificationApplicationRejected = "application-rejected"
	StatusNotificationOliveAccepted       = "olive-accepted"
	StatusNotificationOliveRejected       = "olive-rejected"
	StatusNotificationMemberRemoved       = "member-removed"
)

type StatusNotification struct {
	ID               int64      `db:"id" json:"id"`
	UserID           int        `db:"user_id" json:"-"`
	Type             string     `db:"type" json:"type"`
	ApplicationID    *int       `db:"application_id" json:"applicationId,omitempty"`
	OliveBranchID    *int       `db:"olive_branch_id" json:"oliveBranchId,omitempty"`
	MemberRemovalID  *int64     `db:"member_removal_id" json:"memberRemovalId,omitempty"`
	Priority         int        `db:"priority" json:"priority"`
	ProjectID        int        `db:"project_id" json:"projectId"`
	ProjectName      string     `db:"project_name" json:"projectName"`
	AppliedAt        time.Time  `db:"applied_at" json:"appliedAt"`
	DiscussingAt     *time.Time `db:"discussing_at" json:"discussingAt,omitempty"`
	RejectedAt       *time.Time `db:"rejected_at" json:"rejectedAt,omitempty"`
	JoinedAt         *time.Time `db:"joined_at" json:"joinedAt,omitempty"`
	ReviewerRoleName *string    `db:"reviewer_role_name" json:"reviewerRoleName,omitempty"`
	AssignedRoleName *string    `db:"assigned_role_name" json:"assignedRoleName,omitempty"`
	OperatorName     *string    `db:"operator_name" json:"operatorName,omitempty"`
	RemovedAt        *time.Time `db:"removed_at" json:"removedAt,omitempty"`
	CreatedAt        time.Time  `db:"created_at" json:"createdAt"`
}
