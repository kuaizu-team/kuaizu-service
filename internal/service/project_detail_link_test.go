package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/wechat"
)

type projectURLLinkGenerator struct {
	calls  int
	inputs []wechat.URLLinkRequest
	err    error
}

func (f *projectURLLinkGenerator) GenerateURLLink(_ context.Context, input wechat.URLLinkRequest) (string, error) {
	f.calls++
	f.inputs = append(f.inputs, input)
	if f.err != nil {
		return "", f.err
	}
	return "https://wxaurl.cn/project-ticket-" + input.Query, nil
}

type projectDetailLookupStub struct {
	projects map[int]*models.Project
	err      error
}

func (s *projectDetailLookupStub) GetByID(_ context.Context, id int) (*models.Project, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.projects[id], nil
}

func existingProjectLookup(ids ...int) *projectDetailLookupStub {
	projects := make(map[int]*models.Project, len(ids))
	for _, id := range ids {
		projects[id] = &models.Project{ID: id}
	}
	return &projectDetailLookupStub{projects: projects}
}

func TestProjectDetailLinkCachesEachProjectURL(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	generator := &projectURLLinkGenerator{}
	service := NewProjectDetailLinkService(generator, existingProjectLookup(402, 403))
	service.now = func() time.Time { return now }

	first, err := service.URL(context.Background(), 402, 2)
	if err != nil {
		t.Fatalf("first URL: %v", err)
	}
	second, err := service.URL(context.Background(), 402, 2)
	if err != nil {
		t.Fatalf("second URL: %v", err)
	}
	other, err := service.URL(context.Background(), 403, 2)
	if err != nil {
		t.Fatalf("other URL: %v", err)
	}

	if first != second {
		t.Fatalf("cached URLs differ: %q != %q", first, second)
	}
	if first == other {
		t.Fatalf("different projects share URL: %q", first)
	}
	if generator.calls != 2 {
		t.Fatalf("generator calls = %d, want 2", generator.calls)
	}
	if got := generator.inputs[0]; got.Path != projectDetailPagePath || got.Query != "id=402&source=2" || got.ExpireInterval != 30 || got.EnvVersion != "release" {
		t.Fatalf("unexpected first input: %#v", got)
	}
}

func TestProjectDetailLinkUsesValidCacheWhenRefreshFails(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	generator := &projectURLLinkGenerator{}
	service := NewProjectDetailLinkService(generator, existingProjectLookup(402))
	service.now = func() time.Time { return now }

	want, err := service.URL(context.Background(), 402, 2)
	if err != nil {
		t.Fatalf("initial URL: %v", err)
	}
	now = now.Add(urlLinkRefreshAfter + time.Hour)
	generator.err = errors.New("wechat unavailable")

	got, err := service.URL(context.Background(), 402, 2)
	if err != nil {
		t.Fatalf("cached URL after refresh failure: %v", err)
	}
	if got != want {
		t.Fatalf("cached URL = %q, want %q", got, want)
	}
}

func TestProjectDetailLinkRejectsMissingProjectAndUnsupportedSource(t *testing.T) {
	generator := &projectURLLinkGenerator{}
	service := NewProjectDetailLinkService(generator, existingProjectLookup())

	if _, err := service.URL(context.Background(), 0, 2); err == nil {
		t.Fatal("expected invalid project id error")
	}
	if _, err := service.URL(context.Background(), 402, 3); err == nil {
		t.Fatal("expected unsupported source error")
	}
	if _, err := service.URL(context.Background(), 402, 2); !errors.Is(err, ErrProjectDetailLinkNotFound) {
		t.Fatalf("missing project error = %v", err)
	}
	if generator.calls != 0 {
		t.Fatalf("generator calls = %d, want 0", generator.calls)
	}
}
