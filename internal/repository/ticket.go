package repository

import (
	"context"
	"errors"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository/dao"
	"github.com/Duke1616/eflow/pkg/sqlx"
	"github.com/ecodeclub/ekit/slice"
	"gorm.io/gorm"
)

// TicketRepository 工单数据访问仓库接口
type TicketRepository interface {
	// CreateBizTicket 插入一条带有外部关联场景和单号的物理工单
	CreateBizTicket(ctx context.Context, ticket domain.Ticket) (domain.Ticket, error)
	// CreateTicket 插入一条基础物理工单并返回生成的主键 ID
	CreateTicket(ctx context.Context, req domain.Ticket) (int64, error)
	// DetailByProcessInstId 根据工作流引擎的流程实例 ID 检索本地工单数据
	DetailByProcessInstId(ctx context.Context, instanceId int) (domain.Ticket, error)
	// AccessibleDetailByProcessInstId 获取经过 PBAC AccessScope 约束的单条工单。
	AccessibleDetailByProcessInstId(ctx context.Context, instanceId int) (domain.Ticket, error)
	// Detail 根据主键 ID 获取指定的物理工单明细数据
	Detail(ctx context.Context, id int64) (domain.Ticket, error)
	// RegisterProcessInstanceId 登记并将生成的流程实例 ID 回写绑定到工单记录中
	RegisterProcessInstanceId(ctx context.Context, id int64, instanceId int) error
	// MarkProcessStartFailed 将尚未绑定流程实例的工单标记为流程启动失败并记录原因
	MarkProcessStartFailed(ctx context.Context, id int64, reason string) error
	// PrepareProcessRestart 将流程启动失败工单原子切换回启动中状态
	PrepareProcessRestart(ctx context.Context, id int64) error
	// ListProcessStartFailed 分页查询流程启动中或启动失败的工单
	ListProcessStartFailed(ctx context.Context, userId string, offset, limit int64) ([]domain.Ticket, error)
	// CountProcessStartFailed 统计流程启动中或启动失败的工单数量
	CountProcessStartFailed(ctx context.Context, userId string) (int64, error)
	// ListTicketByProcessInstanceIds 根据引擎实例 ID 集合批量高效拉取工单记录
	ListTicketByProcessInstanceIds(ctx context.Context, instanceIds []int) ([]domain.Ticket, error)
	// UpdateStatusByInstanceId 根据流程实例 ID 更新本地物理工单的最新审批流转状态
	UpdateStatusByInstanceId(ctx context.Context, instanceId int, status uint8) error
	// ListTicket 分页检索指定用户相关的、符合指定状态集合的物理工单记录
	ListTicket(ctx context.Context, userId string, status []int, offset, limit int64) ([]domain.Ticket, error)
	// CountTicket 统计指定用户相关、符合指定状态集合的物理工单总记录条数
	CountTicket(ctx context.Context, userId string, status []int) (int64, error)
	// ListHistory 获取经过 PBAC AccessScope 约束的历史工单列表，userId 仅作为附加收窄条件。
	ListHistory(ctx context.Context, userId string, status []int, offset, limit int64) ([]domain.Ticket, error)
	// CountHistory 使用与 ListHistory 相同的 PBAC 和查询条件统计历史工单总数。
	CountHistory(ctx context.Context, userId string, status []int) (int64, error)
	// FindByBizIdAndKey 依据外部业务场景 ID 及业务唯一单据号 Key 查找匹配工单
	FindByBizIdAndKey(ctx context.Context, bizId int64, key string, status []domain.Status) (domain.Ticket, error)
	// MergeTicketData 高效增量合并更新工单内已存盘的动态表单变量变量集
	MergeTicketData(ctx context.Context, id int64, data map[string]interface{}) error
	// CreateTaskForm 物理留存审批步骤所提交的控件表单字段快照记录
	CreateTaskForm(ctx context.Context, taskId int, ticketId int64, fields []domain.FormValue) error
	// FindTaskFormsBatch 批量依据步骤任务 ID 集合加载对应的节点表单数据映射
	FindTaskFormsBatch(ctx context.Context, taskIds []int) (map[int][]domain.FormValue, error)
	// FindTaskFormsByTicketID 检索并统计指定工单下所有历史步骤节点所存盘的表单数据集
	FindTaskFormsByTicketID(ctx context.Context, ticketID int64) ([]domain.FormValue, error)
}

