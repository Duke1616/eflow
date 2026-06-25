package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	notificationv1 "github.com/Duke1616/eflow/api/proto/gen/ealert/notification/v1"
	templatev1 "github.com/Duke1616/eflow/api/proto/gen/ealert/template/v1"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/notification"
	"github.com/Duke1616/eflow/internal/pkg/notification/provider"
	"github.com/Duke1616/eflow/internal/service/event/errs"
	"github.com/Duke1616/enotify/notify/feishu/card"
	"github.com/google/uuid"
	"github.com/gotomicro/ego/core/elog"
	"google.golang.org/protobuf/types/known/structpb"
)

type grpcProvider struct {
	notification notificationv1.NotificationServiceClient
	template     templatev1.TemplateServiceClient
	logger       *elog.Component
}

func NewGRPCProvider(notification notificationv1.NotificationServiceClient, template templatev1.TemplateServiceClient) provider.Provider {
	return &grpcProvider{
		notification: notification,
		template:     template,
		logger:       elog.DefaultLogger.With(elog.FieldComponentName("grpc_provider")),
	}
}

func (f *grpcProvider) Send(ctx context.Context, src notification.Notification) (notification.Response, error) {
	// 如果是修改情况，直接退出
	if src.IsPatch() {
		return notification.Response{}, fmt.Errorf("grpc notification 不支持修改模式")
	}

	// 1. 创建 Builder 并应用通用属性
	builder := card.NewApprovalCardBuilder().
		SetToTitle(src.Template.Title).
		SetToFields(toCardFields(src.Template.Fields)).
		SetToHideForm(src.Template.HideForm).
		SetWantResult(src.Template.Remark).
		SetToCallbackValue(toCardValues(src.Template.Values)).
		SetInputFields(toCardInputFields(src.Template.InputFields))

	// 2. 根据类型应用特殊属性
	if src.IsProgressImageResult() {
		builder.SetImageKey(src.Template.ImageKey)
	}

	var rawMap map[string]interface{}
	bytes, err := json.Marshal(builder.Build())
	if err != nil {
		return notification.NewErrorResponse(string(errs.ErrorCodeBuildFailed), err.Error()), fmt.Errorf("%w: %v", errs.ErrBuildMessage, err)
	}
	if err = json.Unmarshal(bytes, &rawMap); err != nil {
		return notification.NewErrorResponse(string(errs.ErrorCodeParseFailed), err.Error()), fmt.Errorf("%w: %v", errs.ErrParseMessage, err)
	}

	params, err := structpb.NewStruct(rawMap)
	if err != nil {
		return notification.NewErrorResponse(string(errs.ErrorCodeParseFailed), err.Error()), fmt.Errorf("%w: %v", errs.ErrParseMessage, err)
	}

	// 这里的超时时间设置非常短 (3s)，目的是快速探测 gRPC 服务状态。
	// 如果 gRPC 服务未启动或网络不通，应该尽快超时报错，从而触发上层 channel 的 Fallback 机制 (降级到 Feishu Card 直连)。
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// 通过模板集 key 和 channel 动态解析渠道模板 ID
	templateID, err := f.resolveTemplateID(ctx, src.Template.Name, notificationv1.Channel_LARK_CARD)
	if err != nil {
		return notification.Response{}, err
	}
	msg, err := f.notification.SendNotification(ctx, &notificationv1.SendNotificationRequest{Notification: &notificationv1.Notification{
		BizId:          notificationv1.Business_TICKET,
		Key:            uuid.New().String(),
		Receivers:      []string{src.Receiver},
		ReceiverType:   toReceiverType(src.ReceiverType),
		Channel:        notificationv1.Channel_LARK_CARD,
		TemplateId:     templateID,
		TemplateParams: params,
	}})

	if err != nil {
		return notification.NewErrorResponse(string(errs.ErrorCodeServiceUnavailable), err.Error()), fmt.Errorf("%w: %v", errs.ErrNotificationUnavailable, err)
	}

	if msg.Status != notificationv1.SendStatus_SUCCEEDED {
		return notification.NewErrorResponseWithID(
			int64(msg.NotificationId),
			msg.Status.String(),
			string(errs.ErrorCodeUnknown),
			"消息发送未成功",
		), fmt.Errorf("%w: status=%v, code=%s, msg=%s", errs.ErrNotificationFailed, msg.Status, msg.ErrorCode, msg.ErrorMessage)
	}

	return notification.NewSuccessResponse(int64(msg.NotificationId), msg.Status.String()), nil
}

func (f *grpcProvider) resolveTemplateID(ctx context.Context, templateName domain.NotifyType,
	channel notificationv1.Channel) (int64, error) {
	resp, err := f.template.ResolveTemplateID(ctx, &templatev1.ResolveTemplateIDRequest{
		BizId:   int64(notificationv1.Business_TICKET),
		Key:     domain.NotifyTemplateSetKey(templateName),
		Channel: channel,
	})
	if err != nil {
		return 0, fmt.Errorf("解析通知模板ID失败: %w", err)
	}
	if resp == nil || resp.TemplateId == 0 {
		return 0, fmt.Errorf("解析通知模板ID失败: notify_type=%s, channel=%s", templateName, channel.String())
	}
	return resp.TemplateId, nil
}
