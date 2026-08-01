package service

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"testing"

	"github.com/jmoiron/sqlx"
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
		{name: "receiver phone format is invalid", mutate: func(_ *models.OrderDeliveryIntent, repo *repository.Repository) {
			invalidPhone := "11111"
			repo.User = smsNoticeUserRepoStub{user: &models.User{ID: receiverID, Phone: &invalidPhone}}
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

func TestInitiatePaymentRejectsPersistedInvalidReceiverPhone(t *testing.T) {
	invalidPhone := "53339458637"
	reviewerID, receiverID, applicationID, projectID := 9, 12, 7, 42
	scene := models.OrderDeliverySceneSMSNotice
	payload := `{"scene":"sms_notice","receiverUserId":12,"applicationId":7,"projectId":42,"noticeType":"accepted"}`
	repo := &repository.Repository{
		Order: smsNoticeOrderRepoStub{order: &models.Order{
			ID: 52, UserID: reviewerID, ProductID: 2, Status: models.OrderStatusPending,
			DeliveryScene: &scene, DeliveryPayload: &payload,
		}},
		Product: smsNoticeProductRepoStub{product: smsDeliveryIntentProduct()},
		Application: smsNoticeApplicationRepoStub{application: &models.ProjectApplication{
			ID: applicationID, ProjectID: projectID, UserID: receiverID, ReviewerID: &reviewerID,
			Status: models.ApplicationStatusJoined,
		}},
		User:    smsNoticeUserRepoStub{user: &models.User{ID: receiverID, Phone: &invalidPhone}},
		Project: smsNoticeProjectRepoStub{project: &models.Project{ID: projectID}},
	}
	svc := NewOrderService(repo, nil, errors.New("payment client must not be reached"))

	params, err := svc.InitiatePayment(context.Background(), reviewerID, "openid", 52)

	require.Nil(t, params)
	require.Error(t, err)
	var serviceErr *ServiceError
	require.ErrorAs(t, err, &serviceErr)
	assert.Equal(t, ErrCodeBadRequest, serviceErr.Code)
}

func TestMemberRemovalDeliveryRevalidatesPhoneAtCreateAndPayment(t *testing.T) {
	const driverName = "member_removal_delivery_validation"
	sql.Register(driverName, memberRemovalValidationDriver{})
	db, err := sql.Open(driverName, "")
	require.NoError(t, err)
	defer db.Close()

	phone := "13800138000"
	operatorID, receiverID, projectID := 9, 12, 42
	removalID := int64(77)
	noticeType := "removed"
	repo := repository.New(sqlx.NewDb(db, driverName))
	repo.User = smsNoticeUserRepoStub{user: &models.User{ID: receiverID, Phone: &phone}}
	repo.Project = smsNoticeProjectRepoStub{project: &models.Project{ID: projectID}}
	repo.Product = smsNoticeProductRepoStub{product: smsDeliveryIntentProduct()}
	intent := &models.OrderDeliveryIntent{
		Scene: models.OrderDeliverySceneSMSNotice, ReceiverUserID: &receiverID,
		MemberRemovalID: &removalID, ProjectID: &projectID, NoticeType: &noticeType,
	}
	svc := NewOrderService(repo, nil, errors.New("payment client must not be reached"))

	require.NoError(t, svc.validateDeliveryIntent(context.Background(), operatorID, smsDeliveryIntentProduct(), intent),
		"valid member-removal intent must pass order creation validation")

	invalidPhone := "11111"
	repo.User = smsNoticeUserRepoStub{user: &models.User{ID: receiverID, Phone: &invalidPhone}}
	scene := models.OrderDeliverySceneSMSNotice
	payload := `{"scene":"sms_notice","receiverUserId":12,"memberRemovalId":77,"projectId":42,"noticeType":"removed"}`
	repo.Order = smsNoticeOrderRepoStub{order: &models.Order{
		ID: 52, UserID: operatorID, ProductID: 2, Status: models.OrderStatusPending,
		DeliveryScene: &scene, DeliveryPayload: &payload,
	}}

	params, err := svc.InitiatePayment(context.Background(), operatorID, "openid", 52)
	require.Nil(t, params)
	require.Error(t, err)
	var serviceErr *ServiceError
	require.ErrorAs(t, err, &serviceErr)
	assert.Equal(t, ErrCodeBadRequest, serviceErr.Code)
}

type memberRemovalValidationDriver struct{}

func (memberRemovalValidationDriver) Open(string) (driver.Conn, error) {
	return memberRemovalValidationConn{}, nil
}

type memberRemovalValidationConn struct{}

func (memberRemovalValidationConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is not supported")
}
func (memberRemovalValidationConn) Close() error { return nil }
func (memberRemovalValidationConn) Begin() (driver.Tx, error) {
	return nil, errors.New("transaction is not supported")
}
func (memberRemovalValidationConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return &memberRemovalValidationRows{}, nil
}

type memberRemovalValidationRows struct {
	read bool
}

func (*memberRemovalValidationRows) Columns() []string {
	return []string{"user_id", "project_id", "operator_id"}
}
func (*memberRemovalValidationRows) Close() error { return nil }
func (r *memberRemovalValidationRows) Next(values []driver.Value) error {
	if r.read {
		return io.EOF
	}
	r.read = true
	values[0], values[1], values[2] = int64(12), int64(42), int64(9)
	return nil
}
