package flowdsl

import (
	"encoding/json"
	"fmt"
)

// IParser 前端流程图画布解析器多态行为契约。
// 设计原则：每种前端图设计器（如 LogicFlow、AntV X6、BPMN 等）具备差异化的序列化协议，
// 通过实现该契约将各异的表现层原始报文转换为系统统一的拓扑图契约 IGraph，实现表现层与业务内核彻底解耦。
type IParser interface {
	// Match 判断原始报文特征是否匹配当前画布类型的特征指纹
	Match(payload []byte) bool

	// Parse 执行具体画布协议的逆序列化与拓扑重组，返回标准图契约 IGraph
	Parse(payload []byte) (IGraph, error)
}

// 内置画布解析器链表（不可变只读切片，零并发读写锁竞争，零全局状态副作用）
var defaultParsers = []IParser{
	&x6Parser{},
	&logicFlowParser{},
}

// Parse 自动探测前端画布结构并解析为统一的领域拓扑图契约 IGraph。
// 依据画布结构特征指纹快速分发，零锁争用、零额外间接层。
func Parse(rawData any) (IGraph, error) {
	if rawData == nil {
		return NewGraph("empty"), nil
	}

	payload, err := normalizeToJSONBytes(rawData)
	if err != nil {
		return nil, fmt.Errorf("格式化画布输入数据失败: %w", err)
	}

	if len(payload) == 0 || string(payload) == `""` {
		return NewGraph("empty"), nil
	}

	for _, p := range defaultParsers {
		if p.Match(payload) {
			return p.Parse(payload)
		}
	}

	return nil, fmt.Errorf("未能识别的画布数据格式，请确认画布拓扑包含 LogicFlow (nodes) 或 AntV X6 (cells) 规范")
}

func normalizeToJSONBytes(raw any) ([]byte, error) {
	switch v := raw.(type) {
	case []byte:
		return v, nil
	case string:
		return []byte(v), nil
	default:
		return json.Marshal(v)
	}
}
