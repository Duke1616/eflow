package easyflow

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Bunny3th/easy-workflow/workflow/model"
	"github.com/Duke1616/eflow/internal/domain"
	repoMocks "github.com/Duke1616/eflow/internal/repository/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gorm.io/gorm"
)

func TestToEasyVariables(t *testing.T) {
	testCases := []struct {
		name       string
		variables  map[string]any
		starter    string
		ticketId   int64
		assertFunc func(t *testing.T, rawJSON string)
	}{
		{
			name: "正常字典映射并且包含starter与ticket_id",
			variables: map[string]any{
				"amount": 100,
				"reason": "办公用品采购",
			},
			starter:  "ZhangSan",
			ticketId: 1001,
			assertFunc: func(t *testing.T, rawJSON string) {
				var vars []model.Variable
				err := json.Unmarshal([]byte(rawJSON), &vars)
				require.NoError(t, err)

				m := make(map[string]string)
				for _, v := range vars {
					m[v.Key] = v.Value
				}
				assert.Equal(t, "100", m["amount"])
				assert.Equal(t, "办公用品采购", m["reason"])
				assert.Equal(t, "ZhangSan", m["starter"])
				assert.Equal(t, "1001", m["ticket_id"])
			},
		},
		{
			name: "变量中已存在starter时不重复追加",
			variables: map[string]any{
				"starter": "custom_user",
			},
			starter:  "ZhangSan",
			ticketId: 0,
			assertFunc: func(t *testing.T, rawJSON string) {
				var vars []model.Variable
				err := json.Unmarshal([]byte(rawJSON), &vars)
				require.NoError(t, err)

				starterCount := 0
				for _, v := range vars {
					if v.Key == "starter" {
						starterCount++
						assert.Equal(t, "custom_user", v.Value)
					}
				}
				assert.Equal(t, 1, starterCount)
			},
		},
		{
			name:      "全空时返回空切片JSON",
			variables: nil,
			starter:   "",
			ticketId:  0,
			assertFunc: func(t *testing.T, rawJSON string) {
				assert.Equal(t, "[]", rawJSON)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			vars := ensureEssentialVariables(tc.variables, tc.starter, tc.ticketId)
			raw := toEasyVariables(vars)
			tc.assertFunc(t, raw)
		})
	}
}

func TestAdapter_Revoke(t *testing.T) {
	testCases := []struct {
		name       string
		req        domain.RevokeReq
		setupMock  func(m *repoMocks.MockIEngineRepository)
		wantErr    bool
		errContain string
	}{
		{
			name: "非法流程实例ID报错",
			req: domain.RevokeReq{
				InstanceID: domain.NewID(0),
			},
			setupMock:  func(m *repoMocks.MockIEngineRepository) {},
			wantErr:    true,
			errContain: "easyflow 流程实例 ID 非法",
		},
		{
			name: "实例不存在时幂等返回成功",
			req: domain.RevokeReq{
				InstanceID: domain.NewID(9999),
				Operator:   "test_user",
			},
			setupMock: func(m *repoMocks.MockIEngineRepository) {
				m.EXPECT().GetInstanceByID(gomock.Any(), 9999).Return(domain.Instance{}, gorm.ErrRecordNotFound)
			},
			wantErr: false,
		},
		{
			name: "实例ID为0的空对象时幂等返回成功",
			req: domain.RevokeReq{
				InstanceID: domain.NewID(8888),
				Operator:   "test_user",
			},
			setupMock: func(m *repoMocks.MockIEngineRepository) {
				m.EXPECT().GetInstanceByID(gomock.Any(), 8888).Return(domain.Instance{ProcInstID: 0, ProcID: 0}, nil)
			},
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := repoMocks.NewMockIEngineRepository(ctrl)
			tc.setupMock(mockRepo)

			engineAdapter := NewEngine(mockRepo)
			err := engineAdapter.Revoke(context.Background(), tc.req)

			if tc.wantErr {
				require.Error(t, err)
				if tc.errContain != "" {
					assert.Contains(t, err.Error(), tc.errContain)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}
