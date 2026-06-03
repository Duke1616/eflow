package ticket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/Bunny3th/easy-workflow/workflow/engine"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/event"
	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	"github.com/Duke1616/eflow/internal/repository"
	engineSvc "github.com/Duke1616/eflow/internal/service/engine"
	templateSvc "github.com/Duke1616/eflow/internal/service/template"
	workflowSvc "github.com/Duke1616/eflow/internal/service/workflow"
	"github.com/ecodeclub/ekit/slice"
	"github.com/gotomicro/ego/core/elog"
	"github.com/xen0n/go-workwx"
	"golang.org/x/sync/errgroup"
)

// ValidationError 局部表单字段校验错误定义
var ValidationError = errors.New("字段校验失败")

// ErrTaskAlreadyFinished 任务已提前流转/结束拦截错误定义
var ErrTaskAlreadyFinished = errors.New("任务已被处理")

// Service 工单核心业务服务接口
type Service interface {
	// CreateBizTicket 直接录入并创建一个带有业务场景属性的物理工单
	CreateBizTicket(ctx context.Context, ticket domain.Ticket) (domain.Ticket, error)
	// CreateTicket 创建工单实例并驱动流程引擎启动对应审批流实例
	CreateTicket(ctx context.Context, req domain.Ticket) error
	// GetByProcessInstanceID 根据流程引擎的实例执行 ID 反查物理工单明细
	GetByProcessInstanceID(ctx context.Context, instanceId int) (domain.Ticket, error)
	// GetByID 根据自增主键 ID 查询物理工单及其动态表单属性详情
	GetByID(ctx context.Context, id int64) (domain.Ticket, error)
	// UpdateStatusByProcessInstanceID 根据引擎流程实例 ID 同步更新本地物理工单的最新流转状态
	UpdateStatusByProcessInstanceID(ctx context.Context, instanceId int, status uint8) error
	// BindProcessInstanceID 在流程引擎成功创建实例后反登记引擎的流程实例 ID 到物理工单上
	BindProcessInstanceID(ctx context.Context, id int64, instanceId int) error
	// ListByProcessInstanceIDs 根据流程实例 ID 列表批量高效反查关联的工单明细列表
	ListByProcessInstanceIDs(ctx context.Context, instanceIds []int) ([]domain.Ticket, error)
	// ListHistory 分页查询指定用户已完成或已撤销的工单流转历史记录
	ListHistory(ctx context.Context, userId string, offset, limit int64) ([]domain.Ticket, int64, error)
	// ListByUser 分页查询指定用户发起的当前正处于流转状态中的活跃工单列表
	ListByUser(ctx context.Context, userId string, offset, limit int64) ([]domain.Ticket, int64, error)
	// MergeData 将审批节点提交的最新变量合并追加到工单已存盘的 Data 变量集中
	MergeData(ctx context.Context, ticketId int64, data map[string]interface{}) error
	// CreateTaskForm 对指定的任务节点进行审批动作提交时的物理表单快照记录
	CreateTaskForm(ctx context.Context, taskId int, ticketId int64, fields []domain.FormValue) error
	// ListTaskFormsByTaskIDs 根据任务 ID 列表批量拉取历史步骤留存的物理表单数据
	ListTaskFormsByTaskIDs(ctx context.Context, taskIds []int) (map[int][]domain.FormValue, error)
	// ListTaskFormsByTicketID 查询某个物理工单下所有审批节点曾提交过的物理表单数据集
	ListTaskFormsByTicketID(ctx context.Context, ticketID int64) ([]domain.FormValue, error)
	// Pass 同意当前代办节点审批，执行变量合并、正则必填校验并驱动引擎向后推进
	Pass(ctx context.Context, taskId int, comment string, extraData map[string]interface{}) error
	// Reject 驳回当前节点审批，触发驳回分支逻辑并对物理工单流转状态进行合理回滚
	Reject(ctx context.Context, taskId int, comment string) error
}

