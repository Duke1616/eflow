package hash

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"log/slog"
)

// Hash 对传入的对象进行 JSON 序列化并计算其 SHA1 哈希值，返回十六进制字符串
func Hash(x interface{}) string {
	hash := sha1.New()
	b, err := json.Marshal(x)
	if err != nil {
		slog.Error("计算哈希失败", "object", x, "error", err)
		return ""
	}
	hash.Write(b)
	return fmt.Sprintf("%x", hash.Sum(nil))
}
