package ioc

import (
	"github.com/Duke1616/eflow/internal/event/ticket"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
	"github.com/spf13/viper"
)

// InitLarkClient 初始化飞书 API 客户端
func InitLarkClient() *lark.Client {
	appID := viper.GetString("lark.app_id")
	appSecret := viper.GetString("lark.app_secret")
	return lark.NewClient(appID, appSecret)
}

// InitLarkDispatcher 初始化飞书事件分发器
func InitLarkDispatcher(handler ticket.ILarkCallbackHandler) *dispatcher.EventDispatcher {
	vToken := viper.GetString("lark.verification_token")
	eKey := viper.GetString("lark.encrypt_key")

	return dispatcher.NewEventDispatcher(vToken, eKey).
		OnP2CardActionTrigger(handler.OnCardAction)
}

// InitLarkServer 初始化飞书长连接服务 (WebSocket)
func InitLarkServer(eventHandler *dispatcher.EventDispatcher) *ticket.LarkCallbackTicketServer {
	appID := viper.GetString("lark.app_id")
	appSecret := viper.GetString("lark.app_secret")

	cli := larkws.NewClient(appID, appSecret,
		larkws.WithEventHandler(eventHandler),
		larkws.WithLogLevel(larkcore.LogLevelError),
	)

	return ticket.NewLarkCallbackTicketServer(cli)
}
