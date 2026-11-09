package admin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/kuaizu-team/kuaizu-service/internal/cooperationfeishu"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/service"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func cooperationContract(t *testing.T) *openapi3.T {
	t.Helper()
	entry, err := os.ReadFile("openapi.yaml")
	require.NoError(t, err)
	var root struct {
		Paths map[string]struct {
			Ref string `yaml:"$ref"`
		}
	}
	require.NoError(t, yaml.Unmarshal(entry, &root))
	rawPaths, err := os.ReadFile("paths/campus_cooperation.yaml")
	require.NoError(t, err)
	var fragments map[string]any
	// Assemble these JSON path fragments in memory: kin-openapi otherwise treats
	// the fragment named "tags" as the external document's OpenAPI tags metadata.
	require.NoError(t, json.Unmarshal([]byte(strings.ReplaceAll(string(rawPaths), "../../common/campus_cooperation.yaml#/schemas/", "#/components/schemas/")), &fragments))
	paths := map[string]any{}
	for path, fragment := range map[string]string{
		"/commercial-orders": "orders", "/campus-cooperations/{id}": "detail", "/campus-cooperations/{id}/feishu-sync": "feishuSync",
	} {
		require.Equal(t, "./paths/campus_cooperation.yaml#/"+fragment, root.Paths[path].Ref)
		paths[path] = fragments[fragment]
	}
	rawCommon, err := os.ReadFile("../common/campus_cooperation.yaml")
	require.NoError(t, err)
	var common map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.ReplaceAll(string(rawCommon), "#/schemas/", "#/components/schemas/")), &common))
	data, err := json.Marshal(map[string]any{
		"openapi": "3.0.3", "info": map[string]any{"title": "Cooperation contract", "version": "1"}, "paths": paths,
		"components": map[string]any{"schemas": common["schemas"], "securitySchemes": map[string]any{"bearerAuth": map[string]any{"type": "http", "scheme": "bearer"}}},
	})
	require.NoError(t, err)
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(data)
	require.NoError(t, err)
	return doc
}

func contractJSON(t *testing.T, value any) any {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	var out any
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

func TestCooperationContractResolvesAndValidates(t *testing.T) {
	doc := cooperationContract(t)
	require.NoError(t, doc.Validate(context.Background()))

	input := doc.Components.Schemas["CommercialOrderInput"].Value
	for _, template := range []json.RawMessage{nil, json.RawMessage(`null`), json.RawMessage(`{"title":"合作模板","introduction":"简介","links":[]}`)} {
		require.NoError(t, input.VisitJSON(contractJSON(t, service.CommercialOrderInput{
			Name: "商单", CompanyName: "公司", Status: "ended", Template: template,
		})))
	}
	require.Error(t, input.VisitJSON(map[string]any{"name": "商单", "companyName": "公司", "status": "unknown"}))
	for _, template := range []any{map[string]any{}, map[string]any{"title": "标题", "introduction": "简介", "tags": []any{"重复", "重复"}}} {
		require.Error(t, input.VisitJSON(map[string]any{"name": "商单", "companyName": "公司", "template": template}))
	}

	orderSchema := doc.Components.Schemas["CommercialOrder"].Value
	order := models.CommercialOrder{ID: 3, Name: "商单", CompanyName: "公司", Status: "ended"}
	// Platform scope returns null; a school owner's scope returns a concrete list.
	require.NoError(t, orderSchema.VisitJSON(contractJSON(t, order)))
	order.ClaimedSchoolIDs = []int{42}
	require.NoError(t, orderSchema.VisitJSON(contractJSON(t, order)))

	campus := doc.Components.Schemas["CampusCooperation"].Value
	for _, key := range []string{"orderStatus", "orderTemplate", "feishuWikiNodeToken"} {
		require.Contains(t, campus.Properties, key)
	}
	require.NoError(t, campus.Properties["orderTemplate"].Value.VisitJSON(nil))
	require.NoError(t, campus.Properties["orderStatus"].Value.VisitJSON("ended"))
	orders := doc.Paths.Find("/commercial-orders").Get
	for _, parameter := range orders.Parameters {
		if parameter.Value.Name == "status" {
			require.NoError(t, parameter.Value.Schema.Value.VisitJSON("ended"))
			return
		}
	}
	t.Fatal("commercial order status parameter missing")
}

func TestFeishuContractMatchesActualStatusResponses(t *testing.T) {
	doc := cooperationContract(t)
	path := doc.Paths.Find("/campus-cooperations/{id}/feishu-sync")
	require.NotNil(t, path)
	for _, operation := range []*openapi3.Operation{path.Get, path.Post} {
		require.NotNil(t, operation)
		require.Nil(t, operation.RequestBody)
		require.Len(t, operation.Parameters, 1)
		require.Equal(t, "id", operation.Parameters[0].Value.Name)
		require.NotEmpty(t, *operation.Security)
		response := operation.Responses.Status(http.StatusOK).Value.Content["application/json"].Schema.Value
		for _, status := range []string{"idle", "running", "succeeded", "failed", "needs_attention"} {
			target := cooperationfeishu.Target{CooperationID: 9, Status: status}
			if status == "needs_attention" {
				target.Retryable = true
				target.WikiURL, target.BaseURL = "https://example.feishu.cn/wiki/order", "https://example.feishu.cn/wiki/base"
				now := time.Now()
				target.SyncedAt, target.UpdatedAt = &now, now
			}
			require.NoError(t, response.VisitJSON(map[string]any{"code": 200, "message": "success", "data": contractJSON(t, target)}))
		}
		require.Error(t, response.Properties["data"].Value.VisitJSON(map[string]any{"cooperationId": 9, "status": "unknown", "message": "", "syncedAt": nil, "updatedAt": "2026-11-09T00:00:00Z"}))
	}
}
