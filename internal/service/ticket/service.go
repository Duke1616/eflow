package ticket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/event"
	"github.com/Duke1616/eflow/internal/pkg/flowdsl"
	"github.com/Duke1616/eflow/internal/repository"
	engineSvc "github.com/Duke1616/eflow/internal/service/engine"
	templateSvc "github.com/Duke1616/eflow/internal/service/template"
	workflowSvc "github.com/Duke1616/eflow/internal/service/workflow"
	"github.com/gotomicro/ego/core/elog"
	"github.com/samber/lo"
	"github.com/xen0n/go-workwx"
)

// ValidationError 局部表单字段校验错误定义
var ValidationError = errors.New("字段校验失败")

// ErrTaskAlreadyFinished 任务已提前流转/结束拦截错误定义（继承自领域层）
var ErrTaskAlreadyFinished = domain.ErrTaskAlreadyFinished

//go:generate mockgen -source=./service.go -package=ticketmocks -destination=./mocks/ticket_svc.mock.go Service

// Service 工单核心业务服务接口
type Service interface {
	// CreateBizTicket 直接录入并创建一个带有业务场景属性的物理工单
	CreateBizTicket(ctx context.Context, ticket domain.Ticket) (domain.Ticket, error)
	// CreateTicket 创建工单实例并驱动流程引擎启动对应审批流实例
	CreateTicket(ctx context.Context, req domain.Ticket) error
	// GetByProcessInstance 根据流程实例唯一标识统一检索工单明细
	GetByProcessInstance(ctx context.Context, instanceID domain.ID) (domain.Ticket, error)
	// GetByProcessInstanceID 根据流程引擎的实例执行 ID 反查物理工单明细
	GetByProcessInstanceID(ctx context.Context, instanceId int) (domain.Ticket, error)
	// GetByProcessInstanceKey 根据现代流程引擎的实例全局唯一 Key 反查物理工单明细
	GetByProcessInstanceKey(ctx context.Context, instanceKey string) (domain.Ticket, error)
	// GetByID 根据自增主键 ID 查询物理工单及其动态表单属性详情
	GetByID(ctx context.Context, id int64) (domain.Ticket, error)
	// UpdateStatusByProcessInstance 根据流程实例唯一标识统一更新本地物理工单状态
	UpdateStatusByProcessInstance(ctx context.Context, instanceID domain.ID, status uint8, wtime ...int64) error
	// UpdateStatusByProcessInstanceID 根据引擎流程实例 ID 同步更新本地物理工单的最新流转状态
	UpdateStatusByProcessInstanceID(ctx context.Context, instanceId int, status uint8) error
	// UpdateStatusByProcessInstanceKey 根据引擎流程实例全局唯一 Key 同步更新本地物理工单的最新流转状态
	UpdateStatusByProcessInstanceKey(ctx context.Context, instanceKey string, status uint8, wtime int64) error
	// BindProcess 在流程引擎成功创建实例后反向登记引擎类型与实例全局唯一标识到工单记录中
	BindProcess(ctx context.Context, id int64, engineType domain.EngineType, instanceID domain.ID) error
	// BindProcessInstanceID 在流程引擎成功创建实例后反登记引擎的流程实例 ID 到物理工单上
	BindProcessInstanceID(ctx context.Context, id int64, instanceId int) error
	// BindProcessInstanceKey 在现代流程引擎成功创建实例后反登记引擎的流程实例全局唯一 Key 到物理工单上
	BindProcessInstanceKey(ctx context.Context, id int64, instanceKey string) error
	// MarkProcessStartFailed 保留工单并记录流程启动失败原因
	MarkProcessStartFailed(ctx context.Context, id int64, reason string) error
	// RestartProcess 使用原工单数据重新发送流程启动事件
	RestartProcess(ctx context.Context, id int64) error
	// MergeData 将审批节点提交的最新变量合并追加到工单已存盘的 Data 变量集中
	MergeData(ctx context.Context, ticketId int64, data map[string]interface{}) error
	// CreateTaskForm 对指定的任务节点进行审批动作提交时的物理表单快照记录
	CreateTaskForm(ctx context.Context, taskId domain.ID, ticketId int64, fields []domain.FormValue) error
	// ListTaskFormsByTaskIDs 根据任务 ID 列表批量拉取历史步骤留存的物理表单数据
	ListTaskFormsByTaskIDs(ctx context.Context, taskIds []int) (map[int][]domain.FormValue, error)
	// ListTaskFormsByTicketID 查询某个物理工单下所有审批节点曾提交过的物理表单数据集
	ListTaskFormsByTicketID(ctx context.Context, ticketID int64) ([]domain.FormValue, error)
	// Pass 同意当前代办节点审批，统一执行领域自卫校验、变量合并与必填校验并驱动引擎推进
	Pass(ctx context.Context, taskId string, operator string, isAdmin bool, comment string, extraData map[string]interface{}) error
	// Reject 驳回当前节点审批，统一执行领域自卫校验并驱动引擎执行驳回流转
	Reject(ctx context.Context, taskId string, operator string, isAdmin bool, comment string) error
	// Transfer 将当前审批任务转签指派给其他用户
	Transfer(ctx context.Context, taskId string, operator string, isAdmin bool, targetUsers []string) error
	// GetTaskFormConfig 获取指定任务节点在当时流程版本中定义的表单字段列表
	GetTaskFormConfig(ctx context.Context, taskId string, workflowId int64) ([]flowdsl.Field, error)
}

