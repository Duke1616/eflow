package flowx

import (
	"testing"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/stretchr/testify/assert"
)

func TestHookContextVariablesShareInstanceMap(t *testing.T) {
	testCases := []struct {
		name    string
		newHook func(*model.Instance) *HookContext
	}{
		{
			name: "node hook",
			newHook: func(instance *model.Instance) *HookContext {
				return NewHookContext(instance, nil, "", nil)
			},
		},
		{
			name: "task hook",
			newHook: func(instance *model.Instance) *HookContext {
				return NewTaskHookContext(instance, nil, nil, "")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			instance := &model.Instance{}
			hook := tc.newHook(instance)

			hook.Variables.Set("direct", true)
			hook.MergeVariables(model.Variables{"merged": "yes"})

			assert.True(t, instance.Variables.GetBool("direct"))
			assert.Equal(t, "yes", instance.Variables.GetString("merged"))
		})
	}
}
