package dao

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/ticketpbac"
	"github.com/Duke1616/eflow/pkg/sqlx"
	"github.com/Duke1616/eiam/pkg/gormx"
	pbacgorm "github.com/Duke1616/eiam/pkg/pbac/gormx"
	"github.com/samber/lo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Ticket 工单记录实体
type Ticket struct {
	Id                   int64                             `gorm:"primaryKey;column:id;type:bigint;autoIncrement;comment:'工单自增ID'"`
	TenantID             int64                             `gorm:"column:tenant_id;type:bigint;not null;index;comment:'多租户隔离标识'"`
	BizID                int64                             `gorm:"column:biz_id;type:bigint;index;comment:'关联业务场景ID'"`
	Key                  string                            `gorm:"column:key;type:varchar(64);index;comment:'工单业务唯一单据号Key'"`
	TemplateId           int64                             `gorm:"column:template_id;type:bigint;not null;index;comment:'绑定工单模板ID'"`
	WorkflowId           int64                             `gorm:"column:workflow_id;type:bigint;not null;index;comment:'绑定工作流流程ID'"`
	EngineType           string                            `gorm:"column:engine_type;type:varchar(32);not null;default:'easy-workflow';index;comment:'流程引擎类型(easy-workflow/flowx)'"`
	ProcessInstanceId    int                               `gorm:"column:process_instance_id;type:int;index;comment:'关联流程实例执行ID'"`
	ProcessInstanceKey   string                            `gorm:"column:process_instance_key;type:varchar(64);not null;default:'';index;comment:'现代流程引擎实例全局唯一标识Key'"`
	CreateBy             string                            `gorm:"column:create_by;type:varchar(128);index;comment:'工单创建人用户名'"`
	Provide              uint8                             `gorm:"column:provide;type:tinyint unsigned;comment:'业务提供属性'"`
	Data                 sqlx.JsonField[domain.TicketData] `gorm:"column:data;type:json;comment:'工单用户填写的动态数据json'"`
	Status               uint8                             `gorm:"column:status;type:tinyint unsigned;index;comment:'工单当前状态'"`
	NotificationConf     sqlx.JsonField[NotificationConf]  `gorm:"column:notification_conf;type:json;comment:'工单通知偏好参数配置json'"`
	Ctime                int64                             `gorm:"column:ctime;type:bigint;comment:'创建发起时间(毫秒)'"`
	Wtime                int64                             `gorm:"column:wtime;type:bigint;comment:'办结归档时间(毫秒)'"`
	RevokeReason         string                            `gorm:"column:revoke_reason;type:varchar(500);not null;default:'';comment:'撤单原因'"`
	ProcessStartError    string                            `gorm:"column:process_start_error;type:varchar(1000);not null;default:'';comment:'流程启动失败原因'"`
	ProcessStartAttempts int                               `gorm:"column:process_start_attempts;type:int;not null;default:0;comment:'流程启动尝试次数'"`
	Utime                int64                             `gorm:"column:utime;type:bigint;comment:'最近修改时间(毫秒)'"`
}

// NotificationConf 工单相关的通知模板与参数配置
type NotificationConf struct {
	TemplateID     int64                     `json:"template_id"`
	TemplateParams domain.NotificationParams `json:"template_params"`
	Channel        string                    `json:"channel"`
}

