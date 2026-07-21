package service

import (
	"context"
	"testing"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPushRetryRejectsAnotherUsersOrder(t *testing.T) {
	repo := &repository.Repository{
		Order: smsNoticeOrderRepoStub{
			order: &models.Order{
				ID: 52, UserID: 2000, Status: models.OrderStatusPaid,
			},
		},
	}
	svc := NewPushRetryService(repo, nil, nil)

	order, err := svc.Retry(context.Background(), 1000, 52)

	require.Nil(t, order)
	var serviceErr *ServiceError
	require.ErrorAs(t, err, &serviceErr)
	assert.Equal(t, ErrCodeForbidden, serviceErr.Code)
}

func TestPushRetryRequiresAtomicFailedToPendingTransition(t *testing.T) {
	repo := &repository.Repository{
		Order: smsNoticeOrderRepoStub{
			order: &models.Order{
				ID: 52, UserID: 1000, Status: models.OrderStatusPaid,
			},
		},
	}
	svc := NewPushRetryService(repo, nil, nil)

	order, err := svc.Retry(context.Background(), 1000, 52)

	require.Nil(t, order)
	var serviceErr *ServiceError
	require.ErrorAs(t, err, &serviceErr)
	assert.Equal(t, ErrCodeBadRequest, serviceErr.Code)
	assert.Equal(t, "仅发送失败的订单可以重试", serviceErr.Message)
}
