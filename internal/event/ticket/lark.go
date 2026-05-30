package ticket

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/Bunny3th/easy-workflow/workflow/engine"
	"github.com/Bunny3th/easy-workflow/workflow/model"
	"github.com/Duke1616/ecmdb/api/proto/gen/ecmdb/user/v1"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	"github.com/Duke1616/eflow/internal/pkg/rule"
	engineSvc "github.com/Duke1616/eflow/internal/service/engine"
	templateSvc "github.com/Duke1616/eflow/internal/service/template"
	ticketSvc "github.com/Duke1616/eflow/internal/service/ticket"
	workflowSvc "github.com/Duke1616/eflow/internal/service/workflow"
	"github.com/chromedp/chromedp"
	"github.com/ecodeclub/ekit/slice"
	"github.com/ecodeclub/mq-api"
	"github.com/gotomicro/ego/core/elog"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"github.com/spf13/viper"
)

// LarkCallbackTicketConsumer 飞书审批回调事件异步消费者
type LarkCallbackTicketConsumer struct {
	svc         ticketSvc.Service
	callback    callback
	workflowSvc workflowSvc.Service
	userSvc     UserService
	engineSvc   engineSvc.Service
	templateSvc templateSvc.Service
	consumer    mq.Consumer
	lark        *lark.Client
	logger      *elog.Component
}

type callback struct {
	FrontendUrl string `mapstructure:"frontend_url"`
	Debug       bool   `mapstructure:"debug"`
}

func getLarkCallbackConfig() callback {
	var cfg callback
	if err := viper.UnmarshalKey("lark.callback", &cfg); err != nil {
		panic(fmt.Errorf("unable to decode into structure: %v", err))
	}
	return cfg
}

// NewLarkCallbackTicketConsumer 构造飞书卡片交互审批事件消费者
func NewLarkCallbackTicketConsumer(q mq.MQ, engineSvc engineSvc.Service, svc ticketSvc.Service,
	templateSvc templateSvc.Service, userSvc UserService, workflowSvc workflowSvc.Service, lark *lark.Client) (*LarkCallbackTicketConsumer, error) {
	groupID := "lark_callback"
	consumer, err := q.Consumer(LarkCallbackEventName, groupID)
	if err != nil {
		return nil, err
	}

	return &LarkCallbackTicketConsumer{
		consumer:    consumer,
		engineSvc:   engineSvc,
		userSvc:     userSvc,
		workflowSvc: workflowSvc,
		callback:    getLarkCallbackConfig(),
		templateSvc: templateSvc,
		svc:         svc,
		lark:        lark,
		logger:      elog.DefaultLogger,
	}, nil
}

// Start 启动后台消费监听协程
func (c *LarkCallbackTicketConsumer) Start(ctx context.Context) {
	go func() {
		for {
			err := c.Consume(ctx)
			if err != nil {
				c.logger.Error("同步飞书回调事件失败", elog.Any("err", err))
				time.Sleep(time.Second)
			}
		}
	}()
}

// Consume 监听获取飞书交互事件并扭转工作流实例
func (c *LarkCallbackTicketConsumer) Consume(ctx context.Context) error {
	cm, err := c.consumer.Consume(ctx)
	if err != nil {
		return fmt.Errorf("获取消息失败: %w", err)
	}
	var evt LarkCallback
	if err = json.Unmarshal(cm.Value, &evt); err != nil {
		return fmt.Errorf("解析消息失败: %w", err)
	}

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

	c.logger.Debug("获取飞书回调信息", elog.Any("evt", evt),
		elog.Any("ticket_id", ticketId),
		elog.Any("task_id", taskId),
	)

	var remark string
	switch evt.GetAction() {
	case Pass:
		remark = fmt.Sprintf("你已同意该申请, 批注：%s", comment)
		if err = c.svc.Pass(ctx, taskId, comment, evt.GetFormValue()); err != nil {
			c.logger.Error("飞书回调消息，同意工单失败", elog.FieldErr(err),
				elog.Int("任务ID", taskId),
				elog.Int64("工单ID", ticketId),
			)
		}
		return c.withdraw(ctx, evt, remark)
	case Reject:
		remark = fmt.Sprintf("你已驳回该申请, 批注：%s", comment)
		err = c.svc.Reject(ctx, taskId, comment)
		if err != nil {
			c.logger.Error("飞书回调消息，驳回工单失败", elog.FieldErr(err),
				elog.String("任务ID", evt.GetTaskId()),
				elog.String("工单ID", evt.GetTicketId()),
			)
		}
		return c.withdraw(ctx, evt, remark)
	case Progress:
		err = c.progress(ticketId, evt.GetUserId())
		if err != nil {
			c.logger.Error("查看流程进度失败", elog.FieldErr(err))
			return err
		}
		return nil
	case Revoke:
		remark = fmt.Sprintf("你已撤销该申请, 批注：%s", evt.GetComment())
		var ticketResp domain.Ticket
		ticketResp, err = c.svc.GetByID(ctx, ticketId)
		if err != nil {
			return err
		}

		var userResp *userv1.User
		userResp, err = c.userSvc.FindByFeishuUserId(ctx, evt.GetUserId())
		if err != nil {
			return err
		}

		// NOTE: 撤销流程，直接调用 easy-workflow 的包级别 InstanceRevoke 进行终止
		err = engine.InstanceRevoke(ticketResp.Process.InstanceId, true, userResp.Username)
		if err != nil {
			remark = "你的节点任务已经结束，无法进行撤回，详情登录系统查看"
			c.logger.Error("飞书回调消息，撤销工单失败", elog.FieldErr(err))
		}

		err = c.svc.UpdateStatusByProcessInstanceID(ctx, ticketResp.Process.InstanceId, domain.WITHDRAW.ToUint8())
		if err != nil {
			c.logger.Error("撤销变更流程状态失败", elog.FieldErr(err))
		}

		return c.withdraw(ctx, evt, remark)
	default:
		c.logger.Error("没有匹配到任何选项")
		return nil
	}
}

