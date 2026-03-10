package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/trv3wood/kuaizu-server/internal/models"
	"github.com/trv3wood/kuaizu-server/internal/repository"
)

// --- Mock Repositories using testify/mock ---

type MockOrderRepo struct {
	mock.Mock
}

func (m *MockOrderRepo) GetByID(ctx context.Context, id int) (*models.Order, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Order), args.Error(1)
}

func (m *MockOrderRepo) Create(ctx context.Context, order *models.Order) (*models.Order, error) {
	args := m.Called(ctx, order)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Order), args.Error(1)
}

func (m *MockOrderRepo) ListByUserID(ctx context.Context, params repository.OrderListParams) ([]*models.Order, int64, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*models.Order), args.Get(1).(int64), args.Error(2)
}

func (m *MockOrderRepo) UpdatePaymentStatus(ctx context.Context, id int, status int, wxPayNo string, payTime time.Time) error {
	args := m.Called(ctx, id, status, wxPayNo, payTime)
	return args.Error(0)
}

func (m *MockOrderRepo) UpdatePaymentStatusTx(ctx context.Context, tx *sqlx.Tx, id int, status int, wxPayNo string, payTime time.Time) error {
	args := m.Called(ctx, tx, id, status, wxPayNo, payTime)
	return args.Error(0)
}

func (m *MockOrderRepo) GetOrderItems(ctx context.Context, orderID int) ([]*models.OrderItem, error) {
	args := m.Called(ctx, orderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.OrderItem), args.Error(1)
}

func (m *MockOrderRepo) UpdateStatus(ctx context.Context, id int, status int) error {
	args := m.Called(ctx, id, status)
	return args.Error(0)
}

type MockProjectRepo struct {
	mock.Mock
}

func (m *MockProjectRepo) GetByID(ctx context.Context, id int) (*models.Project, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Project), args.Error(1)
}

func (m *MockProjectRepo) List(ctx context.Context, params repository.ListParams) ([]models.Project, int64, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]models.Project), args.Get(1).(int64), args.Error(2)
}

func (m *MockProjectRepo) Create(ctx context.Context, p *models.Project) error {
	args := m.Called(ctx, p)
	return args.Error(0)
}

func (m *MockProjectRepo) Update(ctx context.Context, p *models.Project) error {
	args := m.Called(ctx, p)
	return args.Error(0)
}

func (m *MockProjectRepo) Delete(ctx context.Context, id int) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockProjectRepo) IsOwner(ctx context.Context, projectID, userID int) (bool, error) {
	args := m.Called(ctx, projectID, userID)
	return args.Bool(0), args.Error(1)
}

func (m *MockProjectRepo) UpdateStatus(ctx context.Context, id int, status int) error {
	args := m.Called(ctx, id, status)
	return args.Error(0)
}

func (m *MockProjectRepo) IncrementViewCount(ctx context.Context, id int) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

type MockProductRepo struct {
	mock.Mock
}

func (m *MockProductRepo) GetByID(ctx context.Context, id int) (*models.Product, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Product), args.Error(1)
}

func (m *MockProductRepo) GetAll(ctx context.Context) ([]*models.Product, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Product), args.Error(1)
}

type MockEmailPromotionRepo struct {
	mock.Mock
}

func (m *MockEmailPromotionRepo) Create(ctx context.Context, promotion *models.EmailPromotion) error {
	args := m.Called(ctx, promotion)
	return args.Error(0)
}

func (m *MockEmailPromotionRepo) GetByID(ctx context.Context, id int) (*models.EmailPromotion, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.EmailPromotion), args.Error(1)
}

func (m *MockEmailPromotionRepo) GetByOrderID(ctx context.Context, orderID int) (*models.EmailPromotion, error) {
	args := m.Called(ctx, orderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.EmailPromotion), args.Error(1)
}

func (m *MockEmailPromotionRepo) Update(ctx context.Context, promotion *models.EmailPromotion) error {
	args := m.Called(ctx, promotion)
	return args.Error(0)
}

