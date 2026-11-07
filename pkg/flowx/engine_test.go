package flowx

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/Duke1616/eflow/pkg/flowx/storage"
	storagemocks "github.com/Duke1616/eflow/pkg/flowx/storage/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

// buildValidDefinition 构造测试用标准合法流程定义
func buildValidDefinition() model.ProcessDefinition {
	return NewProcess("proc_test", "测试流程").
		Start("start", "开始").
		Approval("audit", "主管审批", model.SignOneVote, "leader_tom").
		End("end", "办结").
		Edge("start", "audit").
		Edge("audit", "end").
		Build()
}

// testCase 统一轻量表驱动测试用例
type testCase struct {
	name      string
	before    func() storage.IStorage
	run       func(ctx context.Context, e Engine) error
	after     func(t *testing.T, err error)
	wantErr   error
	errSubStr string
}

// EngineTestSuite 工作流核心引擎基于 Mock 的单元测试套件
type EngineTestSuite struct {
	suite.Suite
	ctrl *gomock.Controller
}

func (s *EngineTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
}

func (s *EngineTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

// runCases 统一执行表驱动测试，消除重复的子测试与错误校验样板代码
func (s *EngineTestSuite) runCases(cases []testCase) {
	t := s.T()
	ctx := context.Background()
	for _, tc := range cases {
		s.Run(tc.name, func() {
			var store storage.IStorage
			if tc.before != nil {
				store = tc.before()
			}
			engine := NewEngine(store)

			err := tc.run(ctx, engine)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			}
			if tc.errSubStr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errSubStr)
			}
			if tc.wantErr == nil && tc.errSubStr == "" {
				require.NoError(t, err)
			}
			if tc.after != nil {
				tc.after(t, err)
			}
		})
	}
}

func TestEngineTestSuite(t *testing.T) {
	suite.Run(t, new(EngineTestSuite))
}

func (s *EngineTestSuite) TestDeploy() {
	s.runCases([]testCase{
		{
			name: "成功部署合法流程图",
			before: func() storage.IStorage {
				m := storagemocks.NewMockIStorage(s.ctrl)
				m.EXPECT().SaveDefinition(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, d model.ProcessDefinition) error {
						assert.Equal(s.T(), 1, d.Version)
						assert.Equal(s.T(), "proc_test", d.Key)
						return nil
					})
				return m
			},
			run: func(ctx context.Context, e Engine) error {
				deployed, err := e.Deploy(ctx, buildValidDefinition())
				if err != nil {
					return err
				}
				assert.Equal(s.T(), 1, deployed.Version)
				return nil
			},
		},
		{
			name: "版本未指定自动自增_基于最大版本+1",
			before: func() storage.IStorage {
				m := storagemocks.NewMockIStorage(s.ctrl)
				// 历史已有最大版本 2
				m.EXPECT().GetLatestVersion(gomock.Any(), "proc_test").Return(2, nil)
				m.EXPECT().SaveDefinition(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, d model.ProcessDefinition) error {
						assert.Equal(s.T(), 3, d.Version)
						assert.Equal(s.T(), "proc_test", d.Key)
						return nil
					})
				return m
			},
			run: func(ctx context.Context, e Engine) error {
				def := buildValidDefinition()
				def.Version = 0 // 未指定版本
				deployed, err := e.Deploy(ctx, def)
				if err != nil {
					return err
				}
				assert.Equal(s.T(), 3, deployed.Version)
				return nil
			},
		},
		{
			name: "静态拓扑校验失败_缺失结束节点",
			before: func() storage.IStorage {
				return storagemocks.NewMockIStorage(s.ctrl)
			},
			run: func(ctx context.Context, e Engine) error {
				badDef := model.ProcessDefinition{
					Key:   "bad_proc",
					Nodes: []model.NodeSpec{{ID: "start", Type: model.NodeStart}},
				}
				_, err := e.Deploy(ctx, badDef)
				return err
			},
			wantErr:   ErrInvalidDefinition,
			errSubStr: "流程必须至少包含 1 个结束节点",
		},
		{
			name: "底层存储写入失败",
			before: func() storage.IStorage {
				m := storagemocks.NewMockIStorage(s.ctrl)
				m.EXPECT().SaveDefinition(gomock.Any(), gomock.Any()).
					Return(errors.New("db disk full"))
				return m
			},
			run: func(ctx context.Context, e Engine) error {
				_, err := e.Deploy(ctx, buildValidDefinition())
				return err
			},
			errSubStr: "db disk full",
		},
	})
}

