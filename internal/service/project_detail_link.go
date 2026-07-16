package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/wechat"
)

const projectDetailPagePath = "/pages/project-detail/project-detail"

var ErrProjectDetailLinkNotFound = errors.New("project detail link project not found")

type projectDetailLookup interface {
	GetByID(ctx context.Context, id int) (*models.Project, error)
}

type projectDetailLinkKey struct {
	projectID int
	source    int
}

type projectDetailLinkEntry struct {
	url       string
	refreshAt time.Time
	expiresAt time.Time
}

// ProjectDetailLinkService keeps the public email URL stable while rotating
// project-specific WeChat URL Links before their 30-day expiration.
type ProjectDetailLinkService struct {
	generator urlLinkGenerator
	projects  projectDetailLookup
	mu        sync.Mutex
	links     map[projectDetailLinkKey]projectDetailLinkEntry
	now       func() time.Time
}

func NewProjectDetailLinkService(generator urlLinkGenerator, projects projectDetailLookup) *ProjectDetailLinkService {
	return &ProjectDetailLinkService{
		generator: generator,
		projects:  projects,
		links:     make(map[projectDetailLinkKey]projectDetailLinkEntry),
		now:       time.Now,
	}
}

func (s *ProjectDetailLinkService) URL(ctx context.Context, projectID, source int) (string, error) {
	if projectID <= 0 {
		return "", fmt.Errorf("project id must be positive")
	}
	if source != 2 {
		return "", fmt.Errorf("unsupported project link source: %d", source)
	}

	key := projectDetailLinkKey{projectID: projectID, source: source}
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	entry := s.links[key]
	if entry.url != "" && now.Before(entry.refreshAt) {
		return entry.url, nil
	}

	project, err := s.projects.GetByID(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("get project for URL Link: %w", err)
	}
	if project == nil {
		return "", ErrProjectDetailLinkNotFound
	}

	query := url.Values{}
	query.Set("id", strconv.Itoa(projectID))
	query.Set("source", strconv.Itoa(source))
	generatedURL, err := s.generator.GenerateURLLink(ctx, wechat.URLLinkRequest{
		Path:           projectDetailPagePath,
		Query:          query.Encode(),
		ExpireType:     1,
		ExpireInterval: 30,
		EnvVersion:     "release",
	})
	if err != nil {
		// A refresh failure must not break a still-valid cached link.
		if entry.url != "" && now.Before(entry.expiresAt) {
			return entry.url, nil
		}
		return "", err
	}

	entry = projectDetailLinkEntry{
		url:       generatedURL,
		refreshAt: now.Add(urlLinkRefreshAfter),
		expiresAt: now.Add(urlLinkValidity),
	}
	s.links[key] = entry
	return entry.url, nil
}
