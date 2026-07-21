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
	resp    *messagecenter.SmsNoticeResponse
	err     error
	appReqs []messagecenter.ApplicationSmsRequest
	appErr  error
}

func (f fakeSmsNoticeSubmitter) SubmitSmsNotice(ctx context.Context, req messagecenter.SmsNoticeRequest) (*messagecenter.SmsNoticeResponse, error) {
	return f.resp, f.err
}

func (f *fakeSmsNoticeSubmitter) SubmitApplicationSms(ctx context.Context, req messagecenter.ApplicationSmsRequest) error {
	f.appReqs = append(f.appReqs, req)
	return f.appErr
}

type smsNoticeRepoStub struct {
	notice        *models.SmsNotice
	noticeByOrder *models.SmsNotice
	getByIDCalls  chan struct{}
	updateCalls   chan *models.SmsNotice
	createApp     chan *models.SmsNotice
}

func (s *smsNoticeRepoStub) Create(ctx context.Context, notice *models.SmsNotice) error {
	s.notice = notice
	return nil
}

func (s *smsNoticeRepoStub) CreateApplication(ctx context.Context, notice *models.SmsNotice) error {
	notice.ID = 101
	s.notice = notice
	if s.createApp != nil {
		s.createApp <- notice
	}
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
	if s.noticeByOrder != nil {
		return s.noticeByOrder, nil
	}
	return s.notice, nil
}

func TestGetSmsNoticeByOliveBranchReturnsNotFoundWhenNoCurrentNoticeExists(t *testing.T) {
	svc := &SmsNoticeService{
		repo: &repository.Repository{SmsNotice: &smsNoticeRepoStub{}},
	}

	notice, err := svc.GetByOliveBranchRecordID(context.Background(), 1, 42)

	require.Nil(t, notice)
	var serviceErr *ServiceError
	require.ErrorAs(t, err, &serviceErr)
	assert.Equal(t, ErrCodeNotFound, serviceErr.Code)
}

