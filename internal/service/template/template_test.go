package template

import (
	"context"
	"errors"
	"testing"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository"
	repomocks "github.com/Duke1616/eflow/internal/repository/mocks"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestService_DetailTemplate(t *testing.T) {
	testCases := []struct {
		name       string
		mock       func(ctrl *gomock.Controller) repository.ITemplateRepository
		templateID int64
		wantResult domain.Template
		wantErr    error
	}{
		{
			name:       "查询模板成功",
			templateID: 1001,
			mock: func(ctrl *gomock.Controller) repository.ITemplateRepository {
				repo := repomocks.NewMockITemplateRepository(ctrl)
				repo.EXPECT().DetailTemplate(gomock.Any(), int64(1001)).
					Return(domain.Template{
						Id:   1001,
						Name: "测试申请工单",
					}, nil)
				return repo
			},
			wantResult: domain.Template{
				Id:   1001,
				Name: "测试申请工单",
			},
			wantErr: nil,
		},
		{
			name:       "模板不存在返回错误",
			templateID: 9999,
			mock: func(ctrl *gomock.Controller) repository.ITemplateRepository {
				repo := repomocks.NewMockITemplateRepository(ctrl)
				repo.EXPECT().DetailTemplate(gomock.Any(), int64(9999)).
					Return(domain.Template{}, repository.ErrTemplateNotFound)
				return repo
			},
			wantResult: domain.Template{},
			wantErr:    repository.ErrTemplateNotFound,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			repo := tc.mock(ctrl)
			svc := NewTemplateService(repo, nil)

			res, err := svc.DetailTemplate(context.Background(), tc.templateID)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.wantResult, res)
			}
		})
	}
}

func TestService_DeleteGroup(t *testing.T) {
	testCases := []struct {
		name        string
		groupID     int64
		mock        func(ctrl *gomock.Controller) repository.ITemplateRepository
		wantRows    int64
		wantErr     error
	}{
		{
			name:    "删除空分组成功",
			groupID: 10,
			mock: func(ctrl *gomock.Controller) repository.ITemplateRepository {
				repo := repomocks.NewMockITemplateRepository(ctrl)
				repo.EXPECT().DeleteGroup(gomock.Any(), int64(10)).
					Return(int64(1), nil)
				return repo
			},
			wantRows: 1,
			wantErr:  nil,
		},
		{
			name:    "分组下存在模板拒绝删除",
			groupID: 11,
			mock: func(ctrl *gomock.Controller) repository.ITemplateRepository {
				repo := repomocks.NewMockITemplateRepository(ctrl)
				repo.EXPECT().DeleteGroup(gomock.Any(), int64(11)).
					Return(int64(0), ErrTemplateGroupNotEmpty)
				return repo
			},
			wantRows: 0,
			wantErr:  ErrTemplateGroupNotEmpty,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			repo := tc.mock(ctrl)
			svc := NewTemplateService(repo, nil)

			rows, err := svc.DeleteGroup(context.Background(), tc.groupID)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.wantRows, rows)
			}
		})
	}
}

func TestService_ToggleFavorite(t *testing.T) {
	testCases := []struct {
		name       string
		userID     int64
		templateID int64
		mock       func(ctrl *gomock.Controller) repository.ITemplateRepository
		wantStatus bool
		wantErr    error
	}{
		{
			name:       "收藏模板成功",
			userID:     101,
			templateID: 201,
			mock: func(ctrl *gomock.Controller) repository.ITemplateRepository {
				repo := repomocks.NewMockITemplateRepository(ctrl)
				repo.EXPECT().ToggleFavorite(gomock.Any(), int64(101), int64(201)).
					Return(true, nil)
				return repo
			},
			wantStatus: true,
			wantErr:    nil,
		},
		{
			name:       "取消收藏模板成功",
			userID:     101,
			templateID: 201,
			mock: func(ctrl *gomock.Controller) repository.ITemplateRepository {
				repo := repomocks.NewMockITemplateRepository(ctrl)
				repo.EXPECT().ToggleFavorite(gomock.Any(), int64(101), int64(201)).
					Return(false, nil)
				return repo
			},
			wantStatus: false,
			wantErr:    nil,
		},
		{
			name:       "底层仓储错误",
			userID:     101,
			templateID: 201,
			mock: func(ctrl *gomock.Controller) repository.ITemplateRepository {
				repo := repomocks.NewMockITemplateRepository(ctrl)
				repo.EXPECT().ToggleFavorite(gomock.Any(), int64(101), int64(201)).
					Return(false, errors.New("db error"))
				return repo
			},
			wantStatus: false,
			wantErr:    errors.New("db error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			repo := tc.mock(ctrl)
			svc := NewTemplateService(repo, nil)

			status, err := svc.ToggleFavorite(context.Background(), tc.userID, tc.templateID)
			if tc.wantErr != nil {
				assert.EqualError(t, err, tc.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.wantStatus, status)
			}
		})
	}
}
