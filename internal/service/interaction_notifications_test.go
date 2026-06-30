package service

import (
	"context"
	"testing"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

func TestBuildProjectInteractionNotification(t *testing.T) {
	project := &models.Project{ID: 10, CreatorID: 100, Name: "project"}

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
			got, ok := buildProjectInteractionNotification(tt.kind, tt.operator, project, "user", 1)
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
			if got.pagePath != "pages/project-dashboard/project-dashboard?id=10" {
				t.Fatalf("pagePath = %q", got.pagePath)
			}
			if got.data["project_name"] != "project" || got.data[tt.wantField] != "user" || got.data["remark"] != subscribeInteractionRemark {
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
			got, ok := buildTalentInteractionNotification(tt.kind, tt.operator, profile, "user", 1)
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
			if got.pagePath != "pages/talent-dashboard/talent-dashboard?id=20" {
				t.Fatalf("pagePath = %q", got.pagePath)
			}
			if got.data[tt.wantField] != "user" || got.data["remark"] != subscribeInteractionRemark {
				t.Fatalf("unexpected data: %#v", got.data)
			}
		})
	}
}

func TestBuildVisitNotifications(t *testing.T) {
	project := &models.Project{ID: 10, CreatorID: 100, Name: "project"}
	if _, ok := buildProjectVisitNotification(0, project, "visitor", 3); ok {
		t.Fatal("anonymous project visit should not notify")
	}
	if _, ok := buildProjectVisitNotification(100, project, "owner", 3); ok {
		t.Fatal("owner project visit should not notify")
	}
	projectNotice, ok := buildProjectVisitNotification(200, project, "visitor", 3)
	if !ok {
		t.Fatal("visitor project visit should notify")
	}
	if projectNotice.ownerUserID != 100 || projectNotice.bizKey != models.MsgBizKeyProjectVisit {
		t.Fatalf("unexpected project notice: %#v", projectNotice)
	}
	if projectNotice.pagePath != "pages/project-dashboard/project-dashboard?id=10" {
		t.Fatalf("project pagePath = %q", projectNotice.pagePath)
	}
	if projectNotice.data["project_name"] != "project" || projectNotice.data["visit_user"] != "visitor等3人" {
		t.Fatalf("unexpected project data: %#v", projectNotice.data)
	}

	profile := &models.TalentProfile{ID: 20, UserID: 101}
	if _, ok := buildTalentVisitNotification(0, profile, "visitor", 3); ok {
		t.Fatal("anonymous talent visit should not notify")
	}
	if _, ok := buildTalentVisitNotification(101, profile, "owner", 3); ok {
		t.Fatal("owner talent visit should not notify")
	}
	talentNotice, ok := buildTalentVisitNotification(201, profile, "visitor", 3)
	if !ok {
		t.Fatal("visitor talent visit should notify")
	}
	if talentNotice.ownerUserID != 101 || talentNotice.bizKey != models.MsgBizKeyTalentVisit {
		t.Fatalf("unexpected talent notice: %#v", talentNotice)
	}
	if talentNotice.pagePath != "pages/talent-dashboard/talent-dashboard?id=20" {
		t.Fatalf("talent pagePath = %q", talentNotice.pagePath)
	}
	if talentNotice.data["visit_user"] != "visitor等3人" || talentNotice.data["remark"] != subscribeInteractionRemark {
		t.Fatalf("unexpected talent data: %#v", talentNotice.data)
	}
}

func TestNotificationUserNameFallback(t *testing.T) {
	if got := notificationUserName(nil); got != models.DefaultUserNickname {
		t.Fatalf("nil user name = %q", got)
	}
	blank := "  "
	if got := notificationUserName(&models.User{Nickname: &blank}); got != models.DefaultUserNickname {
		t.Fatalf("blank user name = %q", got)
	}
	name := "  user  "
	if got := notificationUserName(&models.User{Nickname: &name}); got != "user" {
		t.Fatalf("trimmed user name = %q", got)
	}
}