func (s *EngineTestSuite) TestStart() {
	var startedInstID string

	s.runCases([]testCase{
		{
			name: "按流程Key启动成功",
			before: func() storage.IStorage {
				m := storagemocks.NewMockIStorage(s.ctrl)
				def := buildValidDefinition()
				m.EXPECT().GetDefinition(gomock.Any(), "proc_test", 1).Return(def, nil)
				m.EXPECT().SaveTasks(gomock.Any(), gomock.Len(1)).Return(nil)
				m.EXPECT().SaveInstance(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, inst *model.Instance) error {
						assert.Equal(s.T(), model.InstanceRunning, inst.Status)
						assert.Equal(s.T(), "audit", inst.CurrentNode)
						assert.Equal(s.T(), "zhangsan", inst.Starter)
						return nil
					})
				return m
			},
			run: func(ctx context.Context, e Engine) (err error) {
				startedInstID, err = e.Start(ctx, StartReq{
					Key:       "proc_test",
					Version:   1,
					Starter:   "zhangsan",
					Variables: map[string]any{"dept": "IT"},
				})
				return err
			},
			after: func(t *testing.T, err error) {
				assert.NotEmpty(t, startedInstID)
			},
		},
		{
			name: "流程定义不存在",
			before: func() storage.IStorage {
				m := storagemocks.NewMockIStorage(s.ctrl)
				m.EXPECT().GetDefinition(gomock.Any(), "unknown_proc", 1).
					Return(model.ProcessDefinition{}, storage.ErrDefinitionNotFound)
				return m
			},
			run: func(ctx context.Context, e Engine) error {
				_, err := e.Start(ctx, StartReq{
					Key:     "unknown_proc",
					Starter: "zhangsan",
				})
				return err
			},
			errSubStr: "获取流程定义失败",
		},
		{
			name: "首批目标直接为END节点_流程自动办结",
			before: func() storage.IStorage {
				m := storagemocks.NewMockIStorage(s.ctrl)
				m.EXPECT().SaveDefinition(gomock.Any(), gomock.Any()).Return(nil)
				m.EXPECT().SaveInstance(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, inst *model.Instance) error {
						assert.Equal(s.T(), model.InstanceFinished, inst.Status)
						assert.Equal(s.T(), "end", inst.CurrentNode)
						assert.NotNil(s.T(), inst.FinishTime)
						return nil
					})
				return m
			},
			run: func(ctx context.Context, e Engine) (err error) {
				instantDef := NewProcess("instant_proc", "秒批流").
					Start("start", "开始").
					End("end", "结束").
					Edge("start", "end").
					Build()
				startedInstID, err = e.Start(ctx, StartReq{
					Starter: "zhangsan",
					Def:     instantDef,
				})
				return err
			},
			after: func(t *testing.T, err error) {
				assert.NotEmpty(t, startedInstID)
			},
		},
		{
			name: "待办任务保存失败_事务阻断",
			before: func() storage.IStorage {
				m := storagemocks.NewMockIStorage(s.ctrl)
				m.EXPECT().GetDefinition(gomock.Any(), "proc_test", 1).Return(buildValidDefinition(), nil)
				m.EXPECT().SaveTasks(gomock.Any(), gomock.Any()).Return(errors.New("insert tasks error"))
				return m
			},
			run: func(ctx context.Context, e Engine) error {
				_, err := e.Start(ctx, StartReq{
					Key:     "proc_test",
					Starter: "zhangsan",
				})
				return err
			},
			errSubStr: "insert tasks error",
		},
	})
}

