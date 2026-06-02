package ioc

import (
	"github.com/Duke1616/eflow/internal/event"
	"github.com/Duke1616/eflow/internal/event/process"
	taskEvent "github.com/Duke1616/eflow/internal/event/task"
	templateEvent "github.com/Duke1616/eflow/internal/event/template"
	ticketEvent "github.com/Duke1616/eflow/internal/event/ticket"
	taskSvc "github.com/Duke1616/eflow/internal/service/task"
	templateSvc "github.com/Duke1616/eflow/internal/service/template"
	ticketSvc "github.com/Duke1616/eflow/internal/service/ticket"
	workflowSvc "github.com/Duke1616/eflow/internal/service/workflow"
	"github.com/ecodeclub/mq-api"
	"github.com/xen0n/go-workwx"
)

func InitProcessEventConsumer(
	q mq.MQ,
	workFlowSvc workflowSvc.Service,
	ticketSvc ticketSvc.Service,
) (*process.ProcessEventConsumer, error) {
	consumer, err := q.Consumer(event.CreateProcessEventName, "create_process_instance")
	if err != nil {
		return nil, err
	}
	return process.NewProcessEventConsumer(workFlowSvc, ticketSvc, consumer), nil
}

func InitExecuteResultConsumer(q mq.MQ, svc taskSvc.Service) (*taskEvent.ExecuteResultConsumer, error) {
	consumer, err := q.Consumer(event.ExecuteResultEventName, "task_receive_execute")
	if err != nil {
		return nil, err
	}
	return taskEvent.NewExecuteResultConsumer(consumer, svc), nil
}

func InitWechatTicketConsumer(
	svc ticketSvc.Service,
	templateSvc templateSvc.Service,
	userSvc ticketEvent.UserService,
	q mq.MQ,
) (*ticketEvent.WechatTicketConsumer, error) {
	consumer, err := q.Consumer(ticketEvent.WechatTicketEventName, "wechat_create_ticket")
	if err != nil {
		return nil, err
	}
	return ticketEvent.NewWechatTicketConsumer(svc, templateSvc, userSvc, consumer), nil
}

func InitWechatApprovalCallbackConsumer(
	svc templateSvc.Service,
	q mq.MQ,
	p templateEvent.WechatTicketEventProducer,
	workApp *workwx.WorkwxApp,
) (*templateEvent.WechatApprovalCallbackConsumer, error) {
	consumer, err := q.Consumer(templateEvent.WechatCallbackEventName, "wechat_oa_callback")
	if err != nil {
		return nil, err
	}
	return templateEvent.NewWechatApprovalCallbackConsumer(svc, consumer, p, workApp), nil
}
