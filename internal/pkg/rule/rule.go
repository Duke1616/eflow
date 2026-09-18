package rule

import (
	"github.com/mitchellh/mapstructure"
)

type Rule struct {
	Type         string                 `json:"type"`
	Field        string                 `json:"field"`
	Title        string                 `json:"title"`
	Style        map[string]interface{} `json:"style"`
	Props        map[string]interface{} `json:"props"`
	NotifyHidden bool                   `json:"notify_hidden"`
	Children     []Rule                 `json:"children"`
	Options      []Options              `json:"options"`
}

type Options struct {
	Label string      `json:"label"`
	Value interface{} `json:"value"`
}

type Rules []Rule

// FlattenRules 扁平化处理函数，排除 type 为 "col" 和 "fcRow" 的规则本身，但保留它们的子规则
func FlattenRules(rules []Rule) []Rule {
	return flatten(rules, nil)
}

func flatten(rules []Rule, res []Rule) []Rule {
	for _, rule := range rules {
		if rule.Type == "col" || rule.Type == "fcRow" {
			if len(rule.Children) > 0 {
				res = flatten(rule.Children, res)
			}
			continue
		}

		res = append(res, Rule{
			Type:         rule.Type,
			Field:        rule.Field,
			Title:        rule.Title,
			Style:        rule.Style,
			Props:        rule.Props,
			NotifyHidden: rule.NotifyHidden,
			Options:      rule.Options,
		})

		if len(rule.Children) > 0 {
			res = flatten(rule.Children, res)
		}
	}
	return res
}

// ParseRules 解析模版字段
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

const (
	SystemProvide = 1
	WechatProvide = 2
)

type Data struct {
	Provide  uint8
	OderData map[string]interface{}
}
