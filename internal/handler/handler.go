package handler

import (
	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/service"
)

// Server implements api.ServerInterface
type Server struct {
	repo *repository.Repository
	svc  *service.Services
}

// Ensure Server implements the generated ServerInterface
var _ api.ServerInterface = (*Server)(nil)

// NewServer creates a new Server instance
func NewServer(repo *repository.Repository, svc *service.Services) *Server {
	return &Server{repo: repo, svc: svc}
}

// GetUserID extracts user ID from context (set by auth middleware)
func GetUserID(ctx interface{ Get(string) interface{} }) int {
	userID, ok := ctx.Get("userID").(int)
	if !ok {
		// This should never happen if JWT middleware is properly configured
		panic("userID not found in context - auth middleware not applied?")
	}
	return userID
}

// GetOpenID extracts OpenID from context (set by auth middleware)
func GetOpenID(ctx interface{ Get(string) interface{} }) string {
	openID, ok := ctx.Get("openID").(string)
	if !ok {
		return ""
	}
	return openID
}
