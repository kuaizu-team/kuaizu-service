package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func smsDeliveryIntentProduct() *models.Product {
	return &models.Product{ID: 2, Name: "短信通知", Type: models.ProductTypeBenefit}
}

func TestValidateDeliveryIntentRejectsApplicationWithoutNoticeType(t *testing.T) {
	service := &OrderService{repo: &repository.Repository{}}
	applicationID, receiverID := 7, 12

	err := service.validateDeliveryIntent(context.Background(), 9, smsDeliveryIntentProduct(), &models.OrderDeliveryIntent{
		Scene: models.OrderDeliverySceneSMSNotice, ApplicationID: &applicationID, ReceiverUserID: &receiverID,
	})

	require.Error(t, err)
}

func TestValidateDeliveryIntentRejectsApplicationBusinessMismatchesBeforePayment(t *testing.T) {
	phone := "13800138000"
	reviewerID, receiverID, applicationID, projectID := 9, 12, 7, 42
	noticeType := "accepted"
	baseRepo := func() *repository.Repository {
		return &repository.Repository{
			Application: smsNoticeApplicationRepoStub{application: &models.ProjectApplication{
				ID: applicationID, ProjectID: projectID, UserID: receiverID, ReviewerID: &reviewerID,
				Status: models.ApplicationStatusJoined,
			}},
			User:    smsNoticeUserRepoStub{user: &models.User{ID: receiverID, Phone: &phone}},
			Project: smsNoticeProjectRepoStub{project: &models.Project{ID: projectID}},
		}
	}

	tests := []struct {
		name   string
		mutate func(*models.OrderDeliveryIntent, *repository.Repository)
	}{
		{name: "application does not exist", mutate: func(_ *models.OrderDeliveryIntent, repo *repository.Repository) {
			repo.Application = smsNoticeApplicationRepoStub{}
		}},
		{name: "invalid notice type", mutate: func(intent *models.OrderDeliveryIntent, _ *repository.Repository) {
			invalid := "unknown"
			intent.NoticeType = &invalid
		}},
		{name: "receiver mismatch", mutate: func(intent *models.OrderDeliveryIntent, _ *repository.Repository) {
			wrong := 13
			intent.ReceiverUserID = &wrong
		}},
		{name: "project mismatch", mutate: func(intent *models.OrderDeliveryIntent, _ *repository.Repository) {
			wrong := 43
			intent.ProjectID = &wrong
		}},
		{name: "payer is not reviewer", mutate: func(_ *models.OrderDeliveryIntent, repo *repository.Repository) {
			other := 10
			repo.Application = smsNoticeApplicationRepoStub{application: &models.ProjectApplication{
				ID: applicationID, ProjectID: projectID, UserID: receiverID, ReviewerID: &other,
				Status: models.ApplicationStatusJoined,
			}}
		}},
		{name: "status mismatch", mutate: func(_ *models.OrderDeliveryIntent, repo *repository.Repository) {
			repo.Application = smsNoticeApplicationRepoStub{application: &models.ProjectApplication{
				ID: applicationID, ProjectID: projectID, UserID: receiverID, ReviewerID: &reviewerID,
				Status: models.ApplicationStatusPending,
			}}
		}},
		{name: "receiver has no phone", mutate: func(_ *models.OrderDeliveryIntent, repo *repository.Repository) {
			repo.User = smsNoticeUserRepoStub{user: &models.User{ID: receiverID}}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := baseRepo()
			intent := &models.OrderDeliveryIntent{
				Scene: models.OrderDeliverySceneSMSNotice, ApplicationID: &applicationID,
				ReceiverUserID: &receiverID, ProjectID: &projectID, NoticeType: &noticeType,
			}
			test.mutate(intent, repo)
			err := (&OrderService{repo: repo}).validateDeliveryIntent(
				context.Background(), reviewerID, smsDeliveryIntentProduct(), intent)
			require.Error(t, err)
		})
	}
}

