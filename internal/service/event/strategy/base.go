package strategy

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/Bunny3th/easy-workflow/workflow/model"
	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	"github.com/Duke1616/eflow/internal/pkg/resolve"
	"github.com/Duke1616/eflow/internal/pkg/rule"
	engineSvc "github.com/Duke1616/eflow/internal/service/engine"
	taskSvc "github.com/Duke1616/eflow/internal/service/task"
	templateSvc "github.com/Duke1616/eflow/internal/service/template"
	ticketSvc "github.com/Duke1616/eflow/internal/service/ticket"
	"github.com/ecodeclub/ekit/retry"
	"github.com/ecodeclub/ekit/slice"
	"github.com/gotomicro/ego/core/elog"
	"github.com/mitchellh/mapstructure"
	"golang.org/x/sync/errgroup"
)

type Service interface {
	// FetchRequiredData 并行获取流程运行的基础通知元数据
	FetchRequiredData(ctx context.Context, info Info, nodes []easyflow.Node) (*NotificationData, error)
	// FetchTasksWithRetry 在引擎异步落库任务时重试查询任务信息
	FetchTasksWithRetry(ctx context.Context, info Info) ([]model.Task, error)
	// GetNodeProperty 查询获取指定流程节点的属性配置快照
	GetNodeProperty(info Info, nodeID string) ([]easyflow.Node, any, error)
	// IsGlobalNotify 校验全局消息通知开关配置是否开启
	IsGlobalNotify(wf domain.Workflow) bool
	// EnrichTargets 解析包装运行时审批人匹配的目标元数据
	EnrichTargets(info Info, assignees []easyflow.Assignee) []resolve.Target
	// PrepareCommonFields 解析工单数据构建通用属性公示字段
	PrepareCommonFields(info Info, data *NotificationData) []rule.Field
	// ResolveAssignees 并发解析当前节点的审批人名单并自动同步注入
	ResolveAssignees(ctx context.Context, info *Info, assignees []easyflow.Assignee) ([]domain.User, error)
	// SafeGo 安全执行带 Panic 恢复机制的异步延时任务
	SafeGo(ctx context.Context, timeout time.Duration, fn func(ctx context.Context))
	// PassTask 调用工作流引擎接口物理流转推进指定任务
	PassTask(ctx context.Context, taskId int, remark string) error
	// FindTaskForms 获取指定工单下所有审批节点填写的表单快照历史
	FindTaskForms(ctx context.Context, ticketId int64) ([]domain.FormValue, error)
	// Logger 获取系统日志输出句柄
	Logger() *elog.Component
}

type service struct {
	templateSvc     templateSvc.Service
	userSvc         userv1.UserServiceClient
	taskSvc         taskSvc.Service
	orderSvc        ticketSvc.Service
	engineSvc       engineSvc.Service
	assigneeService resolve.Engine
	logger          *elog.Component

	InitialInterval time.Duration
	MaxInterval     time.Duration
	MaxRetries      int32
}

func NewService(userSvc userv1.UserServiceClient, templateSvc templateSvc.Service,
	taskSvc taskSvc.Service, orderSvc ticketSvc.Service, engineSvc engineSvc.Service,
	assigneeService resolve.Engine) Service {
	return &service{
		templateSvc:     templateSvc,
		userSvc:         userSvc,
		taskSvc:         taskSvc,
		orderSvc:        orderSvc,
		engineSvc:       engineSvc,
		assigneeService: assigneeService,
		logger:          elog.DefaultLogger,
		InitialInterval: 5 * time.Second,
		MaxInterval:     15 * time.Second,
		MaxRetries:      3,
	}
}

func (s *service) PassTask(ctx context.Context, taskId int, remark string) error {
	return s.engineSvc.Pass(ctx, taskId, remark)
}

func (s *service) FindTaskForms(ctx context.Context, ticketId int64) ([]domain.FormValue, error) {
	return s.orderSvc.ListTaskFormsByTicketID(ctx, ticketId)
}

func (s *service) ResolveAssignees(ctx context.Context, info *Info, assignees []easyflow.Assignee) ([]domain.User, error) {
	targets := s.EnrichTargets(*info, assignees)
	users, err := s.assigneeService.Resolve(ctx, targets)
	if err != nil {
		nodeID := ""
		if info.CurrentNode != nil {
			nodeID = info.CurrentNode.NodeID
		}
		return nil, fmt.Errorf("解析审批人失败 [Node: %s, Workflow: %s]: %w",
			nodeID, info.Workflow.Name, err)
	}

	if info.CurrentNode != nil {
		info.CurrentNode.UserIDs = slice.Map(users, func(idx int, u domain.User) string {
			return u.Username
		})
	}
	return users, nil
}

