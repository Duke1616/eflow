package etask

import (
	"context"
	"fmt"

	runnerv1 "github.com/Duke1616/eflow/api/proto/gen/etask/runner/v1"
)

// Runner 是 eflow 选择执行单元时需要的最小视图。
type Runner struct {
	ID         int64
	CodebookID int64
}

// RunnerCatalog 定义 eflow 对 etask 执行单元目录的查询能力。
type RunnerCatalog interface {
	// FindByCodebookAndTag 根据 Codebook 和标签查找执行单元。
	FindByCodebookAndTag(ctx context.Context, codebookID int64, tag string) (Runner, error)
	// FindByID 根据主键查找执行单元。
	FindByID(ctx context.Context, id int64) (Runner, error)
}

type runnerCatalog struct {
	client runnerv1.RunnerServiceClient
}

// NewRunnerCatalog 创建 etask 执行单元目录适配器。
func NewRunnerCatalog(client *ETASKClient) RunnerCatalog {
	return &runnerCatalog{client: client.RunnerClient}
}

func (r *runnerCatalog) FindByCodebookAndTag(ctx context.Context, codebookID int64,
	tag string) (Runner, error) {
	response, err := r.client.FindRunnerByCodebookIdAndTag(ctx,
		&runnerv1.FindRunnerByCodebookIdAndTagRequest{CodebookId: codebookID, Tag: tag})
	if err != nil {
		return Runner{}, err
	}
	return toRunner(response.GetRunner())
}

func (r *runnerCatalog) FindByID(ctx context.Context, id int64) (Runner, error) {
	response, err := r.client.FindRunnerByID(ctx, &runnerv1.FindRunnerByIDRequest{Id: id})
	if err != nil {
		return Runner{}, err
	}
	return toRunner(response.GetRunner())
}

func toRunner(runner *runnerv1.Runner) (Runner, error) {
	if runner == nil || runner.GetId() <= 0 {
		return Runner{}, fmt.Errorf("未找到匹配的执行单元")
	}
	return Runner{ID: runner.GetId(), CodebookID: runner.GetCodebookId()}, nil
}
