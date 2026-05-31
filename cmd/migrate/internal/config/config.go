package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

const defaultConfigFile = "cmd/migrate/migrate.yaml"

// Config 是迁移命令的完整运行配置。
type Config struct {
	MongoDSN           string
	MongoDBName        string
	MySQLDstDSN        string
	BatchSize          int
	Timeout            time.Duration
	AutoMigrate        bool
	ResetAutoIncrement bool
	Truncate           bool
	DryRun             bool
	ConfigFile         string
}

// Load 从专用迁移配置文件读取配置，并补齐安全默认值。
func Load() (Config, error) {
	v := viper.New()
	v.SetConfigFile(defaultConfigFile)
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		return Config{}, fmt.Errorf("读取迁移配置 %s 失败: %w", defaultConfigFile, err)
	}

	cfg := Config{
		MongoDSN:           v.GetString("source.mongo.dsn"),
		MongoDBName:        v.GetString("source.mongo.database"),
		MySQLDstDSN:        v.GetString("destination.mysql.dsn"),
		BatchSize:          v.GetInt("migration.batch_size"),
		Timeout:            v.GetDuration("migration.timeout"),
		AutoMigrate:        v.GetBool("migration.auto_migrate"),
		ResetAutoIncrement: v.GetBool("migration.reset_auto_increment"),
		Truncate:           v.GetBool("migration.truncate"),
		DryRun:             v.GetBool("migration.dry_run"),
		ConfigFile:         v.ConfigFileUsed(),
	}

	if cfg.BatchSize == 0 {
		cfg.BatchSize = 100
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Minute
	}
	return cfg, cfg.validate()
}

func (cfg Config) validate() error {
	if cfg.BatchSize <= 0 {
		return fmt.Errorf("migration.batch_size 必须大于 0")
	}
	if cfg.Timeout <= 0 {
		return fmt.Errorf("migration.timeout 必须大于 0")
	}
	if cfg.MongoDSN == "" {
		return fmt.Errorf("source.mongo.dsn 不能为空")
	}
	if cfg.MongoDBName == "" {
		return fmt.Errorf("source.mongo.database 不能为空")
	}
	if cfg.MySQLDstDSN == "" {
		return fmt.Errorf("destination.mysql.dsn 不能为空")
	}
	return nil
}
