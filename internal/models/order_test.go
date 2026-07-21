package models

import "testing"

func TestOrderToVOSanitizesPushErrorMessage(t *testing.T) {
	status := "failed"
	rawMessage := "unexpected status 500: provider internal response"

	vo := (&Order{PushStatus: &status, PushErrorMessage: &rawMessage}).ToVO()
	if vo.PushErrorMessage == nil {
		t.Fatal("failed push should expose a user-safe error message")
	}
	if *vo.PushErrorMessage != "消息发送失败，请稍后重试或申请退款" {
		t.Fatalf("unexpected user-safe message: %q", *vo.PushErrorMessage)
	}
	if *vo.PushErrorMessage == rawMessage {
		t.Fatal("raw provider error must not be exposed through OrderVO")
	}
}
