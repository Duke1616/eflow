package repository

import (
	"context"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository/dao"
	"github.com/Duke1616/eflow/pkg/sqlx"
	"github.com/ecodeclub/ekit/slice"
	"gorm.io/gorm"
)

// ErrTicketNotFound 工单记录不存在
var ErrTicketNotFound = gorm.ErrRecordNotFound

// TicketRepository 工单数据访问仓库接口
type TicketRepository interface {
	// CreateBizTicket 插入一条带有外部关联场景和单号的物理工单
	CreateBizTicket(ctx context.Context, ticket domain.Ticket) (domain.Ticket, error)
	// CreateTicket 插入一条基础物理工单并返回生成的主键 ID
	CreateTicket(ctx context.Context, req domain.Ticket) (int64, error)
	// DetailByProcessInstance 根据流程实例标识（自适应整型 ID 或字符串 Key）多态检索本地工单数据
	DetailByProcessInstance(ctx context.Context, instanceID domain.ID) (domain.Ticket, error)
	// DetailByEngineInstance 显式指定工作流引擎类型与实例标识精准检索工单数据
	DetailByEngineInstance(ctx context.Context, engineType domain.EngineType, instanceID domain.ID) (domain.Ticket, error)
	// Detail 根据主键 ID 获取指定的物理工单明细数据
	Detail(ctx context.Context, id int64) (domain.Ticket, error)
	// RegisterProcess 登记并将流程引擎实例类型与唯一标识回写绑定到工单记录中
	RegisterProcess(ctx context.Context, id int64, engineType domain.EngineType, instanceID domain.ID) error
	// MarkProcessStartFailed 将尚未绑定流程实例的工单标记为流程启动失败并记录原因
	MarkProcessStartFailed(ctx context.Context, id int64, reason string) error
	// PrepareProcessRestart 将流程启动失败工单原子切换回启动中状态
	PrepareProcessRestart(ctx context.Context, id int64) error
	// ListProcessStartFailed 分页查询流程启动中或启动失败的工单
	ListProcessStartFailed(ctx context.Context, userId string, offset, limit int64) ([]domain.Ticket, error)
	// CountProcessStartFailed 统计流程启动中或启动失败的工单数量
	CountProcessStartFailed(ctx context.Context, userId string) (int64, error)
	// ListTicketByProcessInstances 根据流程实例标识集合（自动自适应匹配现代全局唯一 Key 或历史自增 ID）批量高效拉取工单记录
	ListTicketByProcessInstances(ctx context.Context, instanceIDs []string) ([]domain.Ticket, error)
	// UpdateStatusByProcessInstance 根据流程实例标识自适应更新本地物理工单的最新审批流转状态
	UpdateStatusByProcessInstance(ctx context.Context, instanceID domain.ID, status uint8, wtime int64) error
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
	CreateTaskForm(ctx context.Context, taskId domain.ID, ticketId int64, fields []domain.FormValue) error
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

func (repo *ticketRepository) DetailByProcessInstance(ctx context.Context, instanceID domain.ID) (domain.Ticket, error) {
	ticket, err := repo.dao.DetailByProcessInstance(ctx, instanceID.String())
	return repo.toDomain(ticket), err
}

func (repo *ticketRepository) DetailByEngineInstance(ctx context.Context, engineType domain.EngineType, instanceID domain.ID) (domain.Ticket, error) {
	switch engineType {
	case domain.EngineTypeFlowx:
		ticket, err := repo.dao.DetailByProcessInstKey(ctx, instanceID.String())
		return repo.toDomain(ticket), err
	case domain.EngineTypeEasyFlow:
		ticket, err := repo.dao.DetailByProcessInstId(ctx, instanceID.Int())
		return repo.toDomain(ticket), err
	default:
		// 未来接入的所有现代工作流引擎（如 Temporal / Camunda），默认均按现代标准 Key 检索
		ticket, err := repo.dao.DetailByProcessInstKey(ctx, instanceID.String())
		return repo.toDomain(ticket), err
	}
}

func (repo *ticketRepository) UpdateStatusByProcessInstance(ctx context.Context, instanceID domain.ID, status uint8, wtime int64) error {
	return repo.dao.UpdateStatusByProcessInstance(ctx, instanceID.String(), status, wtime)
}

func (repo *ticketRepository) Detail(ctx context.Context, id int64) (domain.Ticket, error) {
	ticket, err := repo.dao.Detail(ctx, id)
	return repo.toDomain(ticket), err
}

func (repo *ticketRepository) RegisterProcess(ctx context.Context, id int64, engineType domain.EngineType, instanceID domain.ID) error {
	switch engineType {
	case domain.EngineTypeFlowx:
		return repo.dao.RegisterProcessInstanceKey(ctx, id, instanceID.String(), domain.PROCESS.ToUint8())
	default:
		return repo.dao.RegisterProcessInstanceId(ctx, id, instanceID.Int(), domain.PROCESS.ToUint8())
	}
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

func (repo *ticketRepository) ListTicketByProcessInstances(ctx context.Context, instanceIDs []string) ([]domain.Ticket, error) {
	tickets, err := repo.dao.ListTicketByProcessInstances(ctx, instanceIDs)
	return slice.Map(tickets, func(idx int, src dao.Ticket) domain.Ticket { return repo.toDomain(src) }), err
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

// resolveInstanceStorage 根据引擎类型决议底层持久化存储形态
func resolveInstanceStorage(proc domain.Process) (instID int, instKey string) {
	if proc.EngineType == domain.EngineTypeFlowx {
		return 0, proc.InstanceID.String()
	}
	return proc.InstanceID.Int(), ""
}

func (repo *ticketRepository) toEntity(req domain.Ticket) dao.Ticket {
	instID, instKey := resolveInstanceStorage(req.Process)

	engineType := req.Process.EngineType.String()
	if engineType == "" {
		engineType = domain.EngineTypeEasyFlow.String()
	}

	return dao.Ticket{
		TenantID:             req.TenantID,
		BizID:                req.BizID,
		Key:                  req.Key,
		TemplateId:           req.TemplateId,
		Status:               req.Status.ToUint8(),
		Provide:              req.Provide.ToUint8(),
		WorkflowId:           req.WorkflowId,
		EngineType:           engineType,
		ProcessInstanceId:    instID,
		ProcessInstanceKey:   instKey,
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
	var proc domain.Process
	engineType := domain.EngineType(req.EngineType)
	if engineType == "" {
		// 历史老数据兜底自适应推断
		if req.ProcessInstanceKey != "" {
			engineType = domain.EngineTypeFlowx
		} else {
			engineType = domain.EngineTypeEasyFlow
		}
	}

	if engineType == domain.EngineTypeFlowx && req.ProcessInstanceKey != "" {
		proc = domain.Process{
			EngineType: engineType,
			InstanceID: domain.NewID(req.ProcessInstanceKey),
		}
	} else if req.ProcessInstanceId > 0 {
		proc = domain.Process{
			EngineType: engineType,
			InstanceID: domain.NewID(req.ProcessInstanceId),
		}
	}

	return domain.Ticket{
		Id:                   req.Id,
		TenantID:             req.TenantID,
		BizID:                req.BizID,
		Key:                  req.Key,
		TemplateId:           req.TemplateId,
		Status:               domain.Status(req.Status),
		Provide:              domain.Provide(req.Provide),
		WorkflowId:           req.WorkflowId,
		Process:              proc,
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
