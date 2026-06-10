package runner

import (
	"encoding/json"
	"errors"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/errs"
	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	runnerSvc "github.com/Duke1616/eflow/internal/service/runner"
	workflowSvc "github.com/Duke1616/eflow/internal/service/workflow"
	"github.com/Duke1616/eiam/pkg/web/capability"
	"github.com/ecodeclub/ekit/slice"
	"github.com/ecodeclub/ginx"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	capability.IRegistry
	svc         runnerSvc.Service
	workflowSvc workflowSvc.Service
}

func NewHandler(svc runnerSvc.Service, workflowSvc workflowSvc.Service) *Handler {
	return &Handler{
		svc:         svc,
		workflowSvc: workflowSvc,
		IRegistry:   capability.NewRegistry("ticket", "runner", "脚本引擎/执行单元"),
	}
}

func (h *Handler) PrivateRoutes(server *gin.Engine) {
	g := server.Group("/api/runner")

	g.POST("/register", h.Capability("注册执行单元", "add").
		Handle(ginx.B[RegisterRunnerReq](h.Register)),
	)
	g.POST("/list", h.Capability("执行单元列表", "view").
		Handle(ginx.B[ListRunnerReq](h.ListRunner)),
	)
	g.POST("/list/tags", h.Capability("执行单元标签", "tags").
		Handle(ginx.W(h.ListTags)),
	)
	g.GET("/detail/:id", h.Capability("执行单元详情", "get").
		Handle(ginx.W(h.Detail)),
	)
	g.POST("/update", h.Capability("更新执行单元", "edit").
		Handle(ginx.B[UpdateRunnerReq](h.UpdateRunner)),
	)
	g.DELETE("/delete/:id", h.Capability("删除执行单元", "delete").
		Handle(ginx.W(h.DeleteRunner)),
	)
	g.POST("/list/by_ids", h.Capability("批量查询执行单元", "view_by_ids").
		NoSync().
		Handle(ginx.B[ListRunnerByIds](h.ListByIds)),
	)

	g.POST("/list/by_codebook_uid", h.Capability("当前绑定执行单元", "view_runners").
		Module("codebook").
		Group("脚本引擎").
		Needs("ticket:runner:view_exclude_codebook_uid").
		Handle(ginx.B[ListByCodebookIdReq](h.ListByCodebookId)),
	)
	g.POST("/list/exclude_codebook_uid", h.Capability("复用执行单元", "view_exclude_codebook_uid").
		NoSync().
		Handle(ginx.B[ListByCodebookIdReq](h.ListExcludeCodebookUid)),
	)
	g.POST("/list/by_workflow_id", h.Capability("查询工作流关联执行单元", "view_by_workflow_id").
		NoSync().
		Handle(ginx.B[ListByWorkflowIdReq](h.ListByWorkflowId)),
	)
}

func (h *Handler) Register(ctx *ginx.Context, req RegisterRunnerReq) (ginx.Result, error) {
	id, err := h.svc.Create(ctx.Context, h.toDomain(req))
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{
		Msg:  "注册成功",
		Data: id,
	}, nil
}

func (h *Handler) ListByCodebookId(ctx *ginx.Context, req ListByCodebookIdReq) (ginx.Result, error) {
	rs, total, err := h.svc.ListByCodebookUid(ctx.Context, req.Offset, req.Limit, req.CodebookUid, req.Keyword, req.Kind)
	if err != nil {
		return h.translateError(err), err
	}

	return ginx.Result{
		Msg: "查询 runner 列表成功",
		Data: RetrieveWorkers{
			Total: total,
			Runners: slice.Map(rs, func(idx int, src domain.Runner) RunnerVO {
				return h.toRunnerVo(src)
			}),
		},
	}, nil
}

func (h *Handler) ListExcludeCodebookUid(ctx *ginx.Context, req ListByCodebookIdReq) (ginx.Result, error) {
	rs, total, err := h.svc.ListExcludeCodebookUid(ctx.Context, req.Offset, req.Limit, req.CodebookUid, req.Keyword, req.Kind)
	if err != nil {
		return h.translateError(err), err
	}

	return ginx.Result{
		Msg: "查询 runner 列表成功",
		Data: RetrieveWorkers{
			Total: total,
			Runners: slice.Map(rs, func(idx int, src domain.Runner) RunnerVO {
				return h.toRunnerVo(src)
			}),
		},
	}, nil
}

