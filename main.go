package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Bunny3th/easy-workflow/workflow/engine"
	"github.com/Duke1616/eflow/ioc"
	"github.com/fsnotify/fsnotify"
	"github.com/gotomicro/ego"
	"github.com/gotomicro/ego/core/elog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "eflow",
		Short: "eflow 流程引擎统一入口",
	}

	dir, _ := os.Getwd()
	defaultCfg := dir + "/config/prod.yaml"
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", defaultCfg, "配置文件路径")

	cobra.OnInitialize(initViper)

	serverCmd := &cobra.Command{
		Use:   "server",
		Short: "启动服务节点",
		Run: func(cmd *cobra.Command, args []string) {
			startServer()
		},
	}

	rootCmd.AddCommand(serverCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func startServer() {
	app, err := ioc.InitApp()
	if err != nil {
		elog.Panic("init_app_failed", elog.FieldErr(err))
	}

	// 注册流程引擎驱动事件
	engine.RegisterEvents(app.Event)

	// 启动后台任务
	ctx := context.Background()
	app.StartBackgroundTasks(ctx)

	if err = ego.New().Serve(app.GetServers()...).Run(); err != nil {
		elog.Panic("app_run_error", elog.FieldErr(err))
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
		elog.DefaultLogger.Debug("已根据配置开启 Debug 日志级别")
	} else {
		elog.DefaultLogger.SetLevel(elog.InfoLevel)
	}
}
