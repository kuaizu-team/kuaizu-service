package wechat

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateSubscribeValue(t *testing.T) {
	tests := []struct {
		name        string
		templateKey string
		value       string
		wantErr     bool
	}{
		{name: "thing at limit", templateKey: "thing1", value: strings.Repeat("中", 20)},
		{name: "thing too long", templateKey: "thing1", value: strings.Repeat("中", 21), wantErr: true},
		{name: "phrase at limit", templateKey: "phrase2", value: "互相了解中"},
		{name: "phrase too long", templateKey: "phrase2", value: "已进入互相了解", wantErr: true},
		{name: "valid time", templateKey: "time2", value: "23:59"},
		{name: "datetime is not time", templateKey: "time2", value: "2026-08-01 23:59", wantErr: true},
		{name: "valid number", templateKey: "number1", value: "90.5"},
		{name: "invalid number", templateKey: "number1", value: "九十", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateSubscribeValue(test.templateKey, test.value)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateSubscribeValue(%q, %q) error = %v, wantErr=%v", test.templateKey, test.value, err, test.wantErr)
			}
		})
	}
}

func TestSendByConfigRejectsMissingBusinessFieldBeforeCallingWechat(t *testing.T) {
	client := &Client{}
	err := client.SendByConfig("openid", "template", `{"remark":"thing1"}`, map[string]string{}, "")
	var payloadErr SubscribePayloadError
	if !errors.As(err, &payloadErr) {
		t.Fatalf("SendByConfig() error = %v, want SubscribePayloadError", err)
	}
}
