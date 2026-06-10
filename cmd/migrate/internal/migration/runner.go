package migration

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	easyEngine "github.com/Bunny3th/easy-workflow/workflow/engine"
	"github.com/Duke1616/eflow/cmd/migrate/internal/config"
	"github.com/Duke1616/eflow/internal/repository/dao"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// MigrationRecord 迁移历史记录实体
type MigrationRecord struct {
	Id        int64     `gorm:"primaryKey;column:id;type:bigint;autoIncrement"`
	Name      string    `gorm:"column:name;type:varchar(128);uniqueIndex;not null;comment:'迁移任务名称'"`
	Read      int64     `gorm:"column:read_count;type:bigint;not null;comment:'读取源数据条数'"`
	Converted int64     `gorm:"column:converted_count;type:bigint;not null;comment:'转换成功条数'"`
	Written   int64     `gorm:"column:written_count;type:bigint;not null;comment:'写入目标表条数'"`
	Ctime     time.Time `gorm:"column:ctime;type:datetime;not null;comment:'迁移开始时间'"`
	Utime     time.Time `gorm:"column:utime;type:datetime;not null;comment:'迁移完成时间'"`
}

// Runner 负责连接数据源，并按顺序执行一组迁移任务。
type Runner struct {
	cfg       config.Config
	migrators []Migrator
}

// NewRunner 创建迁移执行器。
func NewRunner(cfg config.Config, migrators []Migrator) *Runner {
	return &Runner{
		cfg:       cfg,
		migrators: migrators,
	}
}

// Run 执行完整迁移流程。
func (r *Runner) Run(ctx context.Context) error {
	// 1. 初始化源端和目标端连接 (MongoDB, MySQL)
	env, mongoClient, mysqlSRC, mysqlDST, err := r.initEnv(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err = mongoClient.Disconnect(context.Background()); err != nil {
			log.Printf("warning: 关闭 MongoDB 连接失败: %v", err)
		}
	}()
	defer closeMySQL("源端 MySQL", mysqlSRC)
	defer closeMySQL("目标端 MySQL", mysqlDST)

	// 2. 初始化目标数据库与表结构
	if err = r.bootstrapDB(mysqlDST); err != nil {
		return err
	}

	// 3. 执行清空目标表（如果配置了 Truncate）
	if err = r.tryTruncate(ctx, mysqlDST); err != nil {
		return err
	}

	// 4. 执行主要迁移流程循环
	log.Printf("开始迁移: batch_size=%d dry_run=%t", r.cfg.BatchSize, r.cfg.DryRun)
	for _, migrator := range r.migrators {
		// 检查该任务是否已成功执行，已执行则跳过
		if skipped, err1 := r.shouldSkip(mysqlDST, migrator); err1 != nil {
			return err1
		} else if skipped {
			continue
		}

		if err = r.runMigrator(ctx, mysqlDST, migrator, env); err != nil {
			return err
		}
	}

	// 5. 序列号重置
	return r.tryResetAutoIncrement(ctx, mysqlDST)
}

// initEnv 统一组装并连接数据源
func (r *Runner) initEnv(ctx context.Context) (MigrationEnv, *mongo.Client, *gorm.DB, *gorm.DB, error) {
	var env MigrationEnv
	mongoClient, err := mongo.Connect(options.Client().ApplyURI(r.cfg.MongoDSN))
	if err != nil {
		return env, nil, nil, nil, fmt.Errorf("连接源端 MongoDB 失败: %w", err)
	}
	if err = mongoClient.Ping(ctx, readpref.Primary()); err != nil {
		mongoClient.Disconnect(context.Background())
		return env, nil, nil, nil, fmt.Errorf("探测源端 MongoDB 失败: %w", err)
	}

	mysqlSRC, err := openMySQL(r.cfg.MySQLSrcDSN)
	if err != nil {
		mongoClient.Disconnect(context.Background())
		return env, nil, nil, nil, fmt.Errorf("连接源端 MySQL 失败: %w", err)
	}

	mysqlDST, err := openMySQL(r.cfg.MySQLDstDSN)
	if err != nil {
		mongoClient.Disconnect(context.Background())
		closeMySQL("源端 MySQL", mysqlSRC)
		return env, nil, nil, nil, fmt.Errorf("连接目标端 MySQL 失败: %w", err)
	}

	env = MigrationEnv{
		MongoDB:   mongoClient.Database(r.cfg.MongoDBName),
		MySQLSrc:  mysqlSRC,
		MySQLDst:  mysqlDST,
		BatchSize: r.cfg.BatchSize,
		DryRun:    r.cfg.DryRun,
	}
	return env, mongoClient, mysqlSRC, mysqlDST, nil
}