type ticketService struct {
	repo        repository.TicketRepository
	templateSvc templateSvc.Service
	engine      engineSvc.IEngine
	workflowSvc workflowSvc.Service
	producer    TicketEventProducer
	l           *elog.Component
}

// NewService 构造工单业务服务实现类
func NewService(repo repository.TicketRepository, templateSvc templateSvc.Service,
	engine engineSvc.IEngine, workflowSvc workflowSvc.Service,
	producer TicketEventProducer) Service {
	return &ticketService{
		repo:        repo,
		templateSvc: templateSvc,
		engine:      engine,
		workflowSvc: workflowSvc,
		producer:    producer,
		l:           elog.DefaultLogger,
	}
}

func (s *ticketService) CreateBizTicket(ctx context.Context, ticket domain.Ticket) (domain.Ticket, error) {
	if err := ticket.Validate(); err != nil {
		return domain.Ticket{}, err
	}

	// 如果是告警转工单，且 Key 和 BizID 不为空，检查是否已有相同 Key 和 BizID 的进行中工单
	if ticket.Provide.IsAlert() && ticket.Key != "" && ticket.BizID > 0 {
		existingTicket, err := s.repo.FindByBizIdAndKey(ctx, ticket.BizID, ticket.Key,
			[]domain.Status{domain.START, domain.PROCESS, domain.WITHDRAWING, domain.START_FAILED})
		if err != nil {
			s.l.Warn("查询已有工单失败",
				elog.FieldErr(err),
				elog.Int64("bizId", ticket.BizID),
				elog.String("key", ticket.Key))
		} else if existingTicket.Id > 0 {
			// 找到已有工单，返回已有工单，并发送追加告警通知
			s.l.Info("找到已有工单，追加告警",
				elog.Int64("existingTicketId", existingTicket.Id),
				elog.Int64("bizId", ticket.BizID),
				elog.String("key", ticket.Key))

			// 异步发送追加告警通知（不阻塞主流程，使用 WithoutCancel 避免请求结束导致 context canceled）
			asyncCtx := context.WithoutCancel(ctx)
			go func() {
				defer func() {
					if r := recover(); r != nil {
						s.l.Error("发送追加告警通知发生panic", elog.Any("recover", r))
					}
				}()
				if err := s.sendAppendAlertNotification(asyncCtx, existingTicket, ticket); err != nil {
					s.l.Error("发送追加告警通知失败",
						elog.FieldErr(err),
						elog.Int64("ticketId", existingTicket.Id))
				}
			}()

			return existingTicket, nil
		}
	}

	// 创建新工单
	bizTicket, err := s.repo.CreateBizTicket(ctx, ticket)
	if err != nil {
		return domain.Ticket{}, err
	}

	templateName := "默认工单"
	if ticket.TemplateId > 0 {
		if tmpl, tmplErr := s.templateSvc.DetailTemplate(ctx, ticket.TemplateId); tmplErr == nil && tmpl.Name != "" {
			templateName = tmpl.Name
		} else if tmplErr != nil {
			s.l.Warn("查询工单模板名称失败，使用默认模版名兜底",
				elog.Int64("templateId", ticket.TemplateId),
				elog.FieldErr(tmplErr))
		}
	}

	err = s.sendGenerateFlowEvent(ctx, ticket, bizTicket.Id, templateName)
	if err != nil {
		_ = s.MarkProcessStartFailed(ctx, bizTicket.Id, err.Error())
		return bizTicket, nil
	}
	return bizTicket, nil
}