func (m *MockEmailPromotionRepo) ListByCreatorID(ctx context.Context, creatorID int, page, size int) ([]models.EmailPromotion, int64, error) {
	args := m.Called(ctx, creatorID, page, size)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]models.EmailPromotion), args.Get(1).(int64), args.Error(2)
}

func (m *MockEmailPromotionRepo) ListByProjectID(ctx context.Context, projectID int) ([]models.EmailPromotion, error) {
	args := m.Called(ctx, projectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.EmailPromotion), args.Error(1)
}

// --- Tests for TriggerPromotion ---

func TestTriggerPromotion_OrderNotFound(t *testing.T) {
	mockOrder := new(MockOrderRepo)
	mockProject := new(MockProjectRepo)
	mockProduct := new(MockProductRepo)
	mockEmailPromotion := new(MockEmailPromotionRepo)

	mockOrder.On("GetByID", mock.Anything, 100).Return(nil, nil)

	repo := &repository.Repository{
		Order:          mockOrder,
		Project:        mockProject,
		Product:        mockProduct,
		EmailPromotion: mockEmailPromotion,
	}

	svc := NewEmailPromotionService(repo)
	_, err := svc.TriggerPromotion(context.Background(), 1, 100, 200)

	assertServiceError(t, err, ErrCodeNotFound, "订单不存在")
	mockOrder.AssertExpectations(t)
}

func TestTriggerPromotion_OrderGetError(t *testing.T) {
	mockOrder := new(MockOrderRepo)
	mockProject := new(MockProjectRepo)
	mockProduct := new(MockProductRepo)
	mockEmailPromotion := new(MockEmailPromotionRepo)

	mockOrder.On("GetByID", mock.Anything, 100).Return(nil, errors.New("db error"))

	repo := &repository.Repository{
		Order:          mockOrder,
		Project:        mockProject,
		Product:        mockProduct,
		EmailPromotion: mockEmailPromotion,
	}

	svc := NewEmailPromotionService(repo)
	_, err := svc.TriggerPromotion(context.Background(), 1, 100, 200)

	assertServiceError(t, err, ErrCodeInternal, "获取订单失败")
	mockOrder.AssertExpectations(t)
}

func TestTriggerPromotion_OrderNotOwnedByUser(t *testing.T) {
	mockOrder := new(MockOrderRepo)
	mockProject := new(MockProjectRepo)
	mockProduct := new(MockProductRepo)
	mockEmailPromotion := new(MockEmailPromotionRepo)

	mockOrder.On("GetByID", mock.Anything, 100).Return(&models.Order{ID: 100, UserID: 999, Status: 1}, nil)

	repo := &repository.Repository{
		Order:          mockOrder,
		Project:        mockProject,
		Product:        mockProduct,
		EmailPromotion: mockEmailPromotion,
	}

	svc := NewEmailPromotionService(repo)
	_, err := svc.TriggerPromotion(context.Background(), 1, 100, 200)

	assertServiceError(t, err, ErrCodeForbidden, "无权操作此订单")
	mockOrder.AssertExpectations(t)
}

func TestTriggerPromotion_OrderNotPaid(t *testing.T) {
	mockOrder := new(MockOrderRepo)
	mockProject := new(MockProjectRepo)
	mockProduct := new(MockProductRepo)
	mockEmailPromotion := new(MockEmailPromotionRepo)

	mockOrder.On("GetByID", mock.Anything, 100).Return(&models.Order{ID: 100, UserID: 1, Status: 0}, nil)

	repo := &repository.Repository{
		Order:          mockOrder,
		Project:        mockProject,
		Product:        mockProduct,
		EmailPromotion: mockEmailPromotion,
	}

	svc := NewEmailPromotionService(repo)
	_, err := svc.TriggerPromotion(context.Background(), 1, 100, 200)

	assertServiceError(t, err, ErrCodeBadRequest, "订单未支付或状态异常")
	mockOrder.AssertExpectations(t)
}

