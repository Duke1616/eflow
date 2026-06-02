package migration

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"strconv"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MigrationEnv 保存单次迁移执行期间共享的数据源连接。
// DefaultTenantID 是迁移时统一覆写的默认租户 ID。
const DefaultTenantID int64 = 2

type MigrationEnv struct {
	MongoDB   *mongo.Database
	MySQLSrc  *gorm.DB
	MySQLDst  *gorm.DB
	BatchSize int
	DryRun    bool
}

// Result 记录单个迁移任务的处理结果。
type Result struct {
	Read      int64
	Converted int64
	Written   int64
}

// Migrator 表示一个可编排的数据迁移任务。
type Migrator interface {
	Name() string
	Destination() any
	Migrate(ctx context.Context, env MigrationEnv) (Result, error)
}

// MongoMigration 定义 MongoDB 源到 MySQL 目标的表级迁移规格。
type MongoMigration[S any, D any] interface {
	Name() string
	CollectionName() string
	Convert(src S) D
}

type mongoMigrator[M any, D any] struct {
	migration MongoMigration[M, D]
}

// NewMongoMigrator 使用具体迁移规格创建 MongoDB 源迁移器。
func NewMongoMigrator[M any, D any](migration MongoMigration[M, D]) Migrator {
	return &mongoMigrator[M, D]{
		migration: migration,
	}
}

func (m *mongoMigrator[M, D]) Name() string {
	return m.migration.Name()
}

func (m *mongoMigrator[M, D]) Destination() any {
	return new(D)
}

func (m *mongoMigrator[M, D]) Migrate(ctx context.Context, env MigrationEnv) (Result, error) {
	cursor, err := env.MongoDB.Collection(m.migration.CollectionName()).Find(ctx, bson.M{})
	if err != nil {
		return Result{}, err
	}
	defer cursor.Close(ctx)

	batch := make([]D, 0, env.BatchSize)
	var result Result
	for cursor.Next(ctx) {
		var src M
		if err = cursor.Decode(&src); err != nil {
			return result, err
		}
		result.Read++
		batch = append(batch, m.migration.Convert(src))
		if len(batch) >= env.BatchSize {
			written, err := writeBatch(ctx, env, batch)
			if err != nil {
				return result, err
			}
			result.Converted += int64(len(batch))
			result.Written += written
			batch = batch[:0]
		}
	}
	if err = cursor.Err(); err != nil {
		return result, err
	}
	written, err := writeBatch(ctx, env, batch)
	if err != nil {
		return result, err
	}
	result.Converted += int64(len(batch))
	result.Written += written
	if result.Read == 0 {
		log.Printf("[%s] 源集合 %s 为空", m.Name(), m.migration.CollectionName())
	}
	return result, nil
}

// MySQLMigration 定义 MySQL 源到 MySQL 目标的表级 1:1 迁移规格。
type MySQLMigration[T any] interface {
	Name() string
	Source() any
	Destination() any
}

type mysqlMigrator[T any] struct {
	migration MySQLMigration[T]
}

// NewMySQLMigrator 使用具体迁移规格创建 MySQL 源迁移器。
func NewMySQLMigrator[T any](migration MySQLMigration[T]) Migrator {
	return &mysqlMigrator[T]{
		migration: migration,
	}
}

func (m *mysqlMigrator[T]) Name() string {
	return m.migration.Name()
}

func (m *mysqlMigrator[T]) Destination() any {
	return m.migration.Destination()
}

func (m *mysqlMigrator[T]) Migrate(ctx context.Context, env MigrationEnv) (Result, error) {
	var result Result
	var offset int
	for {
		batch := make([]T, 0, env.BatchSize)
		if err := env.MySQLSrc.WithContext(ctx).
			Model(m.migration.Source()).
			Order("id asc").
			Offset(offset).
			Limit(env.BatchSize).
			Find(&batch).Error; err != nil {
			return result, err
		}
		if len(batch) == 0 {
			break
		}
		result.Read += int64(len(batch))
		written, err := writeBatch(ctx, env, batch)
		if err != nil {
			return result, err
		}
		result.Converted += int64(len(batch))
		result.Written += written
		offset += len(batch)
	}
	if result.Read == 0 {
		table, err := tableName(env.MySQLSrc, m.migration.Source())
		if err != nil {
			return result, err
		}
		log.Printf("[%s] 源表 %s 为空", m.Name(), table)
	}
	return result, nil
}

func writeBatch[D any](ctx context.Context, env MigrationEnv, records []D) (int64, error) {
	if len(records) == 0 {
		return 0, nil
	}
	if env.DryRun {
		return 0, nil
	}
	// 统一覆写租户 ID
	for i := range records {
		applyDefaultTenant(&records[i], DefaultTenantID)
	}
	err := env.MySQLDst.WithContext(ctx).
		Clauses(clause.OnConflict{UpdateAll: true}).
		CreateInBatches(records, env.BatchSize).Error
	if err != nil {
		return 0, err
	}
	return int64(len(records)), nil
}

// applyDefaultTenant 利用反射将目标结构体的 TenantID 字段统一设置为默认租户 ID。
func applyDefaultTenant(dst any, tenantID int64) {
	v := reflect.ValueOf(dst)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	field := v.FieldByName("TenantID")
	if field.IsValid() && field.CanSet() {
		switch field.Kind() {
		case reflect.String:
			field.SetString(strconv.FormatInt(tenantID, 10))
		case reflect.Int, reflect.Int64:
			field.SetInt(tenantID)
		default:
		}
	}
}

func tableName(db *gorm.DB, model any) (string, error) {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(model); err != nil {
		return "", fmt.Errorf("解析目标表名失败: %w", err)
	}
	if stmt.Schema == nil {
		return "", fmt.Errorf("解析目标表名失败: schema 为空")
	}
	if stmt.Schema.Table == "" {
		return "", fmt.Errorf("解析目标表名失败: 表名为空")
	}
	return stmt.Schema.Table, nil
}
