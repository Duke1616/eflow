package flowx

import (
	"fmt"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/mitchellh/mapstructure"
)

// 节点类型常量，与前端 LogicFlow 画布元素类型对应
const (
	NodeTypeStart     = "start"
	NodeTypeEnd       = "end"
	NodeTypeUser      = "user"
	NodeTypeCondition = "condition" // 兼容旧条件网关
	NodeTypeParallel  = "parallel"  // 并行网关
	NodeTypeInclusion = "inclusion" // 包容网关
	NodeTypeSelective = "selective" // 排他网关
	NodeTypeAuto      = "automation"
	NodeTypeChat      = "chat"
)

// 节点默认名称
const (
	DefaultNameStart     = "开始"
	DefaultNameEnd       = "结束"
	DefaultNameUser      = "人工审批"
	DefaultNameParallel  = "并行网关"
	DefaultNameInclusion = "包容网关"
	DefaultNameSelective = "排他网关"
)

// IConverter 流程画布编译器通用契约
type IConverter interface {
	// Convert 将前端 LogicFlow 画布拓扑数据编译为 flowx 强类型流程定义
	Convert(procKey, procName string, version int, flow domain.LogicFlow) (model.ProcessDefinition, error)
}

// INodeConverter 单个画布节点类型转换器策略接口
type INodeConverter interface {
	// Type 声明支持处理的 LogicFlow 节点类型
	Type() string
	// Convert 将单个 LogicFlow 节点转为 flowx 的 NodeSpec 规格定义
	Convert(ctx *ConvertContext, raw Node) (model.NodeSpec, error)
}

// Node LogicFlow 原始节点结构体
type Node struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Properties any    `json:"properties"`
}

// Edge LogicFlow 原始连线结构体
type Edge struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	SourceNodeId string `json:"sourceNodeId"`
	TargetNodeId string `json:"targetNodeId"`
	Properties   any    `json:"properties"`
}

// EdgeProperty 连线属性定义
type EdgeProperty struct {
	Name       string `json:"name"`
	Expression string `json:"expression"` // 跳转逻辑表达式 (如 "$days > 3")
}

// ConvertContext 画布编译全局上下文
type ConvertContext struct {
	ProcKey  string
	ProcName string
	Version  int
	NodesMap map[string]Node
	OutEdges map[string][]Edge // sourceNodeID -> []Edge
	InEdges  map[string][]Edge // targetNodeID -> []Edge
	AllNodes []Node
	AllEdges []Edge
}

// ParseNodes 从原始 interface 切片反序列化 LogicFlow 节点切片
func ParseNodes(raw any) ([]Node, error) {
	var nodes []Node
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &nodes,
		TagName: "json",
	})
	if err != nil {
		return nil, fmt.Errorf("创建节点解码器失败: %w", err)
	}
	if err = decoder.Decode(raw); err != nil {
		return nil, fmt.Errorf("解码 LogicFlow 节点失败: %w", err)
	}
	return nodes, nil
}

// ParseEdges 从原始 interface 切片反序列化 LogicFlow 连线切片
func ParseEdges(raw any) ([]Edge, error) {
	var edges []Edge
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &edges,
		TagName: "json",
	})
	if err != nil {
		return nil, fmt.Errorf("创建连线解码器失败: %w", err)
	}
	if err = decoder.Decode(raw); err != nil {
		return nil, fmt.Errorf("解码 LogicFlow 连线失败: %w", err)
	}
	return edges, nil
}

// DecodeProperty 辅助泛型工具：安全反序列化节点或连线的 Properties
func DecodeProperty[T any](raw any) (T, error) {
	var target T
	if raw == nil {
		return target, nil
	}
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &target,
		TagName: "json",
	})
	if err != nil {
		return target, fmt.Errorf("创建属性解码器失败: %w", err)
	}
	if err = decoder.Decode(raw); err != nil {
		return target, fmt.Errorf("解码属性失败: %w", err)
	}
	return target, nil
}
