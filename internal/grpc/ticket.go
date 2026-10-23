package grpc

import (
	"context"
	"fmt"

	ticketv1 "github.com/Duke1616/eflow/api/proto/gen/eflow/ticket/v1"
	"github.com/Duke1616/eflow/internal/domain"
	templateSvc "github.com/Duke1616/eflow/internal/service/template"
	ticketSvc "github.com/Duke1616/eflow/internal/service/ticket"
	"github.com/gotomicro/ego/core/elog"
)

// TicketServer 实现工单 gRPC 服务端 API。
type TicketServer struct {
	ticketv1.UnimplementedTicketServiceServer

	svc         ticketSvc.Service
	templateSvc templateSvc.Service
	logger      *elog.Component
}

// NewTicketServer 构造工单 gRPC 服务端实例，直接返回 ticketv1.TicketServiceServer 接口。
func NewTicketServer(svc ticketSvc.Service, templateSvc templateSvc.Service) ticketv1.TicketServiceServer {
	return &TicketServer{
		svc:         svc,
		templateSvc: templateSvc,
		logger:      elog.DefaultLogger,
	}
}

// CreateTicket 通过 gRPC 请求直接创建工单并驱动底层流程引擎启动。
func (s *TicketServer) CreateTicket(ctx context.Context, req *ticketv1.CreateTicketRequest) (*ticketv1.CreateTicketResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("请求参数不能为空")
	}
	if req.GetTemplateId() <= 0 {
		return nil, fmt.Errorf("template_id 必须大于 0")
	}
	if req.GetCreateBy() == "" {
		return nil, fmt.Errorf("create_by 工单发起人不能为空")
	}

	// NOTE: 统一以模板实际绑定的工作流为准，避免外部系统感知底层流程细节或传递过期的 workflow_id
	tpl, err := s.templateSvc.DetailTemplate(ctx, req.GetTemplateId())
	if err != nil {
		return nil, fmt.Errorf("读取工单模板失败: %w", err)
	}
	if tpl.WorkflowId <= 0 {
		return nil, fmt.Errorf("工单模板 (ID=%d) 未绑定有效的工作流流程", req.GetTemplateId())
	}

	var formData map[string]interface{}
	if req.GetData() != nil {
		formData = req.GetData().AsMap()
	}

	provide := domain.SYSTEM
	switch req.GetProvide() {
	case ticketv1.Provide_WECHAT:
		provide = domain.WECHAT
	case ticketv1.Provide_ALERT:
		provide = domain.ALERT
	}

	ticket := domain.Ticket{
		BizID:      req.GetBizId(),
		Key:        req.GetKey(),
		TemplateId: req.GetTemplateId(),
		WorkflowId: tpl.WorkflowId,
		Data:       formData,
		Status:     domain.START,
		Provide:    provide,
		CreateBy:   req.GetCreateBy(),
	}

	created, err := s.svc.CreateBizTicket(ctx, ticket)
	if err != nil {
		return nil, fmt.Errorf("创建工单失败: %w", err)
	}

	s.logger.Info("gRPC 创建工单成功",
		elog.Int64("ticket_id", created.Id),
		elog.String("create_by", created.CreateBy),
		elog.Int64("template_id", created.TemplateId),
		elog.Int64("biz_id", created.BizID),
		elog.String("key", created.Key),
	)

	return &ticketv1.CreateTicketResponse{
		Id: created.Id,
	}, nil
}