func (s *EngineTestSuite) TestPass() {
	s.runCases([]testCase{
		{
			name: "任务不存在",
			before: func() storage.IStorage {
				m := storagemocks.NewMockIStorage(s.ctrl)
				m.EXPECT().GetTaskForUpdate(gomock.Any(), "task_not_found").
					Return(nil, storage.ErrTaskNotFound)
				return m
			},
			run: func(ctx context.Context, e Engine) error {
				return e.Pass(ctx, PassReq{TaskID: "task_not_found", Operator: "leader_tom"})
			},
			wantErr: ErrTaskNotFound,
		},
		{
			name: "任务已非Pending状态_重复操作拦截",
			before: func() storage.IStorage {
				m := storagemocks.NewMockIStorage(s.ctrl)
				m.EXPECT().GetTaskForUpdate(gomock.Any(), "task_passed").
					Return(&model.Task{ID: "task_passed", Status: model.TaskPassed}, nil)
				return m
			},
			run: func(ctx context.Context, e Engine) error {
				return e.Pass(ctx, PassReq{TaskID: "task_passed", Operator: "leader_tom"})
			},
			wantErr: ErrTaskNotPending,
		},
		{
			name: "查询关联实例失败_错误透传",
			before: func() storage.IStorage {
				m := storagemocks.NewMockIStorage(s.ctrl)
				m.EXPECT().GetTaskForUpdate(gomock.Any(), "task_1").
					Return(&model.Task{ID: "task_1", InstanceID: "inst_1", Assignee: "leader_tom", Status: model.TaskPending}, nil)
				m.EXPECT().GetInstanceForUpdate(gomock.Any(), "inst_1").
					Return(nil, storage.ErrInstanceNotFound)
				return m
			},
			run: func(ctx context.Context, e Engine) error {
				return e.Pass(ctx, PassReq{TaskID: "task_1", Operator: "proxy_user"})
			},
			wantErr: ErrInstanceNotFound,
		},
		{
			name: "流程已非Running状态_流转拦截",
			before: func() storage.IStorage {
				m := storagemocks.NewMockIStorage(s.ctrl)
				m.EXPECT().GetTaskForUpdate(gomock.Any(), "task_1").
					Return(&model.Task{ID: "task_1", InstanceID: "inst_1", Assignee: "leader_tom", Status: model.TaskPending}, nil)
				m.EXPECT().GetInstanceForUpdate(gomock.Any(), "inst_1").
					Return(&model.Instance{ID: "inst_1", Status: model.InstanceFinished}, nil)
				return m
			},
			run: func(ctx context.Context, e Engine) error {
				return e.Pass(ctx, PassReq{TaskID: "task_1", Operator: "leader_tom"})
			},
			wantErr: ErrInstanceNotRunning,
		},
		{
			name: "或签通过_推进至End并联动跳过兄弟任务",
			before: func() storage.IStorage {
				m := storagemocks.NewMockIStorage(s.ctrl)
				now := time.Now()
				task1 := &model.Task{
					ID:         "task_1",
					InstanceID: "inst_1",
					NodeID:     "audit",
					BatchCode:  1,
					Assignee:   "leader_tom",
					SignRule:   model.SignOneVote,
					Status:     model.TaskPending,
				}
				task2 := &model.Task{
					ID:         "task_2",
					InstanceID: "inst_1",
					NodeID:     "audit",
					BatchCode:  1,
					Assignee:   "leader_bob",
					SignRule:   model.SignOneVote,
					Status:     model.TaskPending,
				}
				inst := &model.Instance{
					ID:         "inst_1",
					ProcKey:    "proc_test",
					Version:    1,
					Status:     model.InstanceRunning,
					CreateTime: now,
				}

				m.EXPECT().GetTaskForUpdate(gomock.Any(), "task_1").Return(task1, nil)
				m.EXPECT().GetInstanceForUpdate(gomock.Any(), "inst_1").Return(inst, nil)
				m.EXPECT().UpdateTask(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, task *model.Task) error {
						assert.Equal(s.T(), model.TaskPassed, task.Status)
						assert.Equal(s.T(), "审批通过", task.Comment)
						return nil
					})
				m.EXPECT().GetDefinition(gomock.Any(), "proc_test", 1).Return(buildValidDefinition(), nil)
				m.EXPECT().ListTasksByInstance(gomock.Any(), "inst_1").Return([]*model.Task{task1, task2}, nil)
				m.EXPECT().UpdateTasksStatus(gomock.Any(), []string{"task_2"}, model.TaskSkipped).Return(nil)
				m.EXPECT().SaveInstance(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, i *model.Instance) error {
						assert.Equal(s.T(), model.InstanceFinished, i.Status)
						assert.Equal(s.T(), "end", i.CurrentNode)
						assert.Equal(s.T(), true, i.Variables["approved"])
						return nil
					})
				return m
			},
			run: func(ctx context.Context, e Engine) error {
				return e.Pass(ctx, PassReq{
					TaskID:    "task_1",
					Operator:  "leader_tom",
					Comment:   "审批通过",
					Variables: map[string]any{"approved": true},
				})
			},
		},
	})
}

