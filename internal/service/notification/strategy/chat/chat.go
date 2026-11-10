package chat

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"time"

	notificationv1 "github.com/Duke1616/eflow/api/proto/gen/ealert/notification/v1"
	teamv1 "github.com/Duke1616/eflow/api/proto/gen/ealert/team"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/flowdsl"
	"github.com/Duke1616/eflow/internal/pkg/notification"
	"github.com/Duke1616/eflow/internal/pkg/notification/sender"
	"github.com/Duke1616/eflow/internal/pkg/rule"
	"github.com/Duke1616/eflow/internal/service/notification/strategy"
	"github.com/Duke1616/enotify/notify/feishu"
	"github.com/gotomicro/ego/core/elog"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"github.com/samber/lo"
	"google.golang.org/grpc/metadata"
)

type Notification struct {
	strategy.Service
	sender     sender.NotificationSender
	larkClient *lark.Client
	teamSvc    teamv1.TeamServiceClient
}

func NewNotification(base strategy.Service, sender sender.NotificationSender,
	larkClient *lark.Client, teamSvc teamv1.TeamServiceClient) *Notification {
	return &Notification{
		Service:    base,
		sender:     sender,
		larkClient: larkClient,
		teamSvc:    teamSvc,
	}
}

// chatContext 封装发送所需的所有元数据
type chatContext struct {
	*strategy.NotificationData                         // 基础元数据和自动化输出
	property                   flowdsl.ChatGroupConfig // 节点配置属性
	members                    []domain.User           // 运行解析出的成员
	userInputs                 []domain.FormValue      // 动态加载的用户输入
}

// recipient 内部接收者描述
type recipient struct {
	chatID  string
	channel notification.Channel
}

func (n *Notification) Send(ctx context.Context, info strategy.Info) (notification.Response, error) {
	// 1. 获取通知元数据 (同步获取以注入 UserIDs)
	data, err := n.fetchChatData(ctx, info)
	if err != nil {
		return notification.NewErrorResponse(string(notification.ErrorCodeFetchDataFailed), err.Error()), err
	}

	// 2. 注入审批人 (利用 data.members 确保引擎创建任务，以便后续自动推进)
	info.CurrentNode.UserIDs = lo.Map(data.members, func(u domain.User, _ int) string {
		return u.Username
	})

	// 3. 异步处理：等待任务创建、发送群组消息、自动推进流程
	n.SafeGo(ctx, 3*time.Minute, func(sendCtx context.Context) {
		n.asyncHandleChat(sendCtx, info, data)
	})

	return notification.Response{}, nil
}

// asyncHandleChat 异步处理核心逻辑（作为纯粹的只读通知观察者，成败绝不影响工单状态机推进）
func (n *Notification) asyncHandleChat(ctx context.Context, info strategy.Info, data *chatContext) {
	if !n.IsGlobalNotify(info.Workflow) {
		return
	}
	n.sendNotifications(ctx, info, data)
}

func (n *Notification) sendNotifications(ctx context.Context, info strategy.Info, data *chatContext) {
	recipients, err := n.resolveRecipients(ctx, info, data)
	if err != nil {
		n.Logger().Error("Notification 解析接收群组失败", elog.FieldErr(err))
		return
	}
	if len(recipients) == 0 {
		return
	}

	notifications := n.buildNotifications(info, data, recipients)
	if _, err = n.sender.BatchSend(ctx, notifications); err != nil {
		n.Logger().Warn("Notification 发送群组消息失败", elog.FieldErr(err))
	}
}

// resolveRecipients 根据模式分发解析接收者逻辑
func (n *Notification) resolveRecipients(ctx context.Context, info strategy.Info, data *chatContext) ([]recipient, error) {
	switch data.property.Mode {
	case flowdsl.ChatGroupUseExisting:
		return n.handleExistingGroups(ctx, info, data)
	case flowdsl.ChatGroupCreate:
		return n.handleCreateGroup(ctx, info, data)
	default:
		return nil, nil
	}
}

