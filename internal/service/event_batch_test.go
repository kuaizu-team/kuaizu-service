package service

import (
	"context"
	"errors"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"testing"
)

type batchEventRepo struct {
	repository.EventRepo
	ids              []int
	reads            int
	pvErr, reloadErr bool
}

func (r *batchEventRepo) GetByID(context.Context, int) (*models.Event, error) {
	r.reads++
	if r.reads > 1 && r.reloadErr {
		return nil, errors.New("statistics unavailable")
	}
	return &models.Event{ID: 1, Name: "event"}, nil
}
func (r *batchEventRepo) ListTimelineNodes(context.Context, int) ([]models.EventTimelineNode, error) {
	return nil, nil
}
func (r *batchEventRepo) ListProjectIDs(context.Context, int) ([]int, error) { return r.ids, nil }
func (r *batchEventRepo) IncrementViewCount(context.Context, int) error {
	if r.pvErr {
		return errors.New("statistics unavailable")
	}
	return nil
}

type batchEventProjectRepo struct {
	repository.ProjectRepo
	calls int
	rows  []models.Project
}

func (r *batchEventProjectRepo) List(_ context.Context, p repository.ListParams) ([]models.Project, int64, error) {
	r.calls++
	if p.EventID == nil || *p.EventID != 1 || p.Status == nil || *p.Status != models.ProjectStatusApproved || p.Size != 120 || p.Page != 1 {
		return nil, 0, errors.New("wrong batch filter")
	}
	return r.rows, int64(len(r.rows)), nil
}
func TestEventBatchPreservesOrderBeyondOneHundred(t *testing.T) {
	for _, phase := range []string{"normal", "pv failure", "reload failure"} {
		t.Run(phase, func(t *testing.T) {
			e := &batchEventRepo{pvErr: phase == "pv failure", reloadErr: phase == "reload failure"}
			p := &batchEventProjectRepo{}
			for i := 1; i <= 120; i++ {
				e.ids = append(e.ids, i)
				p.rows = append(p.rows, models.Project{ID: 121 - i, Status: models.ProjectStatusApproved})
			}
			s := &EventService{repo: &repository.Repository{Event: e, Project: p}}
			event, rows, _, err := s.GetEvent(context.Background(), 1)
			if err != nil || event == nil || len(rows) != 120 || p.calls != 1 {
				t.Fatalf("rows=%d calls=%d err=%v", len(rows), p.calls, err)
			}
			for i, row := range rows {
				if row.ID != i+1 {
					t.Fatal("association order changed")
				}
			}
		})
	}
}
