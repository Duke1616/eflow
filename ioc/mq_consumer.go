package ioc

import (
	"github.com/Duke1616/eflow/internal/event"
	"github.com/Duke1616/eflow/internal/event/process"
	taskEvent "github.com/Duke1616/eflow/internal/event/task"
	templateEvent "github.com/Duke1616/eflow/internal/event/template"
	wechatEvent "github.com/Duke1616/eflow/internal/event/wechat"
	"github.com/Duke1616/eflow/internal/client/eiam"
	"github.com/Duke1616/eflow/internal/service/engine"
	taskSvc "github.com/Duke1616/eflow/internal/service/task"
	templateSvc "github.com/Duke1616/eflow/internal/service/template"
	ticketSvc "github.com/Duke1616/eflow/internal/service/ticket"
	workflowSvc "github.com/Duke1616/eflow/internal/service/workflow"
	"github.com/Duke1616/eflow/pkg/mqx"
	"github.com/ecodeclub/mq-api"
	"github.com/xen0n/go-workwx"
)

func InitProcessEventConsumer(
	q mq.MQ,
	engine engine.IEngine,
	workFlowSvc workflowSvc.Service,
	ticketSvc ticketSvc.Service,
) (*process.ProcessEventConsumer, error) {
	consumer := mqx.NewResilientConsumer(q, event.CreateProcessEventName, "process_instance_starter")
	return process.NewProcessEventConsumer(engine, workFlowSvc, ticketSvc, consumer), nil
}

func InitExecuteResultConsumer(q mq.MQ, svc taskSvc.Service) (*taskEvent.ExecuteResultConsumer, error) {
	consumer := mqx.NewResilientConsumer(q, event.ExecuteResultEventName, "task_execution_result_consumer")
	return taskEvent.NewExecuteResultConsumer(consumer, svc), nil
}

func InitWechatTicketConsumer(
	svc ticketSvc.Service,
	templateSvc templateSvc.Service,
	userSvc eiam.IUserClient,
	q mq.MQ,
) (*wechatEvent.WechatTicketConsumer, error) {
	consumer := mqx.NewResilientConsumer(q, event.WechatTicketEventName, "wechat_ticket_creator")
	return wechatEvent.NewWechatTicketConsumer(svc, templateSvc, userSvc, consumer), nil
}

func InitWechatApprovalCallbackConsumer(
	svc templateSvc.Service,
	q mq.MQ,
	p templateEvent.WechatTicketEventProducer,
	workApp *workwx.WorkwxApp,
) (*templateEvent.WechatApprovalCallbackConsumer, error) {
	consumer := mqx.NewResilientConsumer(q, templateEvent.WechatCallbackEventName, "wechat_oa_callback_handler")
	return templateEvent.NewWechatApprovalCallbackConsumer(svc, consumer, p, workApp), nil
}