func TestTriggerPromotion_ProjectNotFound(t *testing.T) {
	mockOrder := new(MockOrderRepo)
	mockProject := new(MockProjectRepo)
	mockProduct := new(MockProductRepo)
	mockEmailPromotion := new(MockEmailPromotionRepo)

	mockOrder.On("GetByID", mock.Anything, 100).Return(&models.Order{ID: 100, UserID: 1, Status: 1}, nil)
	mockProject.On("GetByID", mock.Anything, 200).Return(nil, nil)

	repo := &repository.Repository{
		Order:          mockOrder,
		Project:        mockProject,
		Product:        mockProduct,
		EmailPromotion: mockEmailPromotion,
	}

	svc := NewEmailPromotionService(repo)
	_, err := svc.TriggerPromotion(context.Background(), 1, 100, 200)

	assertServiceError(t, err, ErrCodeNotFound, "项目不存在")
	mockOrder.AssertExpectations(t)
	mockProject.AssertExpectations(t)
}

func TestTriggerPromotion_ProjectNotOwnedByUser(t *testing.T) {
	mockOrder := new(MockOrderRepo)
	mockProject := new(MockProjectRepo)
	mockProduct := new(MockProductRepo)
	mockEmailPromotion := new(MockEmailPromotionRepo)

	mockOrder.On("GetByID", mock.Anything, 100).Return(&models.Order{ID: 100, UserID: 1, Status: 1}, nil)
	mockProject.On("GetByID", mock.Anything, 200).Return(&models.Project{ID: 200, CreatorID: 999}, nil)

	repo := &repository.Repository{
		Order:          mockOrder,
		Project:        mockProject,
		Product:        mockProduct,
		EmailPromotion: mockEmailPromotion,
	}

	svc := NewEmailPromotionService(repo)
	_, err := svc.TriggerPromotion(context.Background(), 1, 100, 200)

	assertServiceError(t, err, ErrCodeForbidden, "只能推广自己创建的项目")
	mockOrder.AssertExpectations(t)
	mockProject.AssertExpectations(t)
}

func TestTriggerPromotion_AlreadyTriggered(t *testing.T) {
	mockOrder := new(MockOrderRepo)
	mockProject := new(MockProjectRepo)
	mockProduct := new(MockProductRepo)
	mockEmailPromotion := new(MockEmailPromotionRepo)

	mockOrder.On("GetByID", mock.Anything, 100).Return(&models.Order{ID: 100, UserID: 1, Status: 1}, nil)
	mockProject.On("GetByID", mock.Anything, 200).Return(&models.Project{ID: 200, CreatorID: 1}, nil)
	mockEmailPromotion.On("GetByOrderID", mock.Anything, 100).Return(&models.EmailPromotion{ID: 1, OrderID: 100}, nil)

	repo := &repository.Repository{
		Order:          mockOrder,
		Project:        mockProject,
		Product:        mockProduct,
		EmailPromotion: mockEmailPromotion,
	}

	svc := NewEmailPromotionService(repo)
	_, err := svc.TriggerPromotion(context.Background(), 1, 100, 200)

	assertServiceError(t, err, ErrCodeBadRequest, "此订单已触发过推广")
	mockOrder.AssertExpectations(t)
	mockProject.AssertExpectations(t)
	mockEmailPromotion.AssertExpectations(t)
}

