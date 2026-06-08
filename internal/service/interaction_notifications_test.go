package service

import (
	"testing"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

func TestBuildProjectInteractionNotification(t *testing.T) {
	project := &models.Project{ID: 10, CreatorID: 100, Name: "一种悬浮式盲道创新设计方案"}

	tests := []struct {
		name      string
		kind      string
		operator  int
		wantOK    bool
		wantBiz   string
		wantField string
	}{
		{name: "self like skipped", kind: "like", operator: 100, wantOK: false},
		{name: "like", kind: "like", operator: 200, wantOK: true, wantBiz: models.MsgBizKeyProjectLike, wantField: "like_user"},
		{name: "favorite", kind: "favorite", operator: 200, wantOK: true, wantBiz: models.MsgBizKeyProjectFavorite, wantField: "favorite_user"},
		{name: "share", kind: "share", operator: 200, wantOK: true, wantBiz: models.MsgBizKeyProjectShare, wantField: "share_user"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := buildProjectInteractionNotification(tt.kind, tt.operator, project, "小明")
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if got.ownerUserID != project.CreatorID {
				t.Fatalf("ownerUserID = %d, want %d", got.ownerUserID, project.CreatorID)
			}
			if got.bizKey != tt.wantBiz {
				t.Fatalf("bizKey = %q, want %q", got.bizKey, tt.wantBiz)
			}
			if got.data["project_name"] == "" || got.data[tt.wantField] != "小明" || got.data["remark"] != subscribeInteractionRemark {
				t.Fatalf("unexpected data: %#v", got.data)
			}
		})
	}
}

func TestBuildTalentInteractionNotification(t *testing.T) {
	profile := &models.TalentProfile{ID: 20, UserID: 101}

	tests := []struct {
		name      string
		kind      string
		operator  int
		wantOK    bool
		wantBiz   string
		wantField string
	}{
		{name: "self like skipped", kind: "like", operator: 101, wantOK: false},
		{name: "like", kind: "like", operator: 201, wantOK: true, wantBiz: models.MsgBizKeyTalentLike, wantField: "like_user"},
		{name: "favorite", kind: "favorite", operator: 201, wantOK: true, wantBiz: models.MsgBizKeyTalentFavorite, wantField: "favorite_user"},
		{name: "share", kind: "share", operator: 201, wantOK: true, wantBiz: models.MsgBizKeyTalentShare, wantField: "share_user"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := buildTalentInteractionNotification(tt.kind, tt.operator, profile, "王明")
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if got.ownerUserID != profile.UserID {
				t.Fatalf("ownerUserID = %d, want %d", got.ownerUserID, profile.UserID)
			}
			if got.bizKey != tt.wantBiz {
				t.Fatalf("bizKey = %q, want %q", got.bizKey, tt.wantBiz)
			}
			if got.data[tt.wantField] != "王明" || got.data["remark"] != subscribeInteractionRemark {
				t.Fatalf("unexpected data: %#v", got.data)
			}
		})
	}
}

func TestBuildVisitNotifications(t *testing.T) {
	project := &models.Project{ID: 10, CreatorID: 100, Name: "项目"}
	if _, ok := buildProjectVisitNotification(0, project, "游客"); ok {
		t.Fatal("anonymous project visit should not notify")
	}
	if _, ok := buildProjectVisitNotification(100, project, "本人"); ok {
		t.Fatal("owner project visit should not notify")
	}
	projectNotice, ok := buildProjectVisitNotification(200, project, "访客")
	if !ok {
		t.Fatal("visitor project visit should notify")
	}
	if projectNotice.ownerUserID != 100 || projectNotice.bizKey != models.MsgBizKeyProjectVisit {
		t.Fatalf("unexpected project notice: %#v", projectNotice)
	}
	if projectNotice.data["project_name"] != "项目" || projectNotice.data["visit_user"] != "访客" {
		t.Fatalf("unexpected project data: %#v", projectNotice.data)
	}

	profile := &models.TalentProfile{ID: 20, UserID: 101}
	if _, ok := buildTalentVisitNotification(0, profile, "游客"); ok {
		t.Fatal("anonymous talent visit should not notify")
	}
	if _, ok := buildTalentVisitNotification(101, profile, "本人"); ok {
		t.Fatal("owner talent visit should not notify")
	}
	talentNotice, ok := buildTalentVisitNotification(201, profile, "访客")
	if !ok {
		t.Fatal("visitor talent visit should notify")
	}
	if talentNotice.ownerUserID != 101 || talentNotice.bizKey != models.MsgBizKeyTalentVisit {
		t.Fatalf("unexpected talent notice: %#v", talentNotice)
	}
	if talentNotice.data["visit_user"] != "访客" || talentNotice.data["remark"] != subscribeInteractionRemark {
		t.Fatalf("unexpected talent data: %#v", talentNotice.data)
	}
}

func TestNotificationUserNameFallback(t *testing.T) {
	if got := notificationUserName(nil); got != "用户" {
		t.Fatalf("nil user name = %q", got)
	}
	blank := "  "
	if got := notificationUserName(&models.User{Nickname: &blank}); got != "用户" {
		t.Fatalf("blank user name = %q", got)
	}
	name := "  小明  "
	if got := notificationUserName(&models.User{Nickname: &name}); got != "小明" {
		t.Fatalf("trimmed user name = %q", got)
	}
}
