package engine

import (
	"context"
	"encoding/json"

	"github.com/Bunny3th/easy-workflow/workflow/database"
	"github.com/Bunny3th/easy-workflow/workflow/engine"
	"github.com/Bunny3th/easy-workflow/workflow/model"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository"
	"github.com/Duke1616/eflow/pkg/easyflow"
	"github.com/ecodeclub/ekit/slice"
	"golang.org/x/sync/errgroup"
)

// IEngine 流程引擎服务接口
type IEngine interface {
	// ListTodoTasks 查看指定用户的待办任务列表，支持按时间排序和分页检索
	ListTodoTasks(ctx context.Context, userId, processName string, sortByAse bool, offset, limit int) ([]domain.Instance, int64, error)
	// ListByStartUser 获取我发起的流程实例历史列表，支持分页
	ListByStartUser(ctx context.Context, userId, processName string, offset, limit int) ([]domain.Instance, int64, error)
	// TaskRecord 获取工单流转的任务节点变更和审批历史全量记录
	TaskRecord(ctx context.Context, processInstId, offset, limit int) ([]model.Task, int64, error)
	// IsReject 判断指定任务是否在先前被驳回过
	IsReject(ctx context.Context, taskId int) (bool, error)
	// GetTasksByCurrentNodeId 获取指定流程实例及当前活动节点下的所有挂起任务
	GetTasksByCurrentNodeId(ctx context.Context, processInstId int, currentNodeId string) ([]model.Task, error)
	// UpdateIsFinishedByPreNodeId 由系统触发更新指定前置节点所有相关审批任务的状态为已完结
	UpdateIsFinishedByPreNodeId(ctx context.Context, processInstId int, nodeId string, status int, comment string) error
	// ForceUpdateIsFinishedByPreNodeId 强制归档清理指定前置节点下的所有任务（无论此前是否已完成）
	ForceUpdateIsFinishedByPreNodeId(ctx context.Context, processInstId int, nodeId string, status int, comment string) error
	// ForceUpdateIsFinishedByNodeId 强制归档清理指定节点 ID 下的所有任务记录（无论此前是否已完成）
	ForceUpdateIsFinishedByNodeId(ctx context.Context, processInstId int, nodeId string, status int, comment string) error
	// Pass 审批通过当前代办的流程节点，推进流程至下一层级
	Pass(ctx context.Context, taskId int, comment string) error
	// ListPendingStepsOfMyTask 列出由当前用户发起的指定流程实例列表中当前仍待处理的步骤信息
	ListPendingStepsOfMyTask(ctx context.Context, processInstIds []int, starter string) ([]domain.Instance, error)
	// GetAutomationTask 获取流程实例中等待自动触发执行的脚本或系统节点任务
	GetAutomationTask(ctx context.Context, currentNodeId string, processInstId int) (model.Task, error)
	// GetTasksByInstUsers 获取属于指定流程实例中待指定用户列表审批的待办任务
	GetTasksByInstUsers(ctx context.Context, processInstId int, userIds []string) ([]model.Task, error)
	// GetOrderIdByVariable 从流程实例的绑定参数中检索关联的业务工单唯一 ID 标识
	GetOrderIdByVariable(ctx context.Context, processInstId int) (string, error)
	// Upstream 获取指定任务节点物理链路上的所有上游节点集合
	Upstream(ctx context.Context, taskId int) ([]model.Node, error)
	// TaskInfo 获取指定审批任务的完整技术详情
	TaskInfo(ctx context.Context, taskId int) (model.Task, error)
	// GetProxyPrevNodeID 获取代理自动流转的前置节点 ID（用于跨网关回退与分支清洗）
	GetProxyPrevNodeID(ctx context.Context, processInstId int, prevNodeID string) (string, error)
	// GetProxyNodeByProcessInstId 根据流程实例 ID 获取唯一的自动流转代理临时节点 ID
	GetProxyNodeByProcessInstId(ctx context.Context, processInstId int) (string, error)
	// GetProxyTaskByProcessInstId 根据流程实例 ID 获取自动流转代理节点任务的完整明细
	GetProxyTaskByProcessInstId(ctx context.Context, processInstId int) (model.Task, error)
	// DeleteProxyNodeByNodeId 删除已推进的代理流转临时节点任务
	DeleteProxyNodeByNodeId(ctx context.Context, processInstId int, nodeId string) error
	// UpdateTaskPrevNodeID 修改流转任务的前置路径节点 ID（用于应对网关的动态调整）
	UpdateTaskPrevNodeID(ctx context.Context, taskId int, prevNodeId string) error
	// GetTraversedEdges 计算并解析流程图中已经点亮、流转完成或已被安全跳过的边拓扑路径映射
	GetTraversedEdges(ctx context.Context, record []model.Task, processInstId, processId int, status uint8) (map[string][]string, error)
	// GetInstanceByID 根据实例 ID 获取流程实例基本详情信息（含历史实例归档）
	GetInstanceByID(ctx context.Context, processInstId int) (domain.Instance, error)
	// GetProcessDefineByVersion 获取指定流程 ID 与特定版本下锁定的完整流程图 DSL 定义
	GetProcessDefineByVersion(ctx context.Context, processID, version int) (model.Process, error)
	// GetLatestProcessVersion 获取指定流程的最新发布生效版本号
	GetLatestProcessVersion(ctx context.Context, processID int) (int, error)
	// CreateSkippedTask 在由于分支条件未被选中而跳过该节点时，自动生成一条已归档跳过状态的任务记录
	CreateSkippedTask(ctx context.Context, processInstId int, nodeId, prevNodeId, comment string, status uint8) error
	// ProcessSave 校验并保存设计的流程定义图 DSL 结构并发布生成新 ID
	ProcessSave(ctx context.Context, process *model.Process) (int, error)
	// Transfer 将特定代办审批节点任务转签/委托予新指定的用户列表共同办理
	Transfer(ctx context.Context, taskId int, userIds []string) ([]model.Task, error)
}