// TicketDAO 工单物理数据访问接口
type TicketDAO interface {
	// CreateBizTicket 在 DB 中物理持久化一条带有外部场景属性和唯一键值的工单记录
	CreateBizTicket(ctx context.Context, ticket Ticket) (Ticket, error)
	// CreateTicket 在 DB 中物理持久化一条基础工单并返回自增主键 ID
	CreateTicket(ctx context.Context, req Ticket) (int64, error)
	// DetailByProcessInstId 依据绑定的工作流引擎流程实例 ID 从物理表获取单据
	DetailByProcessInstId(ctx context.Context, instanceId int) (Ticket, error)
	// DetailByProcessInstKey 依据绑定的工作流引擎流程实例全局唯一 Key 从物理表获取单据
	DetailByProcessInstKey(ctx context.Context, instanceKey string) (Ticket, error)
	// DetailByProcessInstance 根据流程实例标识（覆盖 key 与历史整型 id）获取工单
	DetailByProcessInstance(ctx context.Context, instanceID string) (Ticket, error)
	// UpdateStatusByProcessInstance 根据流程实例标识更新工单流转状态
	UpdateStatusByProcessInstance(ctx context.Context, instanceID string, status uint8, wtime int64) error
	// Detail 依据物理主键 ID 获取单条工单详情
	Detail(ctx context.Context, id int64) (Ticket, error)
	// RegisterProcessInstanceId 为指定工单登记绑定的引擎实例 ID 并且同步置为流转状态
	RegisterProcessInstanceId(ctx context.Context, id int64, instanceId int, status uint8) error
	// RegisterProcessInstanceKey 为指定工单登记绑定的现代引擎实例全局唯一 Key 并且同步置为流转状态
	RegisterProcessInstanceKey(ctx context.Context, id int64, instanceKey string, status uint8) error
	// RevokeProcessBindingByKey 精确匹配本次预绑定的实例 Key 后解除绑定并将工单复位为待启动态
	RevokeProcessBindingByKey(ctx context.Context, id int64, instanceKey string) error
	// MarkProcessStartFailed 将尚未绑定流程实例的工单标记为流程启动失败并记录原因
	MarkProcessStartFailed(ctx context.Context, id int64, reason string) error
	// PrepareProcessRestart 将流程启动失败工单原子切换回启动中状态
	PrepareProcessRestart(ctx context.Context, id int64) error
	// ListProcessStartFailed 分页查询流程启动中或启动失败的工单
	ListProcessStartFailed(ctx context.Context, userId string, offset, limit int64) ([]Ticket, error)
	// CountProcessStartFailed 统计流程启动中或启动失败的工单数量
	CountProcessStartFailed(ctx context.Context, userId string) (int64, error)
	// ListTicketByProcessInstances 根据流程实例标识集合（自动自适应匹配现代全局唯一 Key 或历史自增 ID）批量高效拉取工单记录
	ListTicketByProcessInstances(ctx context.Context, instanceIDs []string) ([]Ticket, error)
	// ListTicket 分页条件过滤获取与指定用户相关且在状态集合内的物理记录列表
	ListTicket(ctx context.Context, userId string, status []int, offset, limit int64) ([]Ticket, error)
	// CountTicket 统计与指定用户相关且在指定状态集合内的工单物理记录总条数
	CountTicket(ctx context.Context, userId string, status []int) (int64, error)
	// ListHistory 分页查询历史工单并应用上下文中的 PBAC AccessScope，userId 仅作为附加收窄条件。
	ListHistory(ctx context.Context, userId string, status []int, offset, limit int64) ([]Ticket, error)
	// CountHistory 使用与 ListHistory 相同的 PBAC 和查询条件统计历史工单总数。
	CountHistory(ctx context.Context, userId string, status []int) (int64, error)
	// FindByBizIdAndKey 依据场景 ID、单据号和允许的状态列表查找满足条件的唯一活跃工单记录
	FindByBizIdAndKey(ctx context.Context, bizId int64, key string, status []int) (Ticket, error)
	// MergeTicketData 高效利用原子更新将新表单属性合并至已持久化的 JSON 列中
	MergeTicketData(ctx context.Context, id int64, data map[string]interface{}) error
}

type gormTicketDAO struct {
	db *gorm.DB
}

func NewTicketDAO(db *gorm.DB) TicketDAO {
	return &gormTicketDAO{
		db: db,
	}
}

func (g *gormTicketDAO) CreateBizTicket(ctx context.Context, ticket Ticket) (Ticket, error) {
	now := time.Now().UnixMilli()
	ticket.Ctime, ticket.Utime = now, now
	err := g.db.WithContext(ctx).Create(&ticket).Error
	return ticket, err
}

func (g *gormTicketDAO) CreateTicket(ctx context.Context, req Ticket) (int64, error) {
	now := time.Now().UnixMilli()
	req.Ctime, req.Utime = now, now
	err := g.db.WithContext(ctx).Create(&req).Error
	return req.Id, err
}

