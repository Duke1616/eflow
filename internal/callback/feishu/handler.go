package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
	"github.com/Duke1616/eflow/internal/callback"
	"github.com/Duke1616/eflow/internal/client/eiam"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/flowdsl"
	"github.com/Duke1616/eflow/internal/pkg/notification"
	"github.com/Duke1616/eflow/internal/pkg/notification/sender"
	"github.com/Duke1616/eflow/internal/pkg/rule"
	channelSvc "github.com/Duke1616/eflow/internal/service/channel"
	engineSvc "github.com/Duke1616/eflow/internal/service/engine"
	templateSvc "github.com/Duke1616/eflow/internal/service/template"
	ticketSvc "github.com/Duke1616/eflow/internal/service/ticket"
	withdrawalSvc "github.com/Duke1616/eflow/internal/service/withdrawal"
	workflowSvc "github.com/Duke1616/eflow/internal/service/workflow"
	"github.com/Duke1616/eflow/pkg/mapx"
	"github.com/Duke1616/eiam/pkg/ctxutil"
	enotifyfeishu "github.com/Duke1616/enotify/notify/feishu"
	"github.com/chromedp/chromedp"
	"github.com/gotomicro/ego/core/elog"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkcallback "github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"github.com/samber/lo"
)

// IFeishuCardCallbackHandler 飞书卡片专属回调处理器契约（特化自跨渠道泛型接口 IChannelCallbackHandler）
type IFeishuCardCallbackHandler interface {
	callback.IChannelCallbackHandler[*larkcallback.CardActionTriggerEvent, *larkcallback.CardActionTriggerResponse]
	// Handle 本地驱动并处理跨渠道标准审批交互动作
	Handle(ctx context.Context, evt domain.ChannelActionEvent) error
	// OnCardAction 兼容飞书原生 SDK 长连接/Webhook 事件分发签名
	OnCardAction(ctx context.Context, cte *larkcallback.CardActionTriggerEvent) (*larkcallback.CardActionTriggerResponse, error)
}

// ILarkCallbackHandler 为保持向后兼容保留的统一类型别名
type ILarkCallbackHandler = IFeishuCardCallbackHandler

type larkCallbackHandler struct {
	logger           *elog.Component
	cfg              callbackConfig
	channelActionSvc channelSvc.IChannelActionService
	engine           engineSvc.IEngine
	svc              ticketSvc.Service
	templateSvc      templateSvc.Service
	userSvc          eiam.IUserClient
	workflowSvc      workflowSvc.Service
	withdrawalSvc    withdrawalSvc.Service
	sender           sender.NotificationSender
	lark             *lark.Client
}

// NewLarkCallbackHandler 构造飞书卡片业务处理器
func NewLarkCallbackHandler(
	channelActionSvc channelSvc.IChannelActionService,
	engine engineSvc.IEngine,
	svc ticketSvc.Service,
	templateSvc templateSvc.Service,
	userSvc eiam.IUserClient,
	workflowSvc workflowSvc.Service,
	withdrawalSvc withdrawalSvc.Service,
	sender sender.NotificationSender,
	lark *lark.Client,
) ILarkCallbackHandler {
	return &larkCallbackHandler{
		logger:           elog.DefaultLogger.With(elog.FieldComponentName("LarkCallbackHandler")),
		cfg:              getLarkCallbackConfig(),
		channelActionSvc: channelActionSvc,
		engine:           engine,
		svc:              svc,
		templateSvc:      templateSvc,
		userSvc:          userSvc,
		workflowSvc:      workflowSvc,
		withdrawalSvc:    withdrawalSvc,
		sender:           sender,
		lark:             lark,
	}
}

// Channel 实现 domain.IChannelCallbackHandler 泛型接口
func (h *larkCallbackHandler) Channel() domain.ChannelType {
	return domain.ChannelFeishu
}

// OnAction 响应第三方交互回调（实现 domain.IChannelCallbackHandler 泛型接口）
func (h *larkCallbackHandler) OnAction(ctx context.Context, cte *larkcallback.CardActionTriggerEvent) (*larkcallback.CardActionTriggerResponse, error) {
	return h.OnCardAction(ctx, cte)
}

// actionToastMap 定义跨渠道动作触发时的即时轻量交互反馈文案
var actionToastMap = map[domain.ChannelActionType]string{
	domain.ChannelActionProgress: "正在生成流程进度图，请稍候...",
	domain.ChannelActionPass:     "正在处理审批通过...",
	domain.ChannelActionReject:   "正在处理审批驳回...",
	domain.ChannelActionRevoke:   "正在处理工单撤销...",
}

