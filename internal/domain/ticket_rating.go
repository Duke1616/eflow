package domain

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const MaxTicketRatingCommentLength = 500

// TicketRating 是提单人对已办结工单的一次整体评价。
type TicketRating struct {
	ID            int64
	TenantID      int64
	TicketID      int64
	RaterUsername string
	Score         uint8
	Comment       string
	CTime         int64
	UTime         int64
}

// Validate 校验评分内容，不包含工单状态和评价人权限判断。
func (r *TicketRating) Validate() error {
	r.Comment = strings.TrimSpace(r.Comment)
	if r.TicketID <= 0 || r.RaterUsername == "" {
		return fmt.Errorf("%w: 工单或评价人不能为空", ErrInvalidParameter)
	}
	if r.Score < 1 || r.Score > 5 {
		return fmt.Errorf("%w: 评分必须在 1 到 5 之间", ErrInvalidParameter)
	}
	if utf8.RuneCountInString(r.Comment) > MaxTicketRatingCommentLength {
		return fmt.Errorf("%w: 评价内容不能超过 %d 个字符", ErrInvalidParameter, MaxTicketRatingCommentLength)
	}
	return nil
}
