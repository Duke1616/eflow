package dao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Bunny3th/easy-workflow/workflow/database"
	"github.com/Bunny3th/easy-workflow/workflow/model"
	"github.com/ecodeclub/ekit/slice"
	"gorm.io/gorm"
)

// IEngineDAO 流程引擎物理数据访问接口
type IEngineDAO interface {
	// CountTodo 统计用户的待办任务总数
	CountTodo(ctx context.Context, userId, processName string) (int64, error)
	// CountStartUser 统计用户发起的流程实例总数
	CountStartUser(ctx context.Context, userId, processName string) (int64, error)
	// ListHistory 获取审批历史记录列表
	ListHistory(ctx context.Context, userId, processName string, offset, limit int)
	// ListStartUser 获取用户发起的流程实例列表
	ListStartUser(ctx context.Context, userId, processName string, offset, limit int) ([]Instance, error)
	// GetTasksByCurrentNodeId 根据当前节点 ID 获取未完成的审批任务
	GetTasksByCurrentNodeId(ctx context.Context, processInstId int, currentNodeId string) ([]model.Task, error)
	// ListTaskRecord 获取指定流程实例的全部流转任务记录（联合查询 proc_task 与 hist_proc_task）
	ListTaskRecord(ctx context.Context, processInstId, offset, limit int) ([]model.Task, error)
	// CountTaskRecord 统计指定流程实例的任务记录总数
	CountTaskRecord(ctx context.Context, processInstId int) (int64, error)
	// SearchStartByProcessInstIds 批量检索指定的流程实例列表
	SearchStartByProcessInstIds(ctx context.Context, processInstIds []int) ([]Instance, error)
	// UpdateIsFinishedByPreNodeId 自动更新当前节点的前置代理流转任务为完成状态
	UpdateIsFinishedByPreNodeId(ctx context.Context, processInstId int, nodeId string, status int, comment string) error
	// ForceUpdateIsFinishedByPreNodeId 强制清理指定前置节点下的所有任务（包含已完成的任务）
	ForceUpdateIsFinishedByPreNodeId(ctx context.Context, processInstId int, nodeId string, status int, comment string) error
	// ForceUpdateIsFinishedByNodeId 强制清理指定节点 ID 的所有任务（包含已完成的任务）
	ForceUpdateIsFinishedByNodeId(ctx context.Context, processInstId int, nodeId string, status int, comment string) error
	// CountReject 统计指定任务 ID 的驳回记录数
	CountReject(ctx context.Context, taskId int) (int64, error)
	// ListTasksByProcInstId 根据流程实例 ID 列表和发起人获取仍在流转中的任务
	ListTasksByProcInstId(ctx context.Context, processInstIds []int, starter string) ([]model.Task, error)
	// GetAutomationTask 获取未完成的自动化节点任务
	GetAutomationTask(ctx context.Context, currentNodeId string, processInstId int) (model.Task, error)
	// GetTasksByInstUsers 查询指定流程实例中待指定用户处理的未完成任务
	GetTasksByInstUsers(ctx context.Context, processInstId int, userIds []string) ([]model.Task, error)
	// GetTicketIdByVariable 从流程实例的全局变量中检索关联的工单 ID
	GetTicketIdByVariable(ctx context.Context, processInstId int) (string, error)
	// GetProxyNodeID 根据前置节点获取代理流转任务节点信息
	GetProxyNodeID(ctx context.Context, processInstId int, prevNodeID string) (model.Task, error)
	// GetProxyNodeByProcessInstId 通过流程实例 ID 获取自动流转的代理任务
	GetProxyNodeByProcessInstId(ctx context.Context, processInstId int) (model.Task, error)
	// DeleteProxyNodeByNodeId 删除特定的代理流转任务记录
	DeleteProxyNodeByNodeId(ctx context.Context, processInstId int, nodeId string) error
	// UpdateTaskPrevNodeID 更新指定任务的前置节点 ID
	UpdateTaskPrevNodeID(ctx context.Context, taskId int, prevNodeId string) error
	// CreateSkippedTask 写入被跳过节点的已完结任务记录
	CreateSkippedTask(ctx context.Context, task model.Task) error
	// GetInstanceByID 根据实例 ID 获取流程实例详情（含历史流程实例）
	GetInstanceByID(ctx context.Context, processInstId int) (Instance, error)
	// GetProcessDefineByVersion 获取指定流程定义及对应版本的具体配置（包含历史版本）
	GetProcessDefineByVersion(ctx context.Context, processID, version int) (model.Process, error)
	// GetLatestProcessVersion 获取指定流程的最新发布版本号
	GetLatestProcessVersion(ctx context.Context, processID int) (int, error)
	// Transfer 将指定任务转签/转交予其他用户进行审批
	Transfer(ctx context.Context, taskId int, userIds []string) ([]model.Task, error)
}