type engineService struct {
	repo repository.IEngineRepository
}

// NewEngineService 初始化流程引擎服务层
func NewEngineService(repo repository.IEngineRepository) IEngine {
	return &engineService{
		repo: repo,
	}
}

func (s *engineService) GetInstanceByID(ctx context.Context, processInstId int) (domain.Instance, error) {
	return s.repo.GetInstanceByID(ctx, processInstId)
}

func (s *engineService) GetProcessDefineByVersion(ctx context.Context, processID, version int) (model.Process, error) {
	return s.repo.GetProcessDefineByVersion(ctx, processID, version)
}

func (s *engineService) GetLatestProcessVersion(ctx context.Context, processID int) (int, error) {
	return s.repo.GetLatestProcessVersion(ctx, processID)
}

func (s *engineService) GetProxyPrevNodeID(ctx context.Context, processInstId int, prevNodeID string) (string, error) {
	procTask, err := s.repo.GetProxyNodeID(ctx, processInstId, prevNodeID)
	return procTask.PrevNodeID, err
}

func (s *engineService) GetProxyNodeByProcessInstId(ctx context.Context, processInstId int) (string, error) {
	procTask, err := s.repo.GetProxyNodeByProcessInstId(ctx, processInstId)
	return procTask.NodeID, err
}

func (s *engineService) GetProxyTaskByProcessInstId(ctx context.Context, processInstId int) (model.Task, error) {
	return s.repo.GetProxyNodeByProcessInstId(ctx, processInstId)
}

func (s *engineService) DeleteProxyNodeByNodeId(ctx context.Context, processInstId int, nodeId string) error {
	return s.repo.DeleteProxyNodeByNodeId(ctx, processInstId, nodeId)
}

func (s *engineService) TaskInfo(ctx context.Context, taskId int) (model.Task, error) {
	return engine.GetTaskInfo(taskId)
}

func (s *engineService) GetTasksByCurrentNodeId(ctx context.Context, processInstId int, currentNodeId string) ([]model.Task, error) {
	return s.repo.GetTasksByCurrentNodeId(ctx, processInstId, currentNodeId)
}

func (s *engineService) Upstream(ctx context.Context, taskId int) ([]model.Node, error) {
	return engine.TaskUpstreamNodeList(taskId)
}

func (s *engineService) GetOrderIdByVariable(ctx context.Context, processInstId int) (string, error) {
	return s.repo.GetOrderIdByVariable(ctx, processInstId)
}

