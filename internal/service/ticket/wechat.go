package ticket

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Duke1616/eflow/internal/event"
	"github.com/samber/lo"
	"github.com/xen0n/go-workwx"
)

// wechatOaData 将工单动态数据反序列化为企微 OA 审批详情模型
func wechatOaData(data map[string]interface{}) (workwx.OAApprovalDetail, error) {
	wechatOaJson, err := json.Marshal(data)
	if err != nil {
		return workwx.OAApprovalDetail{}, fmt.Errorf("序列化企微表单数据失败: %w", err)
	}

	var wechatOaDetail workwx.OAApprovalDetail
	err = json.Unmarshal(wechatOaJson, &wechatOaDetail)
	if err != nil {
		return workwx.OAApprovalDetail{}, fmt.Errorf("反序列化企微审批详情失败: %w", err)
	}

	return wechatOaDetail, nil
}

// convert 将企微 OA 审批数据内容解析映射为统一的工作流事件变量列表
func convert(data []event.Variables, oaData workwx.OAApprovalDetail) []event.Variables {
	oaVars := lo.FilterMap(oaData.ApplyData.Contents, func(c workwx.OAContent, _ int) (event.Variables, bool) {
		val, ok := extractOAContentValue(c)
		if !ok {
			return event.Variables{}, false
		}
		return event.Variables{
			Key:   cleanControlID(c.ID),
			Value: val,
		}, true
	})
	return append(data, oaVars...)
}

func cleanControlID(rawID string) string {
	if idx := strings.Index(rawID, "-"); idx >= 0 && idx < len(rawID)-1 {
		return rawID[idx+1:]
	}
	return rawID
}

func extractOAContentValue(c workwx.OAContent) (any, bool) {
	switch c.Control {
	case "Textarea":
		return c.Value.Text, c.Value.Text != ""
	case "Selector":
		return extractSelectorValue(c.Value.Selector)
	default:
		return nil, false
	}
}

func extractSelectorValue(s workwx.OAContentSelector) (any, bool) {
	switch s.Type {
	case "single":
		if len(s.Options) > 0 && len(s.Options[0].Value) > 0 {
			return s.Options[0].Value[0].Text, true
		}
	case "multi":
		vals := lo.FilterMap(s.Options, func(opt workwx.OAContentSelectorOption, _ int) (string, bool) {
			if len(opt.Value) == 0 {
				return "", false
			}
			return opt.Value[0].Text, true
		})
		if len(vals) > 0 {
			return vals, true
		}
	}
	return nil, false
}