// handleExistingGroups 处理现有群组逻辑：必须显式指定 ChatGroupIDs
func (n *Notification) handleExistingGroups(ctx context.Context, info strategy.Info, data *chatContext) ([]recipient, error) {
	if len(data.property.ChatGroupIDs) == 0 {
		return nil, nil
	}

	// 注入租户上下文并获取群组详情
	rpcCtx := withTenantContext(ctx, info.Ticket.TenantID)
	resp, err := n.teamSvc.GetChatGroupByIds(rpcCtx, &teamv1.GetChatGroupByIdsRequest{Ids: data.property.ChatGroupIDs})
	if err != nil {
		return nil, fmt.Errorf("获取群组详情失败: %w", err)
	}

	// 同步补齐动态成员
	if members := n.extractMemberIDs(data.members); len(members) > 0 {
		for _, cg := range resp.Groups {
			_ = n.addMembersToChat(ctx, cg.ChatId, members)
		}
	}

	return lo.Map(resp.Groups, func(src *teamv1.ChatGroup, _ int) recipient {
		ch := strategy.GetChatChannel(src.Channel.String())
		if src.Channel == notificationv1.Channel_CHANNEL_UNSPECIFIED {
			ch = notification.Channel(info.Channel)
		}
		return recipient{chatID: src.ChatId, channel: ch}
	}), nil
}

// handleCreateGroup 处理动态建群：同名租户公共群优先复用，未命中则按需新建并登记为默认群
func (n *Notification) handleCreateGroup(ctx context.Context, info strategy.Info, data *chatContext) ([]recipient, error) {
	rpcCtx := withTenantContext(ctx, info.Ticket.TenantID)
	var createName string
	if data.property.Create != nil {
		createName = data.property.Create.Name
	}
	chatName := n.resolveChatName(createName, info, data)

	// 1. 同名租户已有群优先复用
	if chat, _ := n.findExistingChatGroup(rpcCtx, chatName); chat != nil {
		n.syncMembersToExistingChat(ctx, chat.ChatId, data)
		return []recipient{{
			chatID:  chat.ChatId,
			channel: strategy.GetChatChannel(chat.Channel.String()),
		}}, nil
	}

	// 2. 首次按需创建新群
	chatID, err := n.createChatGroup(ctx, chatName, data)
	if err != nil {
		return nil, fmt.Errorf("创建飞书通知群组失败: %w", err)
	}

	// 3. 异步关联租户公共群（生命周期脱钩解耦，TeamID 统一归属于租户流程公共资产）
	go n.asyncBindGroup(withTenantContext(context.WithoutCancel(ctx), info.Ticket.TenantID), chatName, chatID)

	return []recipient{{chatID: chatID, channel: notification.Channel(info.Channel)}}, nil
}

// findExistingChatGroup 在当前租户维度查找同名默认/公共群组
func (n *Notification) findExistingChatGroup(ctx context.Context, chatName string) (*teamv1.ChatGroup, error) {
	resp, err := n.teamSvc.GetDefaultChatGroups(ctx, &teamv1.GetDefaultChatGroupsRequest{})
	if err != nil {
		return nil, err
	}

	chat, ok := lo.Find(resp.Groups, func(g *teamv1.ChatGroup) bool {
		return g.Name == chatName && (g.Channel == notificationv1.Channel_LARK_CARD || g.Channel == notificationv1.Channel_CHANNEL_UNSPECIFIED)
	})
	if !ok {
		return nil, nil
	}
	return chat, nil
}

// withTenantContext 为 RPC 调用注入多租户上下文元数据 (x-tenant-id)
func withTenantContext(ctx context.Context, tenantID int64) context.Context {
	if tenantID <= 0 {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, "x-tenant-id", strconv.FormatInt(tenantID, 10))
}

// asyncBindGroup 异步将新群组绑定到租户默认群并持久化
func (n *Notification) asyncBindGroup(ctx context.Context, chatName, chatID string) {
	_, err := n.teamSvc.BindChatGroup(ctx, &teamv1.BindChatGroupRequest{
		Group: &teamv1.ChatGroup{
			TeamId:  0, // 动态创建的流程群统一作为租户公共资产归宿 (TeamID = 0)
			Name:    chatName,
			ChatId:  chatID,
			Channel: notificationv1.Channel_LARK_CARD,
		},
	})
	if err != nil {
		n.Logger().Error("异步绑定群组到租户默认群失败", elog.FieldErr(err), elog.String("chatName", chatName), elog.String("chatId", chatID))
	}
}