func (s *ticketService) CreateTicket(ctx context.Context, req domain.Ticket) error {
	if err := req.Validate(); err != nil {
		return err
	}

	// 1. 前置校验模板合法性并读取模板名称，避免写库成功后因模板异常残留无绑定的孤儿工单
	dTm, err := s.templateSvc.DetailTemplate(ctx, req.TemplateId)
	if err != nil {
		return fmt.Errorf("读取工单模板失败: %w", err)
	}

	// 2. 持久化工单主表记录
	ticketId, err := s.repo.CreateTicket(ctx, req)
	if err != nil {
		return fmt.Errorf("创建工单失败: %w", err)
	}

	// 3. 异步投递启动流程事件
	if err = s.sendGenerateFlowEvent(ctx, req, ticketId, dTm.Name); err != nil {
		_ = s.MarkProcessStartFailed(ctx, ticketId, err.Error())
		return nil
	}
	return nil
}

func (s *ticketService) GetByProcessInstance(ctx context.Context, instanceID domain.ID) (domain.Ticket, error) {
	return s.repo.DetailByProcessInstance(ctx, instanceID)
}

func (s *ticketService) GetByProcessInstanceID(ctx context.Context, instanceId int) (domain.Ticket, error) {
	return s.GetByProcessInstance(ctx, domain.NewID(instanceId))
}

func (s *ticketService) GetByProcessInstanceKey(ctx context.Context, instanceKey string) (domain.Ticket, error) {
	return s.GetByProcessInstance(ctx, domain.NewID(instanceKey))
}

func (s *ticketService) GetByID(ctx context.Context, id int64) (domain.Ticket, error) {
	return s.repo.Detail(ctx, id)
}

func (s *ticketService) UpdateStatusByProcessInstance(ctx context.Context, instanceID domain.ID, status uint8, wtime ...int64) error {
	var wt int64
	if len(wtime) > 0 {
		wt = wtime[0]
	}
	return s.repo.UpdateStatusByProcessInstance(ctx, instanceID, status, wt)
}

func (s *ticketService) UpdateStatusByProcessInstanceID(ctx context.Context, instanceId int, status uint8) error {
	return s.UpdateStatusByProcessInstance(ctx, domain.NewID(instanceId), status)
}

func (s *ticketService) UpdateStatusByProcessInstanceKey(ctx context.Context, instanceKey string, status uint8, wtime int64) error {
	return s.UpdateStatusByProcessInstance(ctx, domain.NewID(instanceKey), status, wtime)
}

func (s *ticketService) BindProcess(ctx context.Context, id int64, engineType domain.EngineType, instanceID domain.ID) error {
	return s.repo.RegisterProcess(ctx, id, engineType, instanceID)
}

func (s *ticketService) BindProcessInstanceID(ctx context.Context, id int64, instanceId int) error {
	return s.repo.RegisterProcessInstanceId(ctx, id, instanceId)
}

func (s *ticketService) BindProcessInstanceKey(ctx context.Context, id int64, instanceKey string) error {
	return s.repo.RegisterProcessInstanceKey(ctx, id, instanceKey)
}

// MarkProcessStartFailed 将尚未绑定流程实例的工单标记为流程启动失败并记录原因。
func (s *ticketService) MarkProcessStartFailed(ctx context.Context, id int64, reason string) error {
	return s.repo.MarkProcessStartFailed(ctx, id, reason)
}

// RestartProcess 校验失败或停滞工单后使用原始数据重新发送流程启动事件。
func (s *ticketService) RestartProcess(ctx context.Context, id int64) error {
	ticket, err := s.repo.Detail(ctx, id)
	if err != nil {
		return err
	}
	if !ticket.CanRestartProcess() {
		return fmt.Errorf("工单 %d 当前不是可重新启动状态", id)
	}
	if err = s.repo.PrepareProcessRestart(ctx, id); err != nil {
		return err
	}

	tmpl, err := s.templateSvc.DetailTemplate(ctx, ticket.TemplateId)
	if err != nil {
		_ = s.MarkProcessStartFailed(ctx, id, err.Error())
		return err
	}
	if err = s.sendGenerateFlowEvent(ctx, ticket, id, tmpl.Name); err != nil {
		_ = s.MarkProcessStartFailed(ctx, id, err.Error())
		return err
	}
	return nil
}