type gormEngineDAO struct {
	db *gorm.DB
}

// NewProcessEngineDAO 初始化流程引擎 GORM DAO
func NewProcessEngineDAO(db *gorm.DB) IEngineDAO {
	return &gormEngineDAO{
		db: db,
	}
}

// Instance 流程实例物理映射模型
type Instance struct {
	TaskID          int        `gorm:"column:task_id;"`          // 任务ID
	ProcInstID      int        `gorm:"column:id;"`               // 流程实例ID
	ProcID          int        `gorm:"column:proc_id"`           // 流程ID
	ProcName        string     `gorm:"column:name"`              // 流程名称
	ProcVersion     int        `gorm:"column:proc_version"`      // 流程版本号
	BusinessID      string     `gorm:"column:business_id"`       // 业务ID
	Starter         string     `gorm:"column:starter"`           // 流程发起人用户ID
	CurrentNodeID   string     `gorm:"column:current_node_id"`   // 当前进行节点ID
	CurrentNodeName string     `gorm:"column:current_node_name"` // 当前进行节点名称
	CreateTime      *time.Time `gorm:"column:create_time"`       // 创建时间
	ApprovedBy      string     `gorm:"column:user_id"`           // 当前处理人
	Status          int        `gorm:"column:status"`            // 0:未完成(审批中) 1:已完成(通过) 2:撤销
}

func (g *gormEngineDAO) UpdateTaskPrevNodeID(ctx context.Context, taskId int, prevNodeId string) error {
	return g.db.WithContext(ctx).Table("proc_task").Where("id = ?", taskId).Update("prev_node_id", prevNodeId).Error
}

func (g *gormEngineDAO) Transfer(ctx context.Context, taskId int, userIds []string) ([]model.Task, error) {
	var newTasks []model.Task
	err := g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 获取旧任务
		var oldTask database.ProcTask
		if err := tx.Table("proc_task").Where("id = ?", taskId).First(&oldTask).Error; err != nil {
			return fmt.Errorf("查询原任务失败: %w", err)
		}

		// 2. 校验状态
		if oldTask.IsFinished == 1 {
			return errors.New("任务已完成，无法转交")
		}

		// 3. 删除旧任务
		if err := tx.Table("proc_task").Delete(&database.ProcTask{}, taskId).Error; err != nil {
			return fmt.Errorf("删除原任务失败: %w", err)
		}

		// 4. 创建新任务
		dbTasks := slice.Map(userIds, func(idx int, uid string) database.ProcTask {
			newTask := oldTask                          // 拷贝全部字段
			newTask.ID = 0                              // 清空 ID 以便自动生成
			newTask.UserID = uid                        // 重新分配用户
			newTask.Status = 0                          // 重置状态
			newTask.IsFinished = 0                      // 重置完成标记
			newTask.CreateTime = database.LTime.Now()   // 设置新任务创建时间
			newTask.FinishedTime = database.LocalTime{} // 重置处理时间
			newTask.Comment = ""                        // 清空意见
			return newTask
		})

		if err := tx.Table("proc_task").Create(&dbTasks).Error; err != nil {
			return fmt.Errorf("批量创建新任务失败: %w", err)
		}

		// 5. 转换为返回模型
		newTasks = slice.Map(dbTasks, func(idx int, dt database.ProcTask) model.Task {
			return model.Task{
				TaskID:             dt.ID,
				ProcID:             dt.ProcID,
				ProcInstID:         dt.ProcInstID,
				BusinessID:         dt.BusinessID,
				Starter:            dt.Starter,
				NodeID:             dt.NodeID,
				NodeName:           dt.NodeName,
				PrevNodeID:         dt.PrevNodeID,
				IsCosigned:         dt.IsCosigned,
				BatchCode:          dt.BatchCode,
				UserID:             dt.UserID,
				Status:             dt.Status,
				IsFinished:         dt.IsFinished,
				Comment:            dt.Comment,
				ProcInstCreateTime: &dt.ProcInstCreateTime,
				CreateTime:         &dt.CreateTime,
				FinishedTime:       &dt.FinishedTime,
			}
		})

		return nil
	})

	return newTasks, err
}

