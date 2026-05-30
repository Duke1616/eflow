package ioc

import (
	lark "github.com/larksuite/oapi-sdk-go/v3"
	"github.com/spf13/viper"
)

// InitLarkClient 初始化飞书 API 客户端
func InitLarkClient() *lark.Client {
	type Config struct {
		AppId     string `mapstructure:"app_id"`
		AppSecret string `mapstructure:"app_secret"`
	}
	var cfg Config
	if err := viper.UnmarshalKey("feishu", &cfg); err != nil {
		panic(err)
	}

	return lark.NewClient(cfg.AppId, cfg.AppSecret)
}
