package ioc

import (
	"fmt"

	"github.com/Duke1616/ecmdb/pkg/cryptox"
	"github.com/spf13/viper"
)

// InitCrypto 实例化加解密引擎组件，提供给任务派发器以及仓储层使用
func InitCrypto() cryptox.Crypto {
	type Config struct {
		Version string `mapstructure:"version"`
		Key     string `mapstructure:"key"`
	}

	var cfg Config

	if err := viper.UnmarshalKey("encryption", &cfg); err != nil {
		panic(fmt.Errorf("unable to decode into structure: %v", err))
	}

	// 验证配置
	if cfg.Version == "" {
		panic(fmt.Errorf("encryption version is required"))
	}
	if cfg.Key == "" {
		panic(fmt.Errorf("encryption key is required"))
	}

	// 构造并直接返回全局唯一加密服务
	return cryptox.NewCryptoManager("V2").
		Register("V2", cryptox.MustNewAESCryptoV2(cfg.Key)).
		Register(cfg.Version, cryptox.MustNewAESCrypto(cfg.Key)).
		WithLegacyAlgo(cfg.Version)
}
