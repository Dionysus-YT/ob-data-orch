package logstream

import "strings"

// Policy contains exact secrets and identity values known only while an
// execution is active. It never exposes these values after construction.
type Policy struct {
	Version     string
	Secrets     []string
	Identifiers []string
}

func (p Policy) Redact(message string) (string, error) {
	if strings.TrimSpace(p.Version) == "" {
		return "", ErrPolicyRejected
	}
	redacted := message
	for _, secret := range append(append([]string(nil), p.Secrets...), p.Identifiers...) {
		if strings.TrimSpace(secret) == "" {
			return "", ErrPolicyRejected
		}
		redacted = strings.ReplaceAll(redacted, secret, Mask)
	}
	for _, key := range []string{"password", "secret", "token", "access_key"} {
		redacted = redactKeyValue(redacted, key)
	}
	return redacted, nil
}

func redactKeyValue(message, key string) string {
	lower := strings.ToLower(message)
	for offset := 0; ; {
		index := strings.Index(lower[offset:], key)
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
		message = message[:cursor] + Mask + message[end:]
		lower = strings.ToLower(message)
		offset = cursor + len(Mask)
	}
}
