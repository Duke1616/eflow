package ticket

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Bunny3th/easy-workflow/workflow/engine"
	"github.com/Bunny3th/easy-workflow/workflow/model"
	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	"github.com/Duke1616/eflow/internal/pkg/rule"
	engineSvc "github.com/Duke1616/eflow/internal/service/engine"
	templateSvc "github.com/Duke1616/eflow/internal/service/template"
	ticketSvc "github.com/Duke1616/eflow/internal/service/ticket"
	workflowSvc "github.com/Duke1616/eflow/internal/service/workflow"
	"github.com/chromedp/chromedp"
	"github.com/ecodeclub/ekit/slice"
	"github.com/gotomicro/ego/core/elog"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkcallback "github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"github.com/spf13/viper"
)

type callback struct {
	Debug       bool   `mapstructure:"debug"`
	FrontendUrl string `mapstructure:"frontend_url"`
}

func getLarkCallbackConfig() callback {
	var cfg callback
	if err := viper.UnmarshalKey("lark.callback", &cfg); err != nil {
		panic(err)
	}
	return cfg
}

// ILarkCallbackHandler 飞书审批卡片回调交互核心业务接口定义
type ILarkCallbackHandler interface {
	// Handle 本地驱动并处理飞书卡片上的用户动作（Pass/Reject/Progress/Revoke），支持卡片状态变更、无头浏览器进度截图
	Handle(ctx context.Context, evt LarkCallback) error
	// OnCardAction 接收飞书卡片上的用户交互点击回调
	OnCardAction(ctx context.Context, cte *larkcallback.CardActionTriggerEvent) (*larkcallback.CardActionTriggerResponse, error)
}

type larkCallbackHandler struct {
	logger      *elog.Component
	cfg         callback
	engineSvc   engineSvc.Service
	svc         ticketSvc.Service
	templateSvc templateSvc.Service
	userSvc     UserService
	workflowSvc workflowSvc.Service
	lark        *lark.Client
}

// NewLarkCallbackHandler 构造飞书卡片业务处理器
func NewLarkCallbackHandler(
	engineSvc engineSvc.Service,
	svc ticketSvc.Service,
	templateSvc templateSvc.Service,
	userSvc UserService,
	workflowSvc workflowSvc.Service,
	lark *lark.Client,
) ILarkCallbackHandler {
	return &larkCallbackHandler{
		logger:      elog.DefaultLogger.With(elog.FieldComponentName("LarkCallbackHandler")),
		cfg:         getLarkCallbackConfig(),
		engineSvc:   engineSvc,
		svc:         svc,
		templateSvc: templateSvc,
		userSvc:     userSvc,
		workflowSvc: workflowSvc,
		lark:        lark,
	}
}

// OnCardAction 接收飞书卡片上的用户交互点击回调
func (h *larkCallbackHandler) OnCardAction(ctx context.Context, cte *larkcallback.CardActionTriggerEvent) (*larkcallback.CardActionTriggerResponse, error) {
	h.logger.Info("捕捉到飞书互动卡片交互点击回调")

	if cte.Event.Action == nil || cte.Event.Action.Value == nil {
		h.logger.Warn("忽略非法的空动作飞书交互事件")
		return &larkcallback.CardActionTriggerResponse{}, nil
	}

	actionValue := cte.Event.Action.Value
	formValue := cte.Event.Action.FormValue

	userID := ""
	if cte.Event.Operator != nil && cte.Event.Operator.UserID != nil {
		userID = *cte.Event.Operator.UserID
	}

	openID := ""
	if cte.Event.Operator != nil {
		openID = cte.Event.Operator.OpenID
	}

	msgID := ""
	if cte.Event.Context != nil {
		msgID = cte.Event.Context.OpenMessageID
	}

	evt := LarkCallback{
		UserId:    userID,
		OpenId:    openID,
		MessageId: msgID,
		Value:     actionValue,
		FormValue: formValue,
	}

	// 异步调用业务处理器的 Handle 方法，面向接口，零耦合瞬间流转工单！
	go func() {
		localCtx := context.Background()
		h.logger.Info("已触发本地异步驱动工单实例流转流程", elog.Any("evt", evt))
		if err := h.Handle(localCtx, evt); err != nil {
			h.logger.Error("本地异步驱动飞书卡片事件流转失败", elog.FieldErr(err))
		} else {
			h.logger.Info("本地异步驱动飞书卡片事件流转成功")
		}
	}()

	return &larkcallback.CardActionTriggerResponse{}, nil
}