func TestSmsNoticeMqPublishRejectedDoesNotOverwriteMessageCenterFailure(t *testing.T) {
	accepted := false
	failedMessage := "failed to publish olive branch sms message"
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

func TestSmsNoticeMqPublishAcceptedDoesNotOverwriteMessageCenterSuccess(t *testing.T) {
	accepted := true
	completedAt := time.Now()
	successfulNotice := &models.SmsNotice{
		ID:                  10,
		OrderID:             20,
		OliveBranchRecordID: 30,
		SenderID:            1,
		ReceiverID:          2,
		Status:              models.SmsNoticeStatusCompleted,
		CompletedAt:         &completedAt,
	}
	repo := &smsNoticeRepoStub{
		notice:      successfulNotice,
		updateCalls: make(chan *models.SmsNotice, 1),
	}
	svc := &SmsNoticeService{
		repo: &repository.Repository{
			SmsNotice: repo,
		},
		messageCenter: fakeSmsNoticeSubmitter{
			resp: &messagecenter.SmsNoticeResponse{
				Accepted: &accepted,
				Provider: "aliyun_sms",
			},
		},
	}

	staleNotice := &models.SmsNotice{
		ID:                  10,
		OrderID:             20,
		OliveBranchRecordID: 30,
		SenderID:            1,
		ReceiverID:          2,
		Status:              models.SmsNoticeStatusSending,
	}
	svc.startAsyncSubmission(staleNotice)

	select {
	case updated := <-repo.updateCalls:
		require.Failf(t, "accepted response should not rewrite shared terminal state", "unexpected update: %+v", updated)
	case <-time.After(50 * time.Millisecond):
	}
	assert.Equal(t, models.SmsNoticeStatusCompleted, repo.notice.Status)
}

func TestSmsNoticeSendRejectsOrderBoundToAnotherOliveBranch(t *testing.T) {
	sceneConfig := `{"scene":"olive_branch_sms_notice"}`
	repo := &repository.Repository{
		OliveBranch: smsNoticeOliveBranchRepoStub{
			branch: &models.OliveBranch{ID: 76, SenderID: 1130, ReceiverID: 1128, RelatedProjectID: 154},
		},
		Order: smsNoticeOrderRepoStub{
			order: &models.Order{ID: 52, UserID: 1130, ProductID: 2, Status: models.OrderStatusPaid},
		},
		Product: smsNoticeProductRepoStub{
			product: &models.Product{ID: 2, ConfigJSON: &sceneConfig},
		},
		Project: smsNoticeProjectRepoStub{
			project: &models.Project{ID: 154, Name: "test project"},
		},
		User: smsNoticeUserRepoStub{
			user: &models.User{ID: 1128},
		},
		SmsNotice: &smsNoticeRepoStub{
			noticeByOrder: &models.SmsNotice{
				ID:                  1,
				OrderID:             52,
				OliveBranchRecordID: 39,
				SenderID:            1130,
				ReceiverID:          1128,
				Status:              models.SmsNoticeStatusFailed,
			},
		},
	}
	svc := &SmsNoticeService{repo: repo}

	notice, err := svc.Send(context.Background(), 1130, SendSmsNoticeInput{
		OrderID:             52,
		ReceiverUserID:      1128,
		OliveBranchRecordID: 76,
		ProjectID:           intPtr(154),
	})

	require.Nil(t, notice)
	require.Error(t, err)
	var svcErr *ServiceError
	require.ErrorAs(t, err, &svcErr)
	assert.Equal(t, ErrCodeBadRequest, svcErr.Code)
	assert.Equal(t, "order has already been used for another sms notice", svcErr.Message)
}

func TestApplicationSmsCreatesNoticeRecordBeforeSubmitting(t *testing.T) {
	sceneConfig := `{"scene":"olive_branch_sms_notice"}`
	phone := "13200000000"
	nickname := "tester"
	reviewerID := 1130
	reviewerRole := "TEAM_LEADER"
	submitter := &fakeSmsNoticeSubmitter{}
	noticeRepo := &smsNoticeRepoStub{
		createApp:   make(chan *models.SmsNotice, 1),
		updateCalls: make(chan *models.SmsNotice, 1),
	}
	repo := &repository.Repository{
		Application: smsNoticeApplicationRepoStub{
			application: &models.ProjectApplication{
				ID:           7,
				ProjectID:    154,
				UserID:       1128,
				Status:       models.ApplicationStatusRejected,
				ReviewerID:   &reviewerID,
				ReviewerRole: &reviewerRole,
			},
		},
		Order: smsNoticeOrderRepoStub{
			order: &models.Order{ID: 52, UserID: reviewerID, ProductID: 2, Status: models.OrderStatusPaid},
		},
		Product: smsNoticeProductRepoStub{
			product: &models.Product{ID: 2, ConfigJSON: &sceneConfig},
		},
		Project: smsNoticeProjectRepoStub{
			project: &models.Project{ID: 154, Name: "test project"},
		},
		User: smsNoticeUserRepoStub{
			user: &models.User{ID: 1128, Phone: &phone, Nickname: &nickname},
		},
		SmsNotice: noticeRepo,
	}
	svc := &SmsNoticeService{repo: repo, messageCenter: submitter}
	noticeType := "rejected"
	projectID := 154
	applicationID := 7

	notice, err := svc.Send(context.Background(), reviewerID, SendSmsNoticeInput{
		OrderID:        52,
		ReceiverUserID: 1128,
		ApplicationID:  &applicationID,
		NoticeType:     &noticeType,
		ProjectID:      &projectID,
	})

	require.NoError(t, err)
	require.NotNil(t, notice)
	created := <-noticeRepo.createApp
	assert.Equal(t, 52, created.OrderID)
	assert.Equal(t, 0, created.OliveBranchRecordID)
	assert.Equal(t, "project_application_sms_rejected", *created.BusinessTag)
	assert.Equal(t, models.SmsNoticeStatusCompleted, notice.Status)
	assert.Len(t, submitter.appReqs, 1)
	assert.Equal(t, "PROJECT_APPLICATION_REJECTED", submitter.appReqs[0].TemplateCode)
}

func TestApplicationSmsRejectsOrderWithTemplateCodeButNoNoticeRecord(t *testing.T) {
	templateCode := "PROJECT_APPLICATION_APPLICANT_REJECTED"
	reviewerID := 1130
	applicationID := 7
	noticeType := "applicant_rejected"
	applicantRejected := true
	repo := &repository.Repository{
		Application: smsNoticeApplicationRepoStub{
			application: &models.ProjectApplication{
				ID:                applicationID,
				ProjectID:         154,
				UserID:            1128,
				Status:            models.ApplicationStatusRejected,
				ReviewerID:        &reviewerID,
				ApplicantRejected: &applicantRejected,
			},
		},
		Order: smsNoticeOrderRepoStub{
			order: &models.Order{ID: 52, UserID: 1128, ProductID: 2, Status: models.OrderStatusPaid, TemplateCode: &templateCode},
		},
		SmsNotice: &smsNoticeRepoStub{},
	}
	svc := &SmsNoticeService{repo: repo}

	notice, err := svc.Send(context.Background(), 1128, SendSmsNoticeInput{
		OrderID:        52,
		ReceiverUserID: reviewerID,
		ApplicationID:  &applicationID,
		NoticeType:     &noticeType,
	})

	require.Nil(t, notice)
	var svcErr *ServiceError
	require.ErrorAs(t, err, &svcErr)
	assert.Equal(t, ErrCodeBadRequest, svcErr.Code)
	assert.Equal(t, "order has already been used for another sms notice", svcErr.Message)
}

func TestApplicationSmsRejectsApplicantRejectedWhenReviewerRejected(t *testing.T) {
	reviewerID := 1130
	applicationID := 7
	noticeType := "applicant_rejected"
	applicantRejected := false
	repo := &repository.Repository{
		Application: smsNoticeApplicationRepoStub{
			application: &models.ProjectApplication{
				ID:                applicationID,
				ProjectID:         154,
				UserID:            1128,
				Status:            models.ApplicationStatusRejected,
				ReviewerID:        &reviewerID,
				ApplicantRejected: &applicantRejected,
			},
		},
		SmsNotice: &smsNoticeRepoStub{},
	}
	svc := &SmsNoticeService{repo: repo}

	notice, err := svc.Send(context.Background(), 1128, SendSmsNoticeInput{
		OrderID:        52,
		ReceiverUserID: reviewerID,
		ApplicationID:  &applicationID,
		NoticeType:     &noticeType,
	})

	require.Nil(t, notice)
	var svcErr *ServiceError
	require.ErrorAs(t, err, &svcErr)
	assert.Equal(t, ErrCodeBadRequest, svcErr.Code)
	assert.Equal(t, "application was not rejected by applicant", svcErr.Message)
}

func TestRetryByOrderReusesApplicationSmsTaskKey(t *testing.T) {
	templateCode := "PROJECT_APPLICATION_REJECTED"
	phone := "13200000000"
	nickname := "applicant"
	reviewerID := 1130
	applicationID := 7
	projectID := 154
	traceID := "PROJECT_APPLICATION_SMS:52"
	businessTag := "project_application_sms_rejected"
	failure := "provider unavailable"
	submitter := &fakeSmsNoticeSubmitter{}
	noticeRepo := &smsNoticeRepoStub{
		noticeByOrder: &models.SmsNotice{
			ID:           101,
			OrderID:      52,
			ProjectID:    &projectID,
			SenderID:     reviewerID,
			ReceiverID:   1128,
			SmsContent:   "PROJECT_APPLICATION_SMS:7:rejected",
			Channel:      stringPtr("SMS"),
			BusinessTag:  &businessTag,
			TraceID:      &traceID,
			Status:       models.SmsNoticeStatusFailed,
			ErrorMessage: &failure,
		},
	}
	repo := &repository.Repository{
		Order: smsNoticeOrderRepoStub{
			order: &models.Order{
				ID: 52, UserID: reviewerID, Status: models.OrderStatusPaid,
				TemplateCode: &templateCode,
			},
		},
		SmsNotice: noticeRepo,
		User: smsNoticeUserRepoStub{
			user: &models.User{ID: 1128, Phone: &phone, Nickname: &nickname},
		},
		Project: smsNoticeProjectRepoStub{
			project: &models.Project{ID: projectID, Name: "test project"},
		},
		Application: smsNoticeApplicationRepoStub{
			application: &models.ProjectApplication{
				ID: applicationID, ProjectID: projectID, UserID: 1128,
			},
		},
	}
	svc := &SmsNoticeService{repo: repo, messageCenter: submitter}

	notice, err := svc.RetryByOrder(context.Background(), reviewerID, 52)

	require.NoError(t, err)
	require.NotNil(t, notice)
	require.Len(t, submitter.appReqs, 1)
	req := submitter.appReqs[0]
	assert.Equal(t, "PROJECT_APPLICATION_SMS:52:rejected", req.TaskKey)
	assert.Equal(t, "PROJECT_APPLICATION_SMS:52", req.TraceID)
	assert.Equal(t, businessTag, req.BusinessTag)
	assert.True(t, req.Retry)
	assert.Equal(t, models.SmsNoticeStatusCompleted, notice.Status)
}

type smsNoticeOliveBranchRepoStub struct {
	repository.OliveBranchRepo
	branch *models.OliveBranch
}

func (s smsNoticeOliveBranchRepoStub) GetByID(ctx context.Context, id int) (*models.OliveBranch, error) {
	return s.branch, nil
}

type smsNoticeOrderRepoStub struct {
	repository.OrderRepo
	order *models.Order
}

func (s smsNoticeOrderRepoStub) GetByID(ctx context.Context, id int) (*models.Order, error) {
	return s.order, nil
}

type smsNoticeProductRepoStub struct {
	repository.ProductRepo
	product *models.Product
}

func (s smsNoticeProductRepoStub) GetByID(ctx context.Context, id int) (*models.Product, error) {
	return s.product, nil
}

type smsNoticeProjectRepoStub struct {
	repository.ProjectRepo
	project *models.Project
}

func (s smsNoticeProjectRepoStub) GetByID(ctx context.Context, id int) (*models.Project, error) {
	return s.project, nil
}

type smsNoticeUserRepoStub struct {
	repository.UserRepo
	user *models.User
}

func (s smsNoticeUserRepoStub) GetByID(ctx context.Context, id int) (*models.User, error) {
	return s.user, nil
}

type smsNoticeApplicationRepoStub struct {
	repository.ApplicationRepo
	application *models.ProjectApplication
}

func (s smsNoticeApplicationRepoStub) GetByID(ctx context.Context, id int) (*models.ProjectApplication, error) {
	return s.application, nil
}

var (
	_ repository.OliveBranchRepo = smsNoticeOliveBranchRepoStub{}
	_ repository.OrderRepo       = smsNoticeOrderRepoStub{}
	_ repository.ProductRepo     = smsNoticeProductRepoStub{}
	_ repository.ProjectRepo     = smsNoticeProjectRepoStub{}
	_ repository.UserRepo        = smsNoticeUserRepoStub{}
	_ repository.ApplicationRepo = smsNoticeApplicationRepoStub{}
)

func intPtr(v int) *int {
	return &v
}
