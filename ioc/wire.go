//go:build wireinject
// +build wireinject

package ioc

import (
	"github.com/Duke1616/eflow/internal/service/workflow"
	"github.com/google/wire"
)

// InitApp 初始化完整应用
func InitApp() (*App, error) {
	wire.Build(
		WebSet,
		wire.Struct(new(App), "*"),
	)
	return nil, nil
}

// InitTemplateSyncer 初始化模板同步器
func InitTemplateSyncer() (workflow.ITemplateSyncer, error) {
	wire.Build(
		WorkflowSet,
		grpcSet,
		BaseSet,
	)
	return nil, nil
}
