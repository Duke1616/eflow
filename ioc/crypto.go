package ioc

import (
	"github.com/Duke1616/ecmdb/pkg/cryptox"
)

type noOpCrypto struct{}

// Encrypt 暂时原样返回，为未来一站式升级 AES 留出标准注入点
func (noOpCrypto) Encrypt(plainText string) (string, error) {
	return plainText, nil
}

// Decrypt 暂时原样返回，兼容明文状态的运行变量
func (noOpCrypto) Decrypt(encryptedText string) (string, error) {
	return encryptedText, nil
}

// InitCrypto 实例化加解密引擎组件，提供给任务派发器使用
func InitCrypto() cryptox.Crypto {
	return noOpCrypto{}
}
