package middleware

import (
	"context"
	"errors"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/labstack/echo/v4"
	"net/http"
	"net/http/httptest"
	"testing"
)

type statusFailUserRepo struct {
	repository.UserRepo
	user *models.User
	err  error
}

func (r *statusFailUserRepo) GetByID(context.Context, int) (*models.User, error) {
	return r.user, r.err
}
func TestStatusLookupFailureCannotEnterBusinessHandler(t *testing.T) {
	for _, tc := range []struct {
		name   string
		user   *models.User
		err    error
		status int
	}{
		{"db failure", nil, errors.New("db down"), 503}, {"missing", nil, nil, 401},
		{"normal", &models.User{}, nil, 204},
		{"banned", &models.User{UserStatus: models.UserStatusBanned}, nil, 403},
		{"graduated", &models.User{UserStatus: models.UserStatusGraduated}, nil, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			rec := httptest.NewRecorder()
			ctx := e.NewContext(httptest.NewRequest("POST", "/business", nil), rec)
			ctx.Set("userID", 7)
			ctx.SetPath("/business")
			entered := false
			h := UserStatusCheck(&repository.Repository{User: &statusFailUserRepo{user: tc.user, err: tc.err}})(func(c echo.Context) error { entered = true; return c.NoContent(http.StatusNoContent) })
			if err := h(ctx); err != nil {
				e.HTTPErrorHandler(err, ctx)
			}
			if rec.Code != tc.status || entered != (tc.status == 204) {
				t.Fatalf("status=%d entered=%v", rec.Code, entered)
			}
		})
	}
}
