package hook

import (
	"context"
	"errors"
	"testing"

	"github.com/Bunny3th/easy-workflow/workflow/model"
	"github.com/Duke1616/eflow/internal/domain"
	ticketSvc "github.com/Duke1616/eflow/internal/service/ticket"
	"github.com/gotomicro/ego/core/elog"
	"github.com/stretchr/testify/require"
)

func TestEventNotifyEndPersistsCompletedStatus(t *testing.T) {
	tickets := &endTicketServiceStub{}
	event := &ProcessEvent{ticketSvc: tickets, logger: elog.DefaultLogger}

	err := event.EventNotify(101, &model.Node{NodeType: model.EndNode}, model.Node{})

	require.NoError(t, err)
	require.Equal(t, domain.END.ToUint8(), tickets.status)
}

func TestEventNotifyEndPropagatesPersistenceFailure(t *testing.T) {
	event := &ProcessEvent{
		ticketSvc: &endTicketServiceStub{err: errors.New("database unavailable")},
		logger:    elog.DefaultLogger,
	}

	err := event.EventNotify(101, &model.Node{NodeType: model.EndNode}, model.Node{})

	require.ErrorContains(t, err, "关闭工单失败")
}

type endTicketServiceStub struct {
	ticketSvc.Service
	status uint8
	err    error
}

func (s *endTicketServiceStub) UpdateStatusByProcessInstance(_ context.Context,
	_ domain.ID, status uint8, _ ...int64) error {
	s.status = status
	return s.err
}
