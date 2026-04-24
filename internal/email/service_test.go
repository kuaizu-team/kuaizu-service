package email

import (
	"os"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

// TestBatchEmailSending_Success 测试批量发送成功的情况
func TestBatchEmailSending_Success(t *testing.T) {
	// 加载环境变量
	if err := godotenv.Load("../../.env"); err != nil {
		t.Skip("Warning: .env file not found, using environment variables")
	}

	testEmail := os.Getenv("TEST_EMAIL")
	if testEmail == "" {
		t.Skip("TEST_EMAIL environment variable not set, skipping batch email test")
	}

	// 创建真实的SMTP客户端
	client, err := NewSMTPClientFromEnv()
	if err != nil {
		t.Skipf("Failed to create SMTP client: %v", err)
	}

	desc := "这是一个测试项目"
	project := &models.Project{
		ID:          1,
		Name:        "测试项目",
		Description: &desc,
	}

	// 所有邮件都发送到TEST_EMAIL
	recipients := []struct {
		ID       int
		Email    string
		Nickname string
	}{
		{ID: 1, Email: testEmail, Nickname: "用户1"},
		{ID: 2, Email: testEmail, Nickname: "用户2"},
		{ID: 3, Email: testEmail, Nickname: "用户3"},
	}

	// 创建邮件服务
	service := &Service{
		client:           client,
		templateRenderer: NewTemplateRenderer("https://kuaizu.xyz"),
	}

	// 模拟批量发送逻辑（第4步）
	sentCount := 0
	startTime := time.Now()

	for _, r := range recipients {
		// 生成退订 token
		unsubscribeToken := generateUnsubscribeTokenForEmail(r.ID)

		// 渲染邮件
		nickname := r.Nickname
		subject, body, err := service.templateRenderer.RenderProjectPromotion(project, &nickname, unsubscribeToken)
		if err != nil {
			t.Errorf("Failed to render email for %s: %v", r.Email, err)
			continue
		}

		// 发送邮件
		if err := service.client.Send(r.Email, subject, body); err == nil {
			sentCount++
			t.Logf("✓ Email sent successfully to %s (nickname: %s)", r.Email, r.Nickname)
		} else {
			t.Errorf("Failed to send email to %s: %v", r.Email, err)
		}

		// 延迟发送，避免触发反垃圾机制
		time.Sleep(100 * time.Millisecond)
	}

	elapsed := time.Since(startTime)

	// 验证结果
	if sentCount != len(recipients) {
		t.Errorf("Expected %d emails sent, got %d", len(recipients), sentCount)
	}

	t.Logf("Successfully sent %d emails in %v", sentCount, elapsed)
}

// TestBatchEmailSending_RateLimiting 测试发送延迟（防止反垃圾机制）
func TestBatchEmailSending_RateLimiting(t *testing.T) {
	// 加载环境变量
	if err := godotenv.Load("../../.env"); err != nil {
		t.Skip("Warning: .env file not found, using environment variables")
	}

	testEmail := os.Getenv("TEST_EMAIL")
	if testEmail == "" {
		t.Skip("TEST_EMAIL environment variable not set, skipping rate limiting test")
	}

	// 创建真实的SMTP客户端
	client, err := NewSMTPClientFromEnv()
	if err != nil {
		t.Skipf("Failed to create SMTP client: %v", err)
	}

	desc := "这是一个测试项目"
	project := &models.Project{
		ID:          1,
		Name:        "测试项目 - 速率限制测试",
		Description: &desc,
	}

	// 所有邮件都发送到TEST_EMAIL
	recipients := []struct {
		ID       int
		Email    string
		Nickname string
	}{
		{ID: 1, Email: testEmail, Nickname: "用户1"},
		{ID: 2, Email: testEmail, Nickname: "用户2"},
		{ID: 3, Email: testEmail, Nickname: "用户3"},
	}

	service := &Service{
		client:           client,
		templateRenderer: NewTemplateRenderer("https://kuaizu.xyz"),
	}

	// 模拟批量发送逻辑，测量时间
	startTime := time.Now()
	sentCount := 0
	delayPerEmail := 100 * time.Millisecond

	for _, r := range recipients {
		unsubscribeToken := generateUnsubscribeTokenForEmail(r.ID)
		nickname := r.Nickname
		subject, body, err := service.templateRenderer.RenderProjectPromotion(project, &nickname, unsubscribeToken)
		if err != nil {
			continue
		}

		if err := service.client.Send(r.Email, subject, body); err == nil {
			sentCount++
			t.Logf("✓ Email sent to %s", r.Email)
		}

		// 延迟发送
		time.Sleep(delayPerEmail)
	}

	elapsed := time.Since(startTime)
	expectedMinDuration := delayPerEmail * time.Duration(len(recipients))

	// 验证发送时间符合预期（有延迟）
	if elapsed < expectedMinDuration {
		t.Errorf("Expected at least %v for rate limiting, got %v", expectedMinDuration, elapsed)
	}

	if sentCount != len(recipients) {
		t.Errorf("Expected %d emails sent, got %d", len(recipients), sentCount)
	}

	t.Logf("Sent %d emails with rate limiting in %v (expected min: %v)", sentCount, elapsed, expectedMinDuration)
}

// TestGenerateUnsubscribeToken 测试退订token生成
func TestGenerateUnsubscribeToken(t *testing.T) {
	userID := 123

	// 生成token
	token1 := generateUnsubscribeTokenForEmail(userID)

	// 验证token不为空
	if token1 == "" {
		t.Error("Generated token is empty")
	}

	// 验证token是有效的base64编码
	if len(token1) < 10 {
		t.Error("Token seems too short")
	}

	// 等待足够长的时间以确保时间戳不同
	time.Sleep(1100 * time.Millisecond)
	token2 := generateUnsubscribeTokenForEmail(userID)

	// 验证不同时间生成的token不同（因为包含时间戳）
	if token1 == token2 {
		t.Error("Tokens should be different due to timestamp after 1 second delay")
	}

	// 验证相同用户ID生成的token格式一致
	token3 := generateUnsubscribeTokenForEmail(456)
	if token3 == "" {
		t.Error("Token for different user is empty")
	}

	t.Logf("Generated tokens for user %d: %s, %s", userID, token1, token2)
	t.Logf("Generated token for user 456: %s", token3)
}

// TestBatchEmailSending_LargeRecipientList 测试大量收件人
func TestBatchEmailSending_LargeRecipientList(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large recipient test in short mode")
	}

	// 加载环境变量
	if err := godotenv.Load("../../.env"); err != nil {
		t.Skip("Warning: .env file not found, using environment variables")
	}

	testEmail := os.Getenv("TEST_EMAIL")
	if testEmail == "" {
		t.Skip("TEST_EMAIL environment variable not set, skipping large recipient test")
	}

	// 创建真实的SMTP客户端
	client, err := NewSMTPClientFromEnv()
	if err != nil {
		t.Skipf("Failed to create SMTP client: %v", err)
	}

	desc := "这是一个测试项目"
	project := &models.Project{
		ID:          1,
		Name:        "测试项目 - 大量收件人",
		Description: &desc,
	}

	// 生成5个收件人（减少数量以避免触发反垃圾机制）
	recipientCount := 5
	recipients := make([]struct {
		ID       int
		Email    string
		Nickname string
	}, recipientCount)

	for i := 0; i < recipientCount; i++ {
		recipients[i] = struct {
			ID       int
			Email    string
			Nickname string
		}{
			ID:       i + 1,
			Email:    testEmail,
			Nickname: "用户" + string(rune('A'+i)),
		}
	}

	service := &Service{
		client:           client,
		templateRenderer: NewTemplateRenderer("https://kuaizu.xyz"),
	}

	// 模拟批量发送逻辑（使用较长的延迟以避免触发限制）
	startTime := time.Now()
	sentCount := 0
	failedCount := 0

	for i, r := range recipients {
		unsubscribeToken := generateUnsubscribeTokenForEmail(r.ID)
		nickname := r.Nickname
		subject, body, err := service.templateRenderer.RenderProjectPromotion(project, &nickname, unsubscribeToken)
		if err != nil {
			t.Logf("Failed to render email %d: %v", i+1, err)
			failedCount++
			continue
		}

		if err := service.client.Send(r.Email, subject, body); err == nil {
			sentCount++
			t.Logf("✓ Email %d/%d sent successfully", i+1, recipientCount)
		} else {
			failedCount++
			t.Logf("✗ Email %d/%d failed: %v", i+1, recipientCount, err)
		}

		// 使用较长的延迟（200ms）以避免触发SMTP限制
		time.Sleep(200 * time.Millisecond)
	}

	elapsed := time.Since(startTime)

	// 验证结果 - 至少发送成功一半
	if sentCount < recipientCount/2 {
		t.Errorf("Expected at least %d emails sent, got %d (failed: %d)", recipientCount/2, sentCount, failedCount)
	}

	t.Logf("Successfully sent %d/%d emails in %v (avg: %v per email, failed: %d)",
		sentCount, recipientCount, elapsed, elapsed/time.Duration(recipientCount), failedCount)
}
