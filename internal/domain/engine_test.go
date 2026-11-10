package domain

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestID_JSON(t *testing.T) {
	testCases := []struct {
		name    string
		input   string
		want    ID
		wantErr bool
	}{
		{name: "string ID", input: `"task_a"`, want: "task_a"},
		{name: "numeric ID", input: `123`, want: "123"},
		{name: "null ID", input: `null`, want: ""},
		{name: "boolean is invalid", input: `true`, wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var id ID
			err := json.Unmarshal([]byte(tc.input), &id)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.want, id)
		})
	}

	encoded, err := json.Marshal(ID("task_a"))
	assert.NoError(t, err)
	assert.Equal(t, `"task_a"`, string(encoded))
}
