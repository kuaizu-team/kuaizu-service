package service

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/messagecenter"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSmsNoticeSubmitter struct {
	resp    *messagecenter.SmsNoticeResponse
	err     error
	smsReqs chan messagecenter.SmsNoticeRequest
	appReqs []messagecenter.ApplicationSmsRequest
	appErr  error
}

func (f fakeSmsNoticeSubmitter) SubmitSmsNotice(ctx context.Context, req messagecenter.SmsNoticeRequest) (*messagecenter.SmsNoticeResponse, error) {
	if f.smsReqs != nil {
		f.smsReqs <- req
	}
	return f.resp, f.err
}

func (f *fakeSmsNoticeSubmitter) SubmitApplicationSms(ctx context.Context, req messagecenter.ApplicationSmsRequest) error {
	f.appReqs = append(f.appReqs, req)
	return f.appErr
}

type smsNoticeFailureCall struct {
	noticeID    int
	orderID     int
	message     string
	completedAt time.Time
}

type orderPushClaimStub struct {
	claimed       bool
	calls         int
	released      bool
	releaseCalls  int
	releaseErr    error
	releaseCtxErr error
}

func (s *orderPushClaimStub) BeginOrderPushDeliveryForUser(context.Context, int, int) (bool, error) {
	s.calls++
	return s.claimed, nil
}

func (s *orderPushClaimStub) ReleaseOrderPushDeliveryForUser(ctx context.Context, _ int, _ int) (bool, error) {
	s.releaseCalls++
	s.releaseCtxErr = ctx.Err()
	return s.released, s.releaseErr
}

type smsNoticeRepoStub struct {
	notice           *models.SmsNotice
	noticeByOrder    *models.SmsNotice
	updateCalls      chan *models.SmsNotice
	createApp        chan *models.SmsNotice
	markFailedCalls  chan smsNoticeFailureCall
	markFailedResult bool
	markFailedErr    error
	createErr        error
	createAppErr     error
	createOutcomeErr error
	createRemovalErr error
}

func (s *smsNoticeRepoStub) Create(ctx context.Context, notice *models.SmsNotice) error {
	if s.createErr != nil {
		return s.createErr
	}
	notice.ID = 101
	s.notice = notice
	return nil
}

func (s *smsNoticeRepoStub) CreateApplication(ctx context.Context, notice *models.SmsNotice) error {
	if s.createAppErr != nil {
		return s.createAppErr
	}
	notice.ID = 101
	s.notice = notice
	if s.createApp != nil {
		s.createApp <- notice
	}
	return nil
}

func (s *smsNoticeRepoStub) CreateOutcome(ctx context.Context, notice *models.SmsNotice) error {
	if s.createOutcomeErr != nil {
		return s.createOutcomeErr
	}
	notice.ID = 101
	s.notice = notice
	return nil
}

func (s *smsNoticeRepoStub) CreateMemberRemoval(ctx context.Context, notice *models.SmsNotice, removalID int64) error {
	if s.createRemovalErr != nil {
		return s.createRemovalErr
	}
	notice.ID = 101
	notice.MemberRemovalID = &removalID
	s.notice = notice
	return nil
}

func (s *smsNoticeRepoStub) CompleteMemberRemoval(ctx context.Context, notice *models.SmsNotice) error {
	return s.Update(ctx, notice)
}

func (s *smsNoticeRepoStub) Update(ctx context.Context, notice *models.SmsNotice) error {
	if s.updateCalls != nil {
		s.updateCalls <- notice
	}
	s.notice = notice
	return nil
}

func (s *smsNoticeRepoStub) MarkFailedAndOrderPushIfNotCompleted(
	ctx context.Context, noticeID, orderID int, message string, completedAt time.Time,
) (bool, error) {
	if s.markFailedCalls != nil {
		s.markFailedCalls <- smsNoticeFailureCall{
			noticeID: noticeID, orderID: orderID, message: message, completedAt: completedAt,
		}
	}
	return s.markFailedResult, s.markFailedErr
}

