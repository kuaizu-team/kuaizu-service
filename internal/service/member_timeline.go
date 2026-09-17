package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jmoiron/sqlx"
)

type TimelineRelatedMember struct {
	UserID   int    `json:"userId"`
	Nickname string `json:"nickname"`
	HasLeft  bool   `json:"hasLeft"`
}

type MemberTimelineInput struct {
	Title          string `json:"title"`
	Detail         string `json:"detail"`
	RelatedUserIDs []int  `json:"relatedUserIds"`
}

type MemberTimelineNode struct {
	ID             int                     `db:"id" json:"id"`
	ProjectID      int                     `db:"project_id" json:"projectId"`
	UserID         int                     `db:"user_id" json:"userId"`
	Title          string                  `db:"title" json:"title"`
	Detail         string                  `db:"detail" json:"detail"`
	RelatedJSON    string                  `db:"related_members" json:"-"`
	RelatedMembers []TimelineRelatedMember `db:"-" json:"relatedMembers"`
	CreatedAt      time.Time               `db:"created_at" json:"createdAt"`
	UpdatedAt      time.Time               `db:"updated_at" json:"updatedAt"`
}

func validateMemberTimelineInput(input *MemberTimelineInput, userID int) error {
	input.Title = strings.TrimSpace(input.Title)
	input.Detail = strings.TrimSpace(input.Detail)
	if input.Title == "" || utf8.RuneCountInString(input.Title) > 60 || utf8.RuneCountInString(input.Detail) > 5000 {
		return ErrBadRequest("标题须为1至60字，详情最多5000字")
	}
	if len(input.RelatedUserIDs) > 100 {
		return ErrBadRequest("最多关联100位成员")
	}
	seen := map[int]bool{}
	for _, id := range input.RelatedUserIDs {
		if id <= 0 || id == userID || seen[id] {
			return ErrBadRequest("关联成员无效或重复")
		}
		seen[id] = true
	}
	return nil
}

// Lock the active membership, preventing a concurrent removal from authorizing a write.
func lockTimelineMember(ctx context.Context, tx *sqlx.Tx, projectID, userID int) error {
	if projectID <= 0 || userID <= 0 {
		return ErrForbidden("仅当前团队成员可访问时间线")
	}
	var id int
	err := tx.GetContext(ctx, &id, `SELECT id FROM project_members WHERE project_id=? AND user_id=? FOR UPDATE`, projectID, userID)
	if err == sql.ErrNoRows {
		return ErrForbidden("仅当前团队成员可访问时间线")
	}
	return err
}

func (s *ProjectService) ListMemberTimeline(ctx context.Context, projectID, viewerID, memberID int) ([]MemberTimelineNode, error) {
	tx, err := s.repo.DB().BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = lockTimelineMember(ctx, tx, projectID, viewerID); err != nil {
		return nil, err
	}
	if memberID < 0 {
		return nil, ErrBadRequest("成员ID无效")
	}
	var nodes = make([]MemberTimelineNode, 0)
	// Only current members are listed. Rejoining restores history by project + user.
	err = tx.SelectContext(ctx, &nodes, `SELECT n.* FROM project_member_timeline n
		JOIN project_members m ON m.project_id=n.project_id AND m.user_id=n.user_id
		WHERE n.project_id=? AND (?=0 OR n.user_id=?) ORDER BY n.created_at DESC,n.id DESC`, projectID, memberID, memberID)
	if err != nil {
		return nil, err
	}
	var activeIDs []int
	if err = tx.SelectContext(ctx, &activeIDs, "SELECT user_id FROM project_members WHERE project_id=?", projectID); err != nil {
		return nil, err
	}
	active := map[int]bool{}
	for _, id := range activeIDs {
		active[id] = true
	}
	for i := range nodes {
		nodes[i].RelatedMembers = []TimelineRelatedMember{}
		if err = json.Unmarshal([]byte(nodes[i].RelatedJSON), &nodes[i].RelatedMembers); err != nil {
			return nil, err
		}
	}
	for i := range nodes {
		for j := range nodes[i].RelatedMembers {
			nodes[i].RelatedMembers[j].HasLeft = !active[nodes[i].RelatedMembers[j].UserID]
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return nodes, nil
}

func (s *ProjectService) SaveMemberTimeline(ctx context.Context, projectID, userID, nodeID int, input MemberTimelineInput) (int, error) {
	if nodeID < 0 {
		return 0, ErrBadRequest("节点ID无效")
	}
	if err := validateMemberTimelineInput(&input, userID); err != nil {
		return 0, err
	}
	tx, err := s.repo.DB().BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if err = lockTimelineMember(ctx, tx, projectID, userID); err != nil {
		return 0, err
	}
	previous := map[int]TimelineRelatedMember{}
	if nodeID > 0 {
		var stored string
		err = tx.GetContext(ctx, &stored, `SELECT related_members FROM project_member_timeline WHERE id=? AND project_id=? AND user_id=? FOR UPDATE`, nodeID, projectID, userID)
		if err == sql.ErrNoRows {
			return 0, ErrForbidden("仅可修改自己的时间节点")
		}
		if err != nil {
			return 0, err
		}
		var old []TimelineRelatedMember
		if err = json.Unmarshal([]byte(stored), &old); err != nil {
			return 0, err
		}
		for _, member := range old {
			previous[member.UserID] = member
		}
	}
	related := make([]TimelineRelatedMember, 0, len(input.RelatedUserIDs))
	for _, id := range input.RelatedUserIDs {
		var nickname string
		err = tx.GetContext(ctx, &nickname, `SELECT COALESCE(u.nickname,'快组儿') FROM project_members m JOIN user u ON u.id=m.user_id WHERE m.project_id=? AND m.user_id=? LOCK IN SHARE MODE`, projectID, id)
		if err == sql.ErrNoRows {
			if old, exists := previous[id]; exists {
				old.HasLeft = false
				related = append(related, old)
				continue
			}
			return 0, ErrBadRequest("关联成员必须是当前团队中的其他成员，请刷新后重试")
		}
		if err != nil {
			return 0, err
		}
		related = append(related, TimelineRelatedMember{UserID: id, Nickname: nickname})
	}
	encoded, err := json.Marshal(related)
	if err != nil {
		return 0, err
	}
	if nodeID == 0 {
		result, insertErr := tx.ExecContext(ctx, `INSERT INTO project_member_timeline(project_id,user_id,title,detail,related_members) VALUES(?,?,?,?,?)`, projectID, userID, input.Title, input.Detail, string(encoded))
		if insertErr != nil {
			return 0, insertErr
		}
		id, idErr := result.LastInsertId()
		if idErr != nil {
			return 0, idErr
		}
		nodeID = int(id)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE project_member_timeline SET title=?,detail=?,related_members=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND project_id=? AND user_id=?`, input.Title, input.Detail, string(encoded), nodeID, projectID, userID)
		if err != nil {
			return 0, err
		}
	}
	return nodeID, tx.Commit()
}

func (s *ProjectService) DeleteMemberTimeline(ctx context.Context, projectID, userID, nodeID int) error {
	if nodeID <= 0 {
		return ErrBadRequest("节点ID无效")
	}
	tx, err := s.repo.DB().BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lockTimelineMember(ctx, tx, projectID, userID); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM project_member_timeline WHERE id=? AND project_id=? AND user_id=?`, nodeID, projectID, userID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrForbidden("仅可删除自己的时间节点")
	}
	return tx.Commit()
}
