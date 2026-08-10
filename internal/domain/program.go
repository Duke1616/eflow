package domain

// ProgramKind 描述 Runner 以单文件还是完整项目方式执行程序。
type ProgramKind string

const (
	ProgramInline  ProgramKind = "INLINE"
	ProgramProject ProgramKind = "PROJECT"
)

// Valid 判断程序模式是否是内部可执行的显式模式。
func (k ProgramKind) Valid() bool {
	return k == ProgramInline || k == ProgramProject
}
