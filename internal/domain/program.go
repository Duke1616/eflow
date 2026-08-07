package domain

// ProgramKind 描述自动化程序以单文件还是完整项目执行。
type ProgramKind string

const (
	ProgramInline  ProgramKind = "INLINE"
	ProgramProject ProgramKind = "PROJECT"
)

// Effective 返回实际执行模式，空值兼容为 INLINE。
func (k ProgramKind) Effective() ProgramKind {
	if k == "" {
		return ProgramInline
	}
	return k
}

// Valid 判断程序模式是否是内部可执行的显式模式。
func (k ProgramKind) Valid() bool {
	return k == ProgramInline || k == ProgramProject
}