func (n *Notification) syncMembersToExistingChat(ctx context.Context, chatID string, data *chatContext) {
	members := n.extractMemberIDs(data.members)
	if len(members) == 0 {
		return
	}
	_ = n.addMembersToChat(ctx, chatID, members)
}

// buildNotifications 组装最终的批量通知对象
func (n *Notification) buildNotifications(info strategy.Info, data *chatContext, recipients []recipient) []notification.Notification {
	title := n.resolveTitle(data.property.Title, info, data)
	fields := n.resolveFields(info, data)

	return lo.FilterMap(recipients, func(r recipient, _ int) (notification.Notification, bool) {
		if r.chatID == "" {
			return notification.Notification{}, false
		}

		return notification.Notification{
			Channel:      r.channel,
			ReceiverType: notification.ReceiverTypeChatGroup,
			WorkFlowID:   info.Workflow.Id,
			Receiver:     r.chatID,
			Template: notification.Template{
				Name:     domain.NotifyTypeChat,
				Title:    title,
				Fields:   fields,
				HideForm: true,
				Values:   notification.GenerateCallbackValues(info.Ticket.Id, "", info.Ticket.TenantID),
			},
		}, true
	})
}

// createChatGroup 创建飞书群组
func (n *Notification) createChatGroup(ctx context.Context, chatName string, data *chatContext) (string, error) {
	memberIDs := n.extractMemberIDs(data.members)
	if len(memberIDs) == 0 {
		return "", fmt.Errorf("未找到有效的群成员")
	}

	req := larkim.NewCreateChatReqBuilder().
		UserIdType(feishu.ReceiveIDTypeUserID).
		Body(larkim.NewCreateChatReqBodyBuilder().
			Name(chatName).
			UserIdList(memberIDs).
			Build()).
		Build()

	resp, err := n.larkClient.Im.V1.Chat.Create(ctx, req)
	if err != nil {
		return "", err
	}

	if !resp.Success() {
		return "", fmt.Errorf("飞书创建群组失败: %s", resp.Msg)
	}

	return *resp.Data.ChatId, nil
}

// fetchChatData 获取发送消息所需的全量元数据
func (n *Notification) fetchChatData(ctx context.Context, info strategy.Info) (*chatContext, error) {
	nodes, node, err := n.GetNode(info, info.CurrentNode.NodeID)
	if err != nil {
		return nil, err
	}

	var property flowdsl.ChatGroupConfig
	if chatCfg, ok := node.AsChatGroup(); ok {
		property = *chatCfg
	}

	base, err := n.FetchRequiredData(ctx, info, nodes)
	if err != nil {
		return nil, err
	}

	var inputs []domain.FormValue
	if lo.Contains(property.OutputMode, flowdsl.OutputUserInput) {
		inputs, _ = n.FindTaskForms(ctx, info.Ticket.Id)
	}

	return &chatContext{
		NotificationData: base,
		property:         property,
		userInputs:       inputs,
		members:          n.resolveMembers(ctx, info, property),
	}, nil
}

var variableRegex = regexp.MustCompile(`{{(.*?)}}`)

func (n *Notification) resolveTitle(rule string, info strategy.Info, data *chatContext) string {
	return n.resolveDynamicString(rule, "{{creator}}发起的{{template}}执行结果", info, data)
}

// resolveChatName 解析动态群组名称
func (n *Notification) resolveChatName(rule string, info strategy.Info, data *chatContext) string {
	return n.resolveDynamicString(rule, fmt.Sprintf("【ECMDB】- %s", data.TName), info, data)
}