func (g *gormEngineDAO) CreateSkippedTask(ctx context.Context, task model.Task) error {
	return g.db.WithContext(ctx).Table("proc_task").Omit("name").Create(&task).Error
}

func (g *gormEngineDAO) GetProxyNodeID(ctx context.Context, processInstId int, prevNodeID string) (model.Task, error) {
	var node model.Task
	err := g.db.WithContext(ctx).Table("proc_task").First(&node,
		"proc_inst_id = ? AND prev_node_id = ? AND user_id = ?", processInstId, prevNodeID, "sys_auto").Error
	return node, err
}

func (g *gormEngineDAO) GetProxyNodeByProcessInstId(ctx context.Context, processInstId int) (model.Task, error) {
	var node model.Task
	err := g.db.WithContext(ctx).Table("proc_task").First(&node,
		"proc_inst_id = ? AND user_id = ?", processInstId, "sys_auto").Error
	return node, err
}

func (g *gormEngineDAO) DeleteProxyNodeByNodeId(ctx context.Context, processInstId int, nodeId string) error {
	return g.db.WithContext(ctx).Table("proc_task").
		Where("proc_inst_id = ? AND node_id = ? AND user_id = ?", processInstId, nodeId, "sys_auto").
		Delete(&model.Task{}).Error
}

func (g *gormEngineDAO) GetTasksByCurrentNodeId(ctx context.Context, processInstId int, currentNodeId string) ([]model.Task, error) {
	var res []model.Task
	err := g.db.WithContext(ctx).Model(&model.Task{}).Table("proc_task").
		Where("proc_inst_id = ? AND status = ? AND is_finished = ? AND node_id = ?",
			processInstId, 0, 0, currentNodeId).
		Find(&res).Error

	return res, err
}

func (g *gormEngineDAO) GetTicketIdByVariable(ctx context.Context, processInstId int) (string, error) {
	var res database.ProcInstVariable
	// 1. 优先检索规范化重构后的新键 ticket_id
	err := g.db.WithContext(ctx).Model(&database.ProcInstVariable{}).Table("proc_inst_variable").
		Where("proc_inst_id = ? AND `key` = ?", processInstId, "ticket_id").
		First(&res).Error
	if err == nil {
		return res.Value, nil
	}

	// 2. 如果不存在，退避匹配老数据中的历史键 order_id，确保平滑升级 100% 成功
	err = g.db.WithContext(ctx).Model(&database.ProcInstVariable{}).Table("proc_inst_variable").
		Where("proc_inst_id = ? AND `key` = ?", processInstId, "order_id").
		First(&res).Error

	return res.Value, err
}

func (g *gormEngineDAO) GetTasksByInstUsers(ctx context.Context, processInstId int, userIds []string) ([]model.Task, error) {
	var res []model.Task
	err := g.db.WithContext(ctx).Model(&model.Task{}).Table("proc_task").
		Where("proc_inst_id = ? AND status = ? AND is_finished = ? AND user_id IN ?",
			processInstId, 0, 0, userIds).
		Find(&res).Error

	return res, err
}

func (g *gormEngineDAO) GetAutomationTask(ctx context.Context, currentNodeId string, processInstId int) (model.Task, error) {
	var res model.Task
	err := g.db.WithContext(ctx).Model(&model.Task{}).Table("proc_task").
		Where("node_id = ? AND proc_inst_id = ? AND is_finished = ? AND status = ?",
			currentNodeId, processInstId, 0, 0).
		First(&res).Error
	return res, err
}

func (g *gormEngineDAO) ListTasksByProcInstId(ctx context.Context, processInstIds []int, starter string) ([]model.Task, error) {
	var res []model.Task
	err := g.db.WithContext(ctx).Model(&model.Task{}).Table("proc_task").
		Where("starter = ? AND is_finished = 0 AND proc_inst_id IN ?", starter, processInstIds).
		Find(&res).Error

	return res, err
}

func (g *gormEngineDAO) CountReject(ctx context.Context, taskId int) (int64, error) {
	var res int64
	err := g.db.WithContext(ctx).Model(&database.ProcTask{}).
		Where("id = ? AND status = ?", taskId, 2).
		Select("COUNT(id)").Count(&res).Error
	return res, err
}