func TestTriggerPromotion_NoEmailPromotionProduct(t *testing.T) {
	mockOrder := new(MockOrderRepo)
	mockProject := new(MockProjectRepo)
	mockProduct := new(MockProductRepo)
	mockEmailPromotion := new(MockEmailPromotionRepo)

	order := &models.Order{
		ID:     100,
		UserID: 1,
		Status: 1,
		Items:  []*models.OrderItem{{ProductID: 10, Quantity: 1}},
	}

	mockOrder.On("GetByID", mock.Anything, 100).Return(order, nil)
	mockProject.On("GetByID", mock.Anything, 200).Return(&models.Project{ID: 200, CreatorID: 1}, nil)
	mockEmailPromotion.On("GetByOrderID", mock.Anything, 100).Return(nil, nil)
	mockProduct.On("GetByID", mock.Anything, 10).Return(&models.Product{ID: 10, Type: 1}, nil)

	repo := &repository.Repository{
		Order:          mockOrder,
		Project:        mockProject,
		Product:        mockProduct,
		EmailPromotion: mockEmailPromotion,
	}

	svc := NewEmailPromotionService(repo)
	_, err := svc.TriggerPromotion(context.Background(), 1, 100, 200)

	assertServiceError(t, err, ErrCodeBadRequest, "订单中没有邮件推广商品")
	mockOrder.AssertExpectations(t)
	mockProject.AssertExpectations(t)
	mockEmailPromotion.AssertExpectations(t)
	mockProduct.AssertExpectations(t)
}

func TestTriggerPromotion_Success(t *testing.T) {
	mockOrder := new(MockOrderRepo)
	mockProject := new(MockProjectRepo)
	mockProduct := new(MockProductRepo)
	mockEmailPromotion := new(MockEmailPromotionRepo)

	order := &models.Order{
		ID:     100,
		UserID: 1,
		Status: 1,
		Items: []*models.OrderItem{
			{ProductID: 10, Quantity: 50},
			{ProductID: 11, Quantity: 30},
		},
	}

	mockOrder.On("GetByID", mock.Anything, 100).Return(order, nil)
	mockProject.On("GetByID", mock.Anything, 200).Return(&models.Project{ID: 200, CreatorID: 1}, nil)
	mockEmailPromotion.On("GetByOrderID", mock.Anything, 100).Return(nil, nil)
	mockProduct.On("GetByID", mock.Anything, 10).Return(&models.Product{ID: 10, Type: 2}, nil)
	mockProduct.On("GetByID", mock.Anything, 11).Return(&models.Product{ID: 11, Type: 2}, nil)
	mockEmailPromotion.On("Create", mock.Anything, mock.MatchedBy(func(p *models.EmailPromotion) bool {
		return p.OrderID == 100 && p.ProjectID == 200 && p.CreatorID == 1 && p.MaxRecipients == 80
	})).Run(func(args mock.Arguments) {
		promotion := args.Get(1).(*models.EmailPromotion)
		promotion.ID = 1
	}).Return(nil)
	// Mock the Update call that happens in the async goroutine
	mockEmailPromotion.On("Update", mock.Anything, mock.Anything).Return(nil).Maybe()

	repo := &repository.Repository{
		Order:          mockOrder,
		Project:        mockProject,
		Product:        mockProduct,
		EmailPromotion: mockEmailPromotion,
	}

	svc := NewEmailPromotionService(repo)
	result, err := svc.TriggerPromotion(context.Background(), 1, 100, 200)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 80, result.MaxRecipients)
	assert.Equal(t, 100, result.Promotion.OrderID)
	assert.Equal(t, 200, result.Promotion.ProjectID)
	assert.Equal(t, 1, result.Promotion.CreatorID)
	assert.Equal(t, models.EmailPromotionStatusPending, result.Promotion.Status)

	mockOrder.AssertExpectations(t)
	mockProject.AssertExpectations(t)
	mockEmailPromotion.AssertExpectations(t)
	mockProduct.AssertExpectations(t)
}