// resolveDynamicString 解析动态字符串 (支持变量替换)
func (n *Notification) resolveDynamicString(value, defaultVal string, info strategy.Info, data *chatContext) string {
	target := value
	if target == "" {
		target = defaultVal
	}

	vars := map[string]string{
		"ticket_id": fmt.Sprintf("%d", info.Ticket.Id),
		"template":  data.TName,
		"creator":   data.StartUser.DisplayName,
	}

	for k, v := range info.Ticket.Data {
		vars["field."+k] = fmt.Sprintf("%v", v)
	}

	result := variableRegex.ReplaceAllStringFunc(target, func(match string) string {
		res := variableRegex.FindStringSubmatch(match)
		if len(res) < 2 {
			return match
		}
		if val, ok := vars[res[1]]; ok {
			return val
		}
		return match
	})

	if result == "" {
		return defaultVal
	}
	return result
}

// addMembersToChat 为现有群组添加成员
func (n *Notification) addMembersToChat(ctx context.Context, chatID string, memberIDs []string) error {
	req := larkim.NewCreateChatMembersReqBuilder().
		ChatId(chatID).
		MemberIdType(feishu.ReceiveIDTypeUserID).
		Body(larkim.NewCreateChatMembersReqBodyBuilder().
			IdList(memberIDs).
			Build()).
		Build()

	resp, err := n.larkClient.Im.V1.ChatMembers.Create(ctx, req)
	if err != nil {
		return err
	}

	if !resp.Success() {
		// 忽略重复加入的错误 (99991663: 已经在群中; 99991668: 部分已经在群中)
		if resp.Code == 99991663 || resp.Code == 99991668 {
			return nil
		}
		return fmt.Errorf("飞书群组拉人失败[code: %d]: %s", resp.Code, resp.Msg)
	}

	return nil
}

// resolveMembers 解析规则获取最终用户列表
func (n *Notification) resolveMembers(ctx context.Context, info strategy.Info, property flowdsl.ChatGroupConfig) []domain.User {
	if len(property.Assignees) == 0 {
		return nil
	}
	users, _ := n.ResolveAssignees(ctx, &info, property.Assignees)
	return users
}

// resolveFields 依据 OutputMode 解析通知内容字段
func (n *Notification) resolveFields(info strategy.Info, data *chatContext) []notification.Field {
	var fields []notification.Field

	modeSet := lo.SliceToMap(data.property.OutputMode, func(m flowdsl.OutputMode) (flowdsl.OutputMode, struct{}) {
		return m, struct{}{}
	})

	// 1. 工单信息
	if _, ok := modeSet[flowdsl.OutputTicketData]; ok {
		ruleFields := rule.GetFields(data.Rules, info.Ticket.Provide.ToUint8(), info.Ticket.Data)
		modeFields := strategy.ConvertRuleFields(ruleFields)
		if len(modeFields) > 0 {
			fields = append(fields, sectionHeader("📋 工单信息"))
			fields = append(fields, modeFields...)
		}
	}

	// 2. 用户提交
	if _, ok := modeSet[flowdsl.OutputUserInput]; ok {
		if len(data.userInputs) > 0 {
			fields = append(fields, sectionHeader("✍️ 用户提交"))
			inputFields := lo.Map(data.userInputs, func(input domain.FormValue, _ int) notification.Field {
				return notification.Field{
					IsShort: true,
					Tag:     "lark_md",
					Content: fmt.Sprintf("**%s:**\n%v", input.Name, input.Value),
				}
			})
			fields = append(fields, notification.AddRowSpacers(inputFields)...)
		}
	}

	// 3. 执行结果
	if _, ok := modeSet[flowdsl.OutputAutoTask]; ok {
		modeFields := strategy.BuildAutomationOutputFields(data.AutomationOutput)
		if len(modeFields) > 0 {
			fields = append(fields, sectionHeader("⚙️ 执行结果"))
			fields = append(fields, modeFields...)
		}
	}

	return fields
}

// sectionHeader 生成全宽小标题字段，用于在飞书卡片中分隔不同数据区块
func sectionHeader(title string) notification.Field {
	return notification.Field{
		IsDivider: true,
		Tag:       "lark_md",
		Content:   "**" + title + "**",
	}
}

// extractMemberIDs 提取飞书 ID 列表
func (n *Notification) extractMemberIDs(users []domain.User) []string {
	return lo.FilterMap(users, func(u domain.User, _ int) (string, bool) {
		return u.LarkUserId, u.LarkUserId != ""
	})
}