func (g *gormEngineDAO) UpdateIsFinishedByPreNodeId(ctx context.Context, processInstId int, nodeId string, status int, comment string) error {
	proTask := database.ProcTask{Status: status, IsFinished: 1, Comment: comment,
		FinishedTime: database.LTime.Now()}

	return g.db.WithContext(ctx).
		Where("proc_inst_id = ? AND prev_node_id = ? AND is_finished = ? AND status = ?", processInstId, nodeId, 0, 0).
		Updates(proTask).Error
}

func (g *gormEngineDAO) ForceUpdateIsFinishedByPreNodeId(ctx context.Context, processInstId int, nodeId string, status int, comment string) error {
	proTask := database.ProcTask{
		Status:       status,
		IsFinished:   1,
		Comment:      comment,
		FinishedTime: database.LTime.Now(),
	}

	return g.db.WithContext(ctx).
		Where("proc_inst_id = ? AND prev_node_id = ?", processInstId, nodeId).
		Updates(proTask).Error
}

func (g *gormEngineDAO) ForceUpdateIsFinishedByNodeId(ctx context.Context, processInstId int, nodeId string, status int, comment string) error {
	proTask := database.ProcTask{
		Status:       status,
		IsFinished:   1,
		Comment:      comment,
		FinishedTime: database.LTime.Now(),
	}

	return g.db.WithContext(ctx).
		Where("proc_inst_id = ? AND node_id = ?", processInstId, nodeId).
		Updates(proTask).Error
}

func (g *gormEngineDAO) ListTaskRecord(ctx context.Context, processInstId, offset, limit int) ([]model.Task, error) {
	var res []model.Task
	procInstDb := g.db.WithContext(ctx).Model(&model.Task{}).Table("proc_task").
		Select("id, proc_id, proc_inst_id, business_id,starter,node_id,node_name,"+
			"prev_node_id,is_cosigned,batch_code,user_id,status,is_finished,comment,proc_inst_create_time,"+
			"create_time,finished_time").
		Where("proc_inst_id = ?", processInstId)
	procHistInstDb := g.db.WithContext(ctx).Model(&model.Task{}).Table("hist_proc_task").
		Select("task_id,proc_id, proc_inst_id,business_id,starter,node_id,node_name,"+
			"prev_node_id,is_cosigned,batch_code,user_id,status,is_finished,comment,proc_inst_create_time,"+
			"create_time,finished_time").
		Where("proc_inst_id = ?", processInstId)

	query := g.db.Raw("? UNION ALL ?", procInstDb, procHistInstDb)
	db := g.db.Table("(?) as a", query).Select("a.id,a.proc_id,b.name,a.proc_inst_id," +
		"a.business_id,a.starter,a.node_id,a.node_name,a.prev_node_id,a.is_cosigned,a.batch_code,a.user_id,a.status," +
		"a.is_finished,a.comment,a.proc_inst_create_time,a.create_time,a.finished_time").
		Joins("JOIN proc_def b ON a.proc_id = b.id").
		Offset(offset).
		Limit(limit)

	err := db.Scan(&res).Error
	return res, err
}

func (g *gormEngineDAO) CountTaskRecord(ctx context.Context, processInstId int) (int64, error) {
	var res int64
	procInstDb := g.db.WithContext(ctx).Model(&model.Task{}).Table("proc_task").
		Select("id, proc_id, proc_inst_id, business_id,starter,node_id,node_name,"+
			"prev_node_id,is_cosigned,batch_code,user_id,status,is_finished,comment,proc_inst_create_time,"+
			"create_time,finished_time").
		Where("proc_inst_id = ?", processInstId)
	procHistInstDb := g.db.WithContext(ctx).Model(&model.Task{}).Table("hist_proc_task").
		Select("task_id,proc_id, proc_inst_id,business_id,starter,node_id,node_name,"+
			"prev_node_id,is_cosigned,batch_code,user_id,status,is_finished,comment,proc_inst_create_time,"+
			"create_time,finished_time").
		Where("proc_inst_id = ?", processInstId)

	query := g.db.Raw("? UNION ALL ?", procInstDb, procHistInstDb)
	db := g.db.Table("(?) as a", query).Select("a.id,a.proc_id,b.name,a.proc_inst_id," +
		"a.business_id,a.starter,a.node_id,a.node_name,a.prev_node_id,a.is_cosigned,a.batch_code,a.user_id,a.status," +
		"a.is_finished,a.comment,a.proc_inst_create_time,a.create_time,a.finished_time").
		Joins("JOIN proc_def b ON a.proc_id = b.id")

	err := db.Count(&res).Error
	return res, err
}

