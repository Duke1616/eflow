package chat

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/Bunny3th/easy-workflow/workflow/model"
	notificationv1 "github.com/Duke1616/ecmdb/api/proto/gen/ealert/notification/v1"
	teamv1 "github.com/Duke1616/ecmdb/api/proto/gen/ealert/team"
	"github.com/Duke1616/eflow/internal/domain"
	easyflow2 "github.com/Duke1616/eflow/internal/pkg/easyflow"
	"github.com/Duke1616/eflow/internal/pkg/rule"
	"github.com/Duke1616/eflow/internal/service/event/strategy"
	"github.com/ecodeclub/ekit/slice"
	"github.com/gotomicro/ego/core/elog"
)

// Notification 群组通知策略
type Notification struct {
	strategy.Service
	teamSvc teamv1.TeamServiceClient
}

// NewNotification 实例化群组通知策略
func NewNotification(base strategy.Service, teamSvc teamv1.TeamServiceClient) *Notification {
	return &Notification{
		Service: base,
		teamSvc: teamSvc,
	}
}

type chatContext struct {
	*strategy.NotificationData
	property   easyflow2.ChatGroupProperty
	members    []domain.User
	userInputs []domain.FormValue
}

type recipient struct {
	chatID  string
	channel strategy.Channel
}

// Send 执行群通知投递与审批人注入
func (n *Notification) Send(ctx context.Context, info strategy.Info) (strategy.NotificationResponse, error) {
	// 1. 获取通知元数据 (同步获取以注入 UserIDs)
	data, err := n.fetchChatData(ctx, info)
	if err != nil {
		return strategy.NotificationResponse{Msg: err.Error()}, err
	}

	// 2. 注入审批人 (利用 data.members 确保引擎创建任务，以便后续自动推进)
	if len(data.members) > 0 {
		info.CurrentNode.UserIDs = slice.Map(data.members, func(idx int, u domain.User) string {
			return u.Username
		})
	}

	// 3. 异步处理：等待任务创建、发送群组消息、自动推进流程
	n.SafeGo(ctx, 3*time.Minute, func(sendCtx context.Context) {
		n.asyncHandleChat(sendCtx, info, data)
	})

	return strategy.NotificationResponse{Msg: "success"}, nil
}

// asyncHandleChat 异步等待并执行群通知推送
func (n *Notification) asyncHandleChat(ctx context.Context, info strategy.Info, data *chatContext) {
	n.Logger().Info("Notification 开始异步处理群组通知",
		elog.Int("instId", info.InstID),
		elog.String("node", info.CurrentNode.NodeID))

	// 1. 获取任务信息 (带重试，等待引擎完成任务下发)
	tasks, err := n.FetchTasksWithRetry(ctx, info)
	if err != nil {
		n.Logger().Error("Notification 获取任务失败", elog.FieldErr(err), elog.Int("instId", info.InstID))
		return
	}

	// 2. 检查并发送群消息
	if !n.IsGlobalNotify(info.Workflow) {
		n.Logger().Info("Notification 全局流程通知开关已关闭，跳过消息发送，仅执行流程自动推进")
	} else {
		n.sendNotifications(ctx, info, data)
	}

	// 3. 自动推进流程，确保群通知节点完成后自动向下流转
	n.autoPassTasks(ctx, tasks)
}

// autoPassTasks 自动提交并推进群通知节点对应的任务
func (n *Notification) autoPassTasks(ctx context.Context, tasks []model.Task) {
	for _, t := range tasks {
		if err := n.PassTask(ctx, t.TaskID, "ChatGroup Auto Pass"); err != nil {
			n.Logger().Error("Notification 流程自动推进失败",
				elog.FieldErr(err),
				elog.Int("taskId", t.TaskID))
			continue
		}

		n.Logger().Info("Notification 节点任务已自动推进",
			elog.Int("taskId", t.TaskID))

		if t.IsCosigned != 1 {
			return
		}
	}
}

// sendNotifications 仿真并格式化群通知消息批量推送
func (n *Notification) sendNotifications(ctx context.Context, info strategy.Info, data *chatContext) {
	recipients, err := n.resolveRecipients(ctx, info, data)
	if err != nil {
		n.Logger().Error("Notification 解析接收群组失败", elog.FieldErr(err))
		return
	}

	if len(recipients) == 0 {
		n.Logger().Info("Notification 未匹配到合法的群组接收者，跳过发送")
		return
	}

	// 仿真消息卡片发送投递
	title := n.resolveTitle(data.property.Title, info, data)
	fields := n.resolveFields(info, data)

	for _, r := range recipients {
		n.Logger().Info("[NOTIFICATION chat] 模拟向群组发送批量消息通知",
			elog.Int("instance_id", info.InstID),
			elog.String("chat_id", r.chatID),
			elog.Any("channel", r.channel),
			elog.String("title", title),
			elog.Any("fields", fields))
	}
}

