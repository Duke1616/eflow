package etask

import (
	"context"
	"fmt"

	runnerv1 "github.com/Duke1616/eflow/api/proto/gen/etask/runner/v1"
	"github.com/Duke1616/eflow/internal/domain"
)

// Runner 是 eflow 选择执行单元时需要的最小视图。
type Runner struct {
	ID          int64
	Name        string
	CodebookID  int64
	ProgramKind domain.ProgramKind
	Tags        []string
}

// RunnerCatalog 定义 eflow 对 etask 执行单元目录的查询能力。
type RunnerCatalog interface {
	// FindByID 根据主键查找执行单元。
	FindByID(ctx context.Context, id int64) (Runner, error)
	// ListByCodebookID 获取绑定指定脚本文件的执行单元。
	ListByCodebookID(ctx context.Context, codebookID int64) ([]Runner, error)
}

type runnerCatalog struct {
	client runnerv1.RunnerServiceClient
}

// NewRunnerCatalog 创建 etask 执行单元目录适配器。
func NewRunnerCatalog(client *ETASKClient) RunnerCatalog {
	return &runnerCatalog{client: client.RunnerClient}
}

// NewRunnerCatalogFromGRPC 从原始 gRPC 客户端创建执行单元目录适配器。
func NewRunnerCatalogFromGRPC(client runnerv1.RunnerServiceClient) RunnerCatalog {
	return &runnerCatalog{client: client}
}

func (r *runnerCatalog) FindByID(ctx context.Context, id int64) (Runner, error) {
	response, err := r.client.FindRunnerByID(ctx, &runnerv1.FindRunnerByIDRequest{Id: id})
	if err != nil {
		return Runner{}, err
	}
	return runnerFromProto(response.GetRunner())
}

func (r *runnerCatalog) ListByCodebookID(ctx context.Context, codebookID int64) ([]Runner, error) {
	response, err := r.client.ListRunnersByCodebookID(ctx,
		&runnerv1.ListRunnersByCodebookIDRequest{CodebookId: codebookID})
	if err != nil {
		return nil, err
	}
	runners := make([]Runner, 0, len(response.GetRunners()))
	for _, item := range response.GetRunners() {
		runner, convertErr := runnerFromProto(item)
		if convertErr != nil {
			return nil, convertErr
		}
		runners = append(runners, runner)
	}
	return runners, nil
}

func runnerFromProto(runner *runnerv1.Runner) (Runner, error) {
	if runner == nil || runner.GetId() <= 0 {
		return Runner{}, fmt.Errorf("未找到匹配的执行单元")
	}
	return Runner{
		ID: runner.GetId(), Name: runner.GetName(), CodebookID: runner.GetCodebookId(),
		ProgramKind: domain.ProgramKind(runner.GetProgramKind()), Tags: runner.GetTags(),
	}, nil
}
