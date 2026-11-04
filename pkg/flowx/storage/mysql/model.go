package mysql

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/Duke1616/eflow/pkg/flowx/model"
)

// FlowDefinitionPO 流程定义持久化模型
type FlowDefinitionPO struct {
	ID         int64  `gorm:"primaryKey;autoIncrement;column:id"`
	ProcKey    string `gorm:"column:proc_key;type:varchar(64);not null;uniqueIndex:uk_key_version"`
	Name       string `gorm:"column:name;type:varchar(128);not null"`
	Version    int    `gorm:"column:version;not null;uniqueIndex:uk_key_version"`
	DSLContent string `gorm:"column:dsl_content;type:mediumtext;not null"`
	Ctime      int64  `gorm:"column:ctime;not null"`
}

func (FlowDefinitionPO) TableName() string {
	return "flow_definition"
}

// FlowInstancePO 流程运行实例持久化模型
type FlowInstancePO struct {
	ID          string        `gorm:"primaryKey;column:id;type:varchar(64)"`
	ProcKey     string        `gorm:"column:proc_key;type:varchar(64);not null;index:idx_key"`
	Version     int           `gorm:"column:version;not null"`
	Starter     string        `gorm:"column:starter;type:varchar(64);not null;index:idx_starter"`
	Status      uint8         `gorm:"column:status;not null;index:idx_status"`
	CurrentNode string        `gorm:"column:current_node;type:varchar(64);not null"`
	Variables   string        `gorm:"column:variables;type:json;not null"`
	Ctime       int64         `gorm:"column:ctime;not null"`
	Ftime       sql.NullInt64 `gorm:"column:ftime"`
}

func (FlowInstancePO) TableName() string {
	return "flow_instance"
}

// FlowTaskPO 流转任务持久化模型
type FlowTaskPO struct {
	ID         string        `gorm:"primaryKey;column:id;type:varchar(64)"`
	InstanceID string        `gorm:"column:instance_id;type:varchar(64);not null;index:idx_inst_batch,priority:1"`
	NodeID     string        `gorm:"column:node_id;type:varchar(64);not null"`
	NodeName   string        `gorm:"column:node_name;type:varchar(128);not null"`
	BatchCode  int           `gorm:"column:batch_code;not null;index:idx_inst_batch,priority:2"`
	Assignee   string        `gorm:"column:assignee;type:varchar(64);not null;index:idx_assignee_status,priority:1"`
	SignRule   string        `gorm:"column:sign_rule;type:varchar(32);not null"`
	Status     uint8         `gorm:"column:status;not null;index:idx_assignee_status,priority:2"`
	Comment    string        `gorm:"column:comment;type:text"`
	Ctime      int64         `gorm:"column:ctime;not null"`
	Ftime      sql.NullInt64 `gorm:"column:ftime"`
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
	if inst.FinishTime != nil {
		po.Ftime = sql.NullInt64{Int64: inst.FinishTime.UnixMilli(), Valid: true}
	}
	return po, nil
}

func toInstanceDO(po FlowInstancePO) (*model.Instance, error) {
	var vars map[string]any
	if po.Variables != "" {
		_ = json.Unmarshal([]byte(po.Variables), &vars)
	}
	if vars == nil {
		vars = make(map[string]any)
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
		inst.FinishTime = new(time.UnixMilli(po.Ftime.Int64))
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
	if t.FinishTime != nil {
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
		t.FinishTime = new(time.UnixMilli(po.Ftime.Int64))
	}
	return t
}
