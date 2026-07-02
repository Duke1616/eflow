package migrate

import (
	"context"
	"log"
	"os"

	easyEngine "github.com/Bunny3th/easy-workflow/workflow/engine"
	"github.com/Duke1616/eflow/cmd/migrate/internal/config"
	"github.com/Duke1616/eflow/cmd/migrate/internal/migrations"
	"github.com/Duke1616/eflow/internal/repository/dao"
	"github.com/Duke1616/eiam/pkg/migration"
	"github.com/spf13/cobra"
	"gorm.io/gorm"
)

// NewCommand 返回 migrate 子命令。
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "执行数据迁移",
		Run: func(cmd *cobra.Command, args []string) {
			runMigrate(force)
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "强制重新执行迁移（清除历史迁移记录）")
	return cmd
}

var force bool

func init() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
}

func runMigrate(force bool) {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("使用迁移配置: %s", cfg.ConfigFile)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	// NOTE: 转换为 eiam 共享迁移包的配置结构体
	mCfg := migration.Config{
		MongoDSN:               cfg.MongoDSN,
		MongoDBName:            cfg.MongoDBName,
		MySQLSrcDSN:            cfg.MySQLSrcDSN,
		MySQLDstDSN:            cfg.MySQLDstDSN,
		BatchSize:              cfg.BatchSize,
		Timeout:                cfg.Timeout,
		AutoMigrate:            cfg.AutoMigrate,
		Truncate:               cfg.Truncate,
		DryRun:                 cfg.DryRun,
		Force:                  force,
		SkipResetAutoIncrement: !cfg.ResetAutoIncrement,
	}

	var postHooks []migration.Hook
	if cfg.ResetAutoIncrement {
		postHooks = append(postHooks, migrations.SyncProcessInstanceAutoIncrement)
	}
	postHooks = append(postHooks,
		migrations.ResolveTaskCodebookIDs,
		migrations.ResolveWorkflowCodebookIDs,
		migrations.ResolveWorkflowInstanceFlowCodebookIDs,
	)

	// NOTE: 构造 eiam 统一包的迁移器，并注入本地 eflow 特定的自动建表逻辑与默认租户覆盖选项
	runner := migration.NewRunner(mCfg, migrations.All(),
		migration.WithDefaultTenantID(migrations.DefaultTenantID),
		migration.WithAutoMigrateFunc(func(db *gorm.DB) error {
			if err = dao.InitTables(db); err != nil {
				return err
			}
			easyEngine.DB = db
			return easyEngine.DatabaseInitialize()
		}),
		migration.WithPostHooks(postHooks...),
	)

	if err = runner.Run(ctx); err != nil {
		log.Fatal(err)
	}
	log.Println("迁移完成")
}
