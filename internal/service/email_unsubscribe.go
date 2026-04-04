package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

// EmailUnsubscribeService handles email unsubscribe business logic.
type EmailUnsubscribeService struct {
	repo *repository.Repository
}

// NewEmailUnsubscribeService creates a new EmailUnsubscribeService.
func NewEmailUnsubscribeService(repo *repository.Repository) *EmailUnsubscribeService {
	return &EmailUnsubscribeService{repo: repo}
}

// Unsubscribe decodes token and updates user's opt-out status.
func (s *EmailUnsubscribeService) Unsubscribe(ctx context.Context, token string) error {
	if token == "" {
		return ErrBadRequest("无效的退订链接")
	}

	userID, err := decodeUnsubscribeToken(token)
	if err != nil {
		return ErrBadRequest("退订链接已失效或无效")
	}

	if err := s.repo.User.SetEmailOptOut(ctx, userID, true); err != nil {
		return ErrInternal("退订失败，请稍后重试")
	}

	return nil
}

// GenerateUnsubscribeToken generates an unsubscribe token for a user.
// Token format: base64(userID:timestamp:signature), valid for 30 days.
func GenerateUnsubscribeToken(userID int) string {
	timestamp := time.Now().Unix()
	data := fmt.Sprintf("%d:%d", userID, timestamp)

	secretKey := getSecretKey()
	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write([]byte(data))
	sig := hex.EncodeToString(mac.Sum(nil))[:16]

	payload := data + ":" + sig
	return base64.URLEncoding.EncodeToString([]byte(payload))
}

// decodeUnsubscribeToken decodes and verifies an unsubscribe token.
func decodeUnsubscribeToken(token string) (int, error) {
	decoded, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return 0, fmt.Errorf("invalid token encoding")
	}

	parts := strings.Split(string(decoded), ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid token format")
	}

	userIDStr := parts[0]
	timestampStr := parts[1]
	providedSig := parts[2]

	// Verify signature
	data := userIDStr + ":" + timestampStr
	secretKey := getSecretKey()
	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write([]byte(data))
	expectedSig := hex.EncodeToString(mac.Sum(nil))[:16]

	if !hmac.Equal([]byte(providedSig), []byte(expectedSig)) {
		return 0, fmt.Errorf("invalid signature")
	}

	// Verify timestamp (30 day validity)
	timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid timestamp")
	}

	tokenTime := time.Unix(timestamp, 0)
	if time.Since(tokenTime) > 30*24*time.Hour {
		return 0, fmt.Errorf("token expired")
	}

	userID, err := strconv.Atoi(userIDStr)
	if err != nil {
		return 0, fmt.Errorf("invalid user id")
	}

	return userID, nil
}

func getSecretKey() string {
	key := os.Getenv("JWT_SECRET")
	if key == "" {
		key = "kuaizu-default-secret-key"
	}
	return key
}
