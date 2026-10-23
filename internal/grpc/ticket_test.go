package grpc

import (
	"context"
	"errors"
	"testing"

	ticketv1 "github.com/Duke1616/eflow/api/proto/gen/eflow/ticket/v1"
	"github.com/Duke1616/eflow/internal/domain"
	templateSvc "github.com/Duke1616/eflow/internal/service/template"
	ticketSvc "github.com/Duke1616/eflow/internal/service/ticket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
)

type mockTicketService struct {
	ticketSvc.Service
	createBizTicketFn func(ctx context.Context, ticket domain.Ticket) (domain.Ticket, error)
}

func (m *mockTicketService) CreateBizTicket(ctx context.Context, ticket domain.Ticket) (domain.Ticket, error) {
	if m.createBizTicketFn != nil {
		return m.createBizTicketFn(ctx, ticket)
	}
	return domain.Ticket{}, errors.New("not implemented")
}

type mockTemplateService struct {
	templateSvc.Service
	detailTemplateFn func(ctx context.Context, id int64) (domain.Template, error)
}

func (m *mockTemplateService) DetailTemplate(ctx context.Context, id int64) (domain.Template, error) {
	if m.detailTemplateFn != nil {
		return m.detailTemplateFn(ctx, id)
	}
	return domain.Template{}, errors.New("not implemented")
}

func TestTicketServer_CreateTicket(t *testing.T) {
	testCases := []struct {
		name        string
		req         *ticketv1.CreateTicketRequest
		mockTicket  func() *mockTicketService
		mockTpl     func() *mockTemplateService
		wantID      int64
		wantErrText string
	}{
		{
			name:        "nil 请求",
			req:         nil,
			mockTicket:  func() *mockTicketService { return &mockTicketService{} },
			mockTpl:     func() *mockTemplateService { return &mockTemplateService{} },
			wantErrText: "请求参数不能为空",
		},
		{
			name: "缺少 template_id",
			req: &ticketv1.CreateTicketRequest{
				CreateBy: "admin",
			},
			mockTicket:  func() *mockTicketService { return &mockTicketService{} },
			mockTpl:     func() *mockTemplateService { return &mockTemplateService{} },
			wantErrText: "template_id 必须大于 0",
		},
		{
			name: "缺少 create_by",
			req: &ticketv1.CreateTicketRequest{
				TemplateId: 10,
			},
			mockTicket:  func() *mockTicketService { return &mockTicketService{} },
			mockTpl:     func() *mockTemplateService { return &mockTemplateService{} },
			wantErrText: "create_by 工单发起人不能为空",
		},
		{
			name: "通过 template_id 成功创建工单",
			req: &ticketv1.CreateTicketRequest{
				TemplateId: 10,
				CreateBy:   "luankz",
				BizId:      100,
				Key:        "fetcher:batch:100",
				Data: func() *structpb.Struct {
					s, _ := structpb.NewStruct(map[string]any{"task_name": "拉取测试"})
					return s
				}(),
			},
			mockTicket: func() *mockTicketService {
				return &mockTicketService{
					createBizTicketFn: func(ctx context.Context, ticket domain.Ticket) (domain.Ticket, error) {
						assert.Equal(t, int64(10), ticket.TemplateId)
						assert.Equal(t, int64(30), ticket.WorkflowId)
						assert.Equal(t, "luankz", ticket.CreateBy)
						assert.Equal(t, int64(100), ticket.BizID)
						assert.Equal(t, "fetcher:batch:100", ticket.Key)
						assert.Equal(t, "拉取测试", ticket.Data["task_name"])
						return domain.Ticket{Id: 999}, nil
					},
				}
			},
			mockTpl: func() *mockTemplateService {
				return &mockTemplateService{
					detailTemplateFn: func(ctx context.Context, id int64) (domain.Template, error) {
						assert.Equal(t, int64(10), id)
						return domain.Template{Id: 10, WorkflowId: 30}, nil
					},
				}
			},
			wantID: 999,
		},
		{
			name: "模板未绑定工作流流程",
			req: &ticketv1.CreateTicketRequest{
				TemplateId: 10,
				CreateBy:   "luankz",
			},
			mockTicket: func() *mockTicketService {
				return &mockTicketService{}
			},
			mockTpl: func() *mockTemplateService {
				return &mockTemplateService{
					detailTemplateFn: func(ctx context.Context, id int64) (domain.Template, error) {
						return domain.Template{Id: 10, WorkflowId: 0}, nil
					},
				}
			},
			wantErrText: "未绑定有效的工作流流程",
		},
		{
			name: "模板不存在",
			req: &ticketv1.CreateTicketRequest{
				TemplateId: 10,
				CreateBy:   "luankz",
			},
			mockTicket: func() *mockTicketService {
				return &mockTicketService{}
			},
			mockTpl: func() *mockTemplateService {
				return &mockTemplateService{
					detailTemplateFn: func(ctx context.Context, id int64) (domain.Template, error) {
						return domain.Template{}, errors.New("template not found")
					},
				}
			},
			wantErrText: "读取工单模板失败",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := NewTicketServer(tc.mockTicket(), tc.mockTpl())
			resp, err := server.CreateTicket(context.Background(), tc.req)
			if tc.wantErrText != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrText)
				assert.Nil(t, resp)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.Equal(t, tc.wantID, resp.Id)
		})
	}
}