func TestTriggerPromotion_CreateFails(t *testing.T) {
	mockOrder := new(MockOrderRepo)
	mockProject := new(MockProjectRepo)
	mockProduct := new(MockProductRepo)
	mockEmailPromotion := new(MockEmailPromotionRepo)

	order := &models.Order{
		ID:     100,
		UserID: 1,
		Status: 1,
		Items:  []*models.OrderItem{{ProductID: 10, Quantity: 50}},
	}

	mockOrder.On("GetByID", mock.Anything, 100).Return(order, nil)
	mockProject.On("GetByID", mock.Anything, 200).Return(&models.Project{ID: 200, CreatorID: 1}, nil)
	mockEmailPromotion.On("GetByOrderID", mock.Anything, 100).Return(nil, nil)
	mockProduct.On("GetByID", mock.Anything, 10).Return(&models.Product{ID: 10, Type: 2}, nil)
	mockEmailPromotion.On("Create", mock.Anything, mock.Anything).Return(errors.New("db write error"))

	repo := &repository.Repository{
		Order:          mockOrder,
		Project:        mockProject,
		Product:        mockProduct,
		EmailPromotion: mockEmailPromotion,
	}

	svc := NewEmailPromotionService(repo)
	_, err := svc.TriggerPromotion(context.Background(), 1, 100, 200)

	assertServiceError(t, err, ErrCodeInternal, "创建推广记录失败")
	mockOrder.AssertExpectations(t)
	mockProject.AssertExpectations(t)
	mockEmailPromotion.AssertExpectations(t)
	mockProduct.AssertExpectations(t)
}

// --- Tests for GetStatus ---

func TestGetStatus_NotFound(t *testing.T) {
	mockOrder := new(MockOrderRepo)
	mockProject := new(MockProjectRepo)
	mockProduct := new(MockProductRepo)
	mockEmailPromotion := new(MockEmailPromotionRepo)

	mockEmailPromotion.On("GetByID", mock.Anything, 999).Return(nil, nil)

	repo := &repository.Repository{
		Order:          mockOrder,
		Project:        mockProject,
		Product:        mockProduct,
		EmailPromotion: mockEmailPromotion,
	}

	svc := NewEmailPromotionService(repo)
	_, err := svc.GetStatus(context.Background(), 1, 999)

	assertServiceError(t, err, ErrCodeNotFound, "推广记录不存在")
	mockEmailPromotion.AssertExpectations(t)
}

func TestGetStatus_Forbidden(t *testing.T) {
	mockOrder := new(MockOrderRepo)
	mockProject := new(MockProjectRepo)
	mockProduct := new(MockProductRepo)
	mockEmailPromotion := new(MockEmailPromotionRepo)

	mockEmailPromotion.On("GetByID", mock.Anything, 1).Return(&models.EmailPromotion{ID: 1, CreatorID: 999}, nil)

	repo := &repository.Repository{
		Order:          mockOrder,
		Project:        mockProject,
		Product:        mockProduct,
		EmailPromotion: mockEmailPromotion,
	}

	svc := NewEmailPromotionService(repo)
	_, err := svc.GetStatus(context.Background(), 1, 1)

	assertServiceError(t, err, ErrCodeForbidden, "无权查看此推广记录")
	mockEmailPromotion.AssertExpectations(t)
}

func TestGetStatus_Success(t *testing.T) {
	mockOrder := new(MockOrderRepo)
	mockProject := new(MockProjectRepo)
	mockProduct := new(MockProductRepo)
	mockEmailPromotion := new(MockEmailPromotionRepo)

	expectedPromotion := &models.EmailPromotion{
		ID:            1,
		CreatorID:     1,
		OrderID:       100,
		ProjectID:     200,
		MaxRecipients: 50,
		Status:        models.EmailPromotionStatusCompleted,
	}

	mockEmailPromotion.On("GetByID", mock.Anything, 1).Return(expectedPromotion, nil)

	repo := &repository.Repository{
		Order:          mockOrder,
		Project:        mockProject,
		Product:        mockProduct,
		EmailPromotion: mockEmailPromotion,
	}

	svc := NewEmailPromotionService(repo)
	promotion, err := svc.GetStatus(context.Background(), 1, 1)

	require.NoError(t, err)
	assert.Equal(t, 1, promotion.ID)
	assert.Equal(t, models.EmailPromotionStatusCompleted, promotion.Status)
	mockEmailPromotion.AssertExpectations(t)
}