func (s *EngineTestSuite) TestReject() {
	s.runCases([]testCase{
		{
			name: "首个节点驳回_无上游审批节点直接置为Rejected终态",
			before: func() storage.IStorage {
				m := storagemocks.NewMockIStorage(s.ctrl)
				task := &model.Task{
					ID:         "task_1",
					InstanceID: "inst_1",
					NodeID:     "audit",
					BatchCode:  1,
					Assignee:   "leader_tom",
					Status:     model.TaskPending,
				}
				inst := &model.Instance{
					ID:      "inst_1",
					ProcKey: "proc_test",
					Version: 1,
					Status:  model.InstanceRunning,
				}

				m.EXPECT().GetTaskForUpdate(gomock.Any(), "task_1").Return(task, nil)
				m.EXPECT().GetInstanceForUpdate(gomock.Any(), "inst_1").Return(inst, nil)
				m.EXPECT().UpdateTask(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, task *model.Task) error {
						assert.Equal(s.T(), model.TaskRejected, task.Status)
						assert.Equal(s.T(), "材料不合规，驳回", task.Comment)
						return nil
					})
				m.EXPECT().ListTasksByInstance(gomock.Any(), "inst_1").Return([]*model.Task{task}, nil)
				m.EXPECT().GetDefinition(gomock.Any(), "proc_test", 1).Return(buildValidDefinition(), nil)
				m.EXPECT().SaveInstance(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, i *model.Instance) error {
						assert.Equal(s.T(), model.InstanceRejected, i.Status)
						assert.NotNil(s.T(), i.FinishTime)
						return nil
					})
				return m
			},
			run: func(ctx context.Context, e Engine) error {
				return e.Reject(ctx, RejectReq{
					TaskID:   "task_1",
					Operator: "leader_tom",
					Comment:  "材料不合规，驳回",
				})
			},
		},
		{
			name: "中间节点驳回_取消同批兄弟待办并回退激活上游节点_批次号自增",
			before: func() storage.IStorage {
				m := storagemocks.NewMockIStorage(s.ctrl)
				twoStepDef := NewProcess("two_step", "两级审批").
					Start("start", "开始").
					Approval("leader", "主管审批", model.SignOneVote, "leader_tom").
					Approval("director", "总监审批", model.SignAllPass, "director_carol", "director_dave").
					End("end", "结束").
					Edge("start", "leader").
					Edge("leader", "director").
					Edge("director", "end").
					Build()

				currTask := &model.Task{
					ID:         "task_dir_1",
					InstanceID: "inst_2",
					NodeID:     "director",
					BatchCode:  1,
					Assignee:   "director_carol",
					Status:     model.TaskPending,
				}
				siblingTask := &model.Task{
					ID:         "task_dir_2",
					InstanceID: "inst_2",
					NodeID:     "director",
					BatchCode:  1,
					Assignee:   "director_dave",
					Status:     model.TaskPending,
				}
				inst := &model.Instance{
					ID:      "inst_2",
					ProcKey: "two_step",
					Version: 1,
					Status:  model.InstanceRunning,
				}

				m.EXPECT().GetTaskForUpdate(gomock.Any(), "task_dir_1").Return(currTask, nil)
				m.EXPECT().GetInstanceForUpdate(gomock.Any(), "inst_2").Return(inst, nil)
				m.EXPECT().UpdateTask(gomock.Any(), gomock.Any()).Return(nil)
				m.EXPECT().ListTasksByInstance(gomock.Any(), "inst_2").Return([]*model.Task{currTask, siblingTask}, nil)
				m.EXPECT().UpdateTasksStatus(gomock.Any(), []string{"task_dir_2"}, model.TaskCancelled).Return(nil)
				m.EXPECT().GetDefinition(gomock.Any(), "two_step", 1).Return(twoStepDef, nil)
				m.EXPECT().SaveTasks(gomock.Any(), gomock.Len(1)).
					DoAndReturn(func(_ context.Context, tasks []*model.Task) error {
						assert.Equal(s.T(), 2, tasks[0].BatchCode)
						assert.Equal(s.T(), "leader", tasks[0].NodeID)
						assert.Equal(s.T(), "leader_tom", tasks[0].Assignee)
						return nil
					})
				m.EXPECT().SaveInstance(gomock.Any(), gomock.Any()).Return(nil)
				return m
			},
			run: func(ctx context.Context, e Engine) error {
				return e.Reject(ctx, RejectReq{
					TaskID:   "task_dir_1",
					Operator: "director_carol",
					Comment:  "重新审议",
				})
			},
		},
	})
}