func (s *service) SafeGo(ctx context.Context, timeout time.Duration, fn func(ctx context.Context)) {
	go func() {
		sendCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		defer func() {
			if r := recover(); r != nil {
				s.Logger().Error("异步任务执行发生 panic",
					elog.Any("recover", r),
					elog.FieldStack(debug.Stack()))
			}
		}()
		fn(sendCtx)
	}()
}

func (s *service) Logger() *elog.Component {
	return s.logger
}

type NotificationData struct {
	WantResult map[string]interface{}
	Rules      []rule.Rule
	StartUser  domain.User
	TName      string
}

func (s *service) FetchRequiredData(ctx context.Context, info Info, nodes []easyflow.Node) (*NotificationData, error) {
	var data NotificationData
	errGroup, ctx := errgroup.WithContext(ctx)

	errGroup.Go(func() error {
		var err error
		data.WantResult, err = s.wantAllResult(ctx, info.InstID, nodes)
		return err
	})

	errGroup.Go(func() error {
		var err error
		data.Rules, data.TName, err = s.getRules(ctx, info.Order)
		return err
	})

	errGroup.Go(func() error {
		resp, err := s.userSvc.QueryByUsernames(ctx, &userv1.QueryByUsernamesReq{
			Usernames: []string{info.Order.CreateBy},
		})
		if err != nil {
			return err
		}
		if len(resp.Users) > 0 {
			data.StartUser = domain.User{
				Id:           resp.Users[0].Id,
				Username:     resp.Users[0].Username,
				DisplayName:  resp.Users[0].DisplayName,
				Email:        resp.Users[0].Email,
				Phone:        resp.Users[0].Phone,
				LarkUserId:   resp.Users[0].LarkUserId,
				WechatUserId: resp.Users[0].WechatUserId,
				DepartmentId: resp.Users[0].DepartmentId,
			}
		}
		return nil
	})

	if err := errGroup.Wait(); err != nil {
		return nil, err
	}
	return &data, nil
}

func (s *service) FetchTasksWithRetry(ctx context.Context, info Info) ([]model.Task, error) {
	strategy, err := retry.NewExponentialBackoffRetryStrategy(s.InitialInterval, s.MaxInterval, s.MaxRetries)
	if err != nil {
		return nil, err
	}

	for {
		d, ok := strategy.Next()
		if !ok {
			return nil, fmt.Errorf("获取执行任务超过最大重试次数")
		}

		tasks, taskErr := s.engineSvc.GetTasksByCurrentNodeId(ctx, info.InstID, info.CurrentNode.NodeID)
		if taskErr == nil && len(tasks) > 0 {
			s.logger.Debug("成功获取到节点任务信息",
				elog.String("nodeId", info.CurrentNode.NodeID),
				elog.Int("taskCount", len(tasks)))
			return tasks, nil
		}

		s.logger.Debug("尚未查询到节点任务，准备重试",
			elog.Int("instId", info.InstID),
			elog.String("nodeId", info.CurrentNode.NodeID),
			elog.Duration("nextRetry", d))

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(d):
			continue
		}
	}
}

func (s *service) PrepareCommonFields(info Info, data *NotificationData) []rule.Field {
	ruleFields := rule.GetFields(data.Rules, info.Order.Provide.ToUint8(), info.Order.Data)
	fields := slice.Map(ruleFields, func(idx int, src rule.Field) rule.Field {
		return rule.Field{
			IsShort: src.IsShort,
			Tag:     src.Tag,
			Content: src.Content,
		}
	})

	for field, value := range data.WantResult {
		fields = append(fields, rule.Field{
			IsShort: true,
			Tag:     "lark_md",
			Content: fmt.Sprintf(`**%s:**\n%v`, field, value),
		})
	}
	return fields
}

func (s *service) getRules(ctx context.Context, oInfo domain.Ticket) ([]rule.Rule, string, error) {
	t, err := s.templateSvc.DetailTemplate(ctx, oInfo.TemplateId)
	if err != nil {
		return nil, "", err
	}

	rules, err := rule.ParseRules(t.Rules)
	if err != nil {
		return nil, "", err
	}

	return rules, t.Name, nil
}