func (s *engineService) GetTasksByInstUsers(ctx context.Context, processInstId int, userIds []string) ([]model.Task, error) {
	return s.repo.GetTasksByInstUsers(ctx, processInstId, userIds)
}

func (s *engineService) GetAutomationTask(ctx context.Context, currentNodeId string, processInstId int) (model.Task, error) {
	return s.repo.GetAutomationTask(ctx, currentNodeId, processInstId)
}

func (s *engineService) ListPendingStepsOfMyTask(ctx context.Context, processInstIds []int, starter string) (
	[]domain.Instance, error) {
	return s.repo.ListTasksByProcInstIds(ctx, processInstIds, starter)
}

func (s *engineService) IsReject(ctx context.Context, taskId int) (bool, error) {
	total, err := s.repo.CountReject(ctx, taskId)
	if total >= 1 {
		return true, err
	}

	return false, err
}

func (s *engineService) UpdateIsFinishedByPreNodeId(ctx context.Context, processInstId int, nodeId string, status int, comment string) error {
	return s.repo.UpdateIsFinishedByPreNodeId(ctx, processInstId, nodeId, status, comment)
}

func (s *engineService) ForceUpdateIsFinishedByPreNodeId(ctx context.Context, processInstId int, nodeId string, status int, comment string) error {
	return s.repo.ForceUpdateIsFinishedByPreNodeId(ctx, processInstId, nodeId, status, comment)
}

func (s *engineService) ForceUpdateIsFinishedByNodeId(ctx context.Context, processInstId int, nodeId string, status int, comment string) error {
	return s.repo.ForceUpdateIsFinishedByNodeId(ctx, processInstId, nodeId, status, comment)
}

func (s *engineService) Pass(ctx context.Context, taskId int, comment string) error {
	return engine.TaskPass(taskId, comment, "", false)
}

func (s *engineService) UpdateTaskPrevNodeID(ctx context.Context, taskId int, prevNodeId string) error {
	return s.repo.UpdateTaskPrevNodeID(ctx, taskId, prevNodeId)
}

func (s *engineService) Transfer(ctx context.Context, taskId int, userIds []string) ([]model.Task, error) {
	return s.repo.Transfer(ctx, taskId, userIds)
}

func (s *engineService) CreateSkippedTask(ctx context.Context, processInstId int, nodeId, prevNodeId, comment string, status uint8) error {
	inst, err := s.repo.GetInstanceByID(ctx, processInstId)
	if err != nil {
		return err
	}

	now := database.LTime.Now()
	var createTime *database.LocalTime
	if inst.CreateTime != nil {
		t := database.LocalTime(*inst.CreateTime)
		createTime = &t
	}

	task := model.Task{
		ProcInstID:         processInstId,
		ProcID:             inst.ProcID,
		BusinessID:         inst.BusinessID,
		Starter:            inst.Starter,
		NodeID:             nodeId,
		NodeName:           easyflow.SysProxyNodeName,
		PrevNodeID:         prevNodeId,
		UserID:             "sys_skipped",
		Status:             int(status),
		IsFinished:         1,
		Comment:            comment,
		CreateTime:         &now,
		FinishedTime:       &now,
		ProcInstCreateTime: createTime,
	}

	return s.repo.CreateSkippedTask(ctx, task)
}

func (s *engineService) TaskRecord(ctx context.Context, processInstId, offset, limit int) ([]model.Task, int64, error) {
	var (
		eg      errgroup.Group
		records []model.Task
		total   int64
	)
	eg.Go(func() error {
		var err error
		records, err = s.repo.ListTaskRecord(ctx, processInstId, offset, limit)
		return err
	})

	eg.Go(func() error {
		var err error
		total, err = s.repo.CountTaskRecord(ctx, processInstId)
		return err
	})
	if err := eg.Wait(); err != nil {
		return records, total, err
	}
	return records, total, nil
}

func (s *engineService) ProcessSave(ctx context.Context, process *model.Process) (int, error) {
	bs, err := json.Marshal(process)
	if err != nil {
		return 0, err
	}

	return engine.ProcessSave(string(bs), process.ProcessName)
}

