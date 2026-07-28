package rating

import (
	"context"
	"errors"
	"fmt"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository"
)

var (
	ErrTicketNotRateable = errors.New("当前工单不可评价")
	ErrAlreadyRated      = errors.New("工单已经评价，不能重复修改")
)

// Service 提供工单整体评价能力。
type Service interface {
	// Submit 为已办结工单提交一次不可修改的整体评价，重复的相同请求按幂等成功处理。
	Submit(ctx context.Context, ticketID int64, actor string, score uint8, comment string) (domain.TicketRating, error)
	// FindByTicketID 查询工单评价，第二个返回值表示评价是否存在。
	FindByTicketID(ctx context.Context, ticketID int64) (domain.TicketRating, bool, error)
	// ListByTicketIDs 批量查询评价并以工单 ID 建立索引。
	ListByTicketIDs(ctx context.Context, ticketIDs []int64) (map[int64]domain.TicketRating, error)
}

type service struct {
	tickets repository.TicketRepository
	ratings repository.TicketRatingRepository
}

func NewService(tickets repository.TicketRepository, ratings repository.TicketRatingRepository) Service {
	return &service{tickets: tickets, ratings: ratings}
}

func (s *service) Submit(ctx context.Context, ticketID int64, actor string,
	score uint8, comment string) (domain.TicketRating, error) {
	ticket, err := s.tickets.Detail(ctx, ticketID)
	if err != nil {
		return domain.TicketRating{}, errors.Join(ErrTicketNotRateable, fmt.Errorf("查询工单失败: %w", err))
	}
	if ticket.Status != domain.END || ticket.CreateBy != actor {
		return domain.TicketRating{}, ErrTicketNotRateable
	}
	rating := domain.TicketRating{
		TenantID: ticket.TenantID, TicketID: ticketID,
		RaterUsername: actor, Score: score, Comment: comment,
	}
	if err = rating.Validate(); err != nil {
		return domain.TicketRating{}, err
	}

	stored, inserted, err := s.ratings.Create(ctx, rating)
	if err != nil {
		return domain.TicketRating{}, err
	}
	if inserted || sameRating(stored, rating) {
		return stored, nil
	}
	return domain.TicketRating{}, ErrAlreadyRated
}

func (s *service) FindByTicketID(ctx context.Context,
	ticketID int64) (domain.TicketRating, bool, error) {
	return s.ratings.FindByTicketID(ctx, ticketID)
}

func (s *service) ListByTicketIDs(ctx context.Context,
	ticketIDs []int64) (map[int64]domain.TicketRating, error) {
	return s.ratings.ListByTicketIDs(ctx, ticketIDs)
}

func sameRating(left, right domain.TicketRating) bool {
	return left.TicketID == right.TicketID && left.RaterUsername == right.RaterUsername &&
		left.Score == right.Score && left.Comment == right.Comment
}
