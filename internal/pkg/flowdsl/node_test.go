package flowdsl_test

import (
	"testing"

	"github.com/Duke1616/eflow/internal/pkg/flowdsl"
	"github.com/stretchr/testify/assert"
)

func TestNode_AsUserTask(t *testing.T) {
	t.Run("nil 节点安全返回 false", func(t *testing.T) {
		var n *flowdsl.Node
		cfg, ok := n.AsUserTask()
		assert.False(t, ok)
		assert.Nil(t, cfg)
	})

	t.Run("非审批节点返回 false", func(t *testing.T) {
		n := &flowdsl.Node{ID: "auto_1", Type: flowdsl.NodeAutoTask}
		cfg, ok := n.AsUserTask()
		assert.False(t, ok)
		assert.Nil(t, cfg)
	})

	t.Run("有效审批节点成功转换", func(t *testing.T) {
		n := &flowdsl.Node{
			ID:   "user_1",
			Type: flowdsl.NodeUserTask,
			UserTask: &flowdsl.UserTaskConfig{
				Assignees: []flowdsl.Assignee{
					{Rule: flowdsl.APPOINT, Values: []string{"admin"}},
				},
				Fields: []flowdsl.Field{
					{Key: "remark", Name: "备注", Type: flowdsl.FieldInput},
				},
				IsCosigned: true,
				IsCC:       false,
			},
		}

		cfg, ok := n.AsUserTask()
		assert.True(t, ok)
		assert.NotNil(t, cfg)
		assert.Len(t, cfg.Assignees, 1)
		assert.Len(t, cfg.Fields, 1)
		assert.True(t, cfg.IsCosigned)
		assert.False(t, cfg.IsCC)
	})
}

func TestNode_AsAutoTask(t *testing.T) {
	t.Run("nil 节点安全返回 false", func(t *testing.T) {
		var n *flowdsl.Node
		cfg, ok := n.AsAutoTask()
		assert.False(t, ok)
		assert.Nil(t, cfg)
	})

	t.Run("自动化节点但 AutoTask 为 nil 返回 false", func(t *testing.T) {
		n := &flowdsl.Node{ID: "auto_1", Type: flowdsl.NodeAutoTask, AutoTask: nil}
		cfg, ok := n.AsAutoTask()
		assert.False(t, ok)
		assert.Nil(t, cfg)
	})

	t.Run("有效自动化节点成功转换", func(t *testing.T) {
		n := &flowdsl.Node{
			ID:   "auto_1",
			Type: flowdsl.NodeAutoTask,
			AutoTask: &flowdsl.AutoTaskConfig{
				CodebookID: 888,
				RunnerID:   999,
				IsNotify:   true,
			},
		}

		cfg, ok := n.AsAutoTask()
		assert.True(t, ok)
		assert.NotNil(t, cfg)
		assert.Equal(t, int64(888), cfg.CodebookID)
		assert.Equal(t, int64(999), cfg.RunnerID)
		assert.True(t, cfg.IsNotify)
	})
}

func TestNode_AsChatGroup(t *testing.T) {
	t.Run("有效群聊节点成功转换", func(t *testing.T) {
		n := &flowdsl.Node{
			ID:   "chat_1",
			Type: flowdsl.NodeChatGroup,
			ChatGroup: &flowdsl.ChatGroupConfig{
				Title:        "通知群",
				Mode:         flowdsl.ChatGroupUseExisting,
				ChatGroupIDs: []int64{101},
			},
		}

		cfg, ok := n.AsChatGroup()
		assert.True(t, ok)
		assert.NotNil(t, cfg)
		assert.Equal(t, "通知群", cfg.Title)
		assert.Equal(t, flowdsl.ChatGroupUseExisting, cfg.Mode)
	})
}