func (s *engineService) ListTodoTasks(ctx context.Context, userId, processName string, sortByAse bool, offset, limit int) (
	[]domain.Instance, int64, error) {
	var (
		eg    errgroup.Group
		ts    []domain.Instance
		total int64
	)
	eg.Go(func() error {
		var err error
		ts, err = s.repo.TodoList(userId, processName, sortByAse, offset, limit)
		return err
	})

	eg.Go(func() error {
		var err error
		total, err = s.repo.CountTodo(ctx, userId, processName)
		return err
	})
	if err := eg.Wait(); err != nil {
		return ts, total, err
	}
	return ts, total, nil
}

func (s *engineService) ListByStartUser(ctx context.Context, userId, processName string, offset,
	limit int) ([]domain.Instance, int64, error) {

	var (
		eg    errgroup.Group
		ts    []domain.Instance
		total int64
	)
	eg.Go(func() error {
		var err error
		ts, err = s.repo.ListStartUser(ctx, userId, processName, offset, limit)
		return err
	})

	eg.Go(func() error {
		var err error
		total, err = s.repo.CountStartUser(ctx, userId, processName)
		return err
	})
	if err := eg.Wait(); err != nil {
		return ts, total, err
	}
	return ts, total, nil
}

func (s *engineService) GetTraversedEdges(ctx context.Context, record []model.Task, processInstId, processId int, status uint8) (map[string][]string, error) {
	if len(record) == 0 {
		var err error
		record, _, err = s.TaskRecord(ctx, processInstId, 0, 1000)
		if err != nil {
			return nil, err
		}
	}

	inst, err := s.repo.GetInstanceByID(ctx, processInstId)
	if err != nil {
		return nil, err
	}

	define, err := s.repo.GetProcessDefineByVersion(ctx, processId, inst.ProcVersion)
	if err != nil {
		return nil, err
	}

	var rootID string
	nodesMap := slice.ToMap(define.Nodes, func(element model.Node) string {
		if element.NodeType == model.RootNode {
			rootID = element.NodeID
		}
		return element.NodeID
	})
	if rootID == "" && len(record) > 0 {
		rootID = record[0].NodeID
	}

	analyzer := NewNodeStatusAnalyzer(record, nodesMap)
	topology := NewGraphTopologyService(nodesMap, s)

	litEdges := make(map[string][]string)

	for _, task := range record {
		s.recursiveReset(task.NodeID, litEdges, nodesMap)

		if task.Status != 2 && task.Status != 3 && task.PrevNodeID != "" {
			if analyzer.IsBatchTainted(task.NodeID, task.BatchCode) {
				continue
			}

			logicalPrevID := topology.ResolveLogicalPrev(task.PrevNodeID)
			path := topology.FindPath(logicalPrevID, task.NodeID, s)

			if len(path) > 1 {
				for i := 0; i < len(path)-1; i++ {
					uniqueAppend(litEdges, path[i], path[i+1])
				}
			}
		}

		if analyzer.IsBatchEffectivelyPassed(task) {
			if analyzer.HasNewerBatchPending(task.NodeID, task.BatchCode) {
				continue
			}

			nextNodes := topology.ResolveNextNodes(task.NodeID)
			for _, nextID := range nextNodes {
				s.lightUpForward(task.NodeID, nextID, litEdges, nodesMap, topology.effectiveGraph)
			}
		} else if task.Status == 5 {
			nextNodes := topology.ResolveNextNodes(task.NodeID)
			for _, nextID := range nextNodes {
				s.lightUpForward(task.NodeID, nextID, litEdges, nodesMap, topology.effectiveGraph)
			}
		}
	}

	finalEdges := make(map[string][]string)

	if rootID != "" {
		queue := []string{rootID}
		visitedNode := make(map[string]bool)
		visitedNode[rootID] = true

		for len(queue) > 0 {
			curr := queue[0]
			queue = queue[1:]

			targets := litEdges[curr]
			if len(targets) > 0 {
				finalEdges[curr] = targets
				for _, t := range targets {
					if !visitedNode[t] {
						visitedNode[t] = true
						queue = append(queue, t)
					}
				}
			}
		}
	} else {
		finalEdges = litEdges
	}

	return finalEdges, nil
}