// OnCardAction 接收飞书卡片上的用户交互点击回调
func (h *larkCallbackHandler) OnCardAction(ctx context.Context, cte *larkcallback.CardActionTriggerEvent) (*larkcallback.CardActionTriggerResponse, error) {
	h.logger.Info("捕捉到飞书互动卡片交互点击回调")

	if cte.Event.Action == nil || cte.Event.Action.Value == nil {
		h.logger.Warn("忽略非法的空动作飞书交互事件")
		return &larkcallback.CardActionTriggerResponse{}, nil
	}

	domainEvt := extractChannelActionEvent(cte)

	// 异步调用业务处理器的 Handle 方法，使用 context.WithoutCancel 解除父级请求生命周期绑定
	go func() {
		defer func() {
			if r := recover(); r != nil {
				h.logger.Error("本地异步驱动飞书卡片事件发生 panic", elog.Any("recover", r))
			}
		}()
		localCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 60*time.Second)
		defer cancel()
		h.logger.Info("已触发本地异步驱动工单实例流转流程", elog.Any("evt", domainEvt))
		if err := h.Handle(localCtx, domainEvt); err != nil {
			h.logger.Error("本地异步驱动飞书卡片事件流转失败", elog.FieldErr(err))
		} else {
			h.logger.Info("本地异步驱动飞书卡片事件流转成功")
		}
	}()

	// 表驱动法：针对异步耗时操作立即向飞书客户端下发轻量 Toast 响应，消除“点击无反应”假死体验
	var resp larkcallback.CardActionTriggerResponse
	if tip, ok := actionToastMap[domainEvt.Action]; ok {
		resp.Toast = &larkcallback.Toast{
			Type:    "info",
			Content: tip,
		}
	}

	return &resp, nil
}

// Handle 执行具体的流转处理逻辑
func (h *larkCallbackHandler) Handle(ctx context.Context, evt domain.ChannelActionEvent) error {
	// 注入多租户上下文：从飞书卡片传回的 Value 中提取租户 ID 并注入上下文，避免被 GORM 租户插件误杀
	if evt.TenantID > 0 {
		ctx = ctxutil.WithTenantID(ctx, evt.TenantID)
	}

	h.logger.Debug("获取跨渠道标准化交互信息", elog.Any("evt", evt),
		elog.Int64("ticket_id", evt.TicketID),
		elog.String("task_id", evt.TaskID.String()),
	)

	// 核心业务调度：统筹身份精准解析、权限判定与底层工单流转
	result, err := h.channelActionSvc.DispatchAction(ctx, evt)
	if err != nil {
		h.logger.Error("跨渠道业务调度发生未预期系统异常", elog.FieldErr(err))
		return err
	}

	// 状态机守护法则：若操作失败（权限不足、表单校验未过、流程流转异常），严禁将待办卡片刷成成功态（彻底消灭幽灵同意）
	if !result.Success {
		h.logger.Warn("飞书卡片审批操作执行未生效，向操作者推送失败告警并保留待办卡片",
			elog.String("userId", evt.ChannelUserID),
			elog.String("action", string(result.Action)),
			elog.String("remark", result.Remark),
			elog.FieldErr(result.Error),
		)

		// 向操作人推送私信错误提示，告知真实拦截原因，同时保留卡片可再次操作
		_, _ = h.sender.Send(ctx, notification.Notification{
			Receiver:     evt.ChannelUserID,
			ReceiverType: enotifyfeishu.ReceiveIDTypeUserID,
			Channel:      notification.ChannelLarkText,
			Template: notification.Template{
				Text: fmt.Sprintf("【工单操作提醒】您的操作未生效: %s", result.Remark),
			},
		})
		return result.Error
	}

	// 业务成功：按交互类型完成后续卡片状态流转或进度截图渲染
	switch result.Action {
	case domain.ChannelActionProgress:
		if err := h.progress(ctx, evt.TicketID, evt.ChannelUserID); err != nil {
			h.logger.Error("查看流程进度失败", elog.FieldErr(err))
			return err
		}
		return nil
	case domain.ChannelActionPass, domain.ChannelActionReject, domain.ChannelActionRevoke:
		return h.withdraw(ctx, evt, result.Remark)
	default:
		h.logger.Warn("未知的渠道动作类型", elog.String("action", string(result.Action)))
		return nil
	}
}