// resolveRecipients 分发并解析不同模式下的接收者群聊信息
func (n *Notification) resolveRecipients(ctx context.Context, info strategy.Info, data *chatContext) ([]recipient, error) {
	switch data.property.Mode {
	case easyflow2.ChatGroupUseExisting:
		return n.handleExistingGroups(ctx, info, data)
	case easyflow2.ChatGroupCreate:
		return n.handleCreateGroup(ctx, info, data)
	default:
		n.Logger().Warn("未知的群组通知模式", elog.Any("mode", data.property.Mode))
		return nil, nil
	}
}

// handleExistingGroups 获取并处理已存在群组详情
func (n *Notification) handleExistingGroups(ctx context.Context, info strategy.Info, data *chatContext) ([]recipient, error) {
	if len(data.property.ChatGroupIDs) == 0 {
		n.Logger().Warn("Notification (ExistingMode) 未显式配置群组 ID，跳过处理",
			elog.Int("instId", info.InstID))
		return nil, nil
	}

	resp, err := n.teamSvc.GetChatGroupByIds(ctx, &teamv1.GetChatGroupByIdsRequest{Ids: data.property.ChatGroupIDs})
	if err != nil {
		return nil, fmt.Errorf("获取群组详情失败: %w", err)
	}

	// 仿真拉人进群的逻辑
	if len(data.members) > 0 {
		memberIDs := slice.Map(data.members, func(idx int, u domain.User) string { return u.Username })
		for _, cg := range resp.Groups {
			n.Logger().Info("[NOTIFICATION chat] 模拟添加动态成员到现有群组",
				elog.String("chatId", cg.ChatId),
				elog.Any("members", memberIDs))
		}
	}

	return slice.Map(resp.Groups, func(idx int, src *teamv1.ChatGroup) recipient {
		ch := strategy.Channel(src.Channel.String())
		if src.Channel == notificationv1.Channel_CHANNEL_UNSPECIFIED {
			ch = info.Channel
		}
		return recipient{chatID: src.ChatId, channel: ch}
	}), nil
}

// handleCreateGroup 获取、仿真创建并绑定群组
func (n *Notification) handleCreateGroup(ctx context.Context, info strategy.Info, data *chatContext) ([]recipient, error) {
	chatName := n.resolveChatName(data.property.Create.Name, info, data)

	chats, err := n.teamSvc.GetDefaultChatGroups(ctx, &teamv1.GetDefaultChatGroupsRequest{})
	if err != nil {
		return nil, err
	}

	chat, ok := slice.Find(chats.Groups, func(src *teamv1.ChatGroup) bool {
		return src.Name == chatName && src.Channel == notificationv1.Channel_LARK_CARD
	})

	if ok {
		if len(data.members) > 0 {
			memberIDs := slice.Map(data.members, func(idx int, u domain.User) string { return u.Username })
			n.Logger().Info("[NOTIFICATION chat] 模拟同步动态成员到已存在的默认群组",
				elog.String("chatId", chat.ChatId),
				elog.Any("members", memberIDs))
		}
		return []recipient{{
			chatID:  chat.ChatId,
			channel: strategy.Channel(chat.Channel.String()),
		}}, nil
	}

	// 仿真群聊创建
	chatID := fmt.Sprintf("mock_chat_id_%d", time.Now().UnixNano())
	n.Logger().Info("[NOTIFICATION chat] 模拟调用飞书 API 创建全新通知群组",
		elog.String("chatName", chatName),
		elog.Any("members", slice.Map(data.members, func(idx int, u domain.User) string { return u.Username })))

	// 异步绑定团队
	go n.asyncBindGroupToTeam(chatName, chatID)

	return []recipient{{chatID: chatID, channel: info.Channel}}, nil
}

// asyncBindGroupToTeam 异步将新群组绑定到发起人的团队
func (n *Notification) asyncBindGroupToTeam(chatName, chatID string) {
	_, err := n.teamSvc.BindChatGroup(context.Background(), &teamv1.BindChatGroupRequest{
		Group: &teamv1.ChatGroup{
			TeamId:  0,
			Name:    chatName,
			ChatId:  chatID,
			Channel: notificationv1.Channel_LARK_CARD,
		},
	})

	if err != nil {
		n.Logger().Error("异步绑定群组到默认团队失败", elog.FieldErr(err))
	} else {
		n.Logger().Info("成功异步绑定新群组到默认团队", elog.String("chatName", chatName), elog.String("chatID", chatID))
	}
}