// ListByWorkflowId 根据工作流 ID 获取所关联的自动化任务执行器列表
func (h *Handler) ListByWorkflowId(ctx *ginx.Context, req ListByWorkflowIdReq) (ginx.Result, error) {
	// 获取最新版的工作流定义，用于配置/管理场景
	wf, err := h.workflowSvc.Find(ctx.Context, req.WorkflowId)
	if err != nil {
		return h.translateError(err), err
	}

	nodesJSON, err := json.Marshal(wf.FlowData.Nodes)
	if err != nil {
		return SystemErrorResult, err
	}
	var nodes []easyflow.Node
	err = json.Unmarshal(nodesJSON, &nodes)
	if err != nil {
		return SystemErrorResult, err
	}

	codebookUids := make([]string, 0)
	for _, node := range nodes {
		if node.Type == "automation" {
			property, _ := easyflow.ToNodeProperty[easyflow.AutomationProperty](node)
			codebookUids = append(codebookUids, property.CodebookUid)
		}
	}

	if len(codebookUids) == 0 {
		return ErrWorkflowNotBindCodebook, nil
	}

	rs, err := h.svc.ListByCodebookUids(ctx.Context, codebookUids)
	if err != nil {
		return h.translateError(err), err
	}
	if len(rs) == 0 {
		return ErrWorkflowNotBindRunner, nil
	}

	return ginx.Result{
		Msg: "查询 runner 列表成功",
		Data: RetrieveWorkers{
			Total: int64(len(rs)),
			Runners: slice.Map(rs, func(idx int, src domain.Runner) RunnerVO {
				return h.toRunnerVo(src)
			}),
		},
	}, nil
}

func (h *Handler) ListByIds(ctx *ginx.Context, req ListRunnerByIds) (ginx.Result, error) {
	rs, err := h.svc.ListByIds(ctx.Context, req.Ids)
	if err != nil {
		return h.translateError(err), err
	}

	return ginx.Result{
		Msg: "查询 runner 列表成功",
		Data: RetrieveWorkers{
			Total: int64(len(rs)),
			Runners: slice.Map(rs, func(idx int, src domain.Runner) RunnerVO {
				return h.toRunnerVo(src)
			}),
		},
	}, nil
}

func (h *Handler) DeleteRunner(ctx *ginx.Context) (ginx.Result, error) {
	id, err := ctx.Param("id").AsInt64()
	if err != nil {
		return ErrRunnerInvalidId, err
	}

	count, err := h.svc.Delete(ctx.Context, id)
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{
		Msg:  "删除成功",
		Data: count,
	}, nil
}

func (h *Handler) Detail(ctx *ginx.Context) (ginx.Result, error) {
	id, err := ctx.Param("id").AsInt64()
	if err != nil {
		return ErrRunnerInvalidId, err
	}

	runner, err := h.svc.FindById(ctx.Context, id)
	if err != nil {
		return h.translateError(err), err
	}

	return ginx.Result{
		Data: h.toRunnerVo(runner),
	}, nil
}

func (h *Handler) ListRunner(ctx *ginx.Context, req ListRunnerReq) (ginx.Result, error) {
	ws, total, err := h.svc.List(ctx.Context, req.Offset, req.Limit, req.Keyword, req.Kind)
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{
		Msg: "查询 runner 列表成功",
		Data: RetrieveWorkers{
			Total: total,
			Runners: slice.Map(ws, func(idx int, src domain.Runner) RunnerVO {
				return h.toRunnerVo(src)
			}),
		},
	}, nil
}

func (h *Handler) UpdateRunner(ctx *ginx.Context, req UpdateRunnerReq) (ginx.Result, error) {
	runner, err := h.toUpdateDomain(ctx, req)
	if err != nil {
		return h.translateError(err), err
	}

	count, err := h.svc.Update(ctx.Context, runner)
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{
		Msg:  "修改成功",
		Data: count,
	}, nil
}