func (s *smsNoticeRepoStub) GetByID(ctx context.Context, id int) (*models.SmsNotice, error) {
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

func TestSmsNoticeMqPublishRejectedDoesNotMutateCallerOrOverwriteMessageCenterFailure(t *testing.T) {
	accepted := false
	failedMessage := "failed to publish olive branch sms message"
	smsReqs := make(chan messagecenter.SmsNoticeRequest, 1)
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
		notice:      failedNotice,
		updateCalls: make(chan *models.SmsNotice, 1),
	}
	svc := &SmsNoticeService{
		repo: &repository.Repository{
			SmsNotice: repo,
		},
		messageCenter: fakeSmsNoticeSubmitter{
			smsReqs: smsReqs,
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
	case req := <-smsReqs:
		assert.Equal(t, notice.ID, req.NoticeID)
		assert.Equal(t, notice.OrderID, req.OrderID)
	case <-time.After(time.Second):
		require.Fail(t, "timed out waiting for sms submission")
	}

	select {
	case updated := <-repo.updateCalls:
		require.Failf(t, "rejected response should not overwrite notice", "unexpected update: %+v", updated)
	case <-time.After(50 * time.Millisecond):
	}

	assert.Equal(t, models.SmsNoticeStatusSending, notice.Status)
	assert.Nil(t, notice.ErrorMessage)
	assert.Equal(t, models.SmsNoticeStatusFailed, repo.notice.Status)
	assert.Equal(t, failedMessage, *repo.notice.ErrorMessage)
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

func TestSmsNoticeHttpTimeoutDoesNotOverwriteMessageCenterSuccess(t *testing.T) {
	completedAt := time.Now()
	successfulNotice := &models.SmsNotice{
		ID: 10, OrderID: 20, Status: models.SmsNoticeStatusCompleted, CompletedAt: &completedAt,
	}
	repo := &smsNoticeRepoStub{
		notice:           successfulNotice,
		markFailedCalls:  make(chan smsNoticeFailureCall, 1),
		markFailedResult: false,
	}
	svc := &SmsNoticeService{
		repo:          &repository.Repository{SmsNotice: repo},
		messageCenter: fakeSmsNoticeSubmitter{err: context.DeadlineExceeded},
	}
	staleNotice := &models.SmsNotice{
		ID: 10, OrderID: 20, Status: models.SmsNoticeStatusSending,
	}

	svc.startAsyncSubmission(staleNotice)

	select {
	case call := <-repo.markFailedCalls:
		assert.Equal(t, 10, call.noticeID)
		assert.Equal(t, 20, call.orderID)
		assert.Contains(t, call.message, "submit message center failed")
	case <-time.After(2 * time.Second):
		require.Fail(t, "timed out waiting for protected failure transition")
	}
	assert.Equal(t, models.SmsNoticeStatusCompleted, repo.notice.Status)
	assert.Equal(t, completedAt, *repo.notice.CompletedAt)
	assert.Equal(t, models.SmsNoticeStatusSending, staleNotice.Status)
}

func TestSmsNoticeHttpFailureAtomicallyFailsNoticeAndOrder(t *testing.T) {
	repo := &smsNoticeRepoStub{
		markFailedCalls:  make(chan smsNoticeFailureCall, 1),
		markFailedResult: true,
	}
	svc := &SmsNoticeService{
		repo:          &repository.Repository{SmsNotice: repo},
		messageCenter: fakeSmsNoticeSubmitter{err: context.DeadlineExceeded},
	}
	notice := &models.SmsNotice{
		ID: 10, OrderID: 20, Status: models.SmsNoticeStatusSending,
	}

	svc.startAsyncSubmission(notice)

	select {
	case call := <-repo.markFailedCalls:
		assert.Equal(t, 10, call.noticeID)
		assert.Equal(t, 20, call.orderID)
		assert.Contains(t, call.message, "submit message center failed")
		assert.False(t, call.completedAt.IsZero())
	case <-time.After(2 * time.Second):
		require.Fail(t, "timed out waiting for atomic failure transition")
	}
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

func TestApplicationSmsLosingOrderClaimCreatesNoNoticeOrMessage(t *testing.T) {
	sceneConfig := `{"scene":"olive_branch_sms_notice"}`
	phone := "13200000000"
	reviewerID := 1130
	applicationID := 7
	projectID := 154
	noticeType := "rejected"
	submitter := &fakeSmsNoticeSubmitter{}
	noticeRepo := &smsNoticeRepoStub{}
	claimer := &orderPushClaimStub{claimed: false}
	repo := &repository.Repository{
		Application: smsNoticeApplicationRepoStub{application: &models.ProjectApplication{
			ID: applicationID, ProjectID: projectID, UserID: 1128,
			Status: models.ApplicationStatusRejected, ReviewerID: &reviewerID,
		}},
		Order:     smsNoticeOrderRepoStub{order: &models.Order{ID: 52, UserID: reviewerID, ProductID: 2, Status: models.OrderStatusPaid}},
		Product:   smsNoticeProductRepoStub{product: &models.Product{ID: 2, ConfigJSON: &sceneConfig}},
		Project:   smsNoticeProjectRepoStub{project: &models.Project{ID: projectID, Name: "test project"}},
		User:      smsNoticeUserRepoStub{user: &models.User{ID: 1128, Phone: &phone}},
		SmsNotice: noticeRepo,
	}
	svc := &SmsNoticeService{repo: repo, orderPushClaimer: claimer, messageCenter: submitter}

	notice, err := svc.Send(context.Background(), reviewerID, SendSmsNoticeInput{
		OrderID: 52, ReceiverUserID: 1128, ApplicationID: &applicationID,
		NoticeType: &noticeType, ProjectID: &projectID,
	})

	require.Nil(t, notice)
	require.Error(t, err)
	assert.Equal(t, 1, claimer.calls)
	assert.Nil(t, noticeRepo.notice)
	assert.Empty(t, submitter.appReqs)
}

func TestOrdinarySmsCreateFailureReleasesOrderClaim(t *testing.T) {
	sceneConfig := `{"scene":"olive_branch_sms_notice"}`
	phone := "13800138000"
	claimer := &orderPushClaimStub{claimed: true, released: true}
	repo := &repository.Repository{
		OliveBranch: smsNoticeOliveBranchRepoStub{branch: &models.OliveBranch{
			ID: 76, SenderID: 1130, ReceiverID: 1128, RelatedProjectID: 154,
		}},
		Order:     smsNoticeOrderRepoStub{order: &models.Order{ID: 52, UserID: 1130, ProductID: 2, Status: models.OrderStatusPaid}},
		Product:   smsNoticeProductRepoStub{product: &models.Product{ID: 2, ConfigJSON: &sceneConfig}},
		Project:   smsNoticeProjectRepoStub{project: &models.Project{ID: 154, Name: "project"}},
		User:      smsNoticeUserRepoStub{user: &models.User{ID: 1128, Phone: &phone}},
		SmsNotice: &smsNoticeRepoStub{createErr: errors.New("insert failed")},
	}
	svc := &SmsNoticeService{repo: repo, orderPushClaimer: claimer}

	requestCtx, cancel := context.WithCancel(context.Background())
	cancel()
	notice, err := svc.Send(requestCtx, 1130, SendSmsNoticeInput{
		OrderID: 52, ReceiverUserID: 1128, OliveBranchRecordID: 76, ProjectID: intPtr(154),
	})

	require.Nil(t, notice)
	require.Error(t, err)
	assert.Equal(t, 1, claimer.calls)
	assert.Equal(t, 1, claimer.releaseCalls)
	assert.NoError(t, claimer.releaseCtxErr, "claim compensation must survive request cancellation")
}

func TestOutcomeSmsCreateFailureReleasesOrderClaim(t *testing.T) {
	sceneConfig := `{"scene":"olive_branch_sms_notice"}`
	phone := "13800138000"
	noticeType := "rejected"
	claimer := &orderPushClaimStub{claimed: true, released: true}
	repo := &repository.Repository{
		OliveBranch: smsNoticeOliveBranchRepoStub{branch: &models.OliveBranch{
			ID: 76, SenderID: 1130, ReceiverID: 1128, RelatedProjectID: 154, Status: models.OliveBranchStatusRejected,
		}},
		Order:     smsNoticeOrderRepoStub{order: &models.Order{ID: 52, UserID: 1130, ProductID: 2, Status: models.OrderStatusPaid}},
		Product:   smsNoticeProductRepoStub{product: &models.Product{ID: 2, ConfigJSON: &sceneConfig}},
		Project:   smsNoticeProjectRepoStub{project: &models.Project{ID: 154, Name: "project"}},
		User:      smsNoticeUserRepoStub{user: &models.User{ID: 1128, Phone: &phone}},
		SmsNotice: &smsNoticeRepoStub{createOutcomeErr: errors.New("insert failed")},
	}
	svc := &SmsNoticeService{repo: repo, orderPushClaimer: claimer, messageCenter: &fakeSmsNoticeSubmitter{}}

	notice, err := svc.Send(context.Background(), 1130, SendSmsNoticeInput{
		OrderID: 52, ReceiverUserID: 1128, OliveBranchRecordID: 76, NoticeType: &noticeType,
	})

	require.Nil(t, notice)
	require.Error(t, err)
	assert.Equal(t, 1, claimer.calls)
	assert.Equal(t, 1, claimer.releaseCalls)
}

func TestApplicationSmsCreateFailureReleasesOrderClaim(t *testing.T) {
	sceneConfig := `{"scene":"olive_branch_sms_notice"}`
	phone := "13800138000"
	noticeType := "rejected"
	applicationID, projectID := 7, 154
	claimer := &orderPushClaimStub{claimed: true, released: true}
	repo := &repository.Repository{
		Application: smsNoticeApplicationRepoStub{application: &models.ProjectApplication{
			ID: applicationID, ProjectID: projectID, UserID: 1128,
			Status: models.ApplicationStatusRejected, ReviewerID: intPtr(1130),
		}},
		Order:     smsNoticeOrderRepoStub{order: &models.Order{ID: 52, UserID: 1130, ProductID: 2, Status: models.OrderStatusPaid}},
		Product:   smsNoticeProductRepoStub{product: &models.Product{ID: 2, ConfigJSON: &sceneConfig}},
		Project:   smsNoticeProjectRepoStub{project: &models.Project{ID: projectID, Name: "project"}},
		User:      smsNoticeUserRepoStub{user: &models.User{ID: 1128, Phone: &phone}},
		SmsNotice: &smsNoticeRepoStub{createAppErr: errors.New("insert failed")},
	}
	svc := &SmsNoticeService{repo: repo, orderPushClaimer: claimer, messageCenter: &fakeSmsNoticeSubmitter{}}

	notice, err := svc.Send(context.Background(), 1130, SendSmsNoticeInput{
		OrderID: 52, ReceiverUserID: 1128, ApplicationID: &applicationID,
		NoticeType: &noticeType, ProjectID: &projectID,
	})

	require.Nil(t, notice)
	require.Error(t, err)
	assert.Equal(t, 1, claimer.calls)
	assert.Equal(t, 1, claimer.releaseCalls)
}

func TestMemberRemovalSmsCreateFailureReleasesOrderClaim(t *testing.T) {
	const driverName = "member_removal_sms_create_failure"
	sql.Register(driverName, memberRemovalSmsDriver{})
	db, err := sql.Open(driverName, "")
	require.NoError(t, err)
	defer db.Close()

	sceneConfig := `{"scene":"olive_branch_sms_notice"}`
	phone := "13800138000"
	removalID := int64(77)
	claimer := &orderPushClaimStub{claimed: true, released: true}
	repo := repository.New(sqlx.NewDb(db, driverName))
	repo.Order = smsNoticeOrderRepoStub{order: &models.Order{ID: 52, UserID: 9, ProductID: 2, Status: models.OrderStatusPaid}}
	repo.Product = smsNoticeProductRepoStub{product: &models.Product{ID: 2, ConfigJSON: &sceneConfig}}
	repo.User = smsNoticeUserRepoStub{user: &models.User{ID: 12, Phone: &phone}}
	repo.SmsNotice = &smsNoticeRepoStub{createRemovalErr: errors.New("insert failed")}
	svc := &SmsNoticeService{repo: repo, orderPushClaimer: claimer, messageCenter: &fakeSmsNoticeSubmitter{}}

	notice, err := svc.Send(context.Background(), 9, SendSmsNoticeInput{
		OrderID: 52, ReceiverUserID: 12, MemberRemovalID: &removalID,
	})

	require.Nil(t, notice)
	require.Error(t, err)
	assert.Equal(t, 1, claimer.calls)
	assert.Equal(t, 1, claimer.releaseCalls)
}

func TestTemplateWriteFailureAtomicallyFailsCreatedNoticeAndOrder(t *testing.T) {
	sceneConfig := `{"scene":"olive_branch_sms_notice"}`
	phone := "13800138000"
	noticeType := "rejected"
	applicationID, projectID := 7, 154
	claimer := &orderPushClaimStub{claimed: true}
	noticeRepo := &smsNoticeRepoStub{
		markFailedCalls: make(chan smsNoticeFailureCall, 1), markFailedResult: true,
	}
	repo := &repository.Repository{
		Application: smsNoticeApplicationRepoStub{application: &models.ProjectApplication{
			ID: applicationID, ProjectID: projectID, UserID: 1128,
			Status: models.ApplicationStatusRejected, ReviewerID: intPtr(1130),
		}},
		Order:     smsNoticeOrderRepoStub{order: &models.Order{ID: 52, UserID: 1130, ProductID: 2, Status: models.OrderStatusPaid}},
		Product:   smsNoticeProductRepoStub{product: &models.Product{ID: 2, ConfigJSON: &sceneConfig}},
		Project:   smsNoticeProjectRepoStub{project: &models.Project{ID: projectID, Name: "project"}},
		User:      smsNoticeUserRepoStub{user: &models.User{ID: 1128, Phone: &phone}},
		SmsNotice: noticeRepo,
	}
	svc := &SmsNoticeService{
		repo: repo, orderPushClaimer: claimer, messageCenter: &fakeSmsNoticeSubmitter{},
		orderTemplateRecorder: func(context.Context, int, string) error { return errors.New("template write failed") },
	}

	notice, err := svc.Send(context.Background(), 1130, SendSmsNoticeInput{
		OrderID: 52, ReceiverUserID: 1128, ApplicationID: &applicationID,
		NoticeType: &noticeType, ProjectID: &projectID,
	})

	require.Nil(t, notice)
	require.Error(t, err)
	select {
	case call := <-noticeRepo.markFailedCalls:
		assert.Equal(t, 101, call.noticeID)
		assert.Equal(t, 52, call.orderID)
		assert.Contains(t, call.message, "template write failed")
	default:
		require.Fail(t, "template failure must atomically fail notice and order")
	}
}

func TestApplicationSmsRejectsInvalidReceiverPhoneBeforeCreatingNotice(t *testing.T) {
	sceneConfig := `{"scene":"olive_branch_sms_notice"}`
	invalidPhone := "11111"
	reviewerID, applicationID, projectID := 1130, 7, 154
	noticeType := "rejected"
	submitter := &fakeSmsNoticeSubmitter{}
	noticeRepo := &smsNoticeRepoStub{}
	repo := &repository.Repository{
		Application: smsNoticeApplicationRepoStub{application: &models.ProjectApplication{
			ID: applicationID, ProjectID: projectID, UserID: 1128,
			Status: models.ApplicationStatusRejected, ReviewerID: &reviewerID,
		}},
		Order:     smsNoticeOrderRepoStub{order: &models.Order{ID: 52, UserID: reviewerID, ProductID: 2, Status: models.OrderStatusPaid}},
		Product:   smsNoticeProductRepoStub{product: &models.Product{ID: 2, ConfigJSON: &sceneConfig}},
		Project:   smsNoticeProjectRepoStub{project: &models.Project{ID: projectID, Name: "test project"}},
		User:      smsNoticeUserRepoStub{user: &models.User{ID: 1128, Phone: &invalidPhone}},
		SmsNotice: noticeRepo,
	}
	svc := &SmsNoticeService{repo: repo, messageCenter: submitter}

	notice, err := svc.Send(context.Background(), reviewerID, SendSmsNoticeInput{
		OrderID: 52, ReceiverUserID: 1128, ApplicationID: &applicationID,
		NoticeType: &noticeType, ProjectID: &projectID,
	})

	require.Nil(t, notice)
	require.Error(t, err)
	assert.Nil(t, noticeRepo.notice)
	assert.Empty(t, submitter.appReqs)
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

func TestRecoverByOrderRedrivesPendingApplicationSmsWithStableTaskKey(t *testing.T) {
	phone := "13200000000"
	reviewerID := 1130
	applicationID := 7
	projectID := 154
	traceID := "PROJECT_APPLICATION_SMS:52"
	businessTag := "project_application_sms_rejected"
	submitter := &fakeSmsNoticeSubmitter{}
	noticeRepo := &smsNoticeRepoStub{noticeByOrder: &models.SmsNotice{
		ID: 101, OrderID: 52, ProjectID: &projectID, SenderID: reviewerID, ReceiverID: 1128,
		SmsContent: "PROJECT_APPLICATION_SMS:7:rejected", Channel: stringPtr("SMS"),
		BusinessTag: &businessTag, TraceID: &traceID, Status: models.SmsNoticeStatusPending,
	}}
	repo := &repository.Repository{
		Order:       smsNoticeOrderRepoStub{order: &models.Order{ID: 52, UserID: reviewerID, Status: models.OrderStatusPaid}},
		SmsNotice:   noticeRepo,
		User:        smsNoticeUserRepoStub{user: &models.User{ID: 1128, Phone: &phone}},
		Project:     smsNoticeProjectRepoStub{project: &models.Project{ID: projectID, Name: "test project"}},
		Application: smsNoticeApplicationRepoStub{application: &models.ProjectApplication{ID: applicationID, ProjectID: projectID, UserID: 1128}},
	}
	svc := &SmsNoticeService{repo: repo, messageCenter: submitter}

	notice, err := svc.RecoverByOrder(context.Background(), reviewerID, 52)

	require.NoError(t, err)
	require.NotNil(t, notice)
	require.Len(t, submitter.appReqs, 1)
	assert.Equal(t, "PROJECT_APPLICATION_SMS:52:rejected", submitter.appReqs[0].TaskKey)
	assert.Equal(t, traceID, submitter.appReqs[0].TraceID)
	assert.Equal(t, "PROJECT_APPLICATION_REJECTED", submitter.appReqs[0].TemplateCode)
	assert.True(t, submitter.appReqs[0].Retry)
	assert.Equal(t, models.SmsNoticeStatusCompleted, notice.Status)
}

func TestRecoverByOrderRedrivesPersistedOliveBranchSmsNotice(t *testing.T) {
	traceID := "OLIVE_BRANCH_SMS:52"
	businessTag := smsNoticeScene
	projectID := 154
	smsReqs := make(chan messagecenter.SmsNoticeRequest, 1)
	noticeRepo := &smsNoticeRepoStub{noticeByOrder: &models.SmsNotice{
		ID: 101, OrderID: 52, OliveBranchRecordID: 76, ProjectID: &projectID,
		SenderID: 1130, ReceiverID: 1128, SmsContent: "persisted content",
		BusinessTag: &businessTag, TraceID: &traceID, Status: models.SmsNoticeStatusSending,
	}}
	repo := &repository.Repository{
		Order:     smsNoticeOrderRepoStub{order: &models.Order{ID: 52, UserID: 1130, Status: models.OrderStatusPaid}},
		SmsNotice: noticeRepo,
	}
	svc := &SmsNoticeService{repo: repo, messageCenter: fakeSmsNoticeSubmitter{smsReqs: smsReqs}}

	notice, err := svc.RecoverByOrder(context.Background(), 1130, 52)

	require.NoError(t, err)
	require.NotNil(t, notice)
	select {
	case req := <-smsReqs:
		assert.Equal(t, traceID, req.TraceID)
		assert.Equal(t, 101, req.NoticeID)
		assert.Equal(t, 52, req.OrderID)
		assert.Equal(t, 76, req.OliveBranchRecordID)
	case <-time.After(time.Second):
		require.Fail(t, "timed out waiting for recovered sms submission")
	}
}

func TestConcurrentFailedRetryLoserDoesNotOverwriteWinner(t *testing.T) {
	projectID := 154
	phone := "13800138000"
	staleFailed := &models.SmsNotice{
		ID: 101, OrderID: 52, OliveBranchRecordID: 76, ProjectID: &projectID,
		SenderID: 1130, ReceiverID: 1128, SmsContent: "original failed content",
		Status: models.SmsNoticeStatusFailed,
	}
	persistedWinner := &models.SmsNotice{
		ID: 101, OrderID: 52, SmsContent: "winner completed content",
		Status: models.SmsNoticeStatusCompleted,
	}
	noticeRepo := &smsNoticeRepoStub{notice: persistedWinner, updateCalls: make(chan *models.SmsNotice, 1)}
	claimer := &orderPushClaimStub{claimed: false}
	svc := &SmsNoticeService{
		repo:             &repository.Repository{SmsNotice: noticeRepo},
		orderPushClaimer: claimer,
	}

	notice, err := svc.handleExistingNotice(
		context.Background(), staleFailed,
		SendSmsNoticeInput{OrderID: 52, ReceiverUserID: 1128, OliveBranchRecordID: 76},
		&models.OliveBranch{ID: 76, SenderID: 1130, ReceiverID: 1128},
		&models.Order{ID: 52},
		&models.Project{ID: projectID, Name: "test project"},
		&models.User{ID: 1128, Phone: &phone},
	)

	require.Nil(t, notice)
	require.Error(t, err)
	assert.Equal(t, 1, claimer.calls)
	assert.Equal(t, models.SmsNoticeStatusFailed, staleFailed.Status)
	assert.Equal(t, "original failed content", staleFailed.SmsContent)
	assert.Equal(t, models.SmsNoticeStatusCompleted, noticeRepo.notice.Status)
	assert.Equal(t, "winner completed content", noticeRepo.notice.SmsContent)
	select {
	case <-noticeRepo.updateCalls:
		require.Fail(t, "losing retry must not update the existing sms notice")
	default:
	}
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

type memberRemovalSmsDriver struct{}

func (memberRemovalSmsDriver) Open(string) (driver.Conn, error) { return memberRemovalSmsConn{}, nil }

type memberRemovalSmsConn struct{}

func (memberRemovalSmsConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is not supported")
}
func (memberRemovalSmsConn) Close() error { return nil }
func (memberRemovalSmsConn) Begin() (driver.Tx, error) {
	return nil, errors.New("transaction is not supported")
}
func (memberRemovalSmsConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return &memberRemovalSmsRows{}, nil
}

type memberRemovalSmsRows struct{ read bool }

func (*memberRemovalSmsRows) Columns() []string {
	return []string{"id", "user_id", "project_id", "operator_id", "role", "project_name"}
}
func (*memberRemovalSmsRows) Close() error { return nil }
func (r *memberRemovalSmsRows) Next(values []driver.Value) error {
	if r.read {
		return io.EOF
	}
	r.read = true
	values[0], values[1], values[2], values[3] = int64(77), int64(12), int64(42), int64(9)
	values[4], values[5] = "member", "project"
	return nil
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

func TestSmsNoticeSendDoesNotCreateTaskForAnotherUsersOrder(t *testing.T) {
	noticeRepo := &smsNoticeRepoStub{}
	repo := &repository.Repository{
		OliveBranch: smsNoticeOliveBranchRepoStub{branch: &models.OliveBranch{
			ID: 76, SenderID: 1, ReceiverID: 2, RelatedProjectID: 154, Status: models.OliveBranchStatusPending,
		}},
		Order:     smsNoticeOrderRepoStub{order: &models.Order{ID: 52, UserID: 999, ProductID: 2, Status: models.OrderStatusPaid}},
		SmsNotice: noticeRepo,
	}
	svc := &SmsNoticeService{repo: repo}

	notice, err := svc.Send(context.Background(), 1, SendSmsNoticeInput{
		OrderID: 52, ReceiverUserID: 2, OliveBranchRecordID: 76,
	})

	require.Nil(t, notice)
	var svcErr *ServiceError
	require.ErrorAs(t, err, &svcErr)
	assert.Equal(t, ErrCodeForbidden, svcErr.Code)
	assert.Nil(t, noticeRepo.notice, "unauthorized order must not create a send record")
}
