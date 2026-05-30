package domain

import (
	"fmt"

	"github.com/Duke1616/eflow/internal/errs"
)

// Codebook 脚本/代码库领域模型定义
// 代表本流程引擎所使用的一个独立脚本单元，比如用于自动步骤节点执行 of 业务脚本。
type Codebook struct {
	Id         int64  // 唯一主键 ID
	TenantID   string // 租户隔离标识
	Name       string // 脚本名称
	Owner      string // 脚本负责人名称或邮箱
	Code       string // 脚本内容（如 Shell、Python 或 Go 代码段）
	Language   string // 脚本语言（如 python, shell, golang 等）
	Secret     string // 该脚本的外部调用安全密钥
	Identifier string // 脚本的唯一业务标识（如 sys-alert-hook）
}

// Validate 创建/更新时的领域模型自身校验
func (c *Codebook) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("%w: Name = %q", errs.ErrInvalidParameter, c.Name)
	}
	if c.Identifier == "" {
		return fmt.Errorf("%w: Identifier = %q", errs.ErrInvalidParameter, c.Identifier)
	}
	if c.Code == "" {
		return fmt.Errorf("%w: Code = %q", errs.ErrInvalidParameter, c.Code)
	}
	return nil
}