func (h *Handler) ListTags(ctx *ginx.Context) (ginx.Result, error) {
	tags, err := h.svc.AggregateTags(ctx.Context)
	if err != nil {
		return h.translateError(err), err
	}

	return ginx.Result{
		Msg: "查询 runner tags 列表成功",
		Data: RetrieveRunnerTags{
			RunnerTags: slice.Map(tags, func(idx int, src domain.RunnerTags) RunnerTags {
				tagDetails := make([]TagDetail, 0, len(src.TagsMapping))
				for tag, detail := range src.TagsMapping {
					tagDetails = append(tagDetails, TagDetail{
						Tag:     tag,
						Kind:    detail.Kind.ToString(),
						Target:  detail.Target,
						Handler: detail.Handler,
					})
				}
				return RunnerTags{
					CodebookName: src.CodebookUid,
					CodebookUid:  src.CodebookUid,
					Tags:         tagDetails,
				}
			}),
		},
	}, nil
}

func (h *Handler) translateError(err error) ginx.Result {
	if errors.Is(err, errs.ErrInvalidParameter) {
		return ErrInvalidParameter(err)
	}
	return SystemErrorResult
}

func (h *Handler) toDomain(req RegisterRunnerReq) domain.Runner {
	r := domain.Runner{
		Name:           req.Name,
		CodebookSecret: req.CodebookSecret,
		CodebookUid:    req.CodebookUid,
		Tags:           req.Tags,
		Kind:           domain.Kind(req.Kind),
		Variables:      h.toVariablesDomain(req.Variables),
		Action:         domain.Action(REGISTER),
		Target:         req.Target,
		Handler:        req.Handler,
		Desc:           req.Desc,
	}

	return r
}

func (h *Handler) toUpdateDomain(ctx *ginx.Context, req UpdateRunnerReq) (domain.Runner, error) {
	runner, err := h.svc.FindById(ctx.Context, req.Id)
	if err != nil {
		return domain.Runner{}, err
	}

	oldVars := slice.ToMap(runner.Variables, func(element domain.Variables) string {
		return element.Key
	})

	r := domain.Runner{
		Id:             req.Id,
		Name:           req.Name,
		CodebookSecret: req.CodebookSecret,
		CodebookUid:    req.CodebookUid,
		Tags:           req.Tags,
		Kind:           domain.Kind(req.Kind),
		Variables:      h.toUpdateVariablesDomain(oldVars, req.Variables),
		Action:         domain.Action(REGISTER),
		Target:         req.Target,
		Handler:        req.Handler,
		Desc:           req.Desc,
	}

	return r, nil
}

func (h *Handler) toUpdateVariablesDomain(oldVars map[string]domain.Variables, req []Variables) []domain.Variables {
	return slice.Map(req, func(idx int, src Variables) domain.Variables {
		value := src.Value
		if src.Secret {
			val, ok := oldVars[src.Key]
			if ok && src.Value == "" {
				value = val.Value
			}
		}

		return domain.Variables{
			Key:    src.Key,
			Secret: src.Secret,
			Value:  value,
		}
	})
}

func (h *Handler) toVariablesDomain(req []Variables) []domain.Variables {
	return slice.Map(req, func(idx int, src Variables) domain.Variables {
		return domain.Variables{
			Key:    src.Key,
			Secret: src.Secret,
			Value:  src.Value,
		}
	})
}

func (h *Handler) toRunnerVo(req domain.Runner) RunnerVO {
	r := RunnerVO{
		Id:          req.Id,
		Name:        req.Name,
		Kind:        req.Kind.ToString(),
		CodebookUid: req.CodebookUid,
		Tags:        req.Tags,
		Desc:        req.Desc,
		Target:      req.Target,
		Handler:     req.Handler,
		Variables: slice.Map(req.Variables, func(idx int, src domain.Variables) Variables {
			if src.Secret {
				return Variables{
					Key:    src.Key,
					Secret: src.Secret,
				}
			}
			return Variables{
				Key:    src.Key,
				Secret: src.Secret,
				Value:  src.Value,
			}
		}),
	}
	return r
}