func (g *gormTicketDAO) DetailByProcessInstId(ctx context.Context, instanceId int) (Ticket, error) {
	var res Ticket
	err := g.db.WithContext(ctx).
		Scopes(gormx.IgnoreTenant()).
		Where("process_instance_id = ?", instanceId).
		First(&res).Error
	return res, err
}

func (g *gormTicketDAO) Detail(ctx context.Context, id int64) (Ticket, error) {
	var res Ticket
	err := g.db.WithContext(ctx).Where("id = ?", id).First(&res).Error
	return res, err
}

func (g *gormTicketDAO) DetailByProcessInstKey(ctx context.Context, instanceKey string) (Ticket, error) {
	var res Ticket
	err := g.db.WithContext(ctx).Where("process_instance_key = ?", instanceKey).First(&res).Error
	return res, err
}

func (g *gormTicketDAO) DetailByProcessInstance(ctx context.Context, instanceID string) (Ticket, error) {
	var res Ticket
	// 优先查询现代引擎全局唯一标识 Key（涵盖纯数字雪花 ID 与字符串 Key）
	err := g.db.WithContext(ctx).Scopes(gormx.IgnoreTenant()).
		Where("process_instance_key = ?", instanceID).
		First(&res).Error
	if err == nil {
		return res, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return res, err
	}
	// 兜底支持历史 EasyFlow 小整型自增 ID
	if intID, parseErr := strconv.Atoi(instanceID); parseErr == nil && intID > 0 {
		return g.DetailByProcessInstId(ctx, intID)
	}
	return res, gorm.ErrRecordNotFound
}

func (g *gormTicketDAO) UpdateStatusByProcessInstance(ctx context.Context, instanceID string, status uint8, wtime int64) error {
	now := time.Now().UnixMilli()
	updates := map[string]interface{}{
		"status": statusGuardExpr(status),
		"utime":  now,
	}
	if wtime > 0 {
		updates["wtime"] = wtime
	}
	res := g.db.WithContext(ctx).Model(&Ticket{}).Scopes(gormx.IgnoreTenant()).
		Where("process_instance_key = ?", instanceID).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		return nil
	}
	if intID, err := strconv.Atoi(instanceID); err == nil && intID > 0 {
		return g.updateStatusByInstanceId(ctx, intID, status)
	}
	return nil
}

// statusGuardExpr 构造引擎事件回写工单状态时的终态守卫表达式
// 撤回是发起人主动表达的更强意图：撤回中与已撤回一律不得被流程办结事件覆盖回 END；
// 而撤回推进自身 (目标态为 WITHDRAW) 必须放行，才能把 WITHDRAWING 正常收敛为 WITHDRAW
func statusGuardExpr(target uint8) clause.Expr {
	if target == domain.WITHDRAW.ToUint8() {
		return gorm.Expr("CASE WHEN status IN (?, ?) THEN status ELSE ? END",
			domain.END.ToUint8(), domain.WITHDRAW.ToUint8(), target)
	}
	return gorm.Expr("CASE WHEN status IN (?, ?, ?) THEN status ELSE ? END",
		domain.END.ToUint8(), domain.WITHDRAW.ToUint8(), domain.WITHDRAWING.ToUint8(), target)
}

func (g *gormTicketDAO) RegisterProcessInstanceId(ctx context.Context, id int64, instanceId int, status uint8) error {
	// 终态保护：若工单在引擎启动过程中已秒级流转至终态 (END 或 WITHDRAW)，严禁回退状态为流转中
	return g.db.WithContext(ctx).Model(&Ticket{}).Where("id = ?", id).Updates(map[string]interface{}{
		"process_instance_id": instanceId,
		"engine_type":         "easy-workflow",
		"status":              gorm.Expr("CASE WHEN status IN (?, ?) THEN status ELSE ? END", domain.END.ToUint8(), domain.WITHDRAW.ToUint8(), status),
		"process_start_error": "",
		"utime":               time.Now().UnixMilli(),
	}).Error
}

