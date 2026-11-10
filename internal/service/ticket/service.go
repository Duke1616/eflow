package ticket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/errs"
	"github.com/Duke1616/eflow/internal/event"
	"github.com/Duke1616/eflow/internal/pkg/flowdsl"
	"github.com/Duke1616/eflow/internal/repository"
	engineSvc "github.com/Duke1616/eflow/internal/service/engine"
	templateSvc "github.com/Duke1616/eflow/internal/service/template"
	workflowSvc "github.com/Duke1616/eflow/internal/service/workflow"
	"github.com/gotomicro/ego/core/elog"
	"github.com/samber/lo"
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
	// GetByID 根据自增主键 ID 查询物理工单及其动态表单属性详情
	GetByID(ctx context.Context, id int64) (domain.Ticket, error)
	// UpdateStatusByProcessInstance 根据流程实例唯一标识统一更新本地物理工单状态
	UpdateStatusByProcessInstance(ctx context.Context, instanceID domain.ID, status uint8, wtime ...int64) error
	// BindProcess 在流程引擎成功创建实例后反向登记引擎类型与实例全局唯一标识到工单记录中
	BindProcess(ctx context.Context, id int64, engineType domain.EngineType, instanceID domain.ID) error
	// MarkProcessStartFailed 保留工单并记录流程启动失败原因
	MarkProcessStartFailed(ctx context.Context, id int64, reason string) error
	// RestartProcess 使用原工单数据重新发送流程启动事件
	RestartProcess(ctx context.Context, id int64) error
	// ListTaskFormsByTicketID 查询某个物理工单下所有审批节点曾提交过的物理表单数据集
	ListTaskFormsByTicketID(ctx context.Context, ticketID int64) ([]domain.FormValue, error)
	// Pass 同意当前代办节点审批，统一执行领域自卫校验、变量合并与必填校验并驱动引擎推进
	Pass(ctx context.Context, ticketId int64, taskId string, operator string, isAdmin bool, comment string, extraData map[string]interface{}) error
	// Reject 驳回当前节点审批，统一执行领域自卫校验并驱动引擎执行驳回流转
	Reject(ctx context.Context, ticketId int64, taskId string, operator string, isAdmin bool, comment string) error
	// Transfer 将当前审批任务转签指派给其他用户
	Transfer(ctx context.Context, ticketId int64, taskId string, operator string, isAdmin bool, targetUsers []string) error
	// GetTaskFormConfig 获取指定任务节点在当时流程版本中定义的表单字段列表
	GetTaskFormConfig(ctx context.Context, ticketId int64, taskId string, workflowId int64) ([]flowdsl.Field, error)
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

	// 领域规则收拢：若符合告警防重合并条件，检查是否存在流转中的同源工单
	if ticket.SupportsDeduplication() {
		existingTicket, err := s.repo.FindByBizIdAndKey(ctx, ticket.BizID, ticket.Key, domain.ActiveStatuses())
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

	templateName := s.resolveTemplateName(ctx, ticket.TemplateId)

	err = s.sendGenerateFlowEvent(ctx, ticket, bizTicket.Id, templateName)
	if err != nil {
		_ = s.MarkProcessStartFailed(ctx, bizTicket.Id, err.Error())
		return bizTicket, nil
	}
	return bizTicket, nil
}

// resolveTemplateName 安全获取模板展示名称，失败或未配置时平滑降级为兜底名称
func (s *ticketService) resolveTemplateName(ctx context.Context, templateID int64) string {
	const defaultName = "默认工单"
	if templateID <= 0 {
		return defaultName
	}
	tmpl, err := s.templateSvc.DetailTemplate(ctx, templateID)
	if err != nil {
		s.l.Warn("查询工单模板名称失败，使用默认模版名兜底",
			elog.Int64("templateId", templateID),
			elog.FieldErr(err),
		)
		return defaultName
	}
	if tmpl.Name != "" {
		return tmpl.Name
	}
	return defaultName
}

