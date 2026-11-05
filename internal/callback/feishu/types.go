package feishu

import (
	"github.com/spf13/viper"
)

// Action 飞书卡片专有交互行为类型
type Action string

const (
	Pass     Action = "pass"
	Reject   Action = "reject"
	Progress Action = "progress"
	Revoke   Action = "revoke"
)

type callbackConfig struct {
	Debug       bool   `mapstructure:"debug"`
	FrontendUrl string `mapstructure:"frontend_url"`
}

func getLarkCallbackConfig() callbackConfig {
	var cfg callbackConfig
	if err := viper.UnmarshalKey("lark.callback", &cfg); err != nil {
		panic(err)
	}
	return cfg
}