// bootstrapDB 初始化控制表以及迁移所需的各项目标物理表
func (r *Runner) bootstrapDB(db *gorm.DB) error {
	if r.cfg.DryRun {
		return nil
	}

	// 始终自动初始化迁移状态控制记录表
	if err := db.AutoMigrate(&MigrationRecord{}); err != nil {
		return fmt.Errorf("初始化迁移记录表失败: %w", err)
	}

	if r.cfg.AutoMigrate {
		log.Println("正在初始化目标端表结构")
		if err := dao.InitTables(db); err != nil {
			return fmt.Errorf("初始化目标端表结构失败: %w", err)
		}

		easyEngine.DB = db
		if err := easyEngine.DatabaseInitialize(); err != nil {
			return fmt.Errorf("初始化目标端表结构失败: %w", err)
		}
	}
	return nil
}

// tryTruncate 执行清空目标数据表
func (r *Runner) tryTruncate(ctx context.Context, db *gorm.DB) error {
	if r.cfg.DryRun || !r.cfg.Truncate {
		return nil
	}
	return r.truncateDestinations(ctx, db)
}

// shouldSkip 判断指定任务是否已存在执行成功的历史，存在则跳过以达到幂等性
func (r *Runner) shouldSkip(db *gorm.DB, migrator Migrator) (bool, error) {
	if r.cfg.DryRun {
		return false, nil
	}

	var record MigrationRecord
	err := db.Where("name = ?", migrator.Name()).First(&record).Error
	if err == nil {
		log.Printf("迁移任务 [%s] 已经于 %s 执行成功过 (读取:%d 写入:%d)，直接跳过。若想重新执行，请在数据库中删除该任务对应的记录。",
			migrator.Name(), record.Utime.Format("2006-01-02 15:04:05"), record.Read, record.Written)
		return true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return false, fmt.Errorf("查询迁移历史记录 [%s] 失败: %w", migrator.Name(), err)
}

// runMigrator 封装执行单个迁移器逻辑并对成功结果记录入库
func (r *Runner) runMigrator(ctx context.Context, db *gorm.DB, migrator Migrator, env MigrationEnv) error {
	startTime := time.Now()
	log.Printf("正在迁移 %s", migrator.Name())
	result, err := migrator.Migrate(ctx, env)
	if err != nil {
		return fmt.Errorf("迁移 %s 失败: %w", migrator.Name(), err)
	}
	log.Printf("完成 %s: read=%d converted=%d written=%d", migrator.Name(), result.Read, result.Converted, result.Written)

	if r.cfg.DryRun {
		return nil
	}

	newRecord := MigrationRecord{
		Name:      migrator.Name(),
		Read:      result.Read,
		Converted: result.Converted,
		Written:   result.Written,
		Ctime:     startTime,
		Utime:     time.Now(),
	}
	if err = db.Create(&newRecord).Error; err != nil {
		return fmt.Errorf("保存迁移历史记录 [%s] 失败: %w", migrator.Name(), err)
	}
	return nil
}

// tryResetAutoIncrement 重置所有自增序列
func (r *Runner) tryResetAutoIncrement(ctx context.Context, db *gorm.DB) error {
	if r.cfg.DryRun || !r.cfg.ResetAutoIncrement {
		return nil
	}
	return r.resetAutoIncrement(ctx, db)
}

func openMySQL(dsn string) (*gorm.DB, error) {
	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             3 * time.Second,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  true,
		},
	)
	return gorm.Open(mysql.Open(dsn), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
		Logger:         newLogger,
	})
}

func pingMySQL(ctx context.Context, db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

func closeMySQL(name string, db *gorm.DB) {
	sqlDB, err := db.DB()
	if err != nil {
		log.Printf("warning: 获取 %s 连接句柄失败: %v", name, err)
		return
	}
	if err = sqlDB.Close(); err != nil {
		log.Printf("warning: 关闭 %s 连接失败: %v", name, err)
	}
}

func (r *Runner) truncateDestinations(ctx context.Context, db *gorm.DB) error {
	for i := len(r.migrators) - 1; i >= 0; i-- {
		table, err := tableName(db, r.migrators[i].Destination())
		if err != nil {
			return err
		}
		log.Printf("正在清空目标表 %s", table)
		if err = db.WithContext(ctx).Exec("TRUNCATE TABLE " + quoteIdentifier(table)).Error; err != nil {
			return fmt.Errorf("清空目标表 %s 失败: %w", table, err)
		}
	}

	log.Println("正在清空迁移历史记录表")
	if err := db.WithContext(ctx).Exec("TRUNCATE TABLE migration_record").Error; err != nil {
		log.Printf("warning: 清空迁移历史记录表失败: %v", err)
	}
	return nil
}

func (r *Runner) resetAutoIncrement(ctx context.Context, db *gorm.DB) error {
	for _, migrator := range r.migrators {
		table, err := tableName(db, migrator.Destination())
		if err != nil {
			return err
		}
		if err = db.WithContext(ctx).Exec("ALTER TABLE " + quoteIdentifier(table) + " AUTO_INCREMENT = 1").Error; err != nil {
			return fmt.Errorf("重置目标表 %s 自增序列失败: %w", table, err)
		}
		log.Printf("已重置目标表 %s 自增序列", table)
	}
	return nil
}

func quoteIdentifier(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}
