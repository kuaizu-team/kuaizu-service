package messagecenter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSubmitProjectPromotionSerializesEmptyRecipientList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/marketing/project-promotion" {
			t.Fatalf("path = %s, want /api/v2/marketing/project-promotion", r.URL.Path)
		}
		var payload map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		raw, ok := payload["recipientUserIds"]
		if !ok {
			t.Fatal("recipientUserIds was omitted")
		}
		var recipientUserIDs []int
		if err := json.Unmarshal(raw, &recipientUserIDs); err != nil {
			t.Fatalf("decode recipientUserIds: %v", err)
		}
		if recipientUserIDs == nil || len(recipientUserIDs) != 0 {
			t.Fatalf("recipientUserIds = %#v, want non-nil empty list", recipientUserIDs)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 200,
			"data": map[string]interface{}{
				"taskId": "PROJECT_PROMOTION:9001", "promotionId": 71,
				"requestedCount": 2, "actualCount": 0,
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", 0)
	_, err := client.SubmitProjectPromotion(context.Background(), ProjectPromotionRequest{
		ProjectID: 42, PromotionCount: 2, CreatorUserID: 1001, OrderID: 9001,
	})
	if err != nil {
		t.Fatalf("SubmitProjectPromotion returned error: %v", err)
	}
}

func TestSubmitProjectPromotionRejectsRecipientCountAbovePaidCount(t *testing.T) {
	client := NewClient("http://127.0.0.1", "test-token", 0)
	_, err := client.SubmitProjectPromotion(context.Background(), ProjectPromotionRequest{
		ProjectID: 42, PromotionCount: 1, CreatorUserID: 1001, OrderID: 9001,
		RecipientUserIDs: []int{10, 11},
	})
	if err == nil || err.Error() != "recipientUserIds exceeds promotionCount" {
		t.Fatalf("error = %v, want recipient count validation error", err)
	}
}

func TestSubmitSmsNoticePreservesRejectedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/sms/send" {
			t.Fatalf("path = %s, want /api/v2/sms/send", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    200,
			"message": "success",
			"data": map[string]interface{}{
				"accepted": false,
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", 0)
	resp, err := client.SubmitSmsNotice(context.Background(), SmsNoticeRequest{
		TraceID: "OLIVE_BRANCH_SMS:115", NoticeID: 15, OrderID: 115,
		SenderUserID: 1, ReceiverUserID: 2, OliveBranchRecordID: 3,
		Content: "hello",
	})
	if err != nil {
		t.Fatalf("SubmitSmsNotice returned error: %v", err)
	}
	if resp.Accepted == nil || *resp.Accepted {
		t.Fatalf("response = %+v, want accepted=false", resp)
	}
}
func TestSubmitApplicationSmsUsesApprovedTemplateVariables(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/sms/application" {
			t.Fatalf("path = %s, want /api/v2/sms/application", r.URL.Path)
		}
		var req struct {
			TemplateCode string `json:"templateCode"`
			BusinessTag  string `json:"businessTag"`
			TaskKey      string `json:"taskKey"`
			TraceID      string `json:"traceId"`
			Retry        bool   `json:"retry"`
			Recipients   []struct {
				Phone string                 `json:"phone"`
				Vars  map[string]interface{} `json:"vars"`
			} `json:"recipients"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.TemplateCode != "PROJECT_APPLICATION_REJECTED" || req.BusinessTag != "project_application_sms_rejected" {
			t.Fatalf("request = %+v", req)
		}
		if req.TaskKey != "PROJECT_APPLICATION_SMS:12:rejected" || req.TraceID != "PROJECT_APPLICATION_SMS:12" || !req.Retry {
			t.Fatalf("recovery identity = %+v", req)
		}
		if len(req.Recipients) != 1 || req.Recipients[0].Phone != "13800138000" {
			t.Fatalf("recipients = %+v", req.Recipients)
		}
		vars := req.Recipients[0].Vars
		if vars["nickname"] != "小明" || vars["projectTitle"] != "测试项目" || vars["teamRole"] != "项目负责人" {
			t.Fatalf("vars = %+v", vars)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", 0)
	err := client.SubmitApplicationSms(context.Background(), ApplicationSmsRequest{
		TaskKey: "PROJECT_APPLICATION_SMS:12:rejected", TemplateCode: "PROJECT_APPLICATION_REJECTED",
		Phone: "13800138000", Nickname: "小明", ProjectTitle: "测试项目", TeamRole: "项目负责人",
		BusinessTag: "project_application_sms_rejected", TraceID: "PROJECT_APPLICATION_SMS:12", Retry: true,
	})
	if err != nil {
		t.Fatalf("SubmitApplicationSms returned error: %v", err)
	}
}

func TestSubmitApplicationSmsUsesLongerTimeoutThanDefaultClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(80 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", 10*time.Millisecond)
	err := client.SubmitApplicationSms(context.Background(), ApplicationSmsRequest{
		TaskKey: "PROJECT_APPLICATION_SMS:12:rejected", TemplateCode: "PROJECT_APPLICATION_REJECTED",
		Phone: "13800138000", Nickname: "小明", ProjectTitle: "测试项目", TeamRole: "项目负责人",
		BusinessTag: "project_application_sms_rejected", TraceID: "PROJECT_APPLICATION_SMS:12",
	})
	if err != nil {
		t.Fatalf("SubmitApplicationSms returned timeout error: %v", err)
	}
}

func TestSendAdminSms(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/admin/sms/send" {
			t.Fatalf("path = %s, want /api/v2/admin/sms/send", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization = %q", got)
		}
		var req AdminSmsSendRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.TemplateKey != "URGE_PROCESS" || req.UserID != 1001 || req.Variables["nickname"] != "张三" {
			t.Fatalf("request = %+v", req)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    200,
			"message": "success",
			"data": map[string]interface{}{
				"success":         true,
				"template_key":    "URGE_PROCESS",
				"user_id":         1001,
				"record_id":       12,
				"provider":        "aliyun_sms",
				"provider_biz_id": "xxx",
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", 0)
	resp, err := client.SendAdminSms(context.Background(), AdminSmsSendRequest{
		TemplateKey: "URGE_PROCESS",
		UserID:      1001,
		Variables:   map[string]interface{}{"nickname": "张三"},
	})
	if err != nil {
		t.Fatalf("SendAdminSms returned error: %v", err)
	}
	if !resp.Success || resp.RecordID != 12 || resp.ProviderBizID != "xxx" {
		t.Fatalf("response = %+v", resp)
	}
}

func TestSendRegisterInviteSmsUsesTemplateVariables(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/sms/register-invite" {
			t.Fatalf("path = %s, want /api/v2/sms/register-invite", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization = %q", got)
		}
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if _, ok := req["teamrole"]; !ok {
			t.Fatalf("teamrole missing from request: %+v", req)
		}
		if req["phone"] != "13857007558" {
			t.Fatalf("phone = %v, want 13857007558", req["phone"])
		}
		if _, ok := req["teamRole"]; ok {
			t.Fatalf("unexpected teamRole key in request: %+v", req)
		}
		if req["projectTitle"] != "项目名称" || req["teamrole"] != "团队成员" {
			t.Fatalf("request = %+v", req)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    200,
			"message": "success",
			"data": map[string]interface{}{
				"provider":      "aliyun_sms",
				"providerBizId": "biz-1",
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", 0)
	resp, err := client.SendRegisterInviteSms(context.Background(), RegisterInviteSmsRequest{
		TraceID:      "REGISTER_INVITE:1",
		RecordID:     1,
		Phone:        "13857007558",
		ProjectID:    2,
		ProjectTitle: "项目名称",
		TeamRole:     "团队成员",
	})
	if err != nil {
		t.Fatalf("SendRegisterInviteSms returned error: %v", err)
	}
	if resp.Provider != "aliyun_sms" || resp.ProviderBizID != "biz-1" {
		t.Fatalf("response = %+v", resp)
	}
}

func TestCountAdminSms(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/admin/sms/send-count" {
			t.Fatalf("path = %s, want /api/v2/admin/sms/send-count", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization = %q", got)
		}
		q := r.URL.Query()
		if q.Get("user_id") != "1001" || q.Get("template_key") != "INVITE_SUPER_ADMIN" || q.Get("days") != "30" {
			t.Fatalf("query = %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    200,
			"message": "success",
			"data": map[string]interface{}{
				"user_id":      1001,
				"template_key": "INVITE_SUPER_ADMIN",
				"days":         30,
				"count":        3,
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", 0)
	resp, err := client.CountAdminSms(context.Background(), 1001, "INVITE_SUPER_ADMIN", 30)
	if err != nil {
		t.Fatalf("CountAdminSms returned error: %v", err)
	}
	if resp.Count != 3 || resp.TemplateKey != "INVITE_SUPER_ADMIN" {
		t.Fatalf("response = %+v", resp)
	}
}
