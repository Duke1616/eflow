package repository

import (
	"context"
	"errors"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository/dao"
	"gorm.io/gorm"
)

// TicketRatingRepository 管理工单评价记录。
type TicketRatingRepository interface {
	// Create 幂等创建评价，第二个返回值表示本次调用是否实际插入了记录。
	Create(ctx context.Context, rating domain.TicketRating) (domain.TicketRating, bool, error)
	// FindByTicketID 查询工单评价，第二个返回值表示评价是否存在。
	FindByTicketID(ctx context.Context, ticketID int64) (domain.TicketRating, bool, error)
	// ListByTicketIDs 批量查询评价并以工单 ID 建立索引。
	ListByTicketIDs(ctx context.Context, ticketIDs []int64) (map[int64]domain.TicketRating, error)
}

type ticketRatingRepository struct{ dao dao.TicketRatingDAO }

func NewTicketRatingRepository(ratingDAO dao.TicketRatingDAO) TicketRatingRepository {
	return &ticketRatingRepository{dao: ratingDAO}
}

func (r *ticketRatingRepository) Create(ctx context.Context,
	rating domain.TicketRating) (domain.TicketRating, bool, error) {
	created, inserted, err := r.dao.Create(ctx, toTicketRatingEntity(rating))
	return toTicketRatingDomain(created), inserted, err
}

func (r *ticketRatingRepository) FindByTicketID(ctx context.Context,
	ticketID int64) (domain.TicketRating, bool, error) {
	rating, err := r.dao.FindByTicketID(ctx, ticketID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.TicketRating{}, false, nil
	}
	return toTicketRatingDomain(rating), err == nil, err
}

func (r *ticketRatingRepository) ListByTicketIDs(ctx context.Context,
	ticketIDs []int64) (map[int64]domain.TicketRating, error) {
	ratings, err := r.dao.ListByTicketIDs(ctx, ticketIDs)
	result := make(map[int64]domain.TicketRating, len(ratings))
	for _, rating := range ratings {
		domainRating := toTicketRatingDomain(rating)
		result[domainRating.TicketID] = domainRating
	}
	return result, err
}

func toTicketRatingEntity(rating domain.TicketRating) dao.TicketRating {
	return dao.TicketRating{
		ID: rating.ID, TenantID: rating.TenantID, TicketID: rating.TicketID,
		RaterUsername: rating.RaterUsername, Score: rating.Score, Comment: rating.Comment,
		CTime: rating.CTime, UTime: rating.UTime,
	}
}

func toTicketRatingDomain(rating dao.TicketRating) domain.TicketRating {
	return domain.TicketRating{
		ID: rating.ID, TenantID: rating.TenantID, TicketID: rating.TicketID,
		RaterUsername: rating.RaterUsername, Score: rating.Score, Comment: rating.Comment,
		CTime: rating.CTime, UTime: rating.UTime,
	}
}