type ticketService struct {
	repo        repository.TicketRepository
	templateSvc templateSvc.Service
	engineSvc   engineSvc.Service
	workflowSvc workflowSvc.Service
	producer    TicketEventProducer
	l           *elog.Component
}

// NewService 构造工单业务服务实现类
func NewService(repo repository.TicketRepository, templateSvc templateSvc.Service,
	engineSvc engineSvc.Service, workflowSvc workflowSvc.Service,
	producer TicketEventProducer) Service {
	return &ticketService{
		repo:        repo,
		templateSvc: templateSvc,
		engineSvc:   engineSvc,
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
		existingTicket, err := s.repo.FindByBizIdAndKey(ctx, ticket.BizID, ticket.Key, []domain.Status{domain.START, domain.PROCESS})
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

			// 异步发送追加告警通知（不阻塞主流程）
			go func() {
				defer func() {
					if r := recover(); r != nil {
						s.l.Error("发送追加告警通知发生panic", elog.Any("recover", r))
					}
				}()
				if err := s.sendAppendAlertNotification(ctx, existingTicket, ticket); err != nil {
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

	return bizTicket, s.sendGenerateFlowEvent(ctx, ticket, bizTicket.Id, "TODO")
}

func (s *ticketService) CreateTicket(ctx context.Context, req domain.Ticket) error {
	if err := req.Validate(); err != nil {
		return err
	}

	var (
		eg       errgroup.Group
		ticketId int64
		dTm      domain.Template
	)
	eg.Go(func() error {
		var err error
		ticketId, err = s.repo.CreateTicket(ctx, req)
		return err
	})

	eg.Go(func() error {
		var err error
		dTm, err = s.templateSvc.DetailTemplate(ctx, req.TemplateId)
		return err
	})
	if err := eg.Wait(); err != nil {
		return err
	}

	return s.sendGenerateFlowEvent(ctx, req, ticketId, dTm.Name)
}

func (s *ticketService) GetByProcessInstanceID(ctx context.Context, instanceId int) (domain.Ticket, error) {
	return s.repo.DetailByProcessInstId(ctx, instanceId)
}

func (s *ticketService) GetByID(ctx context.Context, id int64) (domain.Ticket, error) {
	return s.repo.Detail(ctx, id)
}

func (s *ticketService) UpdateStatusByProcessInstanceID(ctx context.Context, instanceId int, status uint8) error {
	return s.repo.UpdateStatusByInstanceId(ctx, instanceId, status)
}

func (s *ticketService) BindProcessInstanceID(ctx context.Context, id int64, instanceId int) error {
	return s.repo.RegisterProcessInstanceId(ctx, id, instanceId)
}

func (s *ticketService) ListByProcessInstanceIDs(ctx context.Context, instanceIds []int) ([]domain.Ticket, error) {
	return s.repo.ListTicketByProcessInstanceIds(ctx, instanceIds)
}

func (s *ticketService) ListHistory(ctx context.Context, userId string, offset, limit int64) ([]domain.Ticket, int64, error) {
	var (
		eg    errgroup.Group
		ts    []domain.Ticket
		total int64
	)
	eg.Go(func() error {
		var err error
		ts, err = s.repo.ListTicket(ctx, userId, []int{domain.END.ToInt(), domain.WITHDRAW.ToInt()}, offset, limit)
		return err
	})

	eg.Go(func() error {
		var err error
		total, err = s.repo.CountTicket(ctx, userId, []int{domain.END.ToInt(), domain.WITHDRAW.ToInt()})
		return err
	})
	if err := eg.Wait(); err != nil {
		return ts, total, err
	}
	return ts, total, nil
}

func (s *ticketService) ListByUser(ctx context.Context, userId string, offset, limit int64) ([]domain.Ticket, int64, error) {
	var (
		eg    errgroup.Group
		ts    []domain.Ticket
		total int64
	)
	eg.Go(func() error {
		var err error
		ts, err = s.repo.ListTicket(ctx, userId, []int{domain.PROCESS.ToInt()}, offset, limit)
		return err
	})

	eg.Go(func() error {
		var err error
		total, err = s.repo.CountTicket(ctx, userId, []int{domain.PROCESS.ToInt()})
		return err
	})
	if err := eg.Wait(); err != nil {
		return ts, total, err
	}
	return ts, total, nil
}

func (s *ticketService) MergeData(ctx context.Context, ticketId int64, data map[string]interface{}) error {
	return s.repo.MergeTicketData(ctx, ticketId, data)
}

func (s *ticketService) CreateTaskForm(ctx context.Context, taskId int, ticketId int64, fields []domain.FormValue) error {
	return s.repo.CreateTaskForm(ctx, taskId, ticketId, fields)
}

func (s *ticketService) ListTaskFormsByTaskIDs(ctx context.Context, taskIds []int) (map[int][]domain.FormValue, error) {
	return s.repo.FindTaskFormsBatch(ctx, taskIds)
}

func (s *ticketService) ListTaskFormsByTicketID(ctx context.Context, ticketID int64) ([]domain.FormValue, error) {
	return s.repo.FindTaskFormsByTicketID(ctx, ticketID)
}

// Pass 审批通过当前代办的流程节点，包含数据合并、必填校验与动态快照记录
func (s *ticketService) Pass(ctx context.Context, taskId int, comment string, extraData map[string]interface{}) error {
	// 1. 获取审批任务明细
	taskInfo, err := s.engineSvc.TaskInfo(ctx, taskId)
	if err != nil {
		return fmt.Errorf("获取任务详情失败: %w", err)
	}

	// 1.1 防御性拦截：避免因为网络延迟等原因产生的表单二次重复提交
	if taskInfo.IsFinished == 1 {
		s.l.Info("任务已提前处理，本次通过请求被平滑拦截",
			elog.Int("taskId", taskId),
			elog.Int("procInstId", taskInfo.ProcInstID))
		return fmt.Errorf("节点ID%d: %w", taskId, ErrTaskAlreadyFinished)
	}

	// 2. 查询流程实例
	instance, err := s.engineSvc.GetInstanceByID(ctx, taskInfo.ProcInstID)
	if err != nil {
		return err
	}

	// 3. 锁定当时发布的版本快照
	snapshot, err := s.workflowSvc.GetWorkflowSnapshot(ctx, taskInfo.ProcID, instance.ProcVersion)
	if err != nil {
		s.l.Error("获取快照版本失败", elog.FieldErr(err),
			elog.Int("流程ID", taskInfo.ProcID),
			elog.Int("流程版本", instance.ProcVersion))
		return err
	}

	// 4. 解析 node 节点数据并定位当前执行节点
	nodes, _ := easyflow.ParseNodes(snapshot.FlowData.Nodes)
	node, ok := slice.Find(nodes, func(node easyflow.Node) bool {
		return node.ID == taskInfo.NodeID
	})
	if !ok {
		return fmt.Errorf("当前执行节点在流程图中不存在: %s", taskInfo.NodeID)
	}

	property, err := easyflow.ToNodeProperty[easyflow.UserProperty](node)
	if err != nil {
		return err
	}

	// 5. 若无表单字段需要录入，则直接触发引擎通过
	if len(property.Fields) == 0 {
		return engine.TaskPass(taskId, comment, "", false)
	}

	// 6. 执行必填与格式正则校验，汇总合并更新项及快照数据
	mergeData := make(map[string]interface{})
	var formValues []domain.FormValue

	for _, field := range property.Fields {
		// tips 类型仅作为纯文本展示，不做持久化归集
		if field.Type == easyflow.FieldTips {
			continue
		}

		val, exists := extraData[field.Key]

		// 校验格式与必填约束
		if err = s.validateField(field, exists, val); err != nil {
			return err
		}

		if !exists {
			continue
		}

		// 收集需合并入工单主表 Data 里的字段
		if field.Merge {
			mergeData[field.Name] = val
		}

		// 收集任务的表单填写快照
		formValues = append(formValues, domain.FormValue{
			Name:  field.Name,
			Key:   field.Key,
			Type:  field.Type.ToString(),
			Value: val,
		})
	}

	// 7. 查询对应的关联工单实体
	ticket, err := s.repo.DetailByProcessInstId(ctx, taskInfo.ProcInstID)
	if err != nil {
		return fmt.Errorf("查询工单实体关联失败: %w", err)
	}

	// 8. 原子合并入库
	if len(mergeData) > 0 {
		if err = s.repo.MergeTicketData(ctx, ticket.Id, mergeData); err != nil {
			return fmt.Errorf("物理合并工单数据失败: %w", err)
		}
	}

	// 9. 记录当前审批步骤下的快照
	if err = s.repo.CreateTaskForm(ctx, taskId, ticket.Id, formValues); err != nil {
		return fmt.Errorf("物理归档任务快照失败: %w", err)
	}

	return engine.TaskPass(taskId, comment, "", false)
}

// Reject 审批节点驳回
func (s *ticketService) Reject(ctx context.Context, taskId int, comment string) error {
	taskInfo, err := s.engineSvc.TaskInfo(ctx, taskId)
	if err != nil {
		return fmt.Errorf("获取任务明细失败: %w", err)
	}

	if taskInfo.IsFinished == 1 {
		s.l.Info("任务已提前处理，本次驳回请求被平滑拦截",
			elog.Int("taskId", taskId),
			elog.Int("procInstId", taskInfo.ProcInstID))
		return fmt.Errorf("节点ID%d: %w", taskId, ErrTaskAlreadyFinished)
	}

	return engine.TaskReject(taskId, comment, "")
}

// validateField 校验表单字段的规则约束
func (s *ticketService) validateField(field easyflow.Field, exists bool, val any) error {
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

	return nil
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
			strValue := value
			valueType := reflect.TypeOf(value)
			if valueType.Kind() == reflect.Float64 {
				strValue = fmt.Sprintf("%f", value)
			}

			if valueType.Kind() == reflect.Slice || valueType.Kind() == reflect.Array {
				if v, err := json.Marshal(value); err == nil {
					strValue = string(v)
				}
			}

			data = append(data, event.Variables{
				Key:   key,
				Value: strValue,
			})
		}
	}

	return data, nil
}

func (s *ticketService) sendAppendAlertNotification(ctx context.Context, existingTicket domain.Ticket, newAlert domain.Ticket) error {
	if existingTicket.Process.InstanceId == 0 {
		s.l.Info("工单流程未启动，暂不发送追加告警通知",
			elog.Int64("ticketId", existingTicket.Id))
		return nil
	}

	s.l.Info("追加告警到已有工单",
		elog.Int64("ticketId", existingTicket.Id),
		elog.Int("processInstanceId", existingTicket.Process.InstanceId),
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
		return workwx.OAApprovalDetail{}, nil
	}

	var wechatOaDetail workwx.OAApprovalDetail
	err = json.Unmarshal(wechatOaJson, &wechatOaDetail)
	if err != nil {
		return workwx.OAApprovalDetail{}, err
	}

	return wechatOaDetail, nil
}

func convert(data []event.Variables, oaData workwx.OAApprovalDetail) []event.Variables {
	for _, contents := range oaData.ApplyData.Contents {
		id := strings.Split(contents.ID, "-")
		key := id[1]

		switch contents.Control {
		case "Selector":
			switch contents.Value.Selector.Type {
			case "single":
				data = append(data, event.Variables{
					Key:   key,
					Value: contents.Value.Selector.Options[0].Value[0].Text,
				})
			case "multi":
				value := slice.Map(contents.Value.Selector.Options, func(idx int, src workwx.OAContentSelectorOption) string {
					return src.Value[0].Text
				})

				data = append(data, event.Variables{
					Key:   key,
					Value: value,
				})
			}
		case "Textarea":
			data = append(data, event.Variables{
				Key:   key,
				Value: contents.Value.Text,
			})
		case "default":
			fmt.Println("不符合筛选规则")
		}
	}

	return data
}
