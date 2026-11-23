package rule

import (
	"encoding/json"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRule(t *testing.T) {
	// 读取 JSON 文件
	data, err := ioutil.ReadFile("rule.json")
	assert.NoError(t, err)

	// 解析数据
	var rules interface{}
	err = json.Unmarshal(data, &rules)
	assert.NoError(t, err)

	result, err := ParseRules(rules)
	assert.NoError(t, err)

	assert.Len(t, result, 5)
}

func TestField(t *testing.T) {
	// 读取 JSON 文件
	data, err := ioutil.ReadFile("rule.json")
	assert.NoError(t, err)

	// 解析数据
	var rules interface{}
	err = json.Unmarshal(data, &rules)
	assert.NoError(t, err)

	result, err := ParseRules(rules)
	assert.NoError(t, err)
	assert.Len(t, result, 5)

	card := GetFields(result, 1, map[string]interface{}{
		"assets":      []string{"7601628b-3567-472e-af13-4f2c6ad631c4", "b6d7171f-84b1-4778-9f9c-0960cb7b1d42"},
		"environment": "internal",
		"purpose":     "为了荣耀",
		"quantity":    2,
		"role":        1,
	})

	assert.NotEmpty(t, card)
	// 验证带 options 的切片正常解析为 Label (MongoDB, Redis)
	var foundAssets bool
	for _, f := range card {
		if strings.Contains(f.Content, "资产选择") {
			assert.Contains(t, f.Content, "MongoDB, Redis")
			foundAssets = true
		}
	}
	assert.True(t, foundAssets)
}

func TestProcessSliceValueWithoutOptions(t *testing.T) {
	// 测试无 options 的切片（如文件上传 manifest_file 数组）
	rules := []Rule{
		{
			Type:  "upload",
			Field: "manifest_file",
			Title: "提取清单文件",
		},
		{
			Type:  "select",
			Field: "tags",
			Title: "标签",
			Options: []Options{
				{Label: "标签A", Value: "a"},
			},
		},
	}

	data := map[string]interface{}{
		"manifest_file": []string{
			"http://10.31.0.4:3003/api/download-sets/public/manifests/manifest_set_1.json",
			"http://10.31.0.4:3003/api/download-sets/public/manifests/manifest_set_2.json",
		},
		"tags": []string{"a", "b"}, // a 匹配 label，b 回退原值
	}

	fields := GetFields(rules, SystemProvide, data)
	assert.NotEmpty(t, fields)

	var foundManifest, foundTags bool
	for _, f := range fields {
		if strings.Contains(f.Content, "提取清单文件") {
			assert.Contains(t, f.Content, "http://10.31.0.4:3003/api/download-sets/public/manifests/manifest_set_1.json, http://10.31.0.4:3003/api/download-sets/public/manifests/manifest_set_2.json")
			foundManifest = true
		}
		if strings.Contains(f.Content, "标签") {
			assert.Contains(t, f.Content, "标签A, b")
			foundTags = true
		}
	}
	assert.True(t, foundManifest)
	assert.True(t, foundTags)
}

func TestParseRulesPreservesProps(t *testing.T) {
	rules := []map[string]interface{}{{
		"type": "datePicker", "field": "execute_at", "title": "执行时间",
		"props": map[string]interface{}{"type": "datetime"},
	}}

	result, err := ParseRules(rules)
	assert.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "datetime", result[0].Props["type"])
}
func TestFilterNotifyHidden(t *testing.T) {
	rules := []map[string]interface{}{
		{
			"type": "input", "field": "password", "title": "密码",
			"notify_hidden": true,
		},
		{
			"type": "input", "field": "token", "title": "令牌",
			"style": map[string]interface{}{"notify_display": "false"},
		},
		{
			"type": "input", "field": "username", "title": "用户名",
			"notify_hidden": false,
		},
	}

	result, err := ParseRules(rules)
	assert.NoError(t, err)
	assert.Len(t, result, 3)
	assert.True(t, result[0].NotifyHidden)
	assert.False(t, result[2].NotifyHidden)

	data := map[string]interface{}{
		"password": "secret_password",
		"token":    "secret_token",
		"username": "admin",
	}

	fields := GetFields(result, SystemProvide, data)
	assert.Len(t, fields, 1)
	assert.Contains(t, fields[0].Content, "用户名")
	assert.NotContains(t, fields[0].Content, "密码")
	assert.NotContains(t, fields[0].Content, "令牌")
}

