package easyflow

import (
	"context"
	"errors"
	"testing"

	"github.com/Duke1616/eflow/internal/domain"
	engineSvc "github.com/Duke1616/eflow/internal/service/engine"
	ticketSvc "github.com/Duke1616/eflow/internal/service/ticket"
	"github.com/gotomicro/ego/core/elog"
	"github.com/stretchr/testify/require"
)

func TestEventRevokeAcceptsPreparedWithdrawal(t *testing.T) {
	event := &ProcessEvent{
		engineSvc: &revokeEngineServiceStub{ticketID: "42"},
		ticketSvc: &revokeTicketServiceStub{ticket: domain.Ticket{
			Id: 42, TenantID: 7, Status: domain.WITHDRAWING,
		}},
		logger: elog.DefaultLogger,
	}

	err := event.EventRevoke(101, "starter")

	require.NoError(t, err)
}

func TestEventRevokeRejectsUnpreparedWithdrawal(t *testing.T) {
	event := &ProcessEvent{
		engineSvc: &revokeEngineServiceStub{ticketID: "42"},
		ticketSvc: &revokeTicketServiceStub{ticket: domain.Ticket{
			Id: 42, TenantID: 7, Status: domain.PROCESS,
		}},
		logger: elog.DefaultLogger,
	}

	err := event.EventRevoke(101, "starter")

	require.ErrorContains(t, err, "未进入撤回准备状态")
}

func TestEventRevokePropagatesTicketLookupError(t *testing.T) {
	event := &ProcessEvent{
		engineSvc: &revokeEngineServiceStub{ticketID: "42"},
		ticketSvc: &revokeTicketServiceStub{err: errors.New("database unavailable")},
		logger:    elog.DefaultLogger,
	}

	err := event.EventRevoke(101, "starter")

	require.ErrorContains(t, err, "查询撤回工单失败")
}

type revokeEngineServiceStub struct {
	engineSvc.Service
	ticketID string
	err      error
}

func (s *revokeEngineServiceStub) GetTicketIdByVariable(context.Context, int) (string, error) {
	return s.ticketID, s.err
}

type revokeTicketServiceStub struct {
	ticketSvc.Service
	ticket domain.Ticket
	err    error
}

func (s *revokeTicketServiceStub) GetByID(context.Context, int64) (domain.Ticket, error) {
	return s.ticket, s.err
}
