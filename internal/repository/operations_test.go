package repository

import (
	"context"
	"database/sql/driver"
	"testing"

	"github.com/jmoiron/sqlx"
)

func TestOperationsTeamAvatarURLs(t *testing.T) {
	t.Setenv("OSS_DOMAIN", "https://cdn.example.com")
	t.Setenv("OSS_BASE_PATH", "kuaizu_text_img")
	db := openCaptureDB(t)
	defer db.Close()
	columns := []string{"talent_profile_id", "avatar_url", "name", "school_id", "school_name"}
	setCapturedQueryQueue(
		captureQueryResult{columns: columns, rows: [][]driver.Value{{int64(11), "2026/07/24/leader.jpg", "许思扬", int64(1), "测试学校"}}},
		captureQueryResult{columns: columns, rows: [][]driver.Value{
			{int64(12), "2026/07/24/member.jpg", "张三", int64(1), "测试学校"},
			{int64(13), "https://img.example.com/avatar.jpg", "李四", int64(1), "测试学校"},
			{int64(14), nil, "王五", int64(1), "测试学校"},
			{nil, nil, "赵六", int64(1), "测试学校"},
		}},
	)
	teams, err := NewOperationsRepository(sqlx.NewDb(db, "capture_user_repo")).ListSchoolTeams(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(teams) != 1 || len(teams[0].Members) != 4 {
		t.Fatalf("unexpected teams: %#v", teams)
	}
	team := teams[0]
	if team.Leader.AvatarURL == nil || *team.Leader.AvatarURL != "https://cdn.example.com/kuaizu_text_img/2026/07/24/leader.jpg" {
		t.Fatalf("leader avatar: %#v", team.Leader)
	}
	if team.Members[0].AvatarURL == nil || *team.Members[0].AvatarURL != "https://cdn.example.com/kuaizu_text_img/2026/07/24/member.jpg" {
		t.Fatalf("member avatar: %#v", team.Members[0])
	}
	if team.Members[1].AvatarURL == nil || *team.Members[1].AvatarURL != "https://img.example.com/avatar.jpg" {
		t.Fatal("absolute URL changed")
	}
	if team.Members[2].AvatarURL != nil || team.Members[3].AvatarURL != nil || team.Members[3].TalentProfileID != nil {
		t.Fatal("fallback data changed")
	}
	if team.Leader.TalentProfileID == nil || *team.Leader.TalentProfileID != 11 || team.Members[0].Position != "运营成员" {
		t.Fatal("profile or hierarchy changed")
	}
}
