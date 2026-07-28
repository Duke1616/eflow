package ticket

import (
	"testing"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestCanRateTicket(t *testing.T) {
	ticket := domain.Ticket{Status: domain.END, CreateBy: "alice"}

	require.True(t, canRateTicket(ticket, "alice", false))
	require.False(t, canRateTicket(ticket, "bob", false))
	require.False(t, canRateTicket(ticket, "alice", true))
	ticket.Status = domain.WITHDRAW
	require.False(t, canRateTicket(ticket, "alice", false))
}
