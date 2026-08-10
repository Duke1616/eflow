package migrate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"

	runnerv1 "github.com/Duke1616/eflow/api/proto/gen/etask/runner/v1"
	etaskclient "github.com/Duke1616/eflow/internal/client/etask"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository/dao"
	"github.com/Duke1616/eflow/pkg/sqlx"
	"github.com/Duke1616/eiam/pkg/ctxutil"
	grpcpkg "github.com/Duke1616/etask/pkg/grpc"
	"github.com/Duke1616/etask/pkg/grpc/registry"
	etcdregistry "github.com/Duke1616/etask/pkg/grpc/registry/etcd"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

const defaultWorkflowRunnerMigrationTimeout = 10 * time.Minute

type workflowRunnerOptions struct {
	apply   bool
	timeout time.Duration
}

func newWorkflowRunnersCommand() *cobra.Command {
	options := workflowRunnerOptions{timeout: defaultWorkflowRunnerMigrationTimeout}
	cmd := &cobra.Command{
		Use:   "workflow-runners",
		Short: "将旧工作流的 tag 路由迁移为明确的 runner_id",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Root().SilenceUsage = true
			return runWorkflowRunnerMigration(cmd.Context(), options, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&options.apply, "apply", false, "写入迁移结果；默认仅预检")
	cmd.Flags().DurationVar(&options.timeout, "timeout", options.timeout, "迁移超时时间")
	return cmd
}

func runWorkflowRunnerMigration(parent context.Context, options workflowRunnerOptions, output io.Writer) error {
	if options.timeout <= 0 {
		return fmt.Errorf("timeout 必须大于 0")
	}
	ctx, cancel := context.WithTimeout(parent, options.timeout)
	defer cancel()
	env, err := openWorkflowRunnerEnvironment()
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := env.Close(); closeErr != nil {
			fmt.Fprintf(output, "警告: 关闭迁移资源失败: %v\n", closeErr)
		}
	}()

	records, summary, err := loadWorkflowRecords(ctx, env.db)
	if err != nil {
		return err
	}
	updates, err := planWorkflowRunnerUpdates(ctx, records, env.resolver, &summary, output)
	printWorkflowRunnerSummary(output, summary, len(updates), options.apply)
	if err != nil {
		return err
	}
	if !options.apply || len(updates) == 0 {
		return nil
	}
	if err = applyWorkflowRunnerUpdates(ctx, env.db, updates); err != nil {
		return err
	}
	fmt.Fprintf(output, "迁移完成: 已更新 %d 条记录\n", len(updates))
	return nil
}

type workflowRunnerEnvironment struct {
	db       *gorm.DB
	resolver etaskclient.RunnerCatalog
	conn     *grpc.ClientConn
	registry registry.Registry
	etcd     *clientv3.Client
}

func openWorkflowRunnerEnvironment() (*workflowRunnerEnvironment, error) {
	dsn := strings.TrimSpace(viper.GetString("mysql.dsn"))
	if dsn == "" {
		return nil, fmt.Errorf("mysql.dsn 不能为空")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		return nil, fmt.Errorf("连接 eflow 数据库: %w", err)
	}

	env := &workflowRunnerEnvironment{db: db}
	var etcdConfig clientv3.Config
	if err = viper.UnmarshalKey("etcd", &etcdConfig); err != nil {
		_ = env.Close()
		return nil, fmt.Errorf("读取 etcd 配置: %w", err)
	}
	env.etcd, err = clientv3.New(etcdConfig)
	if err != nil {
		_ = env.Close()
		return nil, fmt.Errorf("连接 etcd: %w", err)
	}
	env.registry, err = etcdregistry.NewRegistry(env.etcd)
	if err != nil {
		_ = env.Close()
		return nil, fmt.Errorf("创建服务发现客户端: %w", err)
	}

	var grpcConfig grpcpkg.ClientConfig
	if err = viper.UnmarshalKey("grpc.client.etask", &grpcConfig); err != nil {
		_ = env.Close()
		return nil, fmt.Errorf("读取 etask gRPC 配置: %w", err)
	}
	env.conn, err = grpcpkg.NewClientConn(env.registry,
		grpcpkg.WithServiceName(grpcConfig.Name),
		grpcpkg.WithClientJWTAuth(grpcConfig.AuthToken),
	)
	if err != nil {
		_ = env.Close()
		return nil, fmt.Errorf("连接 etask gRPC: %w", err)
	}
	env.resolver = etaskclient.NewRunnerCatalogFromGRPC(runnerv1.NewRunnerServiceClient(env.conn))
	return env, nil
}

func (e *workflowRunnerEnvironment) Close() error {
	var errs []error
	if e.conn != nil {
		errs = append(errs, e.conn.Close())
	}
	if e.registry != nil {
		errs = append(errs, e.registry.Close())
	}
	if e.etcd != nil {
		errs = append(errs, e.etcd.Close())
	}
	if e.db != nil {
		if sqlDB, err := e.db.DB(); err == nil {
			errs = append(errs, sqlDB.Close())
		}
	}
	return errors.Join(errs...)
}

