package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// MockTalentProfileRepo — 实现 repository.TalentProfileRepo 接口
// ---------------------------------------------------------------------------

type MockTalentProfileRepo struct {
	mock.Mock
}

func (m *MockTalentProfileRepo) List(ctx context.Context, params repository.TalentProfileListParams) ([]models.TalentProfile, int64, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]models.TalentProfile), args.Get(1).(int64), args.Error(2)
}

func (m *MockTalentProfileRepo) ListAdmin(ctx context.Context, params repository.TalentProfileAdminListParams) ([]models.TalentProfile, int64, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]models.TalentProfile), args.Get(1).(int64), args.Error(2)
}

func (m *MockTalentProfileRepo) GetByID(ctx context.Context, id int) (*models.TalentProfile, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.TalentProfile), args.Error(1)
}

func (m *MockTalentProfileRepo) GetByIDForAdmin(ctx context.Context, id int) (*models.TalentProfile, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.TalentProfile), args.Error(1)
}

func (m *MockTalentProfileRepo) GetByUserID(ctx context.Context, userID int) (*models.TalentProfile, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.TalentProfile), args.Error(1)
}

func (m *MockTalentProfileRepo) Upsert(ctx context.Context, p *models.TalentProfile) error {
	args := m.Called(ctx, p)
	return args.Error(0)
}

func (m *MockTalentProfileRepo) UpdateStatus(ctx context.Context, id int, status int) error {
	args := m.Called(ctx, id, status)
	return args.Error(0)
}

func (m *MockTalentProfileRepo) DeleteByUserID(ctx context.Context, userID int) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

// newTalentProfileSvc 构造带 mock repo 的 TalentProfileService；
// contentAudit 传 nil（三个新方法均不触发内容审核）。
func newTalentProfileSvc(mockRepo *MockTalentProfileRepo) *TalentProfileService {
	repo := &repository.Repository{
		TalentProfile: mockRepo,
	}
	return NewTalentProfileService(repo, nil)
}

// ---------------------------------------------------------------------------
// AdminListTalentProfiles
// ---------------------------------------------------------------------------

func TestAdminListTalentProfiles_Success(t *testing.T) {
	status := models.TalentStatusReviewing
	profiles := []models.TalentProfile{
		{ID: 1, UserID: 101, Status: &status},
		{ID: 2, UserID: 102, Status: &status},
	}
	params := repository.TalentProfileAdminListParams{Page: 1, Size: 10}

	mockRepo := new(MockTalentProfileRepo)
	mockRepo.On("ListAdmin", mock.Anything, params).Return(profiles, int64(2), nil)

	svc := newTalentProfileSvc(mockRepo)
	result, total, err := svc.AdminListTalentProfiles(context.Background(), params)

	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, result, 2)
	mockRepo.AssertExpectations(t)
}

func TestAdminListTalentProfiles_RepoError(t *testing.T) {
	params := repository.TalentProfileAdminListParams{Page: 1, Size: 10}

	mockRepo := new(MockTalentProfileRepo)
	mockRepo.On("ListAdmin", mock.Anything, params).Return(nil, int64(0), errors.New("db error"))

	svc := newTalentProfileSvc(mockRepo)
	_, _, err := svc.AdminListTalentProfiles(context.Background(), params)

	assertServiceError(t, err, ErrCodeInternal, "获取名片列表失败")
	mockRepo.AssertExpectations(t)
}

// ---------------------------------------------------------------------------
// AdminGetTalentProfile
// ---------------------------------------------------------------------------

func TestAdminGetTalentProfile_Success(t *testing.T) {
	status := models.TalentStatusOnline
	expected := &models.TalentProfile{ID: 42, UserID: 101, Status: &status}

	mockRepo := new(MockTalentProfileRepo)
	mockRepo.On("GetByIDForAdmin", mock.Anything, 42).Return(expected, nil)

	svc := newTalentProfileSvc(mockRepo)
	profile, err := svc.AdminGetTalentProfile(context.Background(), 42)

	require.NoError(t, err)
	require.NotNil(t, profile)
	assert.Equal(t, 42, profile.ID)
	mockRepo.AssertExpectations(t)
}

func TestAdminGetTalentProfile_NotFound(t *testing.T) {
	mockRepo := new(MockTalentProfileRepo)
	mockRepo.On("GetByIDForAdmin", mock.Anything, 999).Return(nil, nil)

	svc := newTalentProfileSvc(mockRepo)
	_, err := svc.AdminGetTalentProfile(context.Background(), 999)

	assertServiceError(t, err, ErrCodeNotFound, "名片不存在")
	mockRepo.AssertExpectations(t)
}