func (g *gormEngineDAO) CountTodo(ctx context.Context, userId, processName string) (int64, error) {
	var res int64
	db := g.db.WithContext(ctx).Model(&model.Task{}).Table("proc_task")
	if userId != "" {
		db = db.Where("user_id = ?", userId)
	}
	if processName != "" {
		db = db.Where("process_name = ?", processName)
	}

	db = db.Where("is_finished = ?", 0)
	err := db.Count(&res).Error
	return res, err
}

func (g *gormEngineDAO) SearchStartByProcessInstIds(ctx context.Context, processInstIds []int) ([]Instance, error) {
	//TODO implement me
	panic("implement me")
}

func (g *gormEngineDAO) ListHistory(ctx context.Context, userId, processName string, offset, limit int) {
	//TODO implement me
	panic("implement me")
}

func (g *gormEngineDAO) ListStartUser(ctx context.Context, userId, processName string, offset, limit int) ([]Instance, error) {
	var res []Instance
	db := g.db.WithContext(ctx).Table("proc_inst as a").Select("a.id, a.proc_id, a.proc_version, " +
		"a.business_id, a.starter, a.current_node_id, a.create_time, " +
		"a.status, b.name, c.id as task_id, c.user_id, c.node_name as current_node_name").
		Joins("JOIN proc_def b ON a.proc_id = b.id").
		Joins("JOIN proc_task c ON a.id = c.proc_inst_id AND a.current_node_id = c.node_id").
		Order("a.id").
		Limit(limit).
		Offset(offset)

	if userId != "" {
		db = db.Where("c.starter = ?", userId)
	}
	if processName != "" {
		db = db.Where("name = ?", processName)
	}

	err := db.Scan(&res).Error

	return res, err
}

func (g *gormEngineDAO) CountStartUser(ctx context.Context, userId, processName string) (int64, error) {
	var res int64
	db := g.db.WithContext(ctx).Model(&model.Instance{}).Table("proc_inst as a").
		Joins("JOIN proc_def b ON a.proc_id = b.id").
		Joins("JOIN proc_task c ON a.id = c.proc_inst_id AND a.current_node_id = c.node_id " +
			"AND a.starter = c.starter").
		Order("a.id")

	if userId != "" {
		db = db.Where("c.starter = ?", userId)
	}
	if processName != "" {
		db = db.Where("name = ?", processName)
	}

	err := db.Count(&res).Error
	return res, err
}

func (g *gormEngineDAO) GetInstanceByID(ctx context.Context, processInstId int) (Instance, error) {
	var res Instance
	err := g.db.WithContext(ctx).Table("proc_inst").Where("id = ?", processInstId).First(&res).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = g.db.WithContext(ctx).Table("hist_proc_inst").Where("proc_inst_id = ?", processInstId).First(&res).Error
	}

	return res, err
}

func (g *gormEngineDAO) GetProcessDefineByVersion(ctx context.Context, processID, version int) (model.Process, error) {
	var resource string

	subQuery1 := g.db.Table("proc_def").Select("resource, version").Where("id = ?", processID)
	subQuery2 := g.db.Table("hist_proc_def").Select("resource, version").Where("proc_id = ?", processID)

	err := g.db.Table("(?) as t", g.db.Raw("? UNION ALL ?", subQuery1, subQuery2)).
		Select("resource").
		Where("version = ?", version).
		Limit(1).
		Scan(&resource).Error

	if err != nil {
		return model.Process{}, err
	}

	if resource == "" {
		return model.Process{}, fmt.Errorf("definition for process_id=%d version=%d not found", processID, version)
	}

	var process model.Process
	if err = json.Unmarshal([]byte(resource), &process); err != nil {
		return model.Process{}, err
	}
	return process, nil
}

func (g *gormEngineDAO) GetLatestProcessVersion(ctx context.Context, processID int) (int, error) {
	var version int
	err := g.db.WithContext(ctx).Table("proc_def").
		Select("version").
		Where("id = ?", processID).
		Scan(&version).Error
	return version, err
}
