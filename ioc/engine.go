package ioc

import (
	"github.com/Duke1616/eflow/internal/repository"
	engineSvc "github.com/Duke1616/eflow/internal/service/engine"
	easyflowDrv "github.com/Duke1616/eflow/internal/service/engine/easyflow"
	flowxDrv "github.com/Duke1616/eflow/internal/service/engine/flowx"
	"github.com/Duke1616/eflow/pkg/flowx"
	flowxMysql "github.com/Duke1616/eflow/pkg/flowx/storage/mysql"
	"gorm.io/gorm"
)

// InitFlowxEngine 初始化现代工作流引擎 flowx 实例
func InitFlowxEngine(db *gorm.DB) flowx.Engine {
	store := flowxMysql.NewStorage(db)
	return flowx.NewEngine(store)
}

// InitEngineDispatcher 基于微内核驱动架构组装各底层引擎适配器
func InitEngineDispatcher(repo repository.IEngineRepository, flowxEngine flowx.Engine) engineSvc.IEngineDispatcher {
	easyflowAdapter := easyflowDrv.NewAdapter(repo)
	flowxAdapter := flowxDrv.NewAdapter(flowxEngine)
	return engineSvc.NewEngineDispatcher(easyflowAdapter, flowxAdapter)
}