func (h *larkCallbackHandler) progress(ctx context.Context, ticketId int64, userId string) error {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.NoSandbox,
		chromedp.NoFirstRun,
		chromedp.DisableGPU,
		chromedp.Flag("ignore-certificate-errors", true),
		chromedp.Flag("disable-setuid-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("font-render-hinting", "none"),
		chromedp.Flag("force-color-profile", "srgb"),
	)

	if !h.cfg.Debug {
		opts = append(opts, chromedp.Headless)
	} else {
		opts = append(opts, chromedp.Flag("headless", false))
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, opts...)
	defer cancelAlloc()

	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx, chromedp.WithLogf(h.logger.Debugf))
	defer cancelBrowser()

	taskCtx, cancelTask := context.WithTimeout(browserCtx, 30*time.Second)
	defer cancelTask()
	var buf []byte

	ticketDetail, err := h.svc.GetByID(taskCtx, ticketId)
	if err != nil {
		return err
	}

	view, err := h.engine.GetInstanceExecutionView(taskCtx, ticketDetail.Process.EngineType, ticketDetail.Process.InstanceID, ticketDetail.Status.ToUint8())
	if err != nil {
		return err
	}

	wf, err := h.workflowSvc.FindInstanceFlow(taskCtx, ticketDetail.WorkflowId, view.ProcessID, view.ProcessVersion)
	if err != nil {
		return err
	}

	injectData, err := h.getJsCode(wf, view.TraversedEdges, view.NodeStatusMap)
	if err != nil {
		return err
	}

	// 进行截图
	err = chromedp.Run(taskCtx,
		chromedp.EmulateViewport(1920, 1080, chromedp.EmulateScale(1)),
		chromedp.Navigate(h.cfg.FrontendUrl),
		chromedp.WaitReady("body"),
		// 等待数据加载
		chromedp.ActionFunc(func(ctx context.Context) error {
			h.logger.Debug("LF 截图页面加载完成，等待 LF-preview 容器可见...")
			return nil
		}),
		// 注入数据
		chromedp.Evaluate(injectData, nil),

		// 等待 LogicFlow 容器可见
		chromedp.WaitVisible("#LF-preview", chromedp.ByID),

		// 等待前端设置的 data-rendered 标志
		chromedp.WaitVisible(`#LF-preview[data-rendered="true"]`, chromedp.ByQuery),

		// 再等 300ms，防止动画残影
		chromedp.Sleep(300*time.Millisecond),

		// 准备开始截图
		chromedp.ActionFunc(func(ctx context.Context) error {
			h.logger.Debug("LF-preview 容器已就绪，开始截取流程图...")
			return nil
		}),
		// 截取容器截图（比全屏更精准）
		chromedp.Screenshot("#LF-preview", &buf, chromedp.NodeVisible, chromedp.ByID),
	)
	if err != nil {
		return err
	}

	imageKey, err := h.uploadImage(taskCtx, buf)
	if err != nil {
		return err
	}

	return h.sendImage(taskCtx, imageKey, view.PendingUsers, userId)
}

func (h *larkCallbackHandler) sendImage(ctx context.Context, imageKey *string, approvalUsers []string, userId string) error {
	var fields []notification.Field
	us, err := h.userSvc.FindByUsernames(ctx, approvalUsers)
	if err != nil {
		return err
	}

	approval := `**审批人：Null**`
	status := `**状态：<font color='green'> 已结束 </font>**`

	// 过滤有效飞书绑定的用户并生成 at 标签，避免生成破损的空 at 标签
	atTags := lo.FilterMap(us, func(u *userv1.User, _ int) (string, bool) {
		if u != nil && u.LarkUserId != "" {
			return fmt.Sprintf("<at id=%s></at>", u.LarkUserId), true
		}
		return "", false
	})

	if len(atTags) > 0 {
		approval = fmt.Sprintf("**审批人：** %s", strings.Join(atTags, " "))
		status = `**状态：<font color='green'> 审批中 </font>**`
	}

	fields = append(fields, notification.Field{
		Tag:     "markdown",
		Content: approval,
	})

	fields = append(fields, notification.Field{
		Tag:     "markdown",
		Content: status,
	})

	if _, err = h.sender.Send(ctx, notification.Notification{
		Receiver:     userId,
		ReceiverType: enotifyfeishu.ReceiveIDTypeUserID,
		Channel:      notification.ChannelLarkCard,
		Template: notification.Template{
			Name:     domain.NotifyTypeProgressImageResult,
			Title:    "工单流程进度查看",
			Fields:   fields,
			ImageKey: *imageKey,
		},
	}); err != nil {
		return fmt.Errorf("发送流程进度失败: %w", err)
	}

	return nil
}

func (h *larkCallbackHandler) uploadImage(ctx context.Context, buf []byte) (*string, error) {
	req := larkim.NewCreateImageReqBuilder().
		Body(larkim.NewCreateImageReqBodyBuilder().
			ImageType(`message`).
			Image(bytes.NewReader(buf)).
			Build()).
		Build()

	resp, err := h.lark.Im.Image.Create(ctx, req)
	if err != nil {
		return nil, err
	}

	if !resp.Success() {
		return nil, fmt.Errorf("logId: %s, error response: \n%s",
			resp.RequestId(), larkcore.Prettify(resp.CodeError))
	}

	return resp.Data.ImageKey, nil
}