// fetchChatData 并发拉取群消息所需的全量元数据
func (n *Notification) fetchChatData(ctx context.Context, info strategy.Info) (*chatContext, error) {
	nodes, rawProps, err := n.GetNodeProperty(info, info.CurrentNode.NodeID)
	if err != nil {
		return nil, err
	}

	property, err := easyflow2.ToNodeProperty[easyflow2.ChatGroupProperty](easyflow2.Node{Properties: rawProps})
	if err != nil {
		return nil, err
	}

	base, err := n.FetchRequiredData(ctx, info, nodes)
	if err != nil {
		return nil, err
	}

	var inputs []domain.FormValue
	if slice.Contains(property.OutputMode, easyflow2.OutputUserInput) {
		inputs, _ = n.FindTaskForms(ctx, info.Order.Id)
	}

	return &chatContext{
		NotificationData: base,
		property:         property,
		userInputs:       inputs,
		members:          n.resolveMembers(ctx, info, property),
	}, nil
}

var variableRegex = regexp.MustCompile(`{{(.*?)}}`)

// resolveTitle 动态解析生成卡片标题
func (n *Notification) resolveTitle(rule string, info strategy.Info, data *chatContext) string {
	return n.resolveDynamicString(rule, "{{creator}}发起的{{template}}执行结果", info, data)
}

// resolveChatName 动态解析生成群聊名称
func (n *Notification) resolveChatName(rule string, info strategy.Info, data *chatContext) string {
	return n.resolveDynamicString(rule, fmt.Sprintf("【ECMDB】- %s", data.TName), info, data)
}

// resolveDynamicString 统一变量渲染解析替换
func (n *Notification) resolveDynamicString(value, defaultVal string, info strategy.Info, data *chatContext) string {
	target := value
	if target == "" {
		target = defaultVal
	}

	vars := map[string]string{
		"ticket_id": fmt.Sprintf("%d", info.Order.Id),
		"template":  data.TName,
		"creator":   data.StartUser.DisplayName,
	}

	for k, v := range info.Order.Data {
		vars["field."+k] = fmt.Sprintf("%v", v)
	}

	result := variableRegex.ReplaceAllStringFunc(target, func(match string) string {
		res := variableRegex.FindStringSubmatch(match)
		if len(res) < 2 {
			return match
		}

		key := res[1]
		if val, ok := vars[key]; ok {
			return val
		}
		return match
	})

	if result == "" {
		return defaultVal
	}

	return result
}

// resolveMembers 解析节点配置规则以获取动态群成员
func (n *Notification) resolveMembers(ctx context.Context, info strategy.Info, property easyflow2.ChatGroupProperty) []domain.User {
	if len(property.Assignees) == 0 {
		return nil
	}

	users, _ := n.ResolveAssignees(ctx, &info, property.Assignees)
	return users
}

// resolveFields 根据 OutputMode 构造飞书卡片排版字段
func (n *Notification) resolveFields(info strategy.Info, data *chatContext) []rule.Field {
	var fields []rule.Field

	modeSet := make(map[easyflow2.OutputMode]bool, len(data.property.OutputMode))
	for _, mode := range data.property.OutputMode {
		modeSet[mode] = true
	}

	// 1. 工单信息
	if modeSet[easyflow2.OutputTicketData] {
		ruleFields := n.PrepareCommonFields(info, data.NotificationData)
		if len(ruleFields) > 0 {
			fields = append(fields, rule.Field{
				IsShort: false,
				Tag:     "lark_md",
				Content: "**📋 工单信息**",
			})
			fields = append(fields, ruleFields...)
		}
	}

	// 2. 用户提交
	if modeSet[easyflow2.OutputUserInput] {
		if len(data.userInputs) > 0 {
			fields = append(fields, rule.Field{
				IsShort: false,
				Tag:     "lark_md",
				Content: "**✍️ 用户提交**",
			})
			for _, input := range data.userInputs {
				fields = append(fields, rule.Field{
					IsShort: true,
					Tag:     "lark_md",
					Content: fmt.Sprintf("**%s:**\n%v", input.Name, input.Value),
				})
			}
		}
	}

	return fields
}
