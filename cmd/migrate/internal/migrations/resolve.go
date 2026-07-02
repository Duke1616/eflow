package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository/dao"
	"github.com/Duke1616/eiam/pkg/migration"
	"go.mongodb.org/mongo-driver/bson"
	"gorm.io/gorm"
)

// loadCodebookLookup 加载 codebook 标识 -> ID 的映射
func loadCodebookLookup(ctx context.Context, env migration.MigrationEnv) (map[string]int64, error) {
	cursor, err := env.MongoDB.Collection("c_codebook").Find(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("查询源 c_codebook 失败: %w", err)
	}
	defer cursor.Close(ctx)

	lookup := make(map[string]int64)
	for cursor.Next(ctx) {
		var src struct {
			ID         int64  `bson:"id"`
			Identifier string `bson:"identifier"`
		}
		if err := cursor.Decode(&src); err != nil {
			return nil, fmt.Errorf("解码源 c_codebook 失败: %w", err)
		}

		if key := strings.TrimSpace(src.Identifier); key != "" {
			lookup[key] = src.ID
		}
	}

	return lookup, cursor.Err()
}

// SyncProcessInstanceAutoIncrement 校准 easyflow 流程实例表的自增起点。
//
// proc_inst 只保存运行中的实例，结束后的实例会迁移到 hist_proc_inst 并从主表删除。
// 因此 proc_inst 的 MAX(id)+1 不能代表下一个安全的流程实例 ID，必须综合所有引用过
// proc_inst_id 的目标表；源库 information_schema 可读时，再优先参考源库真实自增值。
func SyncProcessInstanceAutoIncrement(ctx context.Context, env migration.MigrationEnv) error {
	if env.DryRun {
		log.Printf("[dry-run] 跳过 proc_inst 自增值校准")
		return nil
	}

	nextID, err := nextProcessInstanceIDFromTarget(ctx, env)
	if err != nil {
		return err
	}

	if srcNextID := readAutoIncrement(ctx, env.MySQLSrc, "源库", "proc_inst"); srcNextID > nextID {
		nextID = srcNextID
	}
	if dstNextID := readAutoIncrement(ctx, env.MySQLDst, "目标库", "proc_inst"); dstNextID > nextID {
		nextID = dstNextID
	}

	if nextID <= 1 {
		return nil
	}

	if err = env.MySQLDst.WithContext(ctx).
		Exec(fmt.Sprintf("ALTER TABLE `proc_inst` AUTO_INCREMENT = %d", nextID)).
		Error; err != nil {
		return fmt.Errorf("校准 proc_inst 自增值失败: %w", err)
	}

	log.Printf("proc_inst 自增值已校准为: %d", nextID)
	return nil
}

func nextProcessInstanceIDFromTarget(ctx context.Context, env migration.MigrationEnv) (int64, error) {
	var nextID int64
	err := env.MySQLDst.WithContext(ctx).Raw(`
SELECT GREATEST(
    COALESCE((SELECT MAX(id) FROM proc_inst), 0),
    COALESCE((SELECT MAX(proc_inst_id) FROM hist_proc_inst), 0),
    COALESCE((SELECT MAX(proc_inst_id) FROM proc_task), 0),
    COALESCE((SELECT MAX(proc_inst_id) FROM hist_proc_task), 0),
    COALESCE((SELECT MAX(proc_inst_id) FROM proc_inst_variable), 0),
    COALESCE((SELECT MAX(proc_inst_id) FROM hist_proc_inst_variable), 0),
    COALESCE((SELECT MAX(process_instance_id) FROM ticket), 0),
    COALESCE((SELECT MAX(process_inst_id) FROM task), 0)
) + 1
`).Scan(&nextID).Error
	if err != nil {
		return 0, fmt.Errorf("计算目标库 proc_inst 下一自增值失败: %w", err)
	}
	return nextID, nil
}

func readAutoIncrement(ctx context.Context, db *gorm.DB, name, table string) int64 {
	if db == nil {
		return 0
	}

	dbName := db.Migrator().CurrentDatabase()
	var autoInc sql.NullInt64
	err := db.WithContext(ctx).Raw(
		"SELECT AUTO_INCREMENT FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?",
		dbName, table,
	).Scan(&autoInc).Error
	if err != nil {
		log.Printf("%s读取 %s.AUTO_INCREMENT 失败，将使用目标引用表兜底: %v", name, table, err)
		return 0
	}
	if !autoInc.Valid {
		return 0
	}
	return autoInc.Int64
}