func TestNotificationGroupUserNameFitsTemplateLimit(t *testing.T) {
	got := notificationGroupUserName("abcdefghijklmnopqrstu", 123)
	if len([]rune(got)) > 20 {
		t.Fatalf("group user name exceeds 20 chars: %q", got)
	}
	if got != "abcdefghijkl...等123人" {
		t.Fatalf("group user name = %q", got)
	}
}

func TestShouldSendGroupedInteractionNotification(t *testing.T) {
	tests := []struct {
		name string
		in   repository.InteractionNotifyProgress
		want bool
	}{
		{name: "third distinct new user sends", in: repository.InteractionNotifyProgress{DistinctUserCount: 3, IsNewUser: true}, want: true},
		{name: "sixth distinct new user sends", in: repository.InteractionNotifyProgress{DistinctUserCount: 6, IsNewUser: true}, want: true},
		{name: "new user below threshold skips", in: repository.InteractionNotifyProgress{DistinctUserCount: 2, IsNewUser: true}},
		{name: "new user non multiple skips", in: repository.InteractionNotifyProgress{DistinctUserCount: 4, IsNewUser: true}},
		{name: "repeat user skips even on threshold", in: repository.InteractionNotifyProgress{DistinctUserCount: 3, IsNewUser: false}},
		{name: "zero count skips", in: repository.InteractionNotifyProgress{DistinctUserCount: 0, IsNewUser: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldSendGroupedInteractionNotification(tt.in); got != tt.want {
				t.Fatalf("shouldSendGroupedInteractionNotification(%#v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestNotificationInteractionUserNameGroupingRules(t *testing.T) {
	grouped := notificationGroupUserName("alice", 3)
	if got := notificationInteractionUserName("like", "alice", 3); got != grouped {
		t.Fatalf("like grouped name = %q, want %q", got, grouped)
	}
	if got := notificationInteractionUserName("share", "alice", 6); got != notificationGroupUserName("alice", 6) {
		t.Fatalf("share grouped name = %q", got)
	}
	if got := notificationInteractionUserName("like", "alice", 2); got != "alice" {
		t.Fatalf("like below threshold name = %q", got)
	}
	if got := notificationInteractionUserName("favorite", "alice", 3); got != "alice" {
		t.Fatalf("favorite should stay immediate single-user name, got %q", got)
	}
}

type fakeSubscribeSender struct {
	userID   int
	bizKey   string
	data     map[string]string
	pagePath string
	usedPage bool
}

func (f *fakeSubscribeSender) SendSubscribeMsgByBizKey(ctx context.Context, userID int, bizKey string, businessData map[string]string) error {
	f.userID = userID
	f.bizKey = bizKey
	f.data = businessData
	return nil
}

func (f *fakeSubscribeSender) SendSubscribeMsgByBizKeyWithPage(ctx context.Context, userID int, bizKey string, businessData map[string]string, pagePath string) error {
	f.userID = userID
	f.bizKey = bizKey
	f.data = businessData
	f.pagePath = pagePath
	f.usedPage = true
	return nil
}

func TestSendSubscribeNotificationUsesDynamicPagePath(t *testing.T) {
	sender := &fakeSubscribeSender{}
	notification := subscribeNotification{
		ownerUserID: 100,
		bizKey:      models.MsgBizKeyProjectLike,
		data:        map[string]string{"project_name": "project"},
		pagePath:    "pages/project-dashboard/project-dashboard?id=10",
	}

	sendSubscribeNotification(context.Background(), sender, notification)

	if !sender.usedPage {
		t.Fatal("expected dynamic page sender to be used")
	}
	if sender.userID != notification.ownerUserID || sender.bizKey != notification.bizKey || sender.pagePath != notification.pagePath {
		t.Fatalf("unexpected send: %#v", sender)
	}
}