type ticketRepository struct {
	dao       dao.TicketDAO
	taskForms dao.TaskFormDAO
}

func NewTicketRepository(dao dao.TicketDAO, taskForms dao.TaskFormDAO) TicketRepository {
	return &ticketRepository{
		dao:       dao,
		taskForms: taskForms,
	}
}

func (repo *ticketRepository) CreateBizTicket(ctx context.Context, ticket domain.Ticket) (domain.Ticket, error) {
	resp, err := repo.dao.CreateBizTicket(ctx, repo.toEntity(ticket))
	return repo.toDomain(resp), err
}

func (repo *ticketRepository) CreateTicket(ctx context.Context, req domain.Ticket) (int64, error) {
	return repo.dao.CreateTicket(ctx, repo.toEntity(req))
}

func (repo *ticketRepository) DetailByProcessInstId(ctx context.Context, instanceId int) (domain.Ticket, error) {
	ticket, err := repo.dao.DetailByProcessInstId(ctx, instanceId)
	return repo.toDomain(ticket), err
}

func (repo *ticketRepository) AccessibleDetailByProcessInstId(ctx context.Context, instanceId int) (domain.Ticket, error) {
	ticket, err := repo.dao.AccessibleDetailByProcessInstId(ctx, instanceId)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Ticket{}, domain.ErrTicketNotAccessible
	}
	return repo.toDomain(ticket), err
}

func (repo *ticketRepository) Detail(ctx context.Context, id int64) (domain.Ticket, error) {
	ticket, err := repo.dao.Detail(ctx, id)
	return repo.toDomain(ticket), err
}

func (repo *ticketRepository) RegisterProcessInstanceId(ctx context.Context, id int64, instanceId int) error {
	return repo.dao.RegisterProcessInstanceId(ctx, id, instanceId, domain.PROCESS.ToUint8())
}

// MarkProcessStartFailed 将尚未绑定流程实例的工单标记为流程启动失败并记录原因。
func (repo *ticketRepository) MarkProcessStartFailed(ctx context.Context, id int64, reason string) error {
	return repo.dao.MarkProcessStartFailed(ctx, id, reason)
}

// PrepareProcessRestart 将流程启动失败工单原子切换回启动中状态。
func (repo *ticketRepository) PrepareProcessRestart(ctx context.Context, id int64) error {
	return repo.dao.PrepareProcessRestart(ctx, id)
}

// ListProcessStartFailed 分页查询流程启动中或启动失败的工单。
func (repo *ticketRepository) ListProcessStartFailed(ctx context.Context, userId string, offset, limit int64) ([]domain.Ticket, error) {
	tickets, err := repo.dao.ListProcessStartFailed(ctx, userId, offset, limit)
	return slice.Map(tickets, func(idx int, src dao.Ticket) domain.Ticket { return repo.toDomain(src) }), err
}

// CountProcessStartFailed 统计流程启动中或启动失败的工单数量。
func (repo *ticketRepository) CountProcessStartFailed(ctx context.Context, userId string) (int64, error) {
	return repo.dao.CountProcessStartFailed(ctx, userId)
}

func (repo *ticketRepository) ListTicketByProcessInstanceIds(ctx context.Context, instanceIds []int) ([]domain.Ticket, error) {
	tickets, err := repo.dao.ListTicketByProcessInstanceIds(ctx, instanceIds)
	return slice.Map(tickets, func(idx int, src dao.Ticket) domain.Ticket { return repo.toDomain(src) }), err
}

func (repo *ticketRepository) UpdateStatusByInstanceId(ctx context.Context, instanceId int, status uint8) error {
	return repo.dao.UpdateStatusByInstanceId(ctx, instanceId, status)
}

