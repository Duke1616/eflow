package flowdsl

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/samber/lo"
)

// ExecutionTrace 描述工作流实例在运行过程中的实际执行轨迹
type ExecutionTrace struct {
	// TraversedEdges 记录已流转通过的有向边映射: SourceNodeID -> []TargetNodeID
	TraversedEdges map[string][]string `json:"traversed_edges"`
	// NodeStatusMap 记录各节点的实际流转状态: NodeID -> Status (5 表示分支条件未满足被跳过)
	NodeStatusMap map[string]int `json:"node_status_map"`
}

// EdgePriority 定义高亮连线的图层渲染优先级 (值越大，在画布中的 Z-Index 越高，置于数组末尾避免被普通连线覆盖)
const (
	EdgePriorityNormal  = 0 // 普通未流转连线（灰色）
	EdgePrioritySkipped = 1 // 因分支网关未命中跳过的连线（虚线/浅灰）
	EdgePriorityPass    = 2 // 正常流转通过的高亮连线（红线/绿线点亮）
)

// HighlightGraph 纯内存图高亮算法：装饰统一标准 Graph 模型。
// 依据执行轨迹标记连线的 IsPass / IsSkipped 状态，并调整 Edges 数组顺序提升高亮连线渲染优先级。
func HighlightGraph(graph *Graph, trace ExecutionTrace) *Graph {
	if graph == nil || len(graph.Edges) == 0 {
		return graph
	}

	for i := range graph.Edges {
		edge := &graph.Edges[i]
		isPass, isSkipped := evaluateEdgeState(edge.Source, edge.Target, trace)
		edge.IsPass = isPass
		edge.IsSkipped = isSkipped
	}

	// 稳定排序：确保高优先级的连线位于数组后部
	sort.SliceStable(graph.Edges, func(i, j int) bool {
		return getEdgePriority(graph.Edges[i].IsPass, graph.Edges[i].IsSkipped) <
			getEdgePriority(graph.Edges[j].IsPass, graph.Edges[j].IsSkipped)
	})

	graph.BuildIndex()
	return graph
}

// HighlightCanvas 自动探测前端画布类型（LogicFlow 或 AntV X6），在原始画布数据结构上注入高亮与跳过标记并优化层级顺序。
func HighlightCanvas(rawCanvas map[string]any, trace ExecutionTrace) (map[string]any, error) {
	if rawCanvas == nil {
		return nil, nil
	}

	// 深拷贝避免对外部传入数据产生不可预期的原地修改副作用
	canvasBytes, err := json.Marshal(rawCanvas)
	if err != nil {
		return nil, fmt.Errorf("序列化画布数据失败: %w", err)
	}

	var cloned map[string]any
	if err = json.Unmarshal(canvasBytes, &cloned); err != nil {
		return nil, fmt.Errorf("深拷贝画布数据失败: %w", err)
	}

	// 1. 判断是否为 AntV X6 画布（特征：包含顶层 cells 字段）
	if _, ok := cloned["cells"]; ok {
		highlightX6Canvas(cloned, trace)
		return cloned, nil
	}

	// 2. 默认判定为 LogicFlow 画布（特征：包含 edges 字段）
	if _, ok := cloned["edges"]; ok {
		highlightLogicFlowCanvas(cloned, trace)
		return cloned, nil
	}

	return cloned, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// 内部算法辅助函数
// ─────────────────────────────────────────────────────────────────────────────

// evaluateEdgeState 判断指定 Source -> Target 连线是否通过或被跳过
func evaluateEdgeState(source, target string, trace ExecutionTrace) (isPass bool, isSkipped bool) {
	if len(trace.TraversedEdges) == 0 {
		return false, false
	}

	targets, ok := trace.TraversedEdges[source]
	if !ok || !lo.Contains(targets, target) {
		return false, false
	}

	// 检查节点状态：
	// 1. 常规节点：Source 或 Target 状态为 5 (跳过)
	// 2. 网关直连场景：引擎在底层注入了虚拟系统代理节点 proxy_{Source}_{Target}，检查其状态是否为 5 (跳过)
	sourceStatus := trace.NodeStatusMap[source]
	targetStatus := trace.NodeStatusMap[target]
	proxyID := fmt.Sprintf("proxy_%s_%s", source, target)
	proxyStatus := trace.NodeStatusMap[proxyID]

	if sourceStatus == 5 || targetStatus == 5 || proxyStatus == 5 {
		return false, true
	}

	return true, false
}

// getEdgePriority 计算连线渲染优先级
func getEdgePriority(isPass, isSkipped bool) int {
	if isPass {
		return EdgePriorityPass
	}
	if isSkipped {
		return EdgePrioritySkipped
	}
	return EdgePriorityNormal
}

// highlightLogicFlowCanvas 装饰 LogicFlow 画布结构
func highlightLogicFlowCanvas(canvas map[string]any, trace ExecutionTrace) {
	edgesRaw, ok := canvas["edges"].([]any)
	if !ok || len(edgesRaw) == 0 {
		return
	}

	type edgeItem struct {
		data     map[string]any
		priority int
	}

	items := make([]edgeItem, 0, len(edgesRaw))
	for _, edgeObj := range edgesRaw {
		edgeMap, isMap := edgeObj.(map[string]any)
		if !isMap {
			continue
		}

		source, _ := edgeMap["sourceNodeId"].(string)
		target, _ := edgeMap["targetNodeId"].(string)

		props, hasProps := edgeMap["properties"].(map[string]any)
		if !hasProps || props == nil {
			props = make(map[string]any)
		}

		isPass, isSkipped := evaluateEdgeState(source, target, trace)
		if isPass {
			props["is_pass"] = true
			delete(props, "is_skipped")
		} else if isSkipped {
			props["is_skipped"] = true
			delete(props, "is_pass")
		}

		edgeMap["properties"] = props
		items = append(items, edgeItem{
			data:     edgeMap,
			priority: getEdgePriority(isPass, isSkipped),
		})
	}

	// 排序：高优先级连线置于末尾（LogicFlow 表现为后定义的线条层级更高）
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].priority < items[j].priority
	})

	canvas["edges"] = lo.Map(items, func(item edgeItem, _ int) any {
		return item.data
	})
}

