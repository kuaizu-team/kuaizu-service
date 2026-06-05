package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/stretchr/testify/mock"
)

func TestMarkDashboardViewedRejectsNonOwner(t *testing.T) {
	projectRepo := new(MockProjectRepo)
	projectRepo.On("GetByID", mock.Anything, 42).Return(&models.Project{ID: 42}, nil)
	projectRepo.On("IsOwner", mock.Anything, 42, 7).Return(false, nil)
	svc := NewInteractionService(&repository.Repository{Project: projectRepo})

	err := svc.MarkDashboardViewed(context.Background(), repository.InteractionProject, 42, 7, nil)
	var serviceErr *ServiceError
	if !errors.As(err, &serviceErr) || serviceErr.Code != ErrCodeForbidden {
		t.Fatalf("err = %#v, want forbidden", err)
	}
}

func TestTargetUnreadRejectsNonOwner(t *testing.T) {
	projectRepo := new(MockProjectRepo)
	projectRepo.On("GetByID", mock.Anything, 42).Return(&models.Project{ID: 42}, nil)
	projectRepo.On("IsOwner", mock.Anything, 42, 7).Return(false, nil)
	svc := NewInteractionService(&repository.Repository{Project: projectRepo})

	_, err := svc.TargetUnread(context.Background(), repository.InteractionProject, 42, 7)
	var serviceErr *ServiceError
	if !errors.As(err, &serviceErr) || serviceErr.Code != ErrCodeForbidden {
		t.Fatalf("err = %#v, want forbidden", err)
	}
}