func (repo *ticketRepository) ListTicket(ctx context.Context, userId string, status []int, offset, limit int64) ([]domain.Ticket, error) {
	tickets, err := repo.dao.ListTicket(ctx, userId, status, offset, limit)
	return slice.Map(tickets, func(idx int, src dao.Ticket) domain.Ticket { return repo.toDomain(src) }), err
}

func (repo *ticketRepository) CountTicket(ctx context.Context, userId string, status []int) (int64, error) {
	return repo.dao.CountTicket(ctx, userId, status)
}

// ListHistory 查询指定状态范围内经 PBAC 决策允许访问的历史工单。
func (repo *ticketRepository) ListHistory(ctx context.Context, userId string, status []int, offset, limit int64) ([]domain.Ticket, error) {
	tickets, err := repo.dao.ListHistory(ctx, userId, status, offset, limit)
	return slice.Map(tickets, func(idx int, src dao.Ticket) domain.Ticket { return repo.toDomain(src) }), err
}

// CountHistory 统计经 PBAC 决策允许访问的历史工单。
func (repo *ticketRepository) CountHistory(ctx context.Context, userId string, status []int) (int64, error) {
	return repo.dao.CountHistory(ctx, userId, status)
}

func (repo *ticketRepository) FindByBizIdAndKey(ctx context.Context, bizId int64, key string, status []domain.Status) (domain.Ticket, error) {
	statusValues := slice.Map(status, func(idx int, src domain.Status) int {
		return src.ToInt()
	})
	ticket, err := repo.dao.FindByBizIdAndKey(ctx, bizId, key, statusValues)
	return repo.toDomain(ticket), err
}

func (repo *ticketRepository) MergeTicketData(ctx context.Context, id int64, data map[string]interface{}) error {
	return repo.dao.MergeTicketData(ctx, id, data)
}

func (repo *ticketRepository) toEntity(req domain.Ticket) dao.Ticket {
	return dao.Ticket{
		TenantID:             req.TenantID,
		BizID:                req.BizID,
		Key:                  req.Key,
		TemplateId:           req.TemplateId,
		Status:               req.Status.ToUint8(),
		Provide:              req.Provide.ToUint8(),
		WorkflowId:           req.WorkflowId,
		CreateBy:             req.CreateBy,
		ProcessStartError:    req.ProcessStartError,
		ProcessStartAttempts: req.ProcessStartAttempts,
		Data:                 sqlx.JsonField[domain.TicketData]{Val: req.Data, Valid: true},
		NotificationConf: sqlx.JsonField[dao.NotificationConf]{
			Val: dao.NotificationConf{
				TemplateID:     req.NotificationConf.TemplateID,
				TemplateParams: req.NotificationConf.TemplateParams,
				Channel:        req.NotificationConf.Channel.String(),
			},
			Valid: true,
		},
	}
}

func (repo *ticketRepository) toDomain(req dao.Ticket) domain.Ticket {
	return domain.Ticket{
		Id:                   req.Id,
		TenantID:             req.TenantID,
		BizID:                req.BizID,
		Key:                  req.Key,
		TemplateId:           req.TemplateId,
		Status:               domain.Status(req.Status),
		Provide:              domain.Provide(req.Provide),
		WorkflowId:           req.WorkflowId,
		Process:              domain.Process{InstanceId: req.ProcessInstanceId},
		CreateBy:             req.CreateBy,
		Data:                 req.Data.Val,
		Ctime:                req.Ctime,
		Wtime:                req.Wtime,
		RevokeReason:         req.RevokeReason,
		ProcessStartError:    req.ProcessStartError,
		ProcessStartAttempts: req.ProcessStartAttempts,
		NotificationConf: domain.NotificationConf{
			TemplateID:     req.NotificationConf.Val.TemplateID,
			TemplateParams: req.NotificationConf.Val.TemplateParams,
			Channel:        domain.Channel(req.NotificationConf.Val.Channel),
		},
	}
}
