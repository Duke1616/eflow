package rating

import (
	"context"
	"testing"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository"
	"github.com/stretchr/testify/require"
)

func TestSubmitRatingBusinessRules(t *testing.T) {
	testCases := []struct {
		name       string
		ticket     domain.Ticket
		actor      string
		stored     domain.TicketRating
		inserted   bool
		wantErr    error
		wantCreate int
	}{
		{name: "creator rates completed ticket",
			ticket: domain.Ticket{Id: 1, TenantID: 7, Status: domain.END, CreateBy: "alice"},
			actor:  "alice", inserted: true, wantCreate: 1},
		{name: "processing ticket cannot be rated",
			ticket: domain.Ticket{Id: 1, Status: domain.PROCESS, CreateBy: "alice"},
			actor:  "alice", wantErr: ErrTicketNotRateable},
		{name: "participant cannot rate creator ticket",
			ticket: domain.Ticket{Id: 1, Status: domain.END, CreateBy: "alice"},
			actor:  "bob", wantErr: ErrTicketNotRateable},
		{name: "same request is idempotent",
			ticket: domain.Ticket{Id: 1, TenantID: 7, Status: domain.END, CreateBy: "alice"},
			actor:  "alice", stored: domain.TicketRating{TicketID: 1, RaterUsername: "alice", Score: 5, Comment: "满意"},
			wantCreate: 1},
		{name: "submitted rating cannot be changed",
			ticket: domain.Ticket{Id: 1, TenantID: 7, Status: domain.END, CreateBy: "alice"},
			actor:  "alice", stored: domain.TicketRating{TicketID: 1, RaterUsername: "alice", Score: 2},
			wantErr: ErrAlreadyRated, wantCreate: 1},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			tickets := &ticketRepositoryStub{ticket: testCase.ticket}
			ratings := &ratingRepositoryStub{stored: testCase.stored, inserted: testCase.inserted}
			svc := NewService(tickets, ratings)

			_, err := svc.Submit(context.Background(), 1, testCase.actor, 5, "满意")

			if testCase.wantErr != nil {
				require.ErrorIs(t, err, testCase.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, testCase.wantCreate, ratings.createCalls)
		})
	}
}

type ticketRepositoryStub struct {
	repository.TicketRepository
	ticket domain.Ticket
}

func (s *ticketRepositoryStub) Detail(context.Context, int64) (domain.Ticket, error) {
	return s.ticket, nil
}

type ratingRepositoryStub struct {
	repository.TicketRatingRepository
	stored      domain.TicketRating
	inserted    bool
	createCalls int
}

func (s *ratingRepositoryStub) Create(_ context.Context,
	rating domain.TicketRating) (domain.TicketRating, bool, error) {
	s.createCalls++
	if s.inserted {
		rating.ID = 10
		return rating, true, nil
	}
	return s.stored, false, nil
}