func (s *ticketService) MergeData(ctx context.Context, ticketId int64, data map[string]interface{}) error {
	return s.repo.MergeTicketData(ctx, ticketId, data)
}

func (s *ticketService) CreateTaskForm(ctx context.Context, taskId domain.ID, ticketId int64, fields []domain.FormValue) error {
	return s.repo.CreateTaskForm(ctx, taskId, ticketId, fields)
}

func (s *ticketService) ListTaskFormsByTaskIDs(ctx context.Context, taskIds []int) (map[int][]domain.FormValue, error) {
	return s.repo.FindTaskFormsBatch(ctx, taskIds)
}

func (s *ticketService) ListTaskFormsByTicketID(ctx context.Context, ticketID int64) ([]domain.FormValue, error) {
	return s.repo.FindTaskFormsByTicketID(ctx, ticketID)
}

// Pass 审批通过当前代办的流程节点，包含数据合并、必填校验与动态快照记录
func (s *ticketService) Pass(ctx context.Context, taskId string, operator string, isAdmin bool, comment string, extraData map[string]interface{}) error {
	tID := domain.NewID(taskId)
	taskInfo, err := s.engine.GetTaskContext(ctx, tID)
	if err != nil {
		return fmt.Errorf("获取任务明细失败: %w", err)
	}

	// 2. 领域层自卫校验：断言是否已被处理及操作人权限
	if err = domain.ValidateTaskOperable(taskInfo.Assignee, taskInfo.IsFinished, operator, isAdmin); err != nil {
		return err
	}

	// 3. 记录管理员代办审计日志
	if isAdmin && taskInfo.Assignee != operator {
		s.l.Info("管理员代办他人审批任务[同意]",
			elog.String("admin", operator),
			elog.String("taskId", taskId),
			elog.String("originalAssignee", taskInfo.Assignee),
		)
	}

	// 4. 统一加载流程图版本拓扑快照（对底层流程引擎透明）
	snapshot, err := s.workflowSvc.GetWorkflowSnapshot(ctx, taskInfo.ProcessID, taskInfo.ProcessVersion)
	if err != nil {
		return fmt.Errorf("获取流程拓扑快照失败: %w", err)
	}

	// 5. 解析标准拓扑图并定位当前执行节点
	graph, err := flowdsl.Parse(snapshot.FlowData)
	if err != nil {
		return fmt.Errorf("解析流程拓扑失败: %w", err)
	}
	node, ok := graph.GetNode(taskInfo.NodeID)
	if !ok {
		return fmt.Errorf("当前执行节点在流程图中不存在: %s", taskInfo.NodeID)
	}

	var nodeFields []flowdsl.Field
	if userCfg, isUser := node.AsUserTask(); isUser {
		nodeFields = userCfg.Fields
	}

	// 6. 若无表单字段需要录入，则直接触发引擎通过
	if len(nodeFields) == 0 {
		return s.engine.Pass(ctx, domain.TaskPassReq{
			EngineType: taskInfo.EngineType,
			TaskId:     tID,
			Operator:   operator,
			Comment:    comment,
		})
	}

	// 7. 执行必填与格式正则校验，汇总合并更新项及快照数据
	mergeData, formValues, err := s.collectAndValidateFields(nodeFields, extraData)
	if err != nil {
		return err
	}

	// 8. 统一加载工单实体以进行数据合并与表单快照落库（仓储层统一抹平实例 ID 与 Key 寻址）
	ticket, err := s.repo.DetailByProcessInstance(ctx, taskInfo.ProcessInstanceID)
	if err != nil {
		return fmt.Errorf("查询工单实体关联失败: %w", err)
	}

	// 9. 原子合并工单业务数据
	if len(mergeData) > 0 {
		if err = s.repo.MergeTicketData(ctx, ticket.Id, mergeData); err != nil {
			return fmt.Errorf("物理合并工单数据失败: %w", err)
		}
	}

	// 10. 记录当前审批步骤下的快照
	if len(formValues) > 0 {
		if err = s.repo.CreateTaskForm(ctx, tID, ticket.Id, formValues); err != nil {
			return fmt.Errorf("物理归档任务快照失败: %w", err)
		}
	}

	return s.engine.Pass(ctx, domain.TaskPassReq{
		EngineType: taskInfo.EngineType,
		TaskId:     tID,
		Operator:   operator,
		Comment:    comment,
		Variables:  mergeData,
	})
}