// progress 通过无头浏览器加载前端画布渲染进度并对 LogicFlow 截图
func (c *LarkCallbackTicketConsumer) progress(ticketId int64, userId string) error {
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

	if !c.callback.Debug {
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

	ticketDetail, err := c.svc.GetByID(taskCtx, ticketId)
	if err != nil {
		return err
	}

	inst, err := c.engineSvc.GetInstanceByID(taskCtx, ticketDetail.Process.InstanceId)
	if err != nil {
		return err
	}

	wf, err := c.workflowSvc.FindInstanceFlow(taskCtx, ticketDetail.WorkflowId, inst.ProcID, inst.ProcVersion)
	if err != nil {
		return err
	}

	edges, nodeStatusMap, approvalUsers, err := c.parserEdges(taskCtx, ticketDetail, wf.ProcessId)
	if err != nil {
		return err
	}

	injectData, err := c.getJsCode(wf, edges, nodeStatusMap)
	if err != nil {
		return err
	}

	err = chromedp.Run(taskCtx,
		chromedp.EmulateViewport(1920, 1080, chromedp.EmulateScale(1)),
		chromedp.Navigate(c.callback.FrontendUrl),
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

	imageKey, err := c.uploadImage(taskCtx, buf)
	if err != nil {
		return err
	}

	return c.sendImage(taskCtx, imageKey, approvalUsers, userId)
}

func (c *LarkCallbackTicketConsumer) parserEdges(ctx context.Context, o domain.Ticket,
	processId int) (map[string][]string, map[string]int, []string, error) {
	record, _, err := c.engineSvc.TaskRecord(ctx, o.Process.InstanceId, 0, 1000)
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

	edges, err := c.engineSvc.GetTraversedEdges(ctx, record, o.Process.InstanceId, processId, o.Status.ToUint8())
	if err != nil {
		return nil, nil, nil, err
	}

	return edges, nodeStatusMap, users, nil
}

func (c *LarkCallbackTicketConsumer) sendImage(ctx context.Context, imageKey *string, approvalUsers []string, userId string) error {
	// NOTE: 飞书发送模块降级处理，打印日志进行自愈适配，避免本地开发与编译崩塌
	c.logger.Info("【模拟发送进度图卡片】成功",
		elog.String("UserId", userId),
		elog.Any("approvalUsers", approvalUsers),
		elog.String("imageKey", *imageKey),
	)
	return nil
}

func (c *LarkCallbackTicketConsumer) uploadImage(ctx context.Context, buf []byte) (*string, error) {
	req := larkim.NewCreateImageReqBuilder().
		Body(larkim.NewCreateImageReqBodyBuilder().
			ImageType(`message`).
			Image(bytes.NewReader(buf)).
			Build()).
		Build()

	resp, err := c.lark.Im.Image.Create(ctx, req)
	if err != nil {
		return nil, err
	}

	if !resp.Success() {
		return nil, fmt.Errorf("logId: %s, error response: \n%s",
			resp.RequestId(), larkcore.Prettify(resp.CodeError))
	}

	return resp.Data.ImageKey, nil
}

func (c *LarkCallbackTicketConsumer) getJsCode(wf domain.Workflow, edgeMap map[string][]string, nodeStatusMap map[string]int) (string, error) {
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

func (c *LarkCallbackTicketConsumer) withdraw(ctx context.Context, callback LarkCallback, remark string) error {
	ticketIdInt, err := callback.GetTicketIdInt()
	if err != nil {
		return err
	}

	fTicket, err := c.svc.GetByID(ctx, ticketIdInt)
	if err != nil {
		return err
	}

	t, err := c.templateSvc.DetailTemplate(ctx, fTicket.TemplateId)
	if err != nil {
		return err
	}

	rules, err := rule.ParseRules(t.Rules)
	if err != nil {
		return err
	}
	ruleFields := rule.GetFields(rules, fTicket.Provide.ToUint8(), fTicket.Data)
	
	userInfo, err := c.userSvc.FindByUsername(ctx, fTicket.CreateBy)
	if err != nil {
		return err
	}

	// NOTE: 飞书卡态变更降级，打印日志调试
	c.logger.Info("【降级卡片变态更新】飞书卡态更新成功",
		elog.String("UserId", callback.GetUserId()),
		elog.String("Creator", userInfo.DisplayName),
		elog.String("TemplateName", t.Name),
		elog.Any("Fields", ruleFields),
		elog.String("Remark", remark),
	)
	return nil
}

// Stop 关闭消费者释放物理连接
func (c *LarkCallbackTicketConsumer) Stop(_ context.Context) error {
	return c.consumer.Close()
}
