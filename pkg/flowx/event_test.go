package flowx

import (
	"testing"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/stretchr/testify/assert"
)

func TestHookContextVariablesShareInstanceMap(t *testing.T) {
	instance := &model.Instance{}
	hook := NewHookContext(instance, nil, "", nil)

	hook.Variables.Set("direct", true)
	hook.MergeVariables(model.Variables{"merged": "yes"})

	assert.True(t, instance.Variables.GetBool("direct"))
	assert.Equal(t, "yes", instance.Variables.GetString("merged"))
}

// TestEventClone_VariablesReuseInstanceSnapshot 三个构造器都直送 inst.Variables，
// Clone 必须复用已克隆的实例变量表（同一张 map），同时与源实例彻底隔离
func TestEventClone_VariablesReuseInstanceSnapshot(t *testing.T) {
	node := &model.NodeSpec{ID: "n", Name: "审批节点"}
	task := &model.Task{ID: "t1", InstanceID: "i1"}

	testCases := []struct {
		name  string
		build func(*model.Instance) Event
	}{
		{"流程级事件", func(inst *model.Instance) Event {
			return NewProcessEvent(EventProcessStarted, inst, "tom", "")
		}},
		{"节点级事件", func(inst *model.Instance) Event {
			return NewNodeEvent(EventNodeEntered, inst, node, "tom", "")
		}},
		{"任务级事件", func(inst *model.Instance) Event {
			return NewTaskEvent(EventTaskCreated, inst, node, task, "tom", "")
		}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			inst := &model.Instance{ID: "i1", Variables: model.Variables{"amount": int(100)}}
			cp := tc.build(inst).Clone()

			// 改副本变量能被副本实例看到，证明二者是同一张表（只深拷贝了一遍）
			cp.Variables["marker"] = "x"
			assert.Equal(t, "x", cp.Instance.Variables["marker"])

			cp.Variables["amount"] = int(999)
			assert.Nil(t, inst.Variables["marker"], "副本写入不得回流源实例")
			assert.Equal(t, int(100), inst.Variables["amount"])
		})
	}
}

// TestEventClone_VariablesNilSemantics 复用优化不得改变 Variables 的有无：
// 原本为 nil 就仍为 nil；没有实例时仍须独立深拷贝，不能与源共享
func TestEventClone_VariablesNilSemantics(t *testing.T) {
	t.Run("有实例但变量为 nil", func(t *testing.T) {
		cp := Event{Instance: &model.Instance{ID: "i1"}}.Clone()
		assert.Nil(t, cp.Variables)
	})

	t.Run("无实例仅变量", func(t *testing.T) {
		vars := model.Variables{"amount": int(100)}
		cp := Event{Variables: vars}.Clone()

		cp.Variables["amount"] = int(999)
		assert.Equal(t, int(100), vars["amount"])
	})
}