// collectAndValidateFields 校验表单字段约束，并分别汇总待合并入工单的业务属性和快照属性
func (s *ticketService) collectAndValidateFields(fields []flowdsl.Field, extraData map[string]interface{}) (map[string]interface{}, []domain.FormValue, error) {
	mergeData := make(map[string]interface{})
	var formValues []domain.FormValue

	for _, field := range fields {
		if field.Type == flowdsl.FieldTips {
			continue
		}

		val, exists := extraData[field.Key]
		if err := s.validateField(field, exists, val); err != nil {
			return nil, nil, err
		}
		if !exists {
			continue
		}

		if field.Merge {
			mergeData[field.Name] = val
		}
		formValues = append(formValues, domain.FormValue{
			Name:  field.Name,
			Key:   field.Key,
			Type:  field.Type.ToString(),
			Value: val,
		})
	}
	return mergeData, formValues, nil
}

// Reject 审批节点驳回
func (s *ticketService) Reject(ctx context.Context, taskId string, operator string, isAdmin bool, comment string) error {
	tID := domain.NewID(taskId)
	taskInfo, err := s.engine.GetTaskContext(ctx, tID)
	if err != nil {
		return fmt.Errorf("获取任务明细失败: %w", err)
	}

	// 领域层自卫校验：断言是否已被处理及操作人权限
	if err = domain.ValidateTaskOperable(taskInfo.Assignee, taskInfo.IsFinished, operator, isAdmin); err != nil {
		return err
	}

	// 记录管理员代办审计日志
	if isAdmin && taskInfo.Assignee != operator {
		s.l.Info("管理员代办他人审批任务[驳回]",
			elog.String("admin", operator),
			elog.String("taskId", taskId),
			elog.String("originalAssignee", taskInfo.Assignee),
		)
	}

	return s.engine.Reject(ctx, domain.TaskRejectReq{
		EngineType: taskInfo.EngineType,
		TaskId:     tID,
		Operator:   operator,
		Comment:    comment,
	})
}

// Transfer 审批任务转签指派
func (s *ticketService) Transfer(ctx context.Context, taskId string, operator string, isAdmin bool, targetUsers []string) error {
	if len(targetUsers) == 0 {
		return fmt.Errorf("%w: 转签目标用户不能为空", domain.ErrInvalidParameter)
	}

	tID := domain.NewID(taskId)
	taskInfo, err := s.engine.GetTaskContext(ctx, tID)
	if err != nil {
		return fmt.Errorf("获取任务明细失败: %w", err)
	}

	// 领域层自卫校验：断言是否已被处理及操作人权限
	if err = domain.ValidateTaskOperable(taskInfo.Assignee, taskInfo.IsFinished, operator, isAdmin); err != nil {
		return err
	}

	// 记录管理员代办审计日志
	if isAdmin && taskInfo.Assignee != operator {
		s.l.Info("管理员代办他人转签任务",
			elog.String("admin", operator),
			elog.String("taskId", taskId),
			elog.String("originalAssignee", taskInfo.Assignee),
		)
	}

	return s.engine.Transfer(ctx, domain.TaskTransferReq{
		EngineType:  taskInfo.EngineType,
		TaskId:      tID,
		TargetUsers: targetUsers,
	})
}

// GetTaskFormConfig 获取指定任务节点在流程版本中定义的表单字段配置
func (s *ticketService) GetTaskFormConfig(ctx context.Context, taskId string, workflowId int64) ([]flowdsl.Field, error) {
	tID := domain.NewID(taskId)
	taskCtx, err := s.engine.GetTaskContext(ctx, tID)
	if err != nil {
		return nil, err
	}

	wf, err := s.workflowSvc.FindInstanceFlow(ctx, workflowId, taskCtx.ProcessID, taskCtx.ProcessVersion)
	if err != nil {
		return nil, err
	}

	graph, err := flowdsl.Parse(wf.FlowData)
	if err != nil {
		return nil, fmt.Errorf("解析流程拓扑失败: %w", err)
	}
	node, ok := graph.GetNode(taskCtx.NodeID)
	if !ok {
		return []flowdsl.Field{}, nil
	}

	if userCfg, isUser := node.AsUserTask(); isUser {
		return userCfg.Fields, nil
	}
	return []flowdsl.Field{}, nil
}