// Handle 执行具体的流转处理逻辑
func (h *larkCallbackHandler) Handle(ctx context.Context, evt LarkCallback) error {
	taskId, err := evt.GetTaskIdInt()
	if err != nil {
		return err
	}

	ticketId, err := evt.GetTicketIdInt()
	if err != nil {
		return err
	}

	comment := evt.GetComment()
	if comment == "" {
		comment = "无"
	}

	h.logger.Debug("获取飞书回调信息", elog.Any("evt", evt),
		elog.Any("ticket_id", ticketId),
		elog.Any("task_id", taskId),
	)

	var remark string
	switch evt.GetAction() {
	case Pass:
		remark = fmt.Sprintf("你已同意该申请, 批注：%s", comment)
		if err = h.svc.Pass(ctx, taskId, comment, evt.GetFormValue()); err != nil {
			h.logger.Error("飞书回调消息，同意工单失败", elog.FieldErr(err),
				elog.Int("任务ID", taskId),
				elog.Int64("工单ID", ticketId),
			)
			return err
		}
		return h.withdraw(ctx, evt, remark)
	case Reject:
		remark = fmt.Sprintf("你已驳回该申请, 批注：%s", comment)
		err = h.svc.Reject(ctx, taskId, comment)
		if err != nil {
			h.logger.Error("飞书回调消息，驳回工单失败", elog.FieldErr(err),
				elog.String("任务ID", evt.GetTaskId()),
				elog.String("工单ID", evt.GetTicketId()),
			)
			return err
		}
		return h.withdraw(ctx, evt, remark)
	case Progress:
		err = h.progress(ticketId, evt.GetUserId())
		if err != nil {
			h.logger.Error("查看流程进度失败", elog.FieldErr(err))
			return err
		}
		return nil
	case Revoke:
		remark = fmt.Sprintf("你已撤销该申请, 批注：%s", evt.GetComment())
		var ticketResp domain.Ticket
		ticketResp, err = h.svc.GetByID(ctx, ticketId)
		if err != nil {
			return err
		}

		var userResp *userv1.User
		userResp, err = h.userSvc.FindByFeishuUserId(ctx, evt.GetUserId())
		if err != nil {
			return err
		}

		// NOTE: 撤销流程，直接调用 easy-workflow 的包级别 InstanceRevoke 进行终止
		err = engine.InstanceRevoke(ticketResp.Process.InstanceId, true, userResp.Username)
		if err != nil {
			remark = "你的节点任务已经结束，无法进行撤回，详情登录系统查看"
			h.logger.Error("飞书回调消息，撤销工单失败", elog.FieldErr(err))
			return err
		}

		err = h.svc.UpdateStatusByProcessInstanceID(ctx, ticketResp.Process.InstanceId, domain.WITHDRAW.ToUint8())
		if err != nil {
			h.logger.Error("撤销变更流程状态失败", elog.FieldErr(err))
			return err
		}

		return h.withdraw(ctx, evt, remark)
	default:
		h.logger.Error("没有匹配到任何选项")
		return nil
	}
}

