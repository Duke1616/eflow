package rule

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestValidateOptionValues 覆盖写入期选项值校验矩阵，并同时反向锚定渲染侧的键约束：
// 被拒绝的取值必须真的会让 GetFields panic（拒绝有必要），放行的取值必须渲染安全（不过度拒绝）。
func TestValidateOptionValues(t *testing.T) {
	testCases := []struct {
		name    string
		rules   string
		wantErr string
	}{
		{
			name:  "字符串选项值放行",
			rules: `[{"type":"select","field":"env","title":"环境","options":[{"label":"内部","value":"internal"}]}]`,
		},
		{
			name:  "数字与布尔选项值放行",
			rules: `[{"type":"select","field":"num","title":"数量","options":[{"label":"一","value":1},{"label":"是","value":true}]}]`,
		},
		{
			name:  "空值选项放行",
			rules: `[{"type":"select","field":"num","title":"数量","options":[{"label":"未选","value":null}]}]`,
		},
		{
			name:  "无选项控件放行",
			rules: `[{"type":"input","field":"purpose","title":"用途"}]`,
		},
		{
			name:  "空规则集放行",
			rules: `[]`,
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
			name:    "混合选项中只要有一个不可哈希即拒绝",
			rules:   `[{"type":"select","field":"mix","title":"混合","options":[{"label":"好","value":"ok"},{"label":"坏","value":[1]}]}]`,
			wantErr: "mix",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var rulesData []map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(testCase.rules), &rulesData))

			rules, err := ParseRules(rulesData)
			require.NoError(t, err)

			err = ValidateOptionValues(rules)
			if testCase.wantErr == "" {
				require.NoError(t, err)
				require.NotPanics(t, func() {
					GetFields(rules, SystemProvide, map[string]interface{}{})
				})
				return
			}
			require.ErrorContains(t, err, testCase.wantErr)
			require.Panics(t, func() {
				GetFields(rules, SystemProvide, map[string]interface{}{})
			})
		})
	}
}