// validateField 校验表单字段的规则约束
func (s *ticketService) validateField(field flowdsl.Field, exists bool, val any) error {
	if field.Required && (!exists || val == nil || val == "") {
		return fmt.Errorf("%w: [%s] 为必填项，请输入填写", ValidationError, field.Name)
	}

	if exists && field.Validate != "" {
		matched, validateErr := regexp.MatchString(field.Validate, fmt.Sprintf("%v", val))
		if validateErr != nil {
			s.l.Error("正则表达式执行失败",
				elog.String("field", field.Name),
				elog.String("regex", field.Validate),
			)
		} else if !matched {
			return fmt.Errorf("%w: [%s] 格式正则验证不满足要求", ValidationError, field.Name)
		}
	}

	return nil
}

func (s *ticketService) sendGenerateFlowEvent(ctx context.Context, req domain.Ticket, ticketId int64, tName string) error {
	if req.Data == nil {
		req.Data = make(map[string]interface{})
	}

	variables, err := s.variables(req)
	if err != nil {
		return err
	}

	// 传递工单 ID 用以引擎事件绑定
	variables = append(variables, event.Variables{
		Key:   "ticket_id",
		Value: strconv.FormatInt(ticketId, 10),
	})

	variables = append(variables, event.Variables{
		Key:   "template_name",
		Value: tName,
	})

	data, err := json.Marshal(variables)
	if err != nil {
		return err
	}

	evt := event.TicketEvent{
		Id:         ticketId,
		Provide:    event.Provide(req.Provide),
		WorkflowId: req.WorkflowId,
		Data:       req.Data,
		Variables:  string(data),
	}

	err = s.producer.Produce(ctx, evt)
	if err != nil {
		s.l.Error("发送创建流程事件失败",
			elog.FieldErr(err),
			elog.Any("evt", evt))
	}

	return err
}

func (s *ticketService) variables(req domain.Ticket) ([]event.Variables, error) {
	var data []event.Variables
	data = append(data, event.Variables{
		Key:   "starter",
		Value: req.CreateBy,
	})

	switch req.Provide {
	case domain.WECHAT:
		oaData, err := wechatOaData(req.Data)
		if err != nil {
			return nil, err
		}

		data = convert(data, oaData)
	case domain.SYSTEM:
		for key, value := range req.Data {
			data = append(data, event.Variables{
				Key:   key,
				Value: formatVariableValue(value),
			})
		}
	}

	return data, nil
}

// formatVariableValue 将表单输入值安全格式化为流程引擎变量，针对浮点数、数组和 nil 提供防 panic 保护
func formatVariableValue(value any) any {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case float64:
		return fmt.Sprintf("%f", v)
	case []any, map[string]any:
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
	}
	return value
}

func (s *ticketService) sendAppendAlertNotification(ctx context.Context, existingTicket domain.Ticket, newAlert domain.Ticket) error {
	if !existingTicket.Process.IsBound() {
		s.l.Info("工单流程未启动，暂不发送追加告警通知",
			elog.Int64("ticketId", existingTicket.Id))
		return nil
	}

	s.l.Info("追加告警到已有工单",
		elog.Int64("ticketId", existingTicket.Id),
		elog.String("processInstanceId", existingTicket.Process.DisplayID()),
		elog.Int64("newAlertBizId", newAlert.BizID),
		elog.String("newAlertKey", newAlert.Key))

	if existingTicket.NotificationConf.TemplateID > 0 {
		s.l.Info("工单配置了通知信息，可以发送追加告警通知",
			elog.Int64("templateId", existingTicket.NotificationConf.TemplateID),
			elog.String("channel", existingTicket.NotificationConf.Channel.String()))
	}

	return nil
}

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

func convert(data []event.Variables, oaData workwx.OAApprovalDetail) []event.Variables {
	for _, c := range oaData.ApplyData.Contents {
		val, ok := extractOAContentValue(c)
		if !ok {
			continue
		}
		data = append(data, event.Variables{
			Key:   cleanControlID(c.ID),
			Value: val,
		})
	}
	return data
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