func (g *gormTicketDAO) RegisterProcessInstanceKey(ctx context.Context, id int64, instanceKey string, status uint8) error {
	// 终态保护：若工单在引擎启动过程中已秒级流转至终态 (END 或 WITHDRAW)，严禁回退状态为流转中
	return g.db.WithContext(ctx).Model(&Ticket{}).Where("id = ?", id).Updates(map[string]interface{}{
		"process_instance_key": instanceKey,
		"engine_type":          "flowx",
		"status":               gorm.Expr("CASE WHEN status IN (?, ?) THEN status ELSE ? END", domain.END.ToUint8(), domain.WITHDRAW.ToUint8(), status),
		"process_start_error":  "",
		"utime":                time.Now().UnixMilli(),
	}).Error
}

// RevokeProcessBindingByKey 精确匹配本次预绑定的实例 Key 后解除绑定并将工单复位为待启动态。
// WHERE 必须同时锁定主键与实例 Key：只允许回滚调用方自己刚刚写入的绑定，
// 严禁放宽条件，否则可能清空另一条正在流转的流程实例绑定造成工单失联。
// 复位为 START 而非 START_FAILED，是为了把失败态与尝试次数的落账权完整交还给上层统一入口，避免重复自增。
func (g *gormTicketDAO) RevokeProcessBindingByKey(ctx context.Context, id int64, instanceKey string) error {
	return g.db.WithContext(ctx).Model(&Ticket{}).
		Scopes(gormx.IgnoreTenant()).
		Where("id = ? AND process_instance_key = ?", id, instanceKey).
		Updates(map[string]interface{}{
			"process_instance_key": "",
			"status": gorm.Expr("CASE WHEN status IN (?, ?) THEN status ELSE ? END",
				domain.END.ToUint8(), domain.WITHDRAW.ToUint8(), domain.START.ToUint8()),
			"utime": time.Now().UnixMilli(),
		}).Error
}

// MarkProcessStartFailed 将尚未绑定流程实例的工单标记为流程启动失败并记录原因。
func (g *gormTicketDAO) MarkProcessStartFailed(ctx context.Context, id int64, reason string) error {
	if len([]rune(reason)) > 1000 {
		reason = string([]rune(reason)[:1000])
	}
	return g.db.WithContext(ctx).Model(&Ticket{}).
		Scopes(gormx.IgnoreTenant()).
		Where("id = ? AND process_instance_id = 0 AND status IN ?", id,
			[]int{domain.START.ToInt(), domain.START_FAILED.ToInt()}).
		Updates(map[string]interface{}{
			"status":                 domain.START_FAILED.ToUint8(),
			"process_start_error":    reason,
			"process_start_attempts": gorm.Expr("process_start_attempts + 1"),
			"utime":                  time.Now().UnixMilli(),
		}).Error
}