type flowRecord struct {
	table    string
	id       int64
	tenantID int64
	version  int64
	flowData dao.LogicFlow
}

type workflowRunnerSummary struct {
	workflows       int
	snapshots       int
	legacyNodes     int
	resolvedNodes   int
	cleanedNodes    int
	noDefaultNodes  int
	multipleMatches int
}

func loadWorkflowRecords(ctx context.Context, db *gorm.DB) ([]flowRecord, workflowRunnerSummary, error) {
	var workflows []dao.Workflow
	if err := db.WithContext(ctx).Select("id", "tenant_id", "utime", "flow_data").Find(&workflows).Error; err != nil {
		return nil, workflowRunnerSummary{}, fmt.Errorf("读取 workflow: %w", err)
	}
	var snapshots []dao.Snapshot
	if err := db.WithContext(ctx).Select("id", "tenant_id", "ctime", "flow_data").Find(&snapshots).Error; err != nil {
		return nil, workflowRunnerSummary{}, fmt.Errorf("读取 workflow_snapshot: %w", err)
	}

	records := make([]flowRecord, 0, len(workflows)+len(snapshots))
	for _, workflow := range workflows {
		records = append(records, flowRecord{
			table: "workflow", id: workflow.Id, tenantID: workflow.TenantID,
			version: workflow.Utime, flowData: workflow.FlowData.Val,
		})
	}
	for _, snapshot := range snapshots {
		records = append(records, flowRecord{
			table: "workflow_snapshot", id: snapshot.Id, tenantID: snapshot.TenantID,
			version: snapshot.Ctime, flowData: snapshot.FlowData.Val,
		})
	}
	return records, workflowRunnerSummary{workflows: len(workflows), snapshots: len(snapshots)}, nil
}

type legacyRunnerSelector struct {
	tenantID    int64
	codebookID  int64
	tag         string
	programKind domain.ProgramKind
}

type cachedRunnerList struct {
	runners []etaskclient.Runner
	err     error
}

func planWorkflowRunnerUpdates(ctx context.Context, records []flowRecord,
	resolver etaskclient.RunnerCatalog, summary *workflowRunnerSummary,
	output io.Writer) ([]flowRecord, error) {
	cache := make(map[[2]int64]cachedRunnerList)
	updates := make([]flowRecord, 0)
	problems := make([]string, 0)

	for i := range records {
		changed := false
		for _, node := range records[i].flowData.Nodes {
			if node["type"] != "automation" {
				continue
			}
			properties, ok := node["properties"].(map[string]interface{})
			if !ok {
				continue
			}
			_, hasTag := properties["tag"]
			_, hasProgramKind := properties["program_kind"]
			if !hasTag && !hasProgramKind {
				continue
			}
			summary.legacyNodes++

			if runnerID, valid := positiveInt64(properties["runner_id"]); valid && runnerID > 0 {
				cleanLegacyRunnerProperties(properties)
				summary.cleanedNodes++
				changed = true
				continue
			}

			selector, selectorErr := legacySelectorFromProperties(records[i].tenantID, properties)
			if selectorErr != nil || selector.tag == "auto" {
				cleanLegacyRunnerProperties(properties)
				summary.noDefaultNodes++
				changed = true
				reason := selectorErr
				if reason == nil {
					reason = fmt.Errorf("历史 auto 路由不代表明确的默认执行单元")
				}
				fmt.Fprintln(output, "警告:", describeLegacyNode(records[i], node, reason))
				continue
			}
			cacheKey := [2]int64{selector.tenantID, selector.codebookID}
			candidates, found := cache[cacheKey]
			if !found {
				requestCtx := ctxutil.WithTenantID(ctx, selector.tenantID)
				value, resolveErr := resolver.ListByCodebookID(requestCtx, selector.codebookID)
				candidates = cachedRunnerList{runners: value, err: resolveErr}
				cache[cacheKey] = candidates
			}
			if candidates.err != nil {
				problems = append(problems, describeLegacyNode(records[i], node, candidates.err))
				continue
			}
			matches := matchLegacyRunners(candidates.runners, selector.tag, selector.programKind)
			if len(matches) == 0 {
				cleanLegacyRunnerProperties(properties)
				summary.noDefaultNodes++
				changed = true
				fmt.Fprintln(output, "警告:", describeLegacyNode(records[i], node,
					fmt.Errorf("未找到旧配置对应的默认执行单元")))
				continue
			}

			properties["runner_id"] = matches[0].ID
			cleanLegacyRunnerProperties(properties)
			summary.resolvedNodes++
			changed = true
			if len(matches) > 1 {
				summary.multipleMatches++
				fmt.Fprintf(output,
					"警告: %s id=%d 节点=%v 匹配到 %d 个执行单元，采用 ID 最小的 %s(%d)\n",
					records[i].table, records[i].id, node["id"], len(matches),
					matches[0].Name, matches[0].ID)
			}
		}
		if changed {
			updates = append(updates, records[i])
		}
	}

	if len(problems) > 0 {
		sort.Strings(problems)
		return nil, fmt.Errorf("迁移执行单元时发生 %d 个查询错误，未写入任何数据:\n- %s",
			len(problems), strings.Join(problems, "\n- "))
	}
	return updates, nil
}

