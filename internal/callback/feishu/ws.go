package feishu

import (
	"context"

	"github.com/gotomicro/ego/core/elog"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

// LarkCallbackServer 飞书长连接实时回调（WebSocket）服务容器实现
// NOTE: 该服务通过 WebSocket 维持与飞书服务器的长连接心跳，在本地测试/开发及线上环境可做到零外网配置实时回调接收
type LarkCallbackServer struct {
	cli    *larkws.Client
	logger *elog.Component
}

// LarkCallbackTicketServer 保持兼容的类型别名
type LarkCallbackTicketServer = LarkCallbackServer

// NewLarkCallbackServer 构造飞书 WebSocket 长连接回调接收服务
func NewLarkCallbackServer(cli *larkws.Client) *LarkCallbackServer {
	return &LarkCallbackServer{
		cli:    cli,
		logger: elog.DefaultLogger.With(elog.FieldComponentName("LarkwsServer")),
	}
}

// NewLarkCallbackTicketServer 保持兼容的构造器
var NewLarkCallbackTicketServer = NewLarkCallbackServer

// Start 启动飞书长连接长驻监听任务（符合 ioc.Task 接口）
func (s *LarkCallbackServer) Start(ctx context.Context) {
	s.logger.Info("正在建立飞书 WebSocket 实时长连接以直接监听审批卡片交互事件...")
	// 阻塞启动长连接并保持心跳
	err := s.cli.Start(ctx)
	if err != nil {
		s.logger.Error("飞书长连接异常断开或遭遇错误退出", elog.FieldErr(err))
	} else {
		s.logger.Info("飞书 WebSocket 长连接已优雅关闭")
	}
}