func (s *EngineTestSuite) TestDoTx() {
	s.runCases([]testCase{
		{
			name: "底层驱动未实现ITxStorage_安全降级执行闭包",
			before: func() storage.IStorage {
				return storagemocks.NewMockIStorage(s.ctrl)
			},
			run: func(ctx context.Context, e Engine) error {
				return e.DoTx(ctx, func(ctx context.Context, txEngine Engine) error {
					return nil
				})
			},
		},
		{
			name: "底层事务闭包正常提交并完整继承节点执行器",
			before: func() storage.IStorage {
				m := storagemocks.NewMockITxStorage(s.ctrl)
				m.EXPECT().DoTx(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, fn func(storage.IStorage) error) error {
						return fn(m)
					})
				return m
			},
			run: func(ctx context.Context, e Engine) error {
				return e.DoTx(ctx, func(ctx context.Context, txEngine Engine) error {
					assert.NotNil(s.T(), txEngine)
					// 验证事务子引擎内部的调度器与 executors 正确初始化
					fe, ok := txEngine.(*flowEngine)
					assert.True(s.T(), ok)
					assert.NotNil(s.T(), fe.executors)
					assert.NotNil(s.T(), fe.executors[model.NodeApproval])
					return nil
				})
			},
		},
		{
			name: "事务内业务报错_向外冒泡触发回滚",
			before: func() storage.IStorage {
				m := storagemocks.NewMockITxStorage(s.ctrl)
				m.EXPECT().DoTx(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, fn func(storage.IStorage) error) error {
						return fn(m)
					})
				return m
			},
			run: func(ctx context.Context, e Engine) error {
				return e.DoTx(ctx, func(ctx context.Context, txEngine Engine) error {
					return errors.New("business validation failed in tx")
				})
			},
			errSubStr: "business validation failed in tx",
		},
	})
}