func TestValidateDeliveryIntentChecksOliveBranchOwnershipReceiverStatusAndProject(t *testing.T) {
	phone := "13800138000"
	senderID, receiverID, branchID, projectID := 9, 12, 7, 42
	validRepo := func() *repository.Repository {
		return &repository.Repository{
			OliveBranch: smsNoticeOliveBranchRepoStub{branch: &models.OliveBranch{
				ID: branchID, SenderID: senderID, ReceiverID: receiverID,
				RelatedProjectID: projectID, Status: models.OliveBranchStatusPending,
			}},
			User:    smsNoticeUserRepoStub{user: &models.User{ID: receiverID, Phone: &phone}},
			Project: smsNoticeProjectRepoStub{project: &models.Project{ID: projectID}},
		}
	}
	newIntent := func() *models.OrderDeliveryIntent {
		return &models.OrderDeliveryIntent{
			Scene: models.OrderDeliverySceneSMSNotice, OliveBranchRecordID: &branchID,
			ReceiverUserID: &receiverID, ProjectID: &projectID,
		}
	}

	assert.NoError(t, (&OrderService{repo: validRepo()}).validateDeliveryIntent(
		context.Background(), senderID, smsDeliveryIntentProduct(), newIntent()))

	tests := []struct {
		name   string
		mutate func(*models.OrderDeliveryIntent, *repository.Repository)
	}{
		{name: "receiver mismatch", mutate: func(intent *models.OrderDeliveryIntent, _ *repository.Repository) {
			wrong := 13
			intent.ReceiverUserID = &wrong
		}},
		{name: "project mismatch", mutate: func(intent *models.OrderDeliveryIntent, _ *repository.Repository) {
			wrong := 43
			intent.ProjectID = &wrong
		}},
		{name: "payer is not sender", mutate: func(_ *models.OrderDeliveryIntent, repo *repository.Repository) {
			repo.OliveBranch = smsNoticeOliveBranchRepoStub{branch: &models.OliveBranch{
				ID: branchID, SenderID: 10, ReceiverID: receiverID,
				RelatedProjectID: projectID, Status: models.OliveBranchStatusPending,
			}}
		}},
		{name: "status is not pending", mutate: func(_ *models.OrderDeliveryIntent, repo *repository.Repository) {
			repo.OliveBranch = smsNoticeOliveBranchRepoStub{branch: &models.OliveBranch{
				ID: branchID, SenderID: senderID, ReceiverID: receiverID,
				RelatedProjectID: projectID, Status: models.OliveBranchStatusAccepted,
			}}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo, intent := validRepo(), newIntent()
			test.mutate(intent, repo)
			require.Error(t, (&OrderService{repo: repo}).validateDeliveryIntent(
				context.Background(), senderID, smsDeliveryIntentProduct(), intent))
		})
	}
}

func TestValidateDeliveryIntentAcceptsValidApplicationSms(t *testing.T) {
	phone := "13800138000"
	reviewerID, receiverID, applicationID, projectID := 9, 12, 7, 42
	noticeType := "accepted"
	repo := &repository.Repository{
		Application: smsNoticeApplicationRepoStub{application: &models.ProjectApplication{
			ID: applicationID, ProjectID: projectID, UserID: receiverID, ReviewerID: &reviewerID,
			Status: models.ApplicationStatusJoined,
		}},
		User:    smsNoticeUserRepoStub{user: &models.User{ID: receiverID, Phone: &phone}},
		Project: smsNoticeProjectRepoStub{project: &models.Project{ID: projectID}},
	}

	err := (&OrderService{repo: repo}).validateDeliveryIntent(context.Background(), reviewerID,
		smsDeliveryIntentProduct(), &models.OrderDeliveryIntent{
			Scene: models.OrderDeliverySceneSMSNotice, ApplicationID: &applicationID,
			ReceiverUserID: &receiverID, ProjectID: &projectID, NoticeType: &noticeType,
		})

	assert.NoError(t, err)
}

func TestInitiatePaymentRevalidatesPersistedDeliveryIntent(t *testing.T) {
	scene := models.OrderDeliverySceneSMSNotice
	payload := `{"scene":"sms_notice","receiverUserId":12,"applicationId":7}`
	repo := &repository.Repository{
		Order: smsNoticeOrderRepoStub{order: &models.Order{
			ID: 52, UserID: 9, ProductID: 2, Status: models.OrderStatusPending,
			DeliveryScene: &scene, DeliveryPayload: &payload,
		}},
		Product: smsNoticeProductRepoStub{product: smsDeliveryIntentProduct()},
	}
	svc := NewOrderService(repo, nil, errors.New("payment client must not be reached"))

	params, err := svc.InitiatePayment(context.Background(), 9, "openid", 52)

	require.Nil(t, params)
	require.Error(t, err)
	var serviceErr *ServiceError
	require.ErrorAs(t, err, &serviceErr)
	assert.Equal(t, ErrCodeBadRequest, serviceErr.Code)
}
