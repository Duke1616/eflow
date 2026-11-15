package template

import (
	"encoding/json"
	"testing"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/stretchr/testify/require"
)

// TestTemplateRulesWriteValidation 创建与更新两条写入路径必须同口径拦截不可哈希的选项值：
// 脏配置一旦落库，通知渲染会在异步协程里 panic，且企微工单同样被连带崩掉。
func TestTemplateRulesWriteValidation(t *testing.T) {
	testCases := []struct {
		name    string
		rules   string
		wantErr string
	}{
		{
			name:  "合法选项值放行",
			rules: `[{"type":"select","field":"env","title":"环境","options":[{"label":"内部","value":"internal"},{"label":"生产","value":2}]}]`,
		},
		{
			name:  "布局嵌套的合法选项值放行",
			rules: `[{"type":"fcRow","children":[{"type":"col","children":[{"type":"select","field":"env","title":"环境","options":[{"label":"内部","value":"internal"}]}]}]}]`,
		},
		{
			name:  "空规则串放行",
			rules: "",
		},
		{
			name:    "数组选项值拒绝",
			rules:   `[{"type":"select","field":"env","title":"环境","options":[{"label":"内部","value":["a","b"]}]}]`,
			wantErr: "env",
		},
		{
			name:    "对象选项值拒绝",
			rules:   `[{"type":"select","field":"env","title":"环境","options":[{"label":"内部","value":{"k":"v"}}]}]`,
			wantErr: "env",
		},
		{
			name:    "布局嵌套子控件的数组选项值拒绝",
			rules:   `[{"type":"fcRow","children":[{"type":"col","children":[{"type":"select","field":"deep","title":"深层","options":[{"label":"X","value":[1]}]}]}]}]`,
			wantErr: "deep",
		},
		{
			name:    "规则 JSON 非法拒绝",
			rules:   `[{"type":`,
			wantErr: "unexpected end of JSON input",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			h := &Handler{}

			created, err := h.toDomain(CreateTemplateReq{Name: "发布申请", Rules: testCase.rules})
			assertRulesWriteResult(t, created.Rules, err, testCase.rules, testCase.wantErr)

			updated, err := h.toUpdateDomain(UpdateTemplateReq{Id: 1, Name: "发布申请", Rules: testCase.rules})
			assertRulesWriteResult(t, updated.Rules, err, testCase.rules, testCase.wantErr)
		})
	}
}

func assertRulesWriteResult(t *testing.T, rules []domain.Rule, err error, raw, wantErr string) {
	t.Helper()
	if wantErr != "" {
		require.ErrorContains(t, err, wantErr)
		require.Empty(t, rules)
		return
	}
	require.NoError(t, err)
	if raw == "" {
		require.Empty(t, rules)
		return
	}
	// 校验只拦截非法配置，不得吞掉或改写合法表单
	gotJSON, err := json.Marshal(rules)
	require.NoError(t, err)
	require.JSONEq(t, raw, string(gotJSON))
}
