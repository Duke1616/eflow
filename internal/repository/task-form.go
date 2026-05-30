package repository

import (
	"context"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository/dao"
	"github.com/Duke1616/eflow/pkg/sqlx"
	"github.com/ecodeclub/ekit/slice"
)

func (repo *ticketRepository) CreateTaskForm(ctx context.Context, taskId int, ticketId int64, fields []domain.FormValue) error {
	forms := slice.Map(fields, func(idx int, src domain.FormValue) dao.TaskForm {
		return dao.TaskForm{
			TicketId: ticketId,
			TaskId:   taskId,
			Name:     src.Name,
			Key:      src.Key,
			Type:     src.Type,
			Value:    sqlx.JsonField[interface{}]{Val: src.Value, Valid: true},
		}
	})
	return repo.taskForms.Create(ctx, forms)
}

func (repo *ticketRepository) FindTaskFormsBatch(ctx context.Context, taskIds []int) (map[int][]domain.FormValue, error) {
	forms, err := repo.taskForms.FindByTaskIds(ctx, taskIds)
	if err != nil {
		return nil, err
	}

	res := make(map[int][]domain.FormValue)
	for _, f := range forms {
		res[f.TaskId] = append(res[f.TaskId], domain.FormValue{
			Name:  f.Name,
			Key:   f.Key,
			Type:  f.Type,
			Value: f.Value.Val,
		})
	}
	return res, nil
}

func (repo *ticketRepository) FindTaskFormsByTicketID(ctx context.Context, ticketID int64) ([]domain.FormValue, error) {
	forms, err := repo.taskForms.FindByTicketID(ctx, ticketID)
	return slice.Map(forms, func(idx int, src dao.TaskForm) domain.FormValue {
		return domain.FormValue{
			Name:  src.Name,
			Key:   src.Key,
			Type:  src.Type,
			Value: src.Value.Val,
		}
	}), err
}
