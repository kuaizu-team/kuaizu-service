package repository

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"strings"
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

func TestOperationsTeamAcademics(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	columns := []string{"talent_profile_id", "avatar_url", "name", "school_id", "school_name", "study_school_name", "major_name", "grade"}
	setCapturedQueryQueue(
		captureQueryResult{columns: columns, rows: [][]driver.Value{{int64(11), nil, "负责人", int64(1), "运营学校", "就读学校", "计算机", int64(2024)}}},
		captureQueryResult{columns: columns, rows: [][]driver.Value{{nil, nil, "成员", int64(1), "运营学校", nil, nil, nil}}},
	)
	teams, err := NewOperationsRepository(sqlx.NewDb(db, "capture_user_repo")).ListSchoolTeams(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	leader := teams[0].Leader
	if leader.SchoolName != "运营学校" || leader.StudySchoolName == nil || *leader.StudySchoolName != "就读学校" || leader.MajorName == nil || *leader.MajorName != "计算机" || leader.Grade == nil || *leader.Grade != 2024 {
		t.Fatalf("academics or operated school changed: %#v", leader)
	}
	member := teams[0].Members[0]
	if member.StudySchoolName != nil || member.MajorName != nil || member.Grade != nil || member.Position != "运营成员" || leader.Position != "运营负责人" {
		t.Fatalf("missing academics or roles changed: %#v", member)
	}
	payload, err := json.Marshal(member)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"studySchoolName":null`, `"majorName":null`, `"grade":null`} {
		if !strings.Contains(string(payload), field) {
			t.Fatalf("missing nullable field %s", field)
		}
	}
	queries, _ := capturedQueriesAndArgs()
	for _, query := range queries {
		for _, projection := range []string{"study_school.id = u.school_id", "m.id = u.major_id", "u.grade"} {
			if !strings.Contains(query, projection) {
				t.Fatalf("wrong academic source: %s", projection)
			}
		}
	}
}

func TestOperationsTeamPersonalTags(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value driver.Value
		want  string
	}{
		{"custom tags", `["领导力","摄影"]`, `"skills":["领导力","摄影"]`},
		{"empty tags", `[]`, `"skills":[]`},
		{"missing profile or tags", nil, `"skills":null`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := openCaptureDB(t)
			defer db.Close()
			columns := []string{"talent_profile_id", "avatar_url", "name", "school_id", "school_name", "skill_summary"}
			setCapturedQueryQueue(
				captureQueryResult{columns: columns, rows: [][]driver.Value{{int64(11), nil, "负责人", int64(1), "学校", tt.value}}},
				captureQueryResult{columns: columns, rows: [][]driver.Value{{int64(12), nil, "成员", int64(1), "学校", tt.value}}},
			)
			teams, err := NewOperationsRepository(sqlx.NewDb(db, "capture_user_repo")).ListSchoolTeams(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			for _, person := range []OperationsPerson{teams[0].Leader, teams[0].Members[0]} {
				payload, err := json.Marshal(person)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(payload), tt.want) || strings.Contains(string(payload), "skill_summary") || strings.Contains(string(payload), "SkillSummary") {
					t.Fatalf("unexpected public tags: %s", payload)
				}
			}
			queries, _ := capturedQueriesAndArgs()
			for _, query := range queries {
				if !strings.Contains(query, "tp.skill_summary") || !strings.Contains(query, "tp.status = 1") {
					t.Fatal("tags must come from the enabled public profile")
				}
			}
		})
	}
}
