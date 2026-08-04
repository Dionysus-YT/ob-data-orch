// Package identifier 提供跨边界标识符的受控生成规则。
package identifier

import (
	"crypto/rand"
	"encoding/hex"
	"io"
)

// NewUUIDV4 使用系统加密随机源生成规范小写 UUIDv4。
// 外部 API 标识符不得回退为 SQLite rowid、计数器或可预测文本；随机源不可用时由调用方失败关闭。
func NewUUIDV4() (string, error) {
	return newUUIDV4(rand.Reader)
}

// IsCanonicalUUIDV4 判断值是否为规范小写 UUIDv4。
// 该校验只用于新生成标识与明确要求 UUIDv4 的边界；历史不透明标识必须由对应协议版本单独兼容，不能静默重写持久化关联。
func IsCanonicalUUIDV4(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' || value[14] != '4' {
		return false
	}
	if value[19] != '8' && value[19] != '9' && value[19] != 'a' && value[19] != 'b' {
		return false
	}
	for index := range len(value) {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			continue
		}
		character := value[index]
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return false
		}
	}
	return true
}

func newUUIDV4(reader io.Reader) (string, error) {
	value := make([]byte, 16)
	if _, err := io.ReadFull(reader, value); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(value)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32], nil
}
