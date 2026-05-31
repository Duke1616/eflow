package main

import (
	"context"
	"log"
	"os"

	"github.com/Duke1616/eflow/cmd/migrate/internal/config"
	"github.com/Duke1616/eflow/cmd/migrate/internal/migration"
	"github.com/Duke1616/eflow/cmd/migrate/internal/migrations"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("使用迁移配置: %s", cfg.ConfigFile)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	runner := migration.NewRunner(cfg, migrations.All())
	if err = runner.Run(ctx); err != nil {
		log.Fatal(err)
	}
	log.Println("迁移完成")
}

func init() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
}
