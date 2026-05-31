package ticket

import (
	"context"

	"github.com/gotomicro/ego/core/elog"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkcallback "github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
	"github.com/spf13/viper"
)

// LarkCallbackTicketServer 飞书长连接实时回调（WebSocket）服务容器实现
// NOTE: 该服务通过 WebSocket 维持与飞书服务器的长连接心跳，在本地测试/开发及线上环境可做到零外网配置实时回调接收
type LarkCallbackTicketServer struct {
	cli     *larkws.Client
	handler ILarkCallbackHandler
	logger  *elog.Component
}

// NewLarkCallbackTicketServer 构造飞书 WebSocket 长连接回调接收服务
func NewLarkCallbackTicketServer(handler ILarkCallbackHandler, client *lark.Client) *LarkCallbackTicketServer {
	type Config struct {
		AppId             string `mapstructure:"app_id"`
		AppSecret         string `mapstructure:"app_secret"`
		EncryptKey        string `mapstructure:"encrypt_key"`
		VerificationToken string `mapstructure:"verification_token"`
	}

	var cfg Config
	// 读取并解构 feishu 配置小节里的各字段密钥
	if err := viper.UnmarshalKey("feishu", &cfg); err != nil {
		elog.Error("解析 feishu 配置块失败", elog.FieldErr(err))
	}

	s := &LarkCallbackTicketServer{
		handler: handler,
		logger:  elog.DefaultLogger.With(elog.FieldComponentName("LarkwsServer")),
	}

	// 绑定飞书卡片按钮点击行为的分发处理器 OnCardAction
	eventHandler := dispatcher.NewEventDispatcher(cfg.VerificationToken, cfg.EncryptKey).
		OnP2CardActionTrigger(s.OnCardAction)

	// 初始化官方 websocket 长连接客户端并挂载事件分发器
	cli := larkws.NewClient(cfg.AppId, cfg.AppSecret,
		larkws.WithEventHandler(eventHandler),
		larkws.WithLogLevel(larkcore.LogLevelInfo),
	)

	s.cli = cli
	return s
}

// Start 启动飞书长连接长驻监听任务（符合 ioc.Task 接口）
func (s *LarkCallbackTicketServer) Start(ctx context.Context) {
	s.logger.Info("正在建立飞书 WebSocket 实时长连接以直接监听审批卡片交互事件...")
	// 阻塞启动长连接并保持心跳
	err := s.cli.Start(ctx)
	if err != nil {
		s.logger.Error("飞书长连接异常断开或遭遇错误退出", elog.FieldErr(err))
	} else {
		s.logger.Info("飞书 WebSocket 长连接已优雅关闭")
	}
}

// OnCardAction 接收飞书卡片上的用户交互点击回调
func (s *LarkCallbackTicketServer) OnCardAction(ctx context.Context, cte *larkcallback.CardActionTriggerEvent) (*larkcallback.CardActionTriggerResponse, error) {
	s.logger.Info("捕捉到飞书互动卡片交互点击回调")

	if cte.Event.Action == nil || cte.Event.Action.Value == nil {
		s.logger.Warn("忽略非法的空动作飞书交互事件")
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
		s.logger.Info("已触发本地异步驱动工单实例流转流程", elog.Any("evt", evt))
		if err := s.handler.Handle(localCtx, evt); err != nil {
			s.logger.Error("本地异步驱动飞书卡片事件流转失败", elog.FieldErr(err))
		} else {
			s.logger.Info("本地异步驱动飞书卡片事件流转成功")
		}
	}()

	return &larkcallback.CardActionTriggerResponse{}, nil
}