func TestFlattenRules(t *testing.T) {
	rules := []Rule{
		{
			Type:  "fcRow",
			Field: "row1",
			Title: "布局行",
			Children: []Rule{
				{
					Type:  "col",
					Field: "col1",
					Title: "布局列1",
					Children: []Rule{
						{
							Type:         "input",
							Field:        "username",
							Title:        "用户名",
							NotifyHidden: false,
						},
					},
				},
				{
					Type:  "col",
					Field: "col2",
					Title: "布局列2",
					Children: []Rule{
						{
							Type:         "input",
							Field:        "password",
							Title:        "密码",
							NotifyHidden: true,
						},
					},
				},
			},
		},
		{
			Type:  "select",
			Field: "role",
			Title: "角色",
		},
	}

	flattened := FlattenRules(rules)
	assert.Len(t, flattened, 3)
	assert.Equal(t, "username", flattened[0].Field)
	assert.False(t, flattened[0].NotifyHidden)
	assert.Equal(t, "password", flattened[1].Field)
	assert.True(t, flattened[1].NotifyHidden)
	assert.Equal(t, "role", flattened[2].Field)
}

func TestProcessWechatFieldsSafe(t *testing.T) {
	// 验证在企微表单的单选未勾选（Options 为空）、Title 为空、控件 ID 无连字符等边界场景下不会发生 Panic
	testData := map[string]interface{}{
		"apply_data": map[string]interface{}{
			"contents": []map[string]interface{}{
				{
					"id":      "Selector-1",
					"title":   []map[string]interface{}{{"text": "单选留空"}},
					"control": "Selector",
					"value": map[string]interface{}{
						"selector": map[string]interface{}{
							"type":    "single",
							"options": []map[string]interface{}{},
						},
					},
				},
				{
					"id":      "no_hyphen_id",
					"title":   []map[string]interface{}{},
					"control": "Textarea",
					"value": map[string]interface{}{
						"text": "文本备注",
					},
				},
				{
					"id":      "Selector-2",
					"title":   []map[string]interface{}{{"text": "多选空值"}},
					"control": "Selector",
					"value": map[string]interface{}{
						"selector": map[string]interface{}{
							"type": "multi",
							"options": []map[string]interface{}{
								{"value": []map[string]interface{}{}},
							},
						},
					},
				},
			},
		},
	}

	fields := GetFields(nil, WechatProvide, testData)
	assert.Len(t, fields, 1)
	assert.Equal(t, "no_hyphen_id", fields[0].Content[2:14])
}

func TestGetFields_ExcludeSystemVariables(t *testing.T) {
	rules := []Rule{
		{Field: "project_name", Title: "项目名称"},
		{Field: "commit_message", Title: "提交信息"},
	}

	data := map[string]interface{}{
		"project_name":   "eflow",
		"commit_message": "fix: 隐藏卡片系统保留变量",
		"starter":        "luankz",
		"template_name":  "产线环境发版",
		"ticket_id":      int64(1130),
		"operator":       "admin",
		"comment":        "同意",
	}

	fields := GetFields(rules, SystemProvide, data)
	assert.NotEmpty(t, fields)

	// 确认正文字段中只包含业务字段，绝不包含 starter、template_name、ticket_id 等系统变量
	for _, f := range fields {
		assert.NotContains(t, f.Content, "starter")
		assert.NotContains(t, f.Content, "luankz")
		assert.NotContains(t, f.Content, "template_name")
		assert.NotContains(t, f.Content, "产线环境发版")
		assert.NotContains(t, f.Content, "ticket_id")
		assert.NotContains(t, f.Content, "1130")
	}

	// 验证业务字段正常呈现
	var hasProject, hasCommit bool
	for _, f := range fields {
		if strings.Contains(f.Content, "项目名称") && strings.Contains(f.Content, "eflow") {
			hasProject = true
		}
		if strings.Contains(f.Content, "提交信息") && strings.Contains(f.Content, "fix: 隐藏卡片系统保留变量") {
			hasCommit = true
		}
	}
	assert.True(t, hasProject)
	assert.True(t, hasCommit)

	// 验证浅拷贝保护：调用方传入的原 map 结构不受影响
	assert.Contains(t, data, "starter")
	assert.Contains(t, data, "ticket_id")
}
