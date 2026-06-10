package workflow

import (
	_ "embed"

	notificationv1 "github.com/Duke1616/eflow/api/proto/gen/ealert/notification/v1"
	"github.com/Duke1616/eflow/internal/domain"
)

//go:embed fs/approval.tmpl
var LarkApprovalTemplate string

//go:embed fs/carbon_copy.tmpl
var LarkApprovalCCTemplate string

//go:embed fs/chat.tmpl
var LarkChatTemplate string

//go:embed fs/progress.tmpl
var LarkApprovalProgressTemplate string

//go:embed fs/progress_image_result.tmpl
var LarkApprovalProgressImageResultTemplate string

//go:embed fs/revoke.tmpl
var LarkApprovalRevokeTemplate string

// templateConfig 定义了同步模板所需要的本地配置元数据属性
type templateConfig struct {
	Name        string
	Desc        string
	Channel     notificationv1.Channel
	VersionName string
	Content     string
	NotifyType  domain.NotifyType
}

// templates 静态声明需要被自愈引导注册的默认通知模板数据集
var templates = []templateConfig{
	{
		Name:        "工单审批通知",
		Desc:        "工单审批流程通知",
		Channel:     notificationv1.Channel_LARK_CARD,
		VersionName: "v1.0.0",
		Content:     LarkApprovalTemplate,
		NotifyType:  domain.NotifyTypeApproval,
	},
	{
		Name:        "工单抄送通知",
		Desc:        "工单抄送通知",
		Channel:     notificationv1.Channel_LARK_CARD,
		VersionName: "v1.0.0",
		Content:     LarkApprovalCCTemplate,
		NotifyType:  domain.NotifyTypeCC,
	},
	{
		Name:        "工单群通知",
		Desc:        "群组节点发送通知，支持分区小标题与 hr 分隔线",
		Channel:     notificationv1.Channel_LARK_CARD,
		VersionName: "v1.0.0",
		Content:     LarkChatTemplate,
		NotifyType:  domain.NotifyTypeChat,
	},
	{
		Name:        "工单进度通知",
		Desc:        "工单进度通知",
		Channel:     notificationv1.Channel_LARK_CARD,
		VersionName: "v1.0.0",
		Content:     LarkApprovalProgressTemplate,
		NotifyType:  domain.NotifyTypeProgress,
	},
	{
		Name:        "工单进度图片通知",
		Desc:        "工单进度图片结果通知",
		Channel:     notificationv1.Channel_LARK_CARD,
		VersionName: "v1.0.0",
		Content:     LarkApprovalProgressImageResultTemplate,
		NotifyType:  domain.NotifyTypeProgressImageResult,
	},
	{
		Name:        "工单撤回通知",
		Desc:        "工单撤回通知",
		Channel:     notificationv1.Channel_LARK_CARD,
		VersionName: "v1.0.0",
		Content:     LarkApprovalRevokeTemplate,
		NotifyType:  domain.NotifyTypeRevoke,
	},
}