// PrepareProcessRestart 将尚未绑定流程实例的工单原子切换回启动中状态。
func (g *gormTicketDAO) PrepareProcessRestart(ctx context.Context, id int64) error {
	result := g.db.WithContext(ctx).Model(&Ticket{}).
		Where("id = ? AND process_instance_id = 0 AND status IN ?", id,
			[]int{domain.START.ToInt(), domain.START_FAILED.ToInt()}).
		Updates(map[string]interface{}{
			"status":              domain.START.ToUint8(),
			"process_start_error": "",
			"utime":               time.Now().UnixMilli(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("工单当前不是可重新启动状态")
	}
	return nil
}

// ListProcessStartFailed 分页查询流程启动中或启动失败的工单。
func (g *gormTicketDAO) ListProcessStartFailed(ctx context.Context, userId string, offset, limit int64) ([]Ticket, error) {
	var res []Ticket
	query := g.ticketQuery(ctx, userId, []int{domain.START.ToInt(), domain.START_FAILED.ToInt()})
	err := query.Order("ctime desc").Limit(int(limit)).Offset(int(offset)).Find(&res).Error
	return res, err
}

// CountProcessStartFailed 统计流程启动中或启动失败的工单数量。
func (g *gormTicketDAO) CountProcessStartFailed(ctx context.Context, userId string) (int64, error) {
	return g.CountTicket(ctx, userId, []int{domain.START.ToInt(), domain.START_FAILED.ToInt()})
}

func (g *gormTicketDAO) ListTicketByProcessInstances(ctx context.Context, instanceIDs []string) ([]Ticket, error) {
	var res []Ticket
	if len(instanceIDs) == 0 {
		return res, nil
	}
	cleanIDs := lo.Uniq(lo.Filter(instanceIDs, func(id string, _ int) bool {
		return strings.TrimSpace(id) != ""
	}))
	if len(cleanIDs) == 0 {
		return res, nil
	}

	var intIDs []int
	for _, id := range cleanIDs {
		if n, err := strconv.Atoi(id); err == nil && n > 0 {
			intIDs = append(intIDs, n)
		}
	}

	query := g.db.WithContext(ctx).Scopes(gormx.IgnoreTenant())
	if len(intIDs) > 0 {
		query = query.Where("process_instance_key IN ? OR process_instance_id IN ?", cleanIDs, intIDs)
	} else {
		query = query.Where("process_instance_key IN ?", cleanIDs)
	}

	err := query.Find(&res).Error
	return res, err
}

func (g *gormTicketDAO) updateStatusByInstanceId(ctx context.Context, instanceId int, status uint8) error {
	now := time.Now().UnixMilli()
	return g.db.WithContext(ctx).Model(&Ticket{}).
		Scopes(gormx.IgnoreTenant()).
		Where("process_instance_id = ?", instanceId).Updates(map[string]interface{}{
		"status": statusGuardExpr(status),
		"utime":  now,
		"wtime":  now,
	}).Error
}

func (g *gormTicketDAO) ListTicket(ctx context.Context, userId string, status []int, offset, limit int64) ([]Ticket, error) {
	var res []Ticket
	query := g.ticketQuery(ctx, userId, status)
	err := query.Order("ctime desc").Limit(int(limit)).Offset(int(offset)).Find(&res).Error
	return res, err
}

func (g *gormTicketDAO) CountTicket(ctx context.Context, userId string, status []int) (int64, error) {
	var total int64
	query := g.ticketQuery(ctx, userId, status)
	err := query.Count(&total).Error
	return total, err
}

// ListHistory 使用历史工单 profile 应用 PBAC 决策后查询可见工单。
func (g *gormTicketDAO) ListHistory(ctx context.Context, userId string, status []int, offset, limit int64) ([]Ticket, error) {
	var result []Ticket
	query, err := pbacgorm.Apply(ctx, g.ticketQuery(ctx, userId, status), ticketpbac.History)
	if err != nil {
		return nil, err
	}
	err = query.Order("ctime desc").Limit(int(limit)).Offset(int(offset)).Find(&result).Error
	return result, err
}

// CountHistory 使用历史工单 profile 应用 PBAC 决策后统计可见工单数。
func (g *gormTicketDAO) CountHistory(ctx context.Context, userId string, status []int) (int64, error) {
	var total int64
	query, err := pbacgorm.Apply(ctx, g.ticketQuery(ctx, userId, status), ticketpbac.History)
	if err != nil {
		return 0, err
	}
	err = query.Count(&total).Error
	return total, err
}

func (g *gormTicketDAO) ticketQuery(ctx context.Context, userId string, status []int) *gorm.DB {
	query := g.db.WithContext(ctx).Model(&Ticket{})
	if userId != "" {
		query = query.Where("create_by = ?", userId)
	}
	if len(status) > 0 {
		query = query.Where("status IN ?", status)
	}
	return query
}

func (g *gormTicketDAO) FindByBizIdAndKey(ctx context.Context, bizId int64, key string, status []int) (Ticket, error) {
	var res Ticket
	query := g.db.WithContext(ctx).Where("biz_id = ? AND `key` = ?", bizId, key)
	if len(status) > 0 {
		query = query.Where("status IN ?", status)
	}
	err := query.Order("ctime desc").First(&res).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Ticket{}, nil
	}
	return res, err
}

func (g *gormTicketDAO) MergeTicketData(ctx context.Context, id int64, data map[string]interface{}) error {
	return g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var t Ticket
		// 使用行级排他锁锁定该记录，防止并发会签或多节点审批产生覆盖写丢失
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).First(&t).Error; err != nil {
			return err
		}
		if t.Data.Val == nil {
			t.Data.Val = make(map[string]interface{})
		}
		for k, v := range data {
			t.Data.Val[k] = v
		}
		return tx.Model(&Ticket{}).Where("id = ?", id).Updates(map[string]interface{}{
			"data":  t.Data,
			"utime": time.Now().UnixMilli(),
		}).Error
	})
}
