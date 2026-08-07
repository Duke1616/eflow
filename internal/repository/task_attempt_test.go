package repository

import (
	"testing"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository/dao"
	"github.com/stretchr/testify/require"
)

func TestToAttemptDomainPreservesProgramKind(t *testing.T) {
	attempt := toAttemptDomain(dao.TaskAttempt{ProgramKind: string(domain.ProgramProject)})
	require.Equal(t, domain.ProgramProject, attempt.ProgramKind)
}

func TestToAttemptDomainNormalizesLegacyProgramKind(t *testing.T) {
	attempt := toAttemptDomain(dao.TaskAttempt{})
	require.Equal(t, domain.ProgramInline, attempt.ProgramKind)
}
