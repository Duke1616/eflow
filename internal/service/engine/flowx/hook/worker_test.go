package hook

import (
	"context"
	"testing"

	"github.com/Duke1616/eflow/internal/domain"
	taskSvc "github.com/Duke1616/eflow/internal/service/task"
	"github.com/Duke1616/eflow/pkg/flowx"
	flowxModel "github.com/Duke1616/eflow/pkg/flowx/model"
	flowxMem "github.com/Duke1616/eflow/pkg/flowx/storage/memory"
	"github.com/stretchr/testify/require"
)

type taskServiceStub struct {
	taskSvc.Service
	ticketID    int64
	instanceID  domain.ID
	nodeID      string
	nodeName    string
	procVersion int
}

func (s *taskServiceStub) CreateTask(_ context.Context, ticketID int64,
	instanceID domain.ID, nodeID, nodeName string, procVersion ...int) (domain.Task, error) {
	s.ticketID = ticketID
	s.instanceID = instanceID
	s.nodeID = nodeID
	s.nodeName = nodeName
	if len(procVersion) > 0 {
		s.procVersion = procVersion[0]
	}
	return domain.Task{}, nil
}

func TestRegisterAutomationWorker(t *testing.T) {
	store := flowxMem.NewStorage()
	engine := flowx.NewEngine(store)
	stub := &taskServiceStub{}

	RegisterAutomationWorker(engine, stub)

	ctx := context.Background()
	def := flowxModel.ProcessDefinition{
		Key:  "wf_auto",
		Name: "自动化流程",
		Nodes: []flowxModel.NodeSpec{
			{ID: "start", Type: flowxModel.NodeStart},
			{ID: "auto_1", Name: "自动化节点", Type: flowxModel.NodeTask, WorkerType: "automation"},
			{ID: "end", Type: flowxModel.NodeEnd},
		},
		Edges: []flowxModel.EdgeSpec{
			{Source: "start", Target: "auto_1"},
			{Source: "auto_1", Target: "end"},
		},
	}
	_, err := engine.Deploy(ctx, def)
	require.NoError(t, err)

	_, err = engine.Start(ctx, flowx.StartReq{
		Key:         "wf_auto",
		BusinessKey: "1001",
		Version:     1,
	})
	require.NoError(t, err)

	require.Equal(t, int64(1001), stub.ticketID)
	require.False(t, stub.instanceID.IsZero())
	require.Equal(t, "auto_1", stub.nodeID)
	require.Equal(t, "自动化节点", stub.nodeName)
	require.Equal(t, 1, stub.procVersion)
}
