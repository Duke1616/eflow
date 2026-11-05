package compiler

import (
	"context"
	"testing"

	easyModel "github.com/Bunny3th/easy-workflow/workflow/model"
	"github.com/Duke1616/eflow/internal/domain"
	flowxModel "github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompositeCompiler(t *testing.T) {
	standardWf := domain.Workflow{
		Id:   1001,
		Name: "请假审批工作流",
		FlowData: domain.CanvasData{
			"nodes": []any{
				map[string]any{"id": "start_1", "type": "start", "properties": map[string]any{"name": "发起申请"}},
				map[string]any{"id": "approval_1", "type": "user", "properties": map[string]any{"name": "主管审批", "approved": []string{"tom"}}},
				map[string]any{"id": "end_1", "type": "end", "properties": map[string]any{"name": "结束"}},
			},
			"edges": []any{
				map[string]any{"id": "e1", "sourceNodeId": "start_1", "targetNodeId": "approval_1"},
				map[string]any{"id": "e2", "sourceNodeId": "approval_1", "targetNodeId": "end_1"},
			},
		},
	}

	testCases := []struct {
		name       string
		wf         domain.Workflow
		assertFunc func(t *testing.T, res CompileResult)
		wantErr    bool
		errSubStr  string
	}{
		{
			name: "默认留空引擎类型_向下兼容自动路由至easyflow引擎",
			wf: func() domain.Workflow {
				w := standardWf
				w.EngineType = ""
				return w
			}(),
			assertFunc: func(t *testing.T, res CompileResult) {
				assert.Equal(t, EngineTypeEasyFlow, res.EngineType)
				proc, ok := res.RawDef.(*easyModel.Process)
				require.True(t, ok)
				assert.NotNil(t, proc)
			},
		},
		{
			name: "显式指定flowx引擎_编译成功",
			wf: func() domain.Workflow {
				w := standardWf
				w.EngineType = EngineTypeFlowx
				return w
			}(),
			assertFunc: func(t *testing.T, res CompileResult) {
				assert.Equal(t, EngineTypeFlowx, res.EngineType)
				def, ok := res.RawDef.(flowxModel.ProcessDefinition)
				require.True(t, ok)
				assert.Equal(t, "wf_1001", def.Key)
				assert.Len(t, def.Nodes, 3)
			},
		},
		{
			name: "指定easy-workflow引擎_兼容编译成功",
			wf: func() domain.Workflow {
				w := standardWf
				w.EngineType = EngineTypeEasyFlow
				return w
			}(),
			assertFunc: func(t *testing.T, res CompileResult) {
				assert.Equal(t, EngineTypeEasyFlow, res.EngineType)
				proc, ok := res.RawDef.(*easyModel.Process)
				require.True(t, ok)
				assert.NotNil(t, proc)
			},
		},
	}

	compiler := NewDefaultCompositeCompiler()
	ctx := context.Background()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := compiler.Compile(ctx, tc.wf)

			if tc.wantErr {
				require.Error(t, err)
				if tc.errSubStr != "" {
					assert.Contains(t, err.Error(), tc.errSubStr)
				}
				return
			}

			require.NoError(t, err)
			if tc.assertFunc != nil {
				tc.assertFunc(t, res)
			}
		})
	}
}
