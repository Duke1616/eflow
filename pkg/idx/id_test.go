package idx

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestID_Basics(t *testing.T) {
	id1 := New(102)
	assert.Equal(t, 102, id1.Int())
	assert.Equal(t, int64(102), id1.Int64())
	assert.Equal(t, "102", id1.String())
	assert.False(t, id1.IsZero())

	id2 := New("task_abc")
	assert.Equal(t, 0, id2.Int())
	assert.Equal(t, "task_abc", id2.String())
	assert.False(t, id2.IsZero())

	var zero ID
	assert.True(t, zero.IsZero())
	assert.Equal(t, 0, zero.Int())
	assert.Equal(t, "", zero.String())
}

func TestID_UnmarshalJSON(t *testing.T) {
	type wrapper struct {
		TaskID ID `json:"task_id"`
	}

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "数字反序列化",
			input:    `{"task_id": 501}`,
			expected: "501",
		},
		{
			name:     "字符串反序列化",
			input:    `{"task_id": "task_123"}`,
			expected: "task_123",
		},
		{
			name:     "null 值",
			input:    `{"task_id": null}`,
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var w wrapper
			err := json.Unmarshal([]byte(tc.input), &w)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, w.TaskID.String())
		})
	}
}
