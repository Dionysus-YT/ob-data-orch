package identifier

import (
	"encoding/hex"
	"errors"
	"testing"
)

func TestNewUUIDV4ReturnsCanonicalRandomUUID(t *testing.T) {
	seen := make(map[string]struct{})
	for range 32 {
		value, err := NewUUIDV4()
		if err != nil {
			t.Fatalf("NewUUIDV4() error = %v", err)
		}
		if !IsCanonicalUUIDV4(value) {
			t.Fatalf("UUIDv4 格式无效: %q", value)
		}
		compact := value[0:8] + value[9:13] + value[14:18] + value[19:23] + value[24:36]
		if _, err := hex.DecodeString(compact); err != nil {
			t.Fatalf("UUID 十六进制无效: %q, %v", value, err)
		}
		if _, exists := seen[value]; exists {
			t.Fatalf("UUID 重复: %q", value)
		}
		seen[value] = struct{}{}
	}
}

func TestIsCanonicalUUIDV4RejectsNonCanonicalValues(t *testing.T) {
	for _, value := range []string{
		"",
		"7a1cae5e-b9cd-11ef-8000-000000000000",
		"7A1CAE5E-B9CD-4EEF-8000-000000000000",
		"7a1cae5e-b9cd-4eef-7000-000000000000",
		"7a1cae5e-b9cd-4eef-c000-000000000000",
		"7a1cae5e-b9cd-4eef-8000-00000000000z",
	} {
		if IsCanonicalUUIDV4(value) {
			t.Fatalf("IsCanonicalUUIDV4(%q) = true，期望 false", value)
		}
	}
}

func TestNewUUIDV4FailsClosedWhenEntropyUnavailable(t *testing.T) {
	if _, err := newUUIDV4(errorReader{}); !errors.Is(err, errEntropyUnavailable) {
		t.Fatalf("newUUIDV4() error = %v，期望熵源错误", err)
	}
}

var errEntropyUnavailable = errors.New("entropy unavailable")

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errEntropyUnavailable
}