func (s *engineService) findPathThroughGateways(startID, endID string,
	effectiveGraph map[string][]string, nodesMap map[string]model.Node) []string {

	type path struct {
		nodes []string
	}

	queue := []path{{nodes: []string{startID}}}
	visited := map[string]bool{startID: true}

	for len(queue) > 0 {
		currPath := queue[0]
		queue = queue[1:]

		currNodeID := currPath.nodes[len(currPath.nodes)-1]

		if currNodeID == endID {
			return currPath.nodes
		}

		if len(currPath.nodes) > 20 {
			continue
		}

		nextIDs := effectiveGraph[currNodeID]
		for _, nextID := range nextIDs {
			isTarget := nextID == endID
			isGateway := false
			if node, ok := nodesMap[nextID]; ok && node.NodeType == model.GateWayNode {
				isGateway = true
			}

			if (isTarget || isGateway) && !visited[nextID] {
				visited[nextID] = true
				newNodes := make([]string, len(currPath.nodes))
				copy(newNodes, currPath.nodes)
				newNodes = append(newNodes, nextID)

				if isTarget {
					return newNodes
				}
				queue = append(queue, path{nodes: newNodes})
			}
		}
	}
	return nil
}

func (s *engineService) recursiveReset(nodeID string, litEdges map[string][]string, nodesMap map[string]model.Node) {
	targets, ok := litEdges[nodeID]
	if !ok {
		return
	}
	delete(litEdges, nodeID)

	for _, tid := range targets {
		tNode, exists := nodesMap[tid]
		if exists && tNode.NodeType == model.GateWayNode {
			s.recursiveReset(tid, litEdges, nodesMap)
		}
	}
}

func (s *engineService) lightUpForward(sourceID, targetID string, litEdges map[string][]string,
	nodesMap map[string]model.Node, effectiveGraph map[string][]string) {

	uniqueAppend(litEdges, sourceID, targetID)

	targetNode, ok := nodesMap[targetID]
	if ok && targetNode.NodeType == model.GateWayNode {
		nextIDs := effectiveGraph[targetID]

		inDegree := len(targetNode.PrevNodeIDs)
		outDegree := len(nextIDs)
		waitType := targetNode.GWConfig.WaitForAllPrevNode

		if waitType == 1 && inDegree > 1 {
			return
		}

		if waitType == 0 {
			return
		}

		if waitType == 3 && outDegree > 1 {
			return
		}

		for _, nextID := range nextIDs {
			s.lightUpForward(targetID, nextID, litEdges, nodesMap, effectiveGraph)
		}
	}
}

func (s *engineService) getLogicalPrevID(rawPrevID string, nodesMap map[string]model.Node) string {
	node, ok := nodesMap[rawPrevID]
	if !ok {
		return rawPrevID
	}

	if s.isProxyNode(node) {
		if len(node.PrevNodeIDs) > 0 {
			return s.getLogicalPrevID(node.PrevNodeIDs[0], nodesMap)
		}
	}
	return rawPrevID
}

func (s *engineService) buildEffectiveGraph(nodesMap map[string]model.Node) map[string][]string {
	graph := make(map[string][]string)

	for id, node := range nodesMap {
		if s.isProxyNode(node) {
			continue
		}

		realPrevs := s.resolveRealPrevs(node, nodesMap)
		for _, prevID := range realPrevs {
			uniqueAppend(graph, prevID, id)
		}
	}
	return graph
}

func (s *engineService) resolveRealPrevs(node model.Node, nodesMap map[string]model.Node) []string {
	var result []string
	for _, prevID := range node.PrevNodeIDs {
		prevNode, exists := nodesMap[prevID]
		if !exists {
			continue
		}

		if s.isProxyNode(prevNode) {
			result = append(result, s.resolveRealPrevs(prevNode, nodesMap)...)
		} else {
			result = append(result, prevID)
		}
	}
	return result
}

func (s *engineService) isProxyNode(node model.Node) bool {
	for _, uid := range node.UserIDs {
		if uid == easyflow.SysAutoUser {
			return true
		}
	}
	return false
}

func uniqueAppend(m map[string][]string, key, val string) {
	if !slice.Contains(m[key], val) {
		m[key] = append(m[key], val)
	}
}

