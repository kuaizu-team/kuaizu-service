package service

import (
	"context"
	"testing"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/messagecenter"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSmsNoticeSubmitter struct {
	resp *messagecenter.SmsNoticeResponse
	err  error
}

func (f fakeSmsNoticeSubmitter) SubmitSmsNotice(ctx context.Context, req messagecenter.SmsNoticeRequest) (*messagecenter.SmsNoticeResponse, error) {
	return f.resp, f.err
}

type smsNoticeRepoStub struct {
	notice       *models.SmsNotice
	getByIDCalls chan struct{}
	updateCalls  chan *models.SmsNotice
}

func (s *smsNoticeRepoStub) Create(ctx context.Context, notice *models.SmsNotice) error {
	s.notice = notice
	return nil
}

func (s *smsNoticeRepoStub) Update(ctx context.Context, notice *models.SmsNotice) error {
	if s.updateCalls != nil {
		s.updateCalls <- notice
	}
	s.notice = notice
	return nil
}

func (s *smsNoticeRepoStub) GetByID(ctx context.Context, id int) (*models.SmsNotice, error) {
	if s.getByIDCalls != nil {
		s.getByIDCalls <- struct{}{}
	}
	return s.notice, nil
}

func (s *smsNoticeRepoStub) GetByOliveBranchRecordID(ctx context.Context, oliveBranchRecordID int) (*models.SmsNotice, error) {
	return s.notice, nil
}

func (s *smsNoticeRepoStub) GetByOrderID(ctx context.Context, orderID int) (*models.SmsNotice, error) {
	return s.notice, nil
}

func TestSmsNoticeSubmissionRejectedDoesNotOverwriteMessageCenterFailure(t *testing.T) {
	accepted := false
	failedMessage := "receiver phone is missing or invalid"
	failedNotice := &models.SmsNotice{
		ID:                  10,
		OrderID:             20,
		OliveBranchRecordID: 30,
		SenderID:            1,
		ReceiverID:          2,
		Status:              models.SmsNoticeStatusFailed,
		ErrorMessage:        &failedMessage,
	}
	repo := &smsNoticeRepoStub{
		notice:       failedNotice,
		getByIDCalls: make(chan struct{}, 1),
		updateCalls:  make(chan *models.SmsNotice, 1),
	}
	svc := &SmsNoticeService{
		repo: &repository.Repository{
			SmsNotice: repo,
		},
		messageCenter: fakeSmsNoticeSubmitter{
			resp: &messagecenter.SmsNoticeResponse{
				Accepted: &accepted,
			},
		},
	}

	notice := &models.SmsNotice{
		ID:                  10,
		OrderID:             20,
		OliveBranchRecordID: 30,
		SenderID:            1,
		ReceiverID:          2,
		Status:              models.SmsNoticeStatusSending,
	}
	svc.startAsyncSubmission(notice)

	select {
	case <-repo.getByIDCalls:
	case <-time.After(time.Second):
		require.Fail(t, "timed out waiting for rejected notice reload")
	}

	select {
	case updated := <-repo.updateCalls:
		require.Failf(t, "rejected response should not overwrite notice", "unexpected update: %+v", updated)
	case <-time.After(50 * time.Millisecond):
	}

	assert.Equal(t, models.SmsNoticeStatusFailed, notice.Status)
	assert.Equal(t, failedMessage, *notice.ErrorMessage)
}