// highlightX6Canvas 装饰 AntV X6 画布结构
func highlightX6Canvas(canvas map[string]any, trace ExecutionTrace) {
	cellsRaw, ok := canvas["cells"].([]any)
	if !ok || len(cellsRaw) == 0 {
		return
	}

	type cellItem struct {
		data     map[string]any
		priority int
	}

	items := make([]cellItem, 0, len(cellsRaw))
	for _, cellObj := range cellsRaw {
		cellMap, isMap := cellObj.(map[string]any)
		if !isMap {
			continue
		}

		shape, _ := cellMap["shape"].(string)
		sourceID := extractX6EndpointID(cellMap["source"])
		targetID := extractX6EndpointID(cellMap["target"])

		isEdge := shape == "edge" || shape == "bpmn-edge" || (sourceID != "" && targetID != "")
		if !isEdge {
			items = append(items, cellItem{data: cellMap, priority: EdgePriorityNormal})
			continue
		}

		isPass, isSkipped := evaluateEdgeState(sourceID, targetID, trace)

		// X6 的属性一般存放于 data 或 properties 中
		updateX6EdgePayload(cellMap, isPass, isSkipped)

		items = append(items, cellItem{
			data:     cellMap,
			priority: getEdgePriority(isPass, isSkipped),
		})
	}

	// 排序：节点保持相对原位，高亮 Edge 置于尾部提升渲染图层
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].priority < items[j].priority
	})

	canvas["cells"] = lo.Map(items, func(item cellItem, _ int) any {
		return item.data
	})
}

// extractX6EndpointID 提取 X6 端点关联的节点 ID
func extractX6EndpointID(endpoint any) string {
	switch v := endpoint.(type) {
	case string:
		return v
	case map[string]any:
		if cellID, ok := v["cell"].(string); ok && cellID != "" {
			return cellID
		}
		if portID, ok := v["port"].(string); ok && portID != "" {
			return portID
		}
	}
	return ""
}

// updateX6EdgePayload 更新 X6 边中保存的流转高亮标记
func updateX6EdgePayload(cell map[string]any, isPass, isSkipped bool) {
	targetKey := "data"
	payload, ok := cell["data"].(map[string]any)
	if !ok || payload == nil {
		if props, hasProps := cell["properties"].(map[string]any); hasProps && props != nil {
			payload = props
			targetKey = "properties"
		} else {
			payload = make(map[string]any)
		}
	}

	if isPass {
		payload["is_pass"] = true
		delete(payload, "is_skipped")
	} else if isSkipped {
		payload["is_skipped"] = true
		delete(payload, "is_pass")
	}

	cell[targetKey] = payload
}
