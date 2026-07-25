package rule

import (
	"encoding/json"
	"io/ioutil"
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

	print(card)
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
