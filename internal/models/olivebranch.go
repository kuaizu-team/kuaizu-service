package models

import (
	"time"

	"github.com/kuaizu-team/kuaizu-service/api"
)

// OliveBranch represents an olive branch record in the database
type OliveBranch struct {
	ID               int       `db:"id"`
	SenderID         int       `db:"sender_id"`
	ReceiverID       int       `db:"receiver_id"`
	RelatedProjectID int       `db:"related_project_id"`
	Type             int       `db:"type"`      // 1-人才互联, 2-项目邀请
	CostType         int       `db:"cost_type"` // 1-免费额度, 2-付费额度
	Status           int       `db:"status"`    // 0-待处理, 1-已接受, 2-已拒绝, 3-已忽略
	IsRead           bool      `db:"is_read"`   // 接收方是否已读
	OperatorRole     *string   `db:"operator_role"`
	OperatorRoleName *string   `db:"operator_role_name"`
	CanReview        *bool     `db:"-"`
	AssignedRole     *string   `db:"assigned_role"`
	AssignedRoleName *string   `db:"assigned_role_name"`
	IsCurrentMember  *bool     `db:"is_current_member"`
	CreatedAt        time.Time `db:"created_at"`
	UpdatedAt        time.Time `db:"updated_at"`

	// Joined fields
	Sender      *User      `db:"-"`
	Receiver    *User      `db:"-"`
	ProjectName *string    `db:"project_name"`
	SmsNotice   *SmsNotice `db:"-"`
}

// ToVO converts OliveBranch to API OliveBranchVO
func (o *OliveBranch) ToVO() *api.OliveBranchVO {
	status := api.OliveBranchStatus(o.Status)

	vo := &api.OliveBranchVO{
		Id:               &o.ID,
		SenderId:         &o.SenderID,
		ReceiverId:       &o.ReceiverID,
		RelatedProjectId: &o.RelatedProjectID,
		CostType:         &o.CostType,
		Status:           &status,
		IsRead:           &o.IsRead,
		OperatorRole:     o.OperatorRole,
		OperatorRoleName: o.OperatorRoleName,
		CanReview:        o.CanReview,
		AssignedRole:     o.AssignedRole,
		AssignedRoleName: o.AssignedRoleName,
		IsCurrentMember:  o.IsCurrentMember,
		CreatedAt:        &o.CreatedAt,
		ProjectName:      o.ProjectName,
	}

	if o.Sender != nil {
		vo.Sender = o.Sender.ToVO()
	}
	if o.Receiver != nil {
		vo.Receiver = o.Receiver.ToVO()
	}
	if o.SmsNotice != nil {
		status := int(o.SmsNotice.Status)
		vo.SmsStatus = &status
		vo.SmsOrderId = &o.SmsNotice.OrderID
		vo.SmsRecordId = &o.SmsNotice.ID
		vo.SmsNotice = &api.SmsNoticeSummaryVO{
			Id:           &o.SmsNotice.ID,
			OrderId:      &o.SmsNotice.OrderID,
			Status:       &status,
			ErrorMessage: o.SmsNotice.ErrorMessage,
			CreatedAt:    &o.SmsNotice.CreatedAt,
			UpdatedAt:    &o.SmsNotice.UpdatedAt,
		}
	}

	return vo
}
