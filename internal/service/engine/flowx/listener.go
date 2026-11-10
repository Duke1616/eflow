package flowx

import (
	"context"
	"fmt"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository"
	"github.com/Duke1616/eflow/pkg/flowx"
	"github.com/gotomicro/ego/core/elog"
)

// EventListener 监听 flowx 流程引擎广播的核心生命周期事件，联动业务工单状态
type EventListener struct {
	repo   repository.TicketRepository
	logger *elog.Component
}

// NewEventListener 创建 flowx 事件监听器
func NewEventListener(repo repository.TicketRepository) flowx.IEventListener {
	return &EventListener{
		repo:   repo,
		logger: elog.DefaultLogger,
	}
}

func (l *EventListener) InterestIn() []flowx.EventType {
	return []flowx.EventType{
		flowx.EventProcessFinished,
		flowx.EventProcessRejected,
		flowx.EventProcessCancelled,
	}
}

func (l *EventListener) OnEvent(ctx context.Context, evt flowx.Event) error {
	// 异步协程生命周期脱钩，防止父 Context 超时或取消导致业务状态更新失败 (遵循并发安全准则)
	safeCtx := context.WithoutCancel(ctx)

	if evt.Instance == nil || evt.Instance.ID == "" {
		return nil
	}

	instanceKey := evt.Instance.ID

	switch evt.Type {
	case flowx.EventProcessFinished:
		// 流程正常办结：同步更新工单状态为 END，并记录办结时间 wtime
		now := time.Now().UnixMilli()
		if err := l.repo.UpdateStatusByProcessInstance(safeCtx, domain.NewID(instanceKey), domain.END.ToUint8(), now); err != nil {
			l.logger.Error("flowx 事件处理: 流程办结同步工单状态失败",
				elog.String("instanceKey", instanceKey),
				elog.FieldErr(err))
			return fmt.Errorf("更新工单办结状态失败: %w", err)
		}
		l.logger.Info("flowx 事件处理: 流程顺利办结，工单已归档", elog.String("instanceKey", instanceKey))

	case flowx.EventProcessRejected:
		// 流程驳回/直接终止：同步更新工单状态为 WITHDRAW
		now := time.Now().UnixMilli()
		if err := l.repo.UpdateStatusByProcessInstance(safeCtx, domain.NewID(instanceKey), domain.WITHDRAW.ToUint8(), now); err != nil {
			l.logger.Error("flowx 事件处理: 流程驳回同步工单状态失败",
				elog.String("instanceKey", instanceKey),
				elog.FieldErr(err))
			return fmt.Errorf("更新工单驳回状态失败: %w", err)
		}
		l.logger.Info("flowx 事件处理: 流程已终止驳回", elog.String("instanceKey", instanceKey))

	case flowx.EventProcessCancelled:
		// 提单人撤销：同步更新工单状态为 WITHDRAW
		now := time.Now().UnixMilli()
		if err := l.repo.UpdateStatusByProcessInstance(safeCtx, domain.NewID(instanceKey), domain.WITHDRAW.ToUint8(), now); err != nil {
			l.logger.Error("flowx 事件处理: 流程撤销同步工单状态失败",
				elog.String("instanceKey", instanceKey),
				elog.FieldErr(err))
			return fmt.Errorf("更新工单撤销状态失败: %w", err)
		}
		l.logger.Info("flowx 事件处理: 流程已撤销", elog.String("instanceKey", instanceKey))
	}

	return nil
}
