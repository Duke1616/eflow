package repository

import (
	"github.com/Duke1616/ecmdb/pkg/cryptox"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository/dao"
	"github.com/ecodeclub/ekit/slice"
)

// toDAOVariables 将领域变量加密并转换为 DAO 物理变量。
func toDAOVariables(crypto cryptox.Crypto, variables []domain.Variables) []dao.Variables {
	return slice.Map(variables, func(idx int, src domain.Variables) dao.Variables {
		val := src.Value
		if src.Secret && val != "" {
			if encVal, err := crypto.Encrypt(val); err == nil {
				val = encVal
			}
		}
		return dao.Variables{
			Key:    src.Key,
			Value:  val,
			Secret: src.Secret,
		}
	})
}

// toDomainVariables 将 DAO 物理变量解密并转换为领域变量。
func toDomainVariables(crypto cryptox.Crypto, variables []dao.Variables) []domain.Variables {
	return slice.Map(variables, func(idx int, src dao.Variables) domain.Variables {
		val := src.Value
		if src.Secret && val != "" {
			if decVal, err := crypto.Decrypt(val); err == nil {
				val = decVal
			}
		}
		return domain.Variables{
			Key:    src.Key,
			Value:  val,
			Secret: src.Secret,
		}
	})
}

// encryptVariables 加密领域敏感环境变量。
func encryptVariables(crypto cryptox.Crypto, variables []domain.Variables) []domain.Variables {
	return slice.Map(variables, func(idx int, src domain.Variables) domain.Variables {
		val := src.Value
		if src.Secret && val != "" {
			if encVal, err := crypto.Encrypt(val); err == nil {
				val = encVal
			}
		}
		return domain.Variables{
			Key:    src.Key,
			Value:  val,
			Secret: src.Secret,
		}
	})
}

// decryptVariables 解密领域敏感环境变量。
func decryptVariables(crypto cryptox.Crypto, variables []domain.Variables) []domain.Variables {
	return slice.Map(variables, func(idx int, src domain.Variables) domain.Variables {
		val := src.Value
		if src.Secret && val != "" {
			if decVal, err := crypto.Decrypt(val); err == nil {
				val = decVal
			}
		}
		return domain.Variables{
			Key:    src.Key,
			Value:  val,
			Secret: src.Secret,
		}
	})
}
