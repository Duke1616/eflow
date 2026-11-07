package mysql

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/Duke1616/eflow/pkg/flowx/model"
)

// FlowDefinitionPO 流程定义持久化模型
type FlowDefinitionPO struct {
	ID         int64  `gorm:"primaryKey;autoIncrement;column:id;comment:'流程定义自增ID'"`
	ProcKey    string `gorm:"column:proc_key;type:varchar(64);not null;uniqueIndex:uk_key_version;comment:'流程唯一标识Key'"`
	Name       string `gorm:"column:name;type:varchar(128);not null;comment:'流程定义展示名称'"`
	Version    int    `gorm:"column:version;not null;uniqueIndex:uk_key_version;comment:'流程定义版本号'"`
	DSLContent string `gorm:"column:dsl_content;type:mediumtext;not null;comment:'流程DSL完整定义(JSON)'"`
	Ctime      int64  `gorm:"column:ctime;not null;comment:'创建时间(毫秒)'"`
}

func (FlowDefinitionPO) TableName() string {
	return "flow_definition"
}

// FlowInstancePO 流程运行实例持久化模型
type FlowInstancePO struct {
	ID          string        `gorm:"primaryKey;column:id;type:varchar(64);comment:'流程运行实例全局唯一Key'"`
	ProcKey     string        `gorm:"column:proc_key;type:varchar(64);not null;index:idx_key;comment:'流程唯一标识Key'"`
	Version     int           `gorm:"column:version;not null;comment:'执行版本号'"`
	Starter     string        `gorm:"column:starter;type:varchar(64);not null;index:idx_starter;comment:'流程发起人用户名'"`
	Status      uint8         `gorm:"column:status;not null;index:idx_status;comment:'实例状态(0:Running, 1:Completed, 2:Rejected, 3:Revoked)'"`
	CurrentNode string        `gorm:"column:current_node;type:varchar(64);not null;comment:'当前流转节点ID'"`
	Variables   string        `gorm:"column:variables;type:json;not null;comment:'流程全局运行变量集(JSON)'"`
	Ctime       int64         `gorm:"column:ctime;not null;comment:'发起创建时间(毫秒)'"`
	Ftime       sql.NullInt64 `gorm:"column:ftime;comment:'办结/终态时间(毫秒)'"`
}

func (FlowInstancePO) TableName() string {
	return "flow_instance"
}

// FlowTaskPO 流转任务持久化模型
type FlowTaskPO struct {
	ID         string        `gorm:"primaryKey;column:id;type:varchar(64);comment:'流转任务全局唯一Key'"`
	InstanceID string        `gorm:"column:instance_id;type:varchar(64);not null;index:idx_inst_batch,priority:1;comment:'关联流程运行实例Key'"`
	NodeID     string        `gorm:"column:node_id;type:varchar(64);not null;comment:'流程图节点ID'"`
	NodeName   string        `gorm:"column:node_name;type:varchar(128);not null;comment:'流程图节点展示名称'"`
	BatchCode  int           `gorm:"column:batch_code;not null;index:idx_inst_batch,priority:2;comment:'节点执行批次序号(用于驳回重审区分)'"`
	Assignee   string        `gorm:"column:assignee;type:varchar(64);not null;index:idx_assignee_status,priority:1;comment:'任务指派审批人用户名'"`
	SignRule   string        `gorm:"column:sign_rule;type:varchar(32);not null;comment:'会签/或签审批规则(or/and)'"`
	Status     uint8         `gorm:"column:status;not null;index:idx_assignee_status,priority:2;comment:'任务状态(0:Pending, 1:Passed, 2:Rejected, 3:Skipped)'"`
	Comment    string        `gorm:"column:comment;type:text;comment:'审批意见/流转批注'"`
	Ctime      int64         `gorm:"column:ctime;not null;comment:'任务生成时间(毫秒)'"`
	Ftime      sql.NullInt64 `gorm:"column:ftime;comment:'任务完成时间(毫秒)'"`
}