func (h *larkCallbackHandler) progress(ticketId int64, userId string) error {
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

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()

	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
	defer cancelBrowser()

	taskCtx, cancelTask := context.WithTimeout(browserCtx, 30*time.Second)
	defer cancelTask()
	var buf []byte

	ticketDetail, err := h.svc.GetByID(taskCtx, ticketId)
	if err != nil {
		return err
	}

	inst, err := h.engineSvc.GetInstanceByID(taskCtx, ticketDetail.Process.InstanceId)
	if err != nil {
		return err
	}

	wf, err := h.workflowSvc.FindInstanceFlow(taskCtx, ticketDetail.WorkflowId, inst.ProcID, inst.ProcVersion)
	if err != nil {
		return err
	}

	edges, nodeStatusMap, approvalUsers, err := h.parserEdges(taskCtx, ticketDetail, wf.ProcessId)
	if err != nil {
		return err
	}

	injectData, err := h.getJsCode(wf, edges, nodeStatusMap)
	if err != nil {
		return err
	}

	err = chromedp.Run(taskCtx,
		chromedp.EmulateViewport(1920, 1080, chromedp.EmulateScale(1)),
		chromedp.Navigate(h.cfg.FrontendUrl),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(injectData, nil),
		chromedp.WaitVisible("#LF-preview", chromedp.ByID),
		chromedp.WaitVisible(`#LF-preview[data-rendered="true"]`, chromedp.ByQuery),
		chromedp.Sleep(300*time.Millisecond),
		chromedp.Screenshot("#LF-preview", &buf, chromedp.NodeVisible, chromedp.ByID),
	)
	if err != nil {
		return err
	}

	imageKey, err := h.uploadImage(taskCtx, buf)
	if err != nil {
		return err
	}

	return h.sendImage(taskCtx, imageKey, approvalUsers, userId)
}

func (h *larkCallbackHandler) parserEdges(ctx context.Context, o domain.Ticket,
	processId int) (map[string][]string, map[string]int, []string, error) {
	record, _, err := h.engineSvc.TaskRecord(ctx, o.Process.InstanceId, 0, 1000)
	if err != nil {
		return nil, nil, nil, err
	}

	users := slice.FilterMap(record, func(idx int, src model.Task) (string, bool) {
		if src.Status == 0 && src.IsFinished == 0 {
			return src.UserID, true
		}
		return "", false
	})

	nodeStatusMap := make(map[string]int)
	for _, task := range record {
		nodeStatusMap[task.NodeID] = task.Status
	}

	edges, err := h.engineSvc.GetTraversedEdges(ctx, record, o.Process.InstanceId, processId, o.Status.ToUint8())
	if err != nil {
		return nil, nil, nil, err
	}

	return edges, nodeStatusMap, users, nil
}

func (h *larkCallbackHandler) sendImage(ctx context.Context, imageKey *string, approvalUsers []string, userId string) error {
	h.logger.Info("【模拟发送进度图卡片】成功",
		elog.String("UserId", userId),
		elog.Any("approvalUsers", approvalUsers),
		elog.String("imageKey", *imageKey),
	)
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
	edges := slice.Map(wf.FlowData.Edges, func(idx int, src domain.FlowEdge) map[string]interface{} {
		return src
	})
	nodes := slice.Map(wf.FlowData.Nodes, func(idx int, src domain.FlowNode) map[string]interface{} {
		return src
	})
	return easyflow.GetJsCode(easyflow.LogicFlow{
		Edges: edges,
		Nodes: nodes,
	}, edgeMap, nodeStatusMap)
}

func (h *larkCallbackHandler) withdraw(ctx context.Context, callback LarkCallback, remark string) error {
	ticketIdInt, err := callback.GetTicketIdInt()
	if err != nil {
		return err
	}

	fTicket, err := h.svc.GetByID(ctx, ticketIdInt)
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

	h.logger.Info("【降级卡片变态更新】飞书卡态更新成功",
		elog.String("UserId", callback.GetUserId()),
		elog.String("Creator", userInfo.DisplayName),
		elog.String("TemplateName", t.Name),
		elog.Any("Fields", ruleFields),
		elog.String("Remark", remark),
	)
	return nil
}
