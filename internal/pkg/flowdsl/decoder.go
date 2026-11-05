package flowdsl

import (
	"encoding/json"

	"github.com/samber/lo"
)

// decodeNodePayload 解析节点前端载荷并填充强类型业务配置。
// 承担 ACL 防腐层职责：历史遗留画布字段（如 approved、is_timing 等）在此处统一归一化为现代领域模型，
// 对外彻底隐藏旧数据兼容细节，保证核心领域对象干净正交。
func decodeNodePayload(raw []byte, node *Node) {
	if len(raw) == 0 || node == nil {
		return
	}

	// 1. 提取所有原始键值对作为通用 Props 备查
	if err := json.Unmarshal(raw, &node.Props); err != nil {
		return
	}

	// 2. 提取节点名称
	if node.Name == "" {
		node.Name, _ = lo.Coalesce(
			castString(node.Props["name"]),
			castString(node.Props["label"]),
			castString(node.Props["title"]),
		)
	}

	// 3. 校准节点类型（若外部类型为空或通用占位符，且内部显式声明了 type，则推导更新）
	isGenericType := lo.Contains([]NodeType{"", "node", "custom-node"}, node.Type)
	if rawType := castString(node.Props["type"]); rawType != "" && isGenericType {
		node.Type = NormalizeNodeType(rawType)
	}

	// 4. 根据标准类型反序列化并自动升级归一化
	switch node.Type {
	case NodeUserTask:
		var prop rawUserProp
		if err := json.Unmarshal(raw, &prop); err == nil {
			node.UserTask = prop.toUserTaskConfig()
		}

	case NodeAutoTask:
		var prop rawAutoProp
		if err := json.Unmarshal(raw, &prop); err == nil {
			node.AutoTask = prop.toAutoTaskConfig()
		}

	case NodeChatGroup:
		var cfg ChatGroupConfig
		if err := json.Unmarshal(raw, &cfg); err == nil {
			node.ChatGroup = &cfg
		}

	case NodeStart:
		var cfg StartConfig
		if err := json.Unmarshal(raw, &cfg); err == nil {
			node.Start = &cfg
		}
	}
}

func castString(val any) string {
	s, _ := val.(string)
	return s
}

// ─────────────────────────────────────────────────────────────────────────────
// 内部防腐辅助结构（不对外暴露，确保老数据在反序列化期平滑升级）
// ─────────────────────────────────────────────────────────────────────────────

type rawUserProp struct {
	Name          string     `json:"name"`
	Approved      []string   `json:"approved"`
	Rule          Rule       `json:"rule"`
	Type          Rule       `json:"type"`
	TemplateField string     `json:"template_field"`
	Assignees     []Assignee `json:"assignees"`
	IsCosigned    bool       `json:"is_cosigned"`
	IsCC          bool       `json:"is_cc"`
	Fields        []Field    `json:"fields"`
}

func (p *rawUserProp) toUserTaskConfig() *UserTaskConfig {
	cfg := &UserTaskConfig{
		Fields:     p.Fields,
		IsCosigned: p.IsCosigned,
		IsCC:       p.IsCC,
	}

	if len(p.Assignees) > 0 {
		cfg.Assignees = lo.Map(p.Assignees, func(a Assignee, _ int) Assignee {
			if a.Rule == "" && len(a.Values) > 0 {
				a.Rule = APPOINT
			}
			return a
		})
		return cfg
	}

	// 历史旧数据平滑升级: approved / template_field / rule / type
	rule, _ := lo.Coalesce(p.Rule, p.Type)
	if rule == "" && len(p.Approved) > 0 {
		rule = APPOINT
	}
	values := lo.Ternary(rule == TEMPLATE, []string{p.TemplateField}, p.Approved)
	if rule != "" || len(values) > 0 {
		cfg.Assignees = []Assignee{
			{Rule: rule, Values: values},
		}
	}
	return cfg
}

type rawAutoProp struct {
	CodebookID         int64           `json:"codebook_id"`
	RunnerID           int64           `json:"runner_id"`
	NotifyMethod       []int64         `json:"notify_method"`
	IsNotify           bool            `json:"is_notify"`
	Schedule           *ScheduleConfig `json:"schedule"`
	CompensationNodeID string          `json:"compensation_node_id"`

	// 历史遗留定时调度字段
	IsTiming      bool   `json:"is_timing"`
	ExecMethod    string `json:"exec_method"`
	Quantity      int64  `json:"quantity"`
	Unit          uint8  `json:"unit"`
	TemplateId    int64  `json:"template_id"`
	TemplateField string `json:"template_field"`
}

func (p *rawAutoProp) toAutoTaskConfig() *AutoTaskConfig {
	cfg := &AutoTaskConfig{
		CodebookID:         p.CodebookID,
		RunnerID:           p.RunnerID,
		NotifyMethod:       p.NotifyMethod,
		IsNotify:           p.IsNotify,
		Schedule:           p.Schedule,
		CompensationNodeID: p.CompensationNodeID,
	}

	if cfg.Schedule != nil {
		return cfg
	}

	// 历史定时配置平滑升级为标准 ScheduleConfig
	if p.IsTiming {
		switch p.ExecMethod {
		case "hand":
			var unit ScheduleUnit = ScheduleUnitMinute
			if p.Unit == 2 {
				unit = ScheduleUnitHour
			} else if p.Unit == 3 {
				unit = ScheduleUnitDay
			}
			cfg.Schedule = &ScheduleConfig{
				Type: ScheduleDelay,
				Source: ScheduleSource{
					Type:  ScheduleSourceFixed,
					Value: p.Quantity,
				},
				Unit: unit,
			}
		case "template":
			cfg.Schedule = &ScheduleConfig{
				Type: ScheduleDelay,
				Source: ScheduleSource{
					Type:       ScheduleSourceTemplateField,
					TemplateID: p.TemplateId,
					Field:      p.TemplateField,
				},
				Unit: ScheduleUnitHour,
			}
		}
	} else {
		cfg.Schedule = &ScheduleConfig{Type: ScheduleImmediate}
	}

	return cfg
}