func (s *ticketService) CreateTicket(ctx context.Context, req domain.Ticket) error {
	if err := req.Validate(); err != nil {
		return err
	}

	// 1. 前置校验模板合法性并读取模板名称，避免写库成功后因模板异常残留无绑定的孤儿工单
	dTm, err := s.templateSvc.DetailTemplate(ctx, req.TemplateId)
	if err != nil {
		return errs.NotFound("TEMPLATE_NOT_FOUND", "指定的工单模板不存在或已下线").WithCause(err)
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
	ticket, err := s.repo.DetailByProcessInstance(ctx, instanceID)
	if err != nil {
		if errors.Is(err, repository.ErrTicketNotFound) {
			return domain.Ticket{}, errs.NotFound("TICKET_NOT_FOUND", fmt.Sprintf("工单流程实例不存在: %s", instanceID)).WithCause(err)
		}
		return domain.Ticket{}, fmt.Errorf("根据流程实例标识 [%s] 获取工单失败: %w", instanceID, err)
	}
	return ticket, nil
}

func (s *ticketService) GetByID(ctx context.Context, id int64) (domain.Ticket, error) {
	ticket, err := s.repo.Detail(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrTicketNotFound) {
			return domain.Ticket{}, errs.NotFound("TICKET_NOT_FOUND", fmt.Sprintf("工单不存在: %d", id)).WithCause(err)
		}
		return domain.Ticket{}, fmt.Errorf("获取工单详情失败: %w", err)
	}
	return ticket, nil
}

func (s *ticketService) UpdateStatusByProcessInstance(ctx context.Context, instanceID domain.ID, status uint8, wtime ...int64) error {
	var wt int64
	if len(wtime) > 0 {
		wt = wtime[0]
	}
	return s.repo.UpdateStatusByProcessInstance(ctx, instanceID, status, wt)
}

