package rule

import (
	"github.com/mitchellh/mapstructure"
)

// Rule 单个表单渲染与校验控件规格模型
type Rule struct {
	Type     string                 `json:"type"`
	Field    string                 `json:"field"`
	Title    string                 `json:"title"`
	Style    map[string]interface{} `json:"style"`
	Children []Rule                 `json:"children"`
	Options  []Options              `json:"options"`
}

// Options 下拉单选等辅助选项值
type Options struct {
	Label string      `json:"label"`
	Value interface{} `json:"value"`
}

// FlattenRules 将多级嵌套的树状表单控件结构扁平化为一维切片 (递归处理)
func FlattenRules(rules []Rule) []Rule {
	var flattened []Rule
	flatten(rules, &flattened)
	return flattened
}

func flatten(rules []Rule, res *[]Rule) {
	for _, rule := range rules {
		if rule.Type == "col" || rule.Type == "fcRow" {
			if len(rule.Children) > 0 {
				flatten(rule.Children, res)
			}
			continue
		}

		*res = append(*res, Rule{
			Type:    rule.Type,
			Field:   rule.Field,
			Title:   rule.Title,
			Style:   rule.Style,
			Options: rule.Options,
		})

		if len(rule.Children) > 0 {
			flatten(rule.Children, res)
		}
	}
}

// ParseRules 反序列化接口类型的 rules 并扁平化解析输出 Rule 数据集
func ParseRules(ruleData interface{}) ([]Rule, error) {
	var rules []Rule
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &rules,
		TagName: "json",
	})
	if err != nil {
		return nil, err
	}

	if err = decoder.Decode(ruleData); err != nil {
		return nil, err
	}

	return FlattenRules(rules), nil
}
