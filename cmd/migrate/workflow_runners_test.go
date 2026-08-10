package migrate

import (
	"bytes"
	"context"
	"errors"
	"testing"

	etaskclient "github.com/Duke1616/eflow/internal/client/etask"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository/dao"
	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/stretchr/testify/require"
)

type stubRunnerCatalog struct {
	list func(context.Context, int64) ([]etaskclient.Runner, error)
}

func (s stubRunnerCatalog) FindByID(context.Context, int64) (etaskclient.Runner, error) {
	return etaskclient.Runner{}, errors.New("unexpected FindByID call")
}

func (s stubRunnerCatalog) ListByCodebookID(ctx context.Context, codebookID int64) ([]etaskclient.Runner, error) {
	return s.list(ctx, codebookID)
}

func legacyWorkflowRecord(properties map[string]interface{}) []flowRecord {
	return []flowRecord{{
		table: "workflow", id: 7, tenantID: 3,
		flowData: dao.LogicFlow{Nodes: []domain.FlowNode{
			{"id": "deploy", "type": "automation", "properties": properties},
		}},
	}}
}

func TestPlanWorkflowRunnerUpdatesResolvesLegacyDefault(t *testing.T) {
	properties := map[string]interface{}{"codebook_id": float64(12), "tag": " linux "}
	catalog := stubRunnerCatalog{list: func(ctx context.Context, codebookID int64) ([]etaskclient.Runner, error) {
		require.Equal(t, int64(3), ctxutil.GetTenantID(ctx).Int64())
		require.Equal(t, int64(12), codebookID)
		return []etaskclient.Runner{
			{ID: 8, Name: "linux-runner", ProgramKind: "INLINE", Tags: []string{"linux"}},
		}, nil
	}}
	summary := workflowRunnerSummary{}

	updates, err := planWorkflowRunnerUpdates(context.Background(), legacyWorkflowRecord(properties),
		catalog, &summary, &bytes.Buffer{})

	require.NoError(t, err)
	require.Len(t, updates, 1)
	require.Equal(t, int64(8), properties["runner_id"])
	require.NotContains(t, properties, "tag")
	require.Equal(t, 1, summary.resolvedNodes)
}

func TestPlanWorkflowRunnerUpdatesChoosesLowestMatchingRunner(t *testing.T) {
	properties := map[string]interface{}{"codebook_id": 12, "tag": "prod"}
	catalog := stubRunnerCatalog{list: func(context.Context, int64) ([]etaskclient.Runner, error) {
		return []etaskclient.Runner{
			{ID: 9, Name: "later", ProgramKind: "INLINE", Tags: []string{"prod"}},
			{ID: 3, Name: "first", ProgramKind: "INLINE", Tags: []string{"prod"}},
			{ID: 1, Name: "project", ProgramKind: "PROJECT", Tags: []string{"prod"}},
		}, nil
	}}
	var output bytes.Buffer
	summary := workflowRunnerSummary{}

	updates, err := planWorkflowRunnerUpdates(context.Background(), legacyWorkflowRecord(properties),
		catalog, &summary, &output)

	require.NoError(t, err)
	require.Len(t, updates, 1)
	require.Equal(t, int64(3), properties["runner_id"])
	require.Equal(t, 1, summary.multipleMatches)
	require.Contains(t, output.String(), "ID 最小")
}

func TestPlanWorkflowRunnerUpdatesKeepsExistingDefault(t *testing.T) {
	properties := map[string]interface{}{
		"codebook_id": 12, "runner_id": float64(8), "tag": "linux", "program_kind": "INLINE",
	}
	catalog := stubRunnerCatalog{list: func(context.Context, int64) ([]etaskclient.Runner, error) {
		t.Fatal("已有默认执行单元时不应查询执行单元列表")
		return nil, nil
	}}
	summary := workflowRunnerSummary{}

	updates, err := planWorkflowRunnerUpdates(context.Background(), legacyWorkflowRecord(properties),
		catalog, &summary, &bytes.Buffer{})

	require.NoError(t, err)
	require.Len(t, updates, 1)
	require.Equal(t, float64(8), properties["runner_id"])
	require.NotContains(t, properties, "tag")
	require.NotContains(t, properties, "program_kind")
	require.Equal(t, 1, summary.cleanedNodes)
}

func TestPlanWorkflowRunnerUpdatesLeavesAutoWithoutDefault(t *testing.T) {
	properties := map[string]interface{}{
		"codebook_id": 12, "codebook_uid": "shell", "tag": "auto", "program_kind": "INLINE",
	}
	catalog := stubRunnerCatalog{list: func(context.Context, int64) ([]etaskclient.Runner, error) {
		t.Fatal("auto 不应被猜测为任意默认执行单元")
		return nil, nil
	}}
	var output bytes.Buffer
	summary := workflowRunnerSummary{}

	updates, err := planWorkflowRunnerUpdates(context.Background(), legacyWorkflowRecord(properties),
		catalog, &summary, &output)

	require.NoError(t, err)
	require.Len(t, updates, 1)
	require.NotContains(t, properties, "runner_id")
	require.NotContains(t, properties, "tag")
	require.NotContains(t, properties, "program_kind")
	require.NotContains(t, properties, "codebook_uid")
	require.Equal(t, 1, summary.noDefaultNodes)
}

func TestPlanWorkflowRunnerUpdatesCleansInvalidLegacyNode(t *testing.T) {
	properties := map[string]interface{}{"codebook_uid": "shell", "tag": "deploy_uat"}
	catalog := stubRunnerCatalog{list: func(context.Context, int64) ([]etaskclient.Runner, error) {
		t.Fatal("缺少 codebook_id 时不应查询执行单元列表")
		return nil, nil
	}}
	summary := workflowRunnerSummary{}

	updates, err := planWorkflowRunnerUpdates(context.Background(), legacyWorkflowRecord(properties),
		catalog, &summary, &bytes.Buffer{})

	require.NoError(t, err)
	require.Len(t, updates, 1)
	require.Empty(t, properties)
	require.Equal(t, 1, summary.noDefaultNodes)
}

func TestPlanWorkflowRunnerUpdatesAbortsOnCatalogFailure(t *testing.T) {
	properties := map[string]interface{}{"codebook_id": 12, "tag": "linux"}
	catalog := stubRunnerCatalog{list: func(context.Context, int64) ([]etaskclient.Runner, error) {
		return nil, errors.New("etask unavailable")
	}}
	summary := workflowRunnerSummary{}

	updates, err := planWorkflowRunnerUpdates(context.Background(), legacyWorkflowRecord(properties),
		catalog, &summary, &bytes.Buffer{})

	require.Nil(t, updates)
	require.ErrorContains(t, err, "查询错误")
	require.Contains(t, properties, "tag")
}