func TestGetStatus_RepoError(t *testing.T) {
	mockOrder := new(MockOrderRepo)
	mockProject := new(MockProjectRepo)
	mockProduct := new(MockProductRepo)
	mockEmailPromotion := new(MockEmailPromotionRepo)

	mockEmailPromotion.On("GetByID", mock.Anything, 1).Return(nil, errors.New("db error"))

	repo := &repository.Repository{
		Order:          mockOrder,
		Project:        mockProject,
		Product:        mockProduct,
		EmailPromotion: mockEmailPromotion,
	}

	svc := NewEmailPromotionService(repo)
	_, err := svc.GetStatus(context.Background(), 1, 1)

	assertServiceError(t, err, ErrCodeInternal, "获取推广记录失败")
	mockEmailPromotion.AssertExpectations(t)
}

// --- Tests for ListByCreator ---

func TestListByCreator_Success(t *testing.T) {
	mockOrder := new(MockOrderRepo)
	mockProject := new(MockProjectRepo)
	mockProduct := new(MockProductRepo)
	mockEmailPromotion := new(MockEmailPromotionRepo)

	expectedPromotions := []models.EmailPromotion{
		{ID: 1, CreatorID: 1},
		{ID: 2, CreatorID: 1},
	}

	mockEmailPromotion.On("ListByCreatorID", mock.Anything, 1, 1, 10).Return(expectedPromotions, int64(2), nil)

	repo := &repository.Repository{
		Order:          mockOrder,
		Project:        mockProject,
		Product:        mockProduct,
		EmailPromotion: mockEmailPromotion,
	}

	svc := NewEmailPromotionService(repo)
	promotions, total, err := svc.ListByCreator(context.Background(), 1, 1, 10)

	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, promotions, 2)
	mockEmailPromotion.AssertExpectations(t)
}

func TestListByCreator_RepoError(t *testing.T) {
	mockOrder := new(MockOrderRepo)
	mockProject := new(MockProjectRepo)
	mockProduct := new(MockProductRepo)
	mockEmailPromotion := new(MockEmailPromotionRepo)

	mockEmailPromotion.On("ListByCreatorID", mock.Anything, 1, 1, 10).Return(nil, int64(0), errors.New("db error"))

	repo := &repository.Repository{
		Order:          mockOrder,
		Project:        mockProject,
		Product:        mockProduct,
		EmailPromotion: mockEmailPromotion,
	}

	svc := NewEmailPromotionService(repo)
	_, _, err := svc.ListByCreator(context.Background(), 1, 1, 10)

	assertServiceError(t, err, ErrCodeInternal, "获取推广记录失败")
	mockEmailPromotion.AssertExpectations(t)
}

func TestListByCreator_Empty(t *testing.T) {
	mockOrder := new(MockOrderRepo)
	mockProject := new(MockProjectRepo)
	mockProduct := new(MockProductRepo)
	mockEmailPromotion := new(MockEmailPromotionRepo)

	mockEmailPromotion.On("ListByCreatorID", mock.Anything, 1, 1, 10).Return(nil, int64(0), nil)

	repo := &repository.Repository{
		Order:          mockOrder,
		Project:        mockProject,
		Product:        mockProduct,
		EmailPromotion: mockEmailPromotion,
	}

	svc := NewEmailPromotionService(repo)
	promotions, total, err := svc.ListByCreator(context.Background(), 1, 1, 10)

	require.NoError(t, err)
	assert.Equal(t, int64(0), total)
	assert.Nil(t, promotions)
	mockEmailPromotion.AssertExpectations(t)
}

// --- Test Helper ---

func assertServiceError(t *testing.T, err error, expectedCode ErrorCode, expectedMsg string) {
	t.Helper()
	require.Error(t, err)
	svcErr, ok := err.(*ServiceError)
	require.True(t, ok, "expected *ServiceError, got %T: %v", err, err)
	assert.Equal(t, expectedCode, svcErr.Code)
	assert.Equal(t, expectedMsg, svcErr.Message)
}