// ResolveTaskCodebookIDs 在 task 数据迁移完成后，回填 task.codebook_id
func ResolveTaskCodebookIDs(ctx context.Context, env migration.MigrationEnv) error {
	if env.DryRun {
		log.Printf("[dry-run] 跳过 task 数据回填")
		return nil
	}

	lookup, err := loadCodebookLookup(ctx, env)
	if err != nil {
		return err
	}

	cursor, err := env.MongoDB.Collection("c_task").Find(ctx, bson.M{})
	if err != nil {
		return fmt.Errorf("查询源 c_task 失败: %w", err)
	}
	defer cursor.Close(ctx)

	err = env.MySQLDst.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for cursor.Next(ctx) {
			var src struct {
				ID          int64  `bson:"id"`
				CodebookUID string `bson:"codebook_uid"`
			}
			if err := cursor.Decode(&src); err != nil {
				return fmt.Errorf("解码源 c_task 失败: %w", err)
			}

			cbID, ok := lookup[strings.TrimSpace(src.CodebookUID)]
			if !ok {
				continue
			}

			if err := tx.Model(&dao.Task{}).
				Where("id = ?", src.ID).
				Update("codebook_id", cbID).Error; err != nil {
				return fmt.Errorf("更新 task codebook_id 失败: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	return cursor.Err()
}

// updateFlowNodesCodebookIDs 将节点列表 properties 中的 codebook_uid 修改为 codebook_id，就地修改并返回是否发生变更
func updateFlowNodesCodebookIDs(nodes []domain.FlowNode, lookup map[string]int64) bool {
	changed := false
	for i, node := range nodes {
		props, ok := node["properties"].(map[string]any)
		if !ok {
			continue
		}
		uidVal, ok := props["codebook_uid"].(string)
		if !ok {
			continue
		}
		if cbID, exists := lookup[strings.TrimSpace(uidVal)]; exists {
			props["codebook_id"] = cbID
			delete(props, "codebook_uid")
			nodes[i]["properties"] = props
			changed = true
		}
	}
	return changed
}

// ResolveWorkflowCodebookIDs 在 workflow 迁移完成后，将 flow_data nodes properties 中的 codebook_uid 回填为 codebook_id
func ResolveWorkflowCodebookIDs(ctx context.Context, env migration.MigrationEnv) error {
	if env.DryRun {
		log.Printf("[dry-run] 跳过 workflow 数据回填")
		return nil
	}

	lookup, err := loadCodebookLookup(ctx, env)
	if err != nil {
		return err
	}

	var wfs []dao.Workflow
	if err := env.MySQLDst.WithContext(ctx).Find(&wfs).Error; err != nil {
		return fmt.Errorf("查询目标 MySQL 中的 workflow 失败: %w", err)
	}

	return env.MySQLDst.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, wf := range wfs {
			if updateFlowNodesCodebookIDs(wf.FlowData.Val.Nodes, lookup) {
				if err := tx.Model(&dao.Workflow{}).
					Where("id = ?", wf.Id).
					Update("flow_data", wf.FlowData).Error; err != nil {
					return fmt.Errorf("更新 workflow flow_data 失败: %w", err)
				}
			}
		}
		return nil
	})
}

// ResolveWorkflowInstanceFlowCodebookIDs 在 workflow_snapshot 迁移完成后，将 flow_data nodes properties 中的 codebook_uid 回填为 codebook_id
func ResolveWorkflowInstanceFlowCodebookIDs(ctx context.Context, env migration.MigrationEnv) error {
	if env.DryRun {
		log.Printf("[dry-run] 跳过 workflow_snapshot 数据回填")
		return nil
	}

	lookup, err := loadCodebookLookup(ctx, env)
	if err != nil {
		return err
	}

	var snapshots []dao.Snapshot
	if err := env.MySQLDst.WithContext(ctx).Find(&snapshots).Error; err != nil {
		return fmt.Errorf("查询目标 MySQL 中的 workflow_snapshot 失败: %w", err)
	}

	return env.MySQLDst.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, snap := range snapshots {
			if updateFlowNodesCodebookIDs(snap.FlowData.Val.Nodes, lookup) {
				if err := tx.Model(&dao.Snapshot{}).
					Where("id = ?", snap.Id).
					Update("flow_data", snap.FlowData).Error; err != nil {
					return fmt.Errorf("更新 workflow_snapshot flow_data 失败: %w", err)
				}
			}
		}
		return nil
	})
}
