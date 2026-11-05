package easyflow

import (
	"encoding/json"
	"testing"

	"github.com/Bunny3th/easy-workflow/workflow/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToEasyVariables(t *testing.T) {
	testCases := []struct {
		name       string
		variables  map[string]any
		starter    string
		ticketId   int64
		assertFunc func(t *testing.T, rawJSON string)
	}{
		{
			name: "正常字典映射并且包含starter与ticket_id",
			variables: map[string]any{
				"amount": 100,
				"reason": "办公用品采购",
			},
			starter:  "ZhangSan",
			ticketId: 1001,
			assertFunc: func(t *testing.T, rawJSON string) {
				var vars []model.Variable
				err := json.Unmarshal([]byte(rawJSON), &vars)
				require.NoError(t, err)

				m := make(map[string]string)
				for _, v := range vars {
					m[v.Key] = v.Value
				}
				assert.Equal(t, "100", m["amount"])
				assert.Equal(t, "办公用品采购", m["reason"])
				assert.Equal(t, "ZhangSan", m["starter"])
				assert.Equal(t, "1001", m["ticket_id"])
			},
		},
		{
			name: "变量中已存在starter时不重复追加",
			variables: map[string]any{
				"starter": "custom_user",
			},
			starter:  "ZhangSan",
			ticketId: 0,
			assertFunc: func(t *testing.T, rawJSON string) {
				var vars []model.Variable
				err := json.Unmarshal([]byte(rawJSON), &vars)
				require.NoError(t, err)

				starterCount := 0
				for _, v := range vars {
					if v.Key == "starter" {
						starterCount++
						assert.Equal(t, "custom_user", v.Value)
					}
				}
				assert.Equal(t, 1, starterCount)
			},
		},
		{
			name:      "全空时返回空切片JSON",
			variables: nil,
			starter:   "",
			ticketId:  0,
			assertFunc: func(t *testing.T, rawJSON string) {
				assert.Equal(t, "[]", rawJSON)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			vars := ensureEssentialVariables(tc.variables, tc.starter, tc.ticketId)
			raw := toEasyVariables(vars)
			tc.assertFunc(t, raw)
		})
	}
}
