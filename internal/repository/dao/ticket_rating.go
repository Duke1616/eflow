package dao

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TicketRating 是一张工单的一次整体评价记录。
type TicketRating struct {
	ID            int64  `gorm:"primaryKey;column:id;type:bigint;autoIncrement;comment:'工单评价主键'"`
	TenantID      int64  `gorm:"column:tenant_id;type:bigint;not null;uniqueIndex:uk_ticket_rating,priority:1;index;comment:'租户 ID'"`
	TicketID      int64  `gorm:"column:ticket_id;type:bigint;not null;uniqueIndex:uk_ticket_rating,priority:2;index;comment:'工单 ID'"`
	RaterUsername string `gorm:"column:rater_username;type:varchar(128);not null;index;comment:'评价人用户名'"`
	Score         uint8  `gorm:"column:score;type:tinyint unsigned;not null;index;comment:'整体评分 1-5'"`
	Comment       string `gorm:"column:comment;type:varchar(500);not null;default:'';comment:'评价内容'"`
	CTime         int64  `gorm:"column:ctime;type:bigint;not null;comment:'创建时间(毫秒)'"`
	UTime         int64  `gorm:"column:utime;type:bigint;not null;comment:'更新时间(毫秒)'"`
}

func (TicketRating) TableName() string { return "ticket_rating" }

// TicketRatingDAO 管理工单评价持久化。
type TicketRatingDAO interface {
	// Create 使用工单唯一约束幂等写入评价，第二个返回值表示是否插入新记录。
	Create(ctx context.Context, rating TicketRating) (TicketRating, bool, error)
	// FindByTicketID 根据工单 ID 查询唯一评价记录。
	FindByTicketID(ctx context.Context, ticketID int64) (TicketRating, error)
	// ListByTicketIDs 批量查询指定工单的评价记录。
	ListByTicketIDs(ctx context.Context, ticketIDs []int64) ([]TicketRating, error)
}

type gormTicketRatingDAO struct{ db *gorm.DB }

func NewTicketRatingDAO(db *gorm.DB) TicketRatingDAO { return &gormTicketRatingDAO{db: db} }

func (g *gormTicketRatingDAO) Create(ctx context.Context, rating TicketRating) (TicketRating, bool, error) {
	now := time.Now().UnixMilli()
	rating.CTime, rating.UTime = now, now
	result := g.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&rating)
	if result.Error != nil {
		return TicketRating{}, false, result.Error
	}
	existing, err := g.FindByTicketID(ctx, rating.TicketID)
	inserted := err == nil && rating.ID > 0 && existing.ID == rating.ID
	return existing, inserted, err
}

func (g *gormTicketRatingDAO) FindByTicketID(ctx context.Context, ticketID int64) (TicketRating, error) {
	var rating TicketRating
	err := g.db.WithContext(ctx).Where("ticket_id = ?", ticketID).First(&rating).Error
	return rating, err
}

func (g *gormTicketRatingDAO) ListByTicketIDs(ctx context.Context, ticketIDs []int64) ([]TicketRating, error) {
	if len(ticketIDs) == 0 {
		return nil, nil
	}
	var ratings []TicketRating
	err := g.db.WithContext(ctx).Where("ticket_id IN ?", ticketIDs).Find(&ratings).Error
	return ratings, err
}