func matchLegacyRunners(runners []etaskclient.Runner, tag string, programKind domain.ProgramKind) []etaskclient.Runner {
	matches := make([]etaskclient.Runner, 0)
	for _, runner := range runners {
		if runner.ProgramKind != programKind || !containsTag(runner.Tags, tag) {
			continue
		}
		matches = append(matches, runner)
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].ID < matches[j].ID })
	return matches
}

func containsTag(tags []string, tag string) bool {
	for _, candidate := range tags {
		if strings.TrimSpace(candidate) == tag {
			return true
		}
	}
	return false
}

func cleanLegacyRunnerProperties(properties map[string]interface{}) {
	delete(properties, "tag")
	delete(properties, "program_kind")
	delete(properties, "codebook_uid")
	if _, valid := positiveInt64(properties["runner_id"]); !valid {
		delete(properties, "runner_id")
	}
}

func legacySelectorFromProperties(tenantID int64, properties map[string]interface{}) (legacyRunnerSelector, error) {
	codebookID, ok := positiveInt64(properties["codebook_id"])
	if !ok || codebookID <= 0 {
		return legacyRunnerSelector{}, fmt.Errorf("codebook_id 非法")
	}
	tag, ok := properties["tag"].(string)
	tag = strings.TrimSpace(tag)
	if !ok || tag == "" {
		return legacyRunnerSelector{}, fmt.Errorf("tag 为空或格式非法")
	}

	programKind := domain.ProgramInline
	if raw, exists := properties["program_kind"]; exists && raw != nil {
		value, valid := raw.(string)
		if !valid {
			return legacyRunnerSelector{}, fmt.Errorf("program_kind 格式非法")
		}
		if value = strings.TrimSpace(value); value != "" {
			programKind = domain.ProgramKind(strings.ToUpper(value))
		}
	}
	if !programKind.Valid() {
		return legacyRunnerSelector{}, fmt.Errorf("program_kind 非法: %s", programKind)
	}
	return legacyRunnerSelector{
		tenantID: tenantID, codebookID: codebookID, tag: tag, programKind: programKind,
	}, nil
}

func positiveInt64(value interface{}) (int64, bool) {
	switch number := value.(type) {
	case int:
		return int64(number), number > 0
	case int32:
		return int64(number), number > 0
	case int64:
		return number, number > 0
	case uint:
		if uint64(number) > math.MaxInt64 {
			return 0, false
		}
		return int64(number), number > 0
	case uint32:
		return int64(number), number > 0
	case uint64:
		if number > math.MaxInt64 {
			return 0, false
		}
		return int64(number), number > 0
	case float64:
		if number <= 0 || number > math.MaxInt64 || math.Trunc(number) != number {
			return 0, false
		}
		return int64(number), true
	default:
		return 0, false
	}
}

func describeLegacyNode(record flowRecord, node domain.FlowNode, err error) string {
	return fmt.Sprintf("%s id=%d tenant_id=%d 节点=%v: %v",
		record.table, record.id, record.tenantID, node["id"], err)
}

func applyWorkflowRunnerUpdates(ctx context.Context, db *gorm.DB, updates []flowRecord) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, update := range updates {
			flowData := sqlx.JsonField[dao.LogicFlow]{Val: update.flowData, Valid: true}
			versionColumn := "utime"
			if update.table == "workflow_snapshot" {
				versionColumn = "ctime"
			}
			result := tx.Table(update.table).
				Where("id = ? AND tenant_id = ?", update.id, update.tenantID).
				Where(versionColumn+" = ?", update.version).
				Update("flow_data", flowData)
			if result.Error != nil {
				return fmt.Errorf("更新 %s id=%d: %w", update.table, update.id, result.Error)
			}
			if result.RowsAffected != 1 {
				return fmt.Errorf("更新 %s id=%d: 记录不存在或已变化", update.table, update.id)
			}
		}
		return nil
	})
}

func printWorkflowRunnerSummary(output io.Writer, summary workflowRunnerSummary, updates int, apply bool) {
	mode := "预检"
	if apply {
		mode = "写入"
	}
	fmt.Fprintf(output,
		"迁移%s: workflow=%d, snapshot=%d, 历史节点=%d, 已迁移默认值=%d, 已有默认值=%d, 无默认值=%d, 多候选=%d, 待更新记录=%d\n",
		mode, summary.workflows, summary.snapshots, summary.legacyNodes, summary.resolvedNodes,
		summary.cleanedNodes, summary.noDefaultNodes, summary.multipleMatches, updates)
	if !apply {
		fmt.Fprintln(output, "当前为 dry-run，确认结果后添加 --apply 执行写入")
	}
}
