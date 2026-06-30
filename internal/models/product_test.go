package models

import "testing"

func TestProductToVOIncludesConfigJSON(t *testing.T) {
	config := `{"scene":"olive_branch_sms_notice"}`
	product := &Product{ID: 12, Name: "短信通知", Type: ProductTypeBenefit, Price: 1, ConfigJSON: &config}

	vo := product.ToVO()

	if vo.ConfigJson == nil || *vo.ConfigJson != config {
		t.Fatalf("ConfigJson = %v, want %q", vo.ConfigJson, config)
	}
}