func (s *service) wantAllResult(ctx context.Context, instanceId int, nodes []easyflow.Node) (map[string]interface{}, error) {
	mergedResult := make(map[string]interface{})
	for _, node := range nodes {
		if node.Type != "automation" {
			continue
		}
		if result, err := s.fetchResult(ctx, instanceId, node.ID); err == nil {
			for k, v := range result {
				mergedResult[k] = v
			}
		}
	}
	return mergedResult, nil
}

func (s *service) fetchResult(ctx context.Context, instanceID int, nodeID string) (map[string]interface{}, error) {
	result, err := s.taskSvc.FindTaskByNodeID(ctx, instanceID, nodeID)
	if err != nil {
		return nil, err
	}

	if result.WantResult == "" {
		return nil, fmt.Errorf("返回值为空, 不做任何数据处理")
	}

	var data map[string]interface{}
	if err = json.Unmarshal([]byte(result.WantResult), &data); err != nil {
		return nil, err
	}
	return data, nil
}

func (s *service) GetNodeProperty(info Info, nodeID string) ([]easyflow.Node, any, error) {
	nodes := info.Nodes
	var err error
	if len(nodes) == 0 {
		nodes, err = UnmarshalNodes(info.Workflow)
		if err != nil {
			return nil, nil, err
		}
	}

	node, ok := slice.Find(nodes, func(src easyflow.Node) bool {
		return src.ID == nodeID
	})
	if ok {
		return nodes, node.Properties, nil
	}
	return nodes, nil, fmt.Errorf("未找到节点 %s", nodeID)
}

func (s *service) IsGlobalNotify(wf domain.Workflow) bool {
	if !wf.IsNotify {
		s.logger.Warn("流程全局消息通知已关闭", elog.Any("wfId", wf.Id))
		return false
	}
	return true
}

func (s *service) EnrichTargets(info Info, assignees []easyflow.Assignee) []resolve.Target {
	return slice.Map(assignees, func(idx int, src easyflow.Assignee) resolve.Target {
		if src.Rule == "" {
			s.logger.Warn("发现未定义的审批规则类型",
				elog.String("nodeId", info.CurrentNode.NodeID),
				elog.Any("assignee", src))
		}
		values := src.Values
		switch src.Rule {
		case easyflow.LEADER, easyflow.MAIN_LEADER, easyflow.FOUNDER:
			if len(values) == 0 {
				values = []string{info.Order.CreateBy}
			}
		case easyflow.TEMPLATE:
			var usernames []string
			for _, field := range values {
				if val, ok := info.Order.Data[field]; ok {
					usernames = append(usernames, s.extractUsernamesFromField(field, val)...)
				}
			}
			values = usernames
		}

		return resolve.Target{
			Type:   string(src.Rule),
			Values: values,
		}
	})
}

func (s *service) extractUsernamesFromField(fieldName string, value interface{}) []string {
	switch v := value.(type) {
	case []string:
		return v
	case string:
		return []string{v}
	case []interface{}:
		return slice.FilterMap(v, func(idx int, item interface{}) (string, bool) {
			if str, ok := item.(string); ok {
				return str, true
			}
			s.logger.Warn("模版字段数组元素类型不支持",
				elog.String("field", fieldName),
				elog.Any("element_type", fmt.Sprintf("%T", item)))
			return "", false
		})
	default:
		s.logger.Warn("模版字段类型不支持",
			elog.String("field", fieldName),
			elog.Any("type", fmt.Sprintf("%T", value)),
			elog.Any("value", value))
		return nil
	}
}

func UnmarshalNodes(wf domain.Workflow) ([]easyflow.Node, error) {
	var nodes []easyflow.Node
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &nodes,
		TagName: "json",
	})
	if err != nil {
		return nil, err
	}

	if err = decoder.Decode(wf.FlowData.Nodes); err != nil {
		return nil, err
	}

	return nodes, nil
}

type RecipientMap struct {
	users   map[string]domain.User
	channel Channel
}

func NewRecipientMap(users []domain.User, channel Channel) RecipientMap {
	return RecipientMap{
		users:   slice.ToMap(users, func(u domain.User) string { return u.Username }),
		channel: channel,
	}
}

func (rm RecipientMap) GetID(username string) string {
	u, ok := rm.users[username]
	if !ok {
		return ""
	}

	switch rm.channel {
	case ChannelWechat:
		return u.WechatUserId
	default:
		return u.LarkUserId
	}
}

func (rm RecipientMap) GetIDs() []string {
	ids := make([]string, 0, len(rm.users))
	for _, u := range rm.users {
		var id string
		switch rm.channel {
		case ChannelWechat:
			id = u.WechatUserId
		default:
			id = u.LarkUserId
		}
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}
