package main

import (
	"fmt"
	"os"

	"github.com/Duke1616/eflow/cmd/migrate"
	"github.com/Duke1616/eflow/cmd/server"
	"github.com/Duke1616/eflow/cmd/sync"
	"github.com/fsnotify/fsnotify"
	"github.com/gotomicro/ego/core/elog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

func main() {
	rootCmd := &cobra.Command{
		Use:   "eflow",
		Short: "eflow 工单系统统一入口",
	}

	// 1. 设置全局配置文件参数
	dir, _ := os.Getwd()
	defaultCfg := dir + "/config/config.yaml"
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", defaultCfg, "配置文件路径")

	// 2. 初始化配置中心
	cobra.OnInitialize(initViper)

	// 3. 注册启动服务的子命令
	rootCmd.AddCommand(server.NewCommand())
	rootCmd.AddCommand(migrate.NewCommand())
	rootCmd.AddCommand(sync.NewCommand())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func initViper() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	}

	viper.WatchConfig()
	if err := viper.ReadInConfig(); err != nil {
		fmt.Printf("Warning: 配置文件读取失败: %v\n", err)
	} else {
		fmt.Printf("Using config file: %s\n", viper.ConfigFileUsed())
		setLogLevel()
	}

	// 监听配置变更，支持动态切换日志级别
	viper.OnConfigChange(func(in fsnotify.Event) {
		setLogLevel()
	})
}

// setLogLevel 根据配置文件中的 log.debug 动态调整全局日志级别
func setLogLevel() {
	if viper.GetBool("log.debug") {
		elog.DefaultLogger.SetLevel(elog.DebugLevel)
	} else {
		elog.DefaultLogger.SetLevel(elog.InfoLevel)
	}
}
