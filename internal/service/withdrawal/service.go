package withdrawal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository"
	engineSvc "github.com/Duke1616/eflow/internal/service/engine"
)

var ErrAutomationRunning = errors.New("自动化任务正在执行，暂时无法撤回")
var ErrInvalidRevokeReason = errors.New("撤单原因不能为空且不能超过 500 个字符")

//go:generate mockgen -source=./service.go -package=withdrawalmocks -destination=./mocks/withdrawal_svc.mock.go Service

// Service 统一编排流程实例撤回及其自动化任务状态迁移。
type Service interface {
	// Revoke 校验撤单原因，撤回流程实例，并启动已成功动作对应的撤回补偿。
	Revoke(ctx context.Context, engineType domain.EngineType, processInstanceID domain.ID, force bool, username, reason string) error
}

type service struct {
	repo    repository.WithdrawalRepository
	planner CompensationPlanner
	revoker processRevoker
}

func NewService(repo repository.WithdrawalRepository, planner CompensationPlanner, engine engineSvc.IEngine) Service {
	return newService(repo, planner, engineRevoker{engine: engine})
}

type processRevoker interface {
	// Revoke 将指定流程实例交由流程引擎撤回。
	Revoke(ctx context.Context, engineType domain.EngineType, processInstanceID domain.ID, force bool, username string) error
}

type engineRevoker struct {
	engine engineSvc.IEngine
}

func (r engineRevoker) Revoke(ctx context.Context, engineType domain.EngineType, processInstanceID domain.ID, force bool, username string) error {
	if r.engine == nil {
		return fmt.Errorf("工作流引擎未配置")
	}
	return r.engine.Revoke(ctx, domain.RevokeReq{
		EngineType: engineType,
		InstanceID: processInstanceID,
		Force:      force,
		Operator:   username,
	})
}

func newService(repo repository.WithdrawalRepository, planner CompensationPlanner, revoker processRevoker) Service {
	return &service{repo: repo, planner: planner, revoker: revoker}
}

func (s *service) Revoke(ctx context.Context, engineType domain.EngineType, processInstanceID domain.ID, force bool, username, reason string) error {
	if processInstanceID.IsZero() || username == "" {
		return fmt.Errorf("撤回流程参数非法")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" || len([]rune(reason)) > 500 {
		return ErrInvalidRevokeReason
	}
	if err := s.repo.Prepare(ctx, processInstanceID, reason); err != nil {
		if errors.Is(err, repository.ErrAutomationRunning) {
			return errors.Join(ErrAutomationRunning, err)
		}
		return err
	}
	plan, err := s.planner.Build(ctx, processInstanceID.Int())
	if err != nil {
		_ = s.repo.Rollback(ctx, processInstanceID)
		return fmt.Errorf("生成撤回补偿计划失败: %w", err)
	}

	revoked := false
	defer func() {
		if !revoked {
			abortCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_ = s.repo.Rollback(abortCtx, processInstanceID)
		}
	}()

	if err := s.revoker.Revoke(ctx, engineType, processInstanceID, force, username); err != nil {
		return err
	}
	revoked = true
	commitCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := s.planner.Apply(commitCtx, processInstanceID.Int(), plan); err != nil {
		return fmt.Errorf("启动撤回补偿失败: %w", err)
	}
	if _, err := s.repo.TryFinalize(commitCtx, processInstanceID); err != nil {
		return fmt.Errorf("完成工单撤回状态失败: %w", err)
	}
	return nil
}