func (s *ticketService) BindProcess(ctx context.Context, id int64, engineType domain.EngineType, instanceID domain.ID) error {
	return s.repo.RegisterProcess(ctx, id, engineType, instanceID)
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
		return errs.BadRequest("TICKET_NOT_RESTARTABLE", fmt.Sprintf("工单 %d 当前不是可重新启动状态", id))
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

func (s *ticketService) ListTaskFormsByTicketID(ctx context.Context, ticketID int64) ([]domain.FormValue, error) {
	return s.repo.FindTaskFormsByTicketID(ctx, ticketID)
}

// operableTaskContext 封装工单与流程任务在审批操作中的运行时上下文
type operableTaskContext struct {
	ticket   domain.Ticket
	taskInfo domain.TaskContext
	tID      domain.ID
}

// loadTaskContext 统一校验工单标识、加载工单实体并提取特定任务上下文
func (s *ticketService) loadTaskContext(ctx context.Context, ticketId int64, taskId string) (*operableTaskContext, error) {
	if ticketId <= 0 {
		return nil, fmt.Errorf("%w: 工单 ID 非法", domain.ErrInvalidParameter)
	}
	ticket, err := s.repo.Detail(ctx, ticketId)
	if err != nil {
		if errors.Is(err, repository.ErrTicketNotFound) {
			return nil, errs.NotFound("TICKET_NOT_FOUND", fmt.Sprintf("工单不存在: %d", ticketId)).WithCause(err)
		}
		return nil, fmt.Errorf("查询工单实体关联失败 [ticketId=%d]: %w", ticketId, err)
	}

	tID := domain.NewID(taskId)
	if tID.IsZero() {
		return nil, fmt.Errorf("%w: 任务 ID 非法", domain.ErrInvalidParameter)
	}

	taskInfo, err := s.engine.GetTaskContext(ctx, ticket.Process.ResolveEngineType(), tID)
	if err != nil {
		return nil, fmt.Errorf("获取任务明细失败: %w", err)
	}

	return &operableTaskContext{
		ticket:   ticket,
		taskInfo: taskInfo,
		tID:      tID,
	}, nil
}

// prepareOperableTask 统一执行审批前置校验：加载工单与任务上下文，断言操作权限与办结状态，并记录管理员代办审计日志
func (s *ticketService) prepareOperableTask(ctx context.Context, ticketId int64, taskId, operator, action string, isAdmin bool) (*operableTaskContext, error) {
	opCtx, err := s.loadTaskContext(ctx, ticketId, taskId)
	if err != nil {
		return nil, err
	}

	// 领域层自卫校验：断言是否已被处理及操作人权限
	if err = opCtx.taskInfo.ValidateOperable(operator, isAdmin); err != nil {
		return nil, err
	}

	// 统一审计日志：记录管理员代办行为
	if isAdmin && opCtx.taskInfo.Assignee != operator {
		s.l.Info("管理员代办他人审批任务",
			elog.String("action", action),
			elog.String("admin", operator),
			elog.String("taskId", taskId),
			elog.String("originalAssignee", opCtx.taskInfo.Assignee),
		)
	}

	return opCtx, nil
}

// Pass 审批通过当前代办的流程节点，包含数据合并、必填校验与动态快照记录
func (s *ticketService) Pass(ctx context.Context, ticketId int64, taskId string, operator string, isAdmin bool, comment string, extraData map[string]interface{}) error {
	opCtx, err := s.prepareOperableTask(ctx, ticketId, taskId, operator, "同意", isAdmin)
	if err != nil {
		return err
	}
	engineType := opCtx.ticket.Process.ResolveEngineType()

	// 4. 统一加载流程图版本拓扑快照（支持按 workflow_id 智能检索与回退兜底，对底层流程引擎透明）
	targetWf, err := s.workflowSvc.FindInstanceFlow(ctx, opCtx.ticket.WorkflowId, opCtx.taskInfo.ProcessID, opCtx.taskInfo.ProcessVersion, engineType)
	if err != nil {
		return fmt.Errorf("获取流程拓扑快照失败: %w", err)
	}

	// 5. 解析标准拓扑图并定位当前执行节点
	graph, err := flowdsl.Parse(targetWf.FlowData)
	if err != nil {
		return fmt.Errorf("解析流程拓扑失败: %w", err)
	}
	node, ok := graph.GetNode(opCtx.taskInfo.NodeID)
	if !ok {
		return fmt.Errorf("当前执行节点在流程图中不存在: %s", opCtx.taskInfo.NodeID)
	}

	var nodeFields []flowdsl.Field
	if userCfg, isUser := node.AsUserTask(); isUser {
		nodeFields = userCfg.Fields
	}

	// 6. 若无表单字段需要录入，则直接触发引擎通过
	if len(nodeFields) == 0 {
		return s.engine.Pass(ctx, domain.TaskPassReq{
			EngineType: engineType,
			TaskId:     opCtx.tID,
			Operator:   operator,
			Comment:    comment,
		})
	}

	// 7. 执行必填与格式正则校验，汇总合并更新项及快照数据
	mergeData, formValues, err := s.collectAndValidateFields(nodeFields, extraData)
	if err != nil {
		return err
	}

	// 8. 驱动工作流引擎推进当前任务审批流转 (先流转防卫，流转成功后方才生效业务数据)
	if err = s.engine.Pass(ctx, domain.TaskPassReq{
		EngineType: engineType,
		TaskId:     opCtx.tID,
		Operator:   operator,
		Comment:    comment,
		Variables:  mergeData,
	}); err != nil {
		return err
	}

	// 9. 引擎流转成功后，原子合并工单业务数据
	if len(mergeData) > 0 {
		if err = s.repo.MergeTicketData(ctx, opCtx.ticket.Id, mergeData); err != nil {
			return fmt.Errorf("物理合并工单数据失败: %w", err)
		}
	}

	// 10. 记录当前审批步骤下的表单快照
	if len(formValues) > 0 {
		if err = s.repo.CreateTaskForm(ctx, opCtx.tID, opCtx.ticket.Id, formValues); err != nil {
			return fmt.Errorf("物理归档任务快照失败: %w", err)
		}
	}

	return nil
}

// collectAndValidateFields 校验表单字段约束，并分别汇总待合并入工单的业务属性和快照属性
func (s *ticketService) collectAndValidateFields(fields []flowdsl.Field, extraData map[string]interface{}) (map[string]interface{}, []domain.FormValue, error) {
	mergeData := make(map[string]interface{})
	var formValues []domain.FormValue

	// 先利用 lo.Filter 滤除纯静态说明文本组件，扁平化主干校验与收集循环
	interactiveFields := lo.Filter(fields, func(f flowdsl.Field, _ int) bool {
		return f.Type != flowdsl.FieldTips
	})

	for _, field := range interactiveFields {
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
func (s *ticketService) Reject(ctx context.Context, ticketId int64, taskId string, operator string, isAdmin bool, comment string) error {
	opCtx, err := s.prepareOperableTask(ctx, ticketId, taskId, operator, "驳回", isAdmin)
	if err != nil {
		return err
	}

	return s.engine.Reject(ctx, domain.TaskRejectReq{
		EngineType: opCtx.ticket.Process.ResolveEngineType(),
		TaskId:     opCtx.tID,
		Operator:   operator,
		Comment:    comment,
	})
}

// Transfer 审批任务转签指派
func (s *ticketService) Transfer(ctx context.Context, ticketId int64, taskId string, operator string, isAdmin bool, targetUsers []string) error {
	if len(targetUsers) == 0 {
		return fmt.Errorf("%w: 转签目标用户不能为空", domain.ErrInvalidParameter)
	}

	opCtx, err := s.prepareOperableTask(ctx, ticketId, taskId, operator, "转签", isAdmin)
	if err != nil {
		return err
	}

	return s.engine.Transfer(ctx, domain.TaskTransferReq{
		EngineType:  opCtx.ticket.Process.ResolveEngineType(),
		TaskId:      opCtx.tID,
		TargetUsers: targetUsers,
	})
}

// GetTaskFormConfig 获取指定任务节点在流程版本中定义的表单字段配置
func (s *ticketService) GetTaskFormConfig(ctx context.Context, ticketId int64, taskId string, workflowId int64) ([]flowdsl.Field, error) {
	opCtx, err := s.loadTaskContext(ctx, ticketId, taskId)
	if err != nil {
		return nil, err
	}

	wfID := workflowId
	if wfID <= 0 {
		wfID = opCtx.ticket.WorkflowId
	}

	engineType := opCtx.ticket.Process.ResolveEngineType()
	targetWf, err := s.workflowSvc.FindInstanceFlow(ctx, wfID, opCtx.taskInfo.ProcessID, opCtx.taskInfo.ProcessVersion, engineType)
	if err != nil {
		return nil, err
	}

	graph, err := flowdsl.Parse(targetWf.FlowData)
	if err != nil {
		return nil, fmt.Errorf("解析流程拓扑失败: %w", err)
	}
	node, ok := graph.GetNode(opCtx.taskInfo.NodeID)
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
	data := []event.Variables{
		{Key: "starter", Value: req.CreateBy},
	}

	switch req.Provide {
	case domain.WECHAT:
		oaData, err := wechatOaData(req.Data)
		if err != nil {
			return nil, err
		}
		data = convert(data, oaData)
	case domain.SYSTEM:
		systemVars := lo.MapToSlice(req.Data, func(key string, value any) event.Variables {
			return event.Variables{
				Key:   key,
				Value: formatVariableValue(value),
			}
		})
		data = append(data, systemVars...)
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
