package models

import (
	"time"

	"github.com/trv3wood/kuaizu-server/api"
)

// OliveBranch represents an olive branch record in the database
type OliveBranch struct {
	ID               int
	SenderID         int
	ReceiverID       int
	RelatedProjectID int
	Type             int // 1-人才互联, 2-项目邀请
	CostType         int // 1-免费额度, 2-付费额度
	Status           int // 0-待处理, 1-已接受, 2-已拒绝, 3-已忽略
	CreatedAt        time.Time
	UpdatedAt        time.Time

	// Joined fields
	Sender      *User
	Receiver    *User
	ProjectName *string
}

// ToVO converts OliveBranch to API OliveBranchVO
func (o *OliveBranch) ToVO() *api.OliveBranchVO {
	status := api.OliveBranchStatus(o.Status)

	vo := &api.OliveBranchVO{
		Id:               &o.ID,
		SenderId:         &o.SenderID,
		ReceiverId:       &o.ReceiverID,
		RelatedProjectId: &o.RelatedProjectID,
		Type:             &o.Type,
		CostType:         &o.CostType,
		Status:           &status,
		CreatedAt:        &o.CreatedAt,
		ProjectName:      o.ProjectName,
	}

	if o.Sender != nil {
		vo.Sender = o.Sender.ToVO()
	}
	if o.Receiver != nil {
		vo.Receiver = o.Receiver.ToVO()
	}

	return vo
}
