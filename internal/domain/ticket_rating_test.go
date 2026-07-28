package domain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTicketRatingValidate(t *testing.T) {
	testCases := []struct {
		name    string
		rating  TicketRating
		wantErr bool
	}{
		{name: "valid", rating: TicketRating{TicketID: 1, RaterUsername: "alice", Score: 5, Comment: " 很满意 "}},
		{name: "score too low", rating: TicketRating{TicketID: 1, RaterUsername: "alice", Score: 0}, wantErr: true},
		{name: "score too high", rating: TicketRating{TicketID: 1, RaterUsername: "alice", Score: 6}, wantErr: true},
		{name: "comment too long", rating: TicketRating{TicketID: 1, RaterUsername: "alice", Score: 3,
			Comment: strings.Repeat("好", MaxTicketRatingCommentLength+1)}, wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.rating.Validate()
			if testCase.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "很满意", testCase.rating.Comment)
		})
	}
}
