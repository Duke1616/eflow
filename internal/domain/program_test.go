package domain

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProgramKindValidity(t *testing.T) {
	require.False(t, ProgramKind("").Valid())
	require.True(t, ProgramInline.Valid())
	require.True(t, ProgramProject.Valid())
	require.False(t, ProgramKind("UNKNOWN").Valid())
}