func (FlowTaskPO) TableName() string {
	return "flow_task"
}

// 转换辅助函数: DO <-> PO

func toDefinitionPO(def model.ProcessDefinition) (FlowDefinitionPO, error) {
	bytes, err := json.Marshal(def)
	if err != nil {
		return FlowDefinitionPO{}, err
	}
	return FlowDefinitionPO{
		ProcKey:    def.Key,
		Name:       def.Name,
		Version:    def.Version,
		DSLContent: string(bytes),
		Ctime:      time.Now().UnixMilli(),
	}, nil
}

func toDefinitionDO(po FlowDefinitionPO) (model.ProcessDefinition, error) {
	var def model.ProcessDefinition
	if err := json.Unmarshal([]byte(po.DSLContent), &def); err != nil {
		return model.ProcessDefinition{}, err
	}
	return def, nil
}

func toInstancePO(inst *model.Instance) (FlowInstancePO, error) {
	var varsJSON string
	if inst.Variables != nil {
		b, err := json.Marshal(inst.Variables)
		if err != nil {
			return FlowInstancePO{}, err
		}
		varsJSON = string(b)
	} else {
		varsJSON = "{}"
	}

	po := FlowInstancePO{
		ID:          inst.ID,
		ProcKey:     inst.ProcKey,
		Version:     inst.Version,
		Starter:     inst.Starter,
		Status:      uint8(inst.Status),
		CurrentNode: inst.CurrentNode,
		Variables:   varsJSON,
		Ctime:       inst.CreateTime.UnixMilli(),
	}
	if !inst.FinishTime.IsZero() {
		po.Ftime = sql.NullInt64{Int64: inst.FinishTime.UnixMilli(), Valid: true}
	}
	return po, nil
}

func toInstanceDO(po FlowInstancePO) (*model.Instance, error) {
	var vars model.Variables
	if po.Variables != "" {
		_ = json.Unmarshal([]byte(po.Variables), &vars)
	}
	if vars == nil {
		vars = make(model.Variables)
	}

	inst := &model.Instance{
		ID:          po.ID,
		ProcKey:     po.ProcKey,
		Version:     po.Version,
		Starter:     po.Starter,
		Status:      model.InstanceStatus(po.Status),
		CurrentNode: po.CurrentNode,
		Variables:   vars,
		CreateTime:  time.UnixMilli(po.Ctime),
	}
	if po.Ftime.Valid {
		inst.FinishTime = time.UnixMilli(po.Ftime.Int64)
	}
	return inst, nil
}

func toTaskPO(t *model.Task) FlowTaskPO {
	po := FlowTaskPO{
		ID:         t.ID,
		InstanceID: t.InstanceID,
		NodeID:     t.NodeID,
		NodeName:   t.NodeName,
		BatchCode:  t.BatchCode,
		Assignee:   t.Assignee,
		SignRule:   string(t.SignRule),
		Status:     uint8(t.Status),
		Comment:    t.Comment,
		Ctime:      t.CreateTime.UnixMilli(),
	}
	if !t.FinishTime.IsZero() {
		po.Ftime = sql.NullInt64{Int64: t.FinishTime.UnixMilli(), Valid: true}
	}
	return po
}

func toTaskDO(po FlowTaskPO) *model.Task {
	t := &model.Task{
		ID:         po.ID,
		InstanceID: po.InstanceID,
		NodeID:     po.NodeID,
		NodeName:   po.NodeName,
		BatchCode:  po.BatchCode,
		Assignee:   po.Assignee,
		SignRule:   model.SignRule(po.SignRule),
		Status:     model.TaskStatus(po.Status),
		Comment:    po.Comment,
		CreateTime: time.UnixMilli(po.Ctime),
	}
	if po.Ftime.Valid {
		t.FinishTime = time.UnixMilli(po.Ftime.Int64)
	}
	return t
}
