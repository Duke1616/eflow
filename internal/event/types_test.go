package event_test

import (
	"testing"

	"github.com/Duke1616/eflow/internal/event"
	"github.com/stretchr/testify/assert"
)

func TestTicketEvent_DecodeVariables(t *testing.T) {
	testCases := []struct {
		name     string
		evt      event.TicketEvent
		assertFn func(t *testing.T, res map[string]any)
	}{
		{
			name: "解码切片数组格式",
			evt: event.TicketEvent{
				Variables: `[{"key":"ticket_id","value":"101"},{"key":"amount","value":99.5}]`,
			},
			assertFn: func(t *testing.T, res map[string]any) {
				assert.Equal(t, "101", res["ticket_id"])
				assert.Equal(t, 99.5, res["amount"])
			},
		},
		{
			name: "解码标准字典格式",
			evt: event.TicketEvent{
				Variables: `{"ticket_id":"102","department":"IT"}`,
			},
			assertFn: func(t *testing.T, res map[string]any) {
				assert.Equal(t, "102", res["ticket_id"])
				assert.Equal(t, "IT", res["department"])
			},
		},
		{
			name: "空字符串与空对象防呆",
			evt: event.TicketEvent{
				Variables: "",
			},
			assertFn: func(t *testing.T, res map[string]any) {
				assert.NotNil(t, res)
				assert.Empty(t, res)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.evt.DecodeVariables()
			tc.assertFn(t, got)
		})
	}
}