func (h *larkCallbackHandler) getJsCode(wf domain.Workflow, edgeMap map[string][]string, nodeStatusMap map[string]int) (string, error) {
	highlighted, err := flowdsl.HighlightCanvas(wf.FlowData, flowdsl.ExecutionTrace{
		TraversedEdges: edgeMap,
		NodeStatusMap:  nodeStatusMap,
	})
	if err != nil {
		return "", err
	}
	dataJSON, err := json.Marshal(highlighted)
	if err != nil {
		return "", fmt.Errorf("序列化高亮画布数据失败: %w", err)
	}
	return fmt.Sprintf("window.__DATA__ = %s;", dataJSON), nil
}

func (h *larkCallbackHandler) withdraw(ctx context.Context, evt domain.ChannelActionEvent, remark string) error {
	fTicket, err := h.svc.GetByID(ctx, evt.TicketID)
	if err != nil {
		return err
	}

	t, err := h.templateSvc.DetailTemplate(ctx, fTicket.TemplateId)
	if err != nil {
		return err
	}

	rules, err := rule.ParseRules(t.Rules)
	if err != nil {
		return err
	}
	ruleFields := rule.GetFields(rules, fTicket.Provide.ToUint8(), fTicket.Data)

	userInfo, err := h.userSvc.FindByUsername(ctx, fTicket.CreateBy)
	if err != nil {
		return err
	}

	// 发送消息通知
	if _, err = h.sender.Send(ctx, notification.Notification{
		Receiver:     evt.ChannelUserID,
		ReceiverType: enotifyfeishu.ReceiveIDTypeUserID,
		MessageID:    evt.MessageID,
		Channel:      notification.ChannelLarkCard,
		Template: notification.Template{
			Name:  domain.NotifyTypeProgress,
			Title: rule.GenerateTitle(userInfo.DisplayName, t.Name),
			Fields: lo.Map(ruleFields, func(src rule.Field, _ int) notification.Field {
				return notification.Field{
					IsShort: src.IsShort,
					Tag:     src.Tag,
					Content: src.Content,
				}
			}),
			Values: getCallbackValue(evt),
			Remark: remark,
		},
	}); err != nil {
		return fmt.Errorf("修改飞书消息失败: %w", err)
	}

	return nil
}

func getCallbackValue(evt domain.ChannelActionEvent) []notification.Value {
	// 优雅利用统一装配生成器获取 ticket_id, task_id 和 tenant_id 标准三元组
	values := notification.GenerateCallbackValues(evt.TicketID, evt.TaskID.String(), evt.TenantID)

	// 追加回执卡片交互专属的 user_id 与行为参数，避免数据泄露或防重复拦截
	values = append(values,
		notification.Value{Key: "user_id", Value: evt.ChannelUserID},
		notification.Value{Key: "action", Value: string(Progress)},
	)
	return values
}

// extractChannelActionEvent 将飞书卡片的原生触发事件解包并标准化为通用的领域交互事件
// 解释设计原因：依托基础设施层通用 MapX 进行强类型安全提取，杜绝手写弱类型探测胶水代码
func extractChannelActionEvent(cte *larkcallback.CardActionTriggerEvent) domain.ChannelActionEvent {
	actionValue := cte.Event.Action.Value
	formValue := cte.Event.Action.FormValue
	actionMap := mapx.Map(actionValue)

	// 1. 安全解引用操作人 ID（依次尝试 UserID、OpenID 及卡片载荷中的 user_id 兜底，避免身份丢失）
	var operatorID, openID string
	if cte.Event.Operator != nil {
		operatorID = lo.FromPtr(cte.Event.Operator.UserID)
		openID = cte.Event.Operator.OpenID
	}
	userID := strings.TrimSpace(lo.CoalesceOrEmpty(operatorID, openID, actionMap.GetString("user_id")))

	var msgID string
	if cte.Event.Context != nil {
		msgID = cte.Event.Context.OpenMessageID
	}

	// 2. 提取并清理批注（直接就地安全清理，避免额外分配新 map）
	var comment string
	if c, ok := formValue["comment"].(string); ok {
		comment = c
		delete(formValue, "comment")
	}

	// 3. 优雅回退动作类型（优先使用 Action.Name，为空时回退使用 Action.Value 载荷中的 action 字段）
	valAction := actionMap.GetString("action")
	actionName := strings.TrimSpace(lo.CoalesceOrEmpty(cte.Event.Action.Name, valAction))

	return domain.ChannelActionEvent{
		Channel:       domain.ChannelFeishu,
		ChannelUserID: userID,
		Action:        domain.ChannelActionType(actionName),
		TenantID:      actionMap.GetInt64("tenant_id"),
		TicketID:      actionMap.GetInt64("ticket_id"),
		TaskID:        domain.NewID(actionMap.GetString("task_id")),
		MessageID:     msgID,
		Comment:       comment,
		FormValues:    formValue,
	}
}
