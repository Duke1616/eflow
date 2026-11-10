package repository

import (
	"context"
	"testing"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository/dao"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubTicketDAO struct {
	dao.TicketDAO
	lastInstID      string
	lastInstInt     int
	lastProcessKey  string
}

func (s *stubTicketDAO) DetailByProcessInstance(_ context.Context, instanceID string) (dao.Ticket, error) {
	s.lastInstID = instanceID
	return dao.Ticket{Id: 2741, ProcessInstanceKey: instanceID}, nil
}

func (s *stubTicketDAO) DetailByProcessInstKey(_ context.Context, instanceKey string) (dao.Ticket, error) {
	s.lastProcessKey = instanceKey
	return dao.Ticket{Id: 2741, ProcessInstanceKey: instanceKey, EngineType: "flowx"}, nil
}

func (s *stubTicketDAO) DetailByProcessInstId(_ context.Context, instanceId int) (dao.Ticket, error) {
	s.lastInstInt = instanceId
	return dao.Ticket{Id: 888, ProcessInstanceId: instanceId, EngineType: "easy-workflow"}, nil
}

func TestTicketRepository_DetailByProcessInstance_Routing(t *testing.T) {
	stub := &stubTicketDAO{}
	repo := &ticketRepository{dao: stub}

	snowflakeID := "377374770910412800"
	ticket, err := repo.DetailByProcessInstance(context.Background(), domain.NewID(snowflakeID))
	require.NoError(t, err)
	assert.Equal(t, int64(2741), ticket.Id)
	assert.Equal(t, snowflakeID, stub.lastInstID)
}

func TestTicketRepository_DetailByEngineInstance(t *testing.T) {
	stub := &stubTicketDAO{}
	repo := &ticketRepository{dao: stub}

	// 1. FlowX 引擎显式路由到 Key
	snowflakeID := "377374770910412800"
	ticketFlowx, err := repo.DetailByEngineInstance(context.Background(), domain.EngineTypeFlowx, domain.NewID(snowflakeID))
	require.NoError(t, err)
	assert.Equal(t, int64(2741), ticketFlowx.Id)
	assert.Equal(t, snowflakeID, stub.lastProcessKey)

	// 2. EasyFlow 引擎显式路由到 Int ID（防腐转换）
	ticketEasy, err := repo.DetailByEngineInstance(context.Background(), domain.EngineTypeEasyFlow, domain.NewID(1001))
	require.NoError(t, err)
	assert.Equal(t, int64(888), ticketEasy.Id)
	assert.Equal(t, 1001, stub.lastInstInt)

	// 3. 未来通用引擎（如 temporal）默认按通用 Key 标准交互
	ticketCustom, err := repo.DetailByEngineInstance(context.Background(), domain.EngineType("temporal"), domain.NewID("wfe_custom_uuid"))
	require.NoError(t, err)
	assert.Equal(t, int64(2741), ticketCustom.Id)
	assert.Equal(t, "wfe_custom_uuid", stub.lastProcessKey)
}
