package cooperationfeishu

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplicationValuesOnlyUseCapturedCardFields(t *testing.T) {
	card := `{"nickname":"同学","phone":"13800000000","skills":["Go","设计"],"unrelatedSecret":"must-not-export"}`
	values, err := (application{ID: 5, UserID: 7, AttemptNo: 2, Status: "pending", Profile: &card}).values()
	require.NoError(t, err)
	require.Equal(t, "13800000000", values["电话"])
	require.Equal(t, "未处理", values["申请结果"])
	require.NotContains(t, values, "unrelatedSecret")
}
