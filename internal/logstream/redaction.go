package logstream

import (
	"bytes"
	"strings"
)

// Policy 定义一个执行期间短时有效的脱敏上下文。
// BytesSecrets/BytesIdentifiers 用于真实执行路径，避免把已解析秘密长期保存在字符串字段中；
// Secrets/Identifiers 仅为既有合成契约与控制面二次脱敏兼容，真实 Agent 不得使用它们保存密码。
type Policy struct {
	Version string // 策略版本必须与日志批次及凭据引用一致。
	// Secrets 与 Identifiers 仅供既有合成契约及控制面兼容路径使用。
	Secrets     []string
	Identifiers []string
	// BytesSecrets 与 BytesIdentifiers 是真实 Agent 的可清零短时敏感值。
	BytesSecrets     [][]byte
	BytesIdentifiers [][]byte
}

// NewBytePolicy 复制调用方提供的短时字节值，防止调用方后续复用缓冲区改变运行中的脱敏规则。
// 调用方在日志收尾后必须调用 Destroy 清零该策略中的副本。
func NewBytePolicy(version string, secrets, identifiers [][]byte) (Policy, error) {
	policy := Policy{Version: version, BytesSecrets: cloneValues(secrets), BytesIdentifiers: cloneValues(identifiers)}
	if err := policy.validate(); err != nil {
		policy.Destroy()
		return Policy{}, err
	}
	return policy, nil
}

// Clone 为异步采集创建独立的短时策略副本，避免调用方清理原策略时影响正在读取的管道。
func (p Policy) Clone() Policy {
	return Policy{
		Version:          p.Version,
		Secrets:          append([]string(nil), p.Secrets...),
		Identifiers:      append([]string(nil), p.Identifiers...),
		BytesSecrets:     cloneValues(p.BytesSecrets),
		BytesIdentifiers: cloneValues(p.BytesIdentifiers),
	}
}

// Destroy 清零真实执行路径保存的秘密和标识副本。
// 它不清零兼容字段中的字符串，因为 Go 字符串不可原地覆写，真实路径不得把秘密放入这些字段。
func (p *Policy) Destroy() {
	if p == nil {
		return
	}
	for _, value := range append(p.BytesSecrets, p.BytesIdentifiers...) {
		for index := range value {
			value[index] = 0
		}
	}
	p.BytesSecrets = nil
	p.BytesIdentifiers = nil
	p.Secrets = nil
	p.Identifiers = nil
	p.Version = ""
}

// Redact 保留既有基于字符串的合成调用入口。
// 真实 Agent 应使用 RedactBytes，确保原始工具记录在转为字符串前已完成脱敏。
func (p Policy) Redact(message string) (string, error) {
	raw := []byte(message)
	defer zero(raw)
	redacted, err := p.RedactBytes(raw)
	if err != nil {
		return "", err
	}
	return string(redacted), nil
}

// RedactBytes 在原始记录仍是字节时完成精确值与敏感键值脱敏。
// 返回值不包含已知秘密；调用方仍须在使用后清零输入缓冲区。
func (p Policy) RedactBytes(message []byte) ([]byte, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	redacted := append([]byte(nil), message...)
	for _, value := range p.values() {
		if !bytes.Contains(redacted, value) {
			continue
		}
		replaced := bytes.ReplaceAll(redacted, value, []byte(Mask))
		zero(redacted)
		redacted = replaced
	}
	for _, key := range [][]byte{[]byte("password"), []byte("secret"), []byte("token"), []byte("access_key")} {
		redacted = redactKeyValueBytes(redacted, key)
	}
	return redacted, nil
}

func (p Policy) validate() error {
	if strings.TrimSpace(p.Version) == "" {
		return ErrPolicyRejected
	}
	for _, value := range p.values() {
		if len(value) == 0 || bytes.ContainsAny(value, "\r\n") {
			return ErrPolicyRejected
		}
	}
	return nil
}

func (p Policy) values() [][]byte {
	values := make([][]byte, 0, len(p.Secrets)+len(p.Identifiers)+len(p.BytesSecrets)+len(p.BytesIdentifiers))
	for _, value := range append(append([]string(nil), p.Secrets...), p.Identifiers...) {
		values = append(values, []byte(value))
	}
	values = append(values, p.BytesSecrets...)
	values = append(values, p.BytesIdentifiers...)
	return values
}

func cloneValues(values [][]byte) [][]byte {
	result := make([][]byte, 0, len(values))
	for _, value := range values {
		result = append(result, append([]byte(nil), value...))
	}
	return result
}

func redactKeyValueBytes(message, key []byte) []byte {
	lower := bytes.ToLower(message)
	defer func() { zero(lower) }()
	for offset := 0; ; {
		index := bytes.Index(lower[offset:], key)
		if index < 0 {
			return message
		}
		index += offset
		cursor := index + len(key)
		for cursor < len(message) && (message[cursor] == ' ' || message[cursor] == '\t') {
			cursor++
		}
		if cursor >= len(message) || (message[cursor] != '=' && message[cursor] != ':') {
			offset = cursor
			continue
		}
		cursor++
		for cursor < len(message) && (message[cursor] == ' ' || message[cursor] == '\t') {
			cursor++
		}
		end := cursor
		for end < len(message) && !strings.ContainsRune(" \t\r\n,;", rune(message[end])) {
			end++
		}
		if end == cursor {
			offset = cursor
			continue
		}
		replaced := append(append(append([]byte(nil), message[:cursor]...), []byte(Mask)...), message[end:]...)
		zero(message)
		message = replaced
		zero(lower)
		lower = bytes.ToLower(message)
		offset = cursor + len(Mask)
	}
}
