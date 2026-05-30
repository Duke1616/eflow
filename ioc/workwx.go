package ioc

import (
	"fmt"

	"github.com/spf13/viper"
	"github.com/xen0n/go-workwx"
)

// InitWorkWx 从系统配置中读取企微凭证并初始化企业微信 OA 应用客户端实例
func InitWorkWx() *workwx.WorkwxApp {
	type Config struct {
		// CorpSecret 应用凭证密钥
		CorpSecret string `mapstructure:"corp_secret"`
		// AgentID 应用 ID
		AgentID int64 `mapstructure:"agent_id"`
		// CorpID 企业微信唯一 ID
		CorpID string `mapstructure:"corp_id"`
	}

	var cfg Config
	if err := viper.UnmarshalKey("wechat", &cfg); err != nil {
		panic(fmt.Errorf("解析企业微信配置失败: %v", err))
	}

	workApp := workwx.New(cfg.CorpID).WithApp(cfg.CorpSecret, cfg.AgentID)

	// 启动企业微信 AccessToken 的定时自动刷新协程保护
	go workApp.SpawnAccessTokenRefresher()

	return workApp
}