// NodeStatusAnalyzer 负责分析节点和批次的状态
type NodeStatusAnalyzer struct {
	stats    map[string]map[string]*BatchStats
	nodesMap map[string]model.Node
}

type BatchStats struct {
	Total      int
	Passed     int // Status = 1
	Pending    int // IsFinished = 0
	SystemPass int // Status = 3
}

func NewNodeStatusAnalyzer(records []model.Task, nodesMap map[string]model.Node) *NodeStatusAnalyzer {
	stats := make(map[string]map[string]*BatchStats)

	for _, t := range records {
		if _, ok := stats[t.NodeID]; !ok {
			stats[t.NodeID] = make(map[string]*BatchStats)
		}

		batchStats, ok := stats[t.NodeID][t.BatchCode]
		if !ok {
			batchStats = &BatchStats{}
			stats[t.NodeID][t.BatchCode] = batchStats
		}

		batchStats.Total++
		if t.IsFinished == 0 {
			batchStats.Pending++
		} else if t.Status == 1 {
			batchStats.Passed++
		} else if t.Status == 3 {
			batchStats.SystemPass++
		}
	}

	return &NodeStatusAnalyzer{
		stats:    stats,
		nodesMap: nodesMap,
	}
}

// IsBatchTainted 判定当前批次是否由 SystemPass (自动跳过/取消) 污染
func (a *NodeStatusAnalyzer) IsBatchTainted(nodeID, batchCode string) bool {
	if batches, ok := a.stats[nodeID]; ok {
		if stats := batches[batchCode]; stats != nil {
			return stats.SystemPass > 0
		}
	}
	return false
}

// IsBatchEffectivelyPassed 判定当前批次是否"有效通过"
func (a *NodeStatusAnalyzer) IsBatchEffectivelyPassed(task model.Task) bool {
	if task.IsFinished != 1 || task.Status != 1 {
		return false
	}

	isCosigned := false
	if nodeDef, exists := a.nodesMap[task.NodeID]; exists && nodeDef.IsCosigned == 1 {
		isCosigned = true
	} else if task.IsCosigned == 1 {
		isCosigned = true
	}

	if isCosigned {
		if batches, ok := a.stats[task.NodeID]; ok {
			stats := batches[task.BatchCode]
			if stats != nil && stats.Passed < stats.Total {
				return false
			}
		}
	}

	return true
}

// HasNewerBatchPending 检查是否存在更新的批次正在进行中
func (a *NodeStatusAnalyzer) HasNewerBatchPending(nodeID, currentBatchCode string) bool {
	batches, ok := a.stats[nodeID]
	if !ok {
		return false
	}

	for batchCode, stats := range batches {
		if batchCode != currentBatchCode && stats.Pending > 0 {
			return true
		}
	}
	return false
}

// GraphTopologyService 负责处理图结构和路径查找
type GraphTopologyService struct {
	nodesMap       map[string]model.Node
	effectiveGraph map[string][]string
}

func NewGraphTopologyService(nodesMap map[string]model.Node, svc *engineService) *GraphTopologyService {
	return &GraphTopologyService{
		nodesMap:       nodesMap,
		effectiveGraph: svc.buildEffectiveGraph(nodesMap),
	}
}

// ResolveLogicalPrev 解析逻辑前置节点
func (g *GraphTopologyService) ResolveLogicalPrev(rawPrevID string) string {
	node, ok := g.nodesMap[rawPrevID]
	if !ok {
		return rawPrevID
	}

	if g.isProxyNode(node) {
		if len(node.PrevNodeIDs) > 0 {
			return g.ResolveLogicalPrev(node.PrevNodeIDs[0])
		}
	}
	return rawPrevID
}

func (g *GraphTopologyService) isProxyNode(node model.Node) bool {
	for _, uid := range node.UserIDs {
		if uid == easyflow.SysAutoUser {
			return true
		}
	}
	return false
}

// FindPath 寻找两个节点之间的路径
func (g *GraphTopologyService) FindPath(startID, endID string, svc *engineService) []string {
	return svc.findPathThroughGateways(startID, endID, g.effectiveGraph, g.nodesMap)
}

// ResolveNextNodes 获取当前节点的后续节点
func (g *GraphTopologyService) ResolveNextNodes(nodeID string) []string {
	return g.effectiveGraph[nodeID]
}