func TestAdminGetTalentProfile_RepoError(t *testing.T) {
	mockRepo := new(MockTalentProfileRepo)
	mockRepo.On("GetByIDForAdmin", mock.Anything, 1).Return(nil, errors.New("db error"))

	svc := newTalentProfileSvc(mockRepo)
	_, err := svc.AdminGetTalentProfile(context.Background(), 1)

	assertServiceError(t, err, ErrCodeInternal, "获取名片详情失败")
	mockRepo.AssertExpectations(t)
}

// ---------------------------------------------------------------------------
// TakedownTalentProfile
// ---------------------------------------------------------------------------

func TestTakedownTalentProfile_Success(t *testing.T) {
	// status=1（已上架）→ 下架后变为 status=3（已下架）
	status := models.TalentStatusOnline
	profile := &models.TalentProfile{ID: 10, Status: &status}

	mockRepo := new(MockTalentProfileRepo)
	mockRepo.On("GetByID", mock.Anything, 10).Return(profile, nil)
	mockRepo.On("UpdateStatus", mock.Anything, 10, models.TalentStatusTakenDown).Return(nil)

	svc := newTalentProfileSvc(mockRepo)
	err := svc.TakedownTalentProfile(context.Background(), 10)

	require.NoError(t, err)
	mockRepo.AssertExpectations(t)
}

func TestTakedownTalentProfile_NotFound(t *testing.T) {
	mockRepo := new(MockTalentProfileRepo)
	mockRepo.On("GetByID", mock.Anything, 999).Return(nil, nil)

	svc := newTalentProfileSvc(mockRepo)
	err := svc.TakedownTalentProfile(context.Background(), 999)

	assertServiceError(t, err, ErrCodeNotFound, "名片不存在")
	mockRepo.AssertExpectations(t)
}

func TestTakedownTalentProfile_NotOnline_Private(t *testing.T) {
	// status=0（隐私/已驳回）不能下架
	status := models.TalentStatusPrivate
	profile := &models.TalentProfile{ID: 5, Status: &status}

	mockRepo := new(MockTalentProfileRepo)
	mockRepo.On("GetByID", mock.Anything, 5).Return(profile, nil)

	svc := newTalentProfileSvc(mockRepo)
	err := svc.TakedownTalentProfile(context.Background(), 5)

	assertServiceError(t, err, ErrCodeBadRequest, "当前名片未上架，无法执行下架操作")
	mockRepo.AssertExpectations(t)
}

func TestTakedownTalentProfile_NotOnline_Reviewing(t *testing.T) {
	// status=2（审核中）不能下架，应走审核驳回流程
	status := models.TalentStatusReviewing
	profile := &models.TalentProfile{ID: 6, Status: &status}

	mockRepo := new(MockTalentProfileRepo)
	mockRepo.On("GetByID", mock.Anything, 6).Return(profile, nil)

	svc := newTalentProfileSvc(mockRepo)
	err := svc.TakedownTalentProfile(context.Background(), 6)

	assertServiceError(t, err, ErrCodeBadRequest, "当前名片未上架，无法执行下架操作")
	mockRepo.AssertExpectations(t)
}

func TestTakedownTalentProfile_NotOnline_AlreadyTakenDown(t *testing.T) {
	// status=3（已下架）不能再次下架
	status := models.TalentStatusTakenDown
	profile := &models.TalentProfile{ID: 8, Status: &status}

	mockRepo := new(MockTalentProfileRepo)
	mockRepo.On("GetByID", mock.Anything, 8).Return(profile, nil)

	svc := newTalentProfileSvc(mockRepo)
	err := svc.TakedownTalentProfile(context.Background(), 8)

	assertServiceError(t, err, ErrCodeBadRequest, "当前名片未上架，无法执行下架操作")
	mockRepo.AssertExpectations(t)
}

func TestTakedownTalentProfile_GetRepoError(t *testing.T) {
	mockRepo := new(MockTalentProfileRepo)
	mockRepo.On("GetByID", mock.Anything, 1).Return(nil, errors.New("db error"))

	svc := newTalentProfileSvc(mockRepo)
	err := svc.TakedownTalentProfile(context.Background(), 1)

	assertServiceError(t, err, ErrCodeInternal, "获取名片失败")
	mockRepo.AssertExpectations(t)
}

func TestTakedownTalentProfile_UpdateRepoError(t *testing.T) {
	status := models.TalentStatusOnline
	profile := &models.TalentProfile{ID: 7, Status: &status}

	mockRepo := new(MockTalentProfileRepo)
	mockRepo.On("GetByID", mock.Anything, 7).Return(profile, nil)
	mockRepo.On("UpdateStatus", mock.Anything, 7, models.TalentStatusTakenDown).Return(errors.New("db write error"))

	svc := newTalentProfileSvc(mockRepo)
	err := svc.TakedownTalentProfile(context.Background(), 7)

	assertServiceError(t, err, ErrCodeInternal, "下架失败")
	mockRepo.AssertExpectations(t)
}
