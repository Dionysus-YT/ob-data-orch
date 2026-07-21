package credential

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type rootKeyFile struct {
	FormatVersion string `json:"formatVersion"`
	KeyID         string `json:"keyId"`
	ProtectedKey  []byte `json:"protectedKey"`
}

// LoadOrCreateRootKey uses the platform-specific protected root-key carrier.
// The caller must keep this file outside the SQLite database and its backups.
func LoadOrCreateRootKey(path, keyID string) ([]byte, error) {
	if path == "" || keyID == "" {
		return nil, errors.New("root key path and key ID are required")
	}
	if content, err := os.ReadFile(path); err == nil {
		defer Zero(content)
		return decodeRootKey(content, keyID)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read protected root key: %w", err)
	}
	if err := ensurePrivateDirectory(filepath.Dir(path)); err != nil {
		return nil, err
	}
	key, err := NewRandomRootKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	protected, err := protectRootKey(key)
	if err != nil {
		Zero(key)
		return nil, err
	}
	record, err := json.Marshal(rootKeyFile{FormatVersion: "root-key-v1", KeyID: keyID, ProtectedKey: protected})
	Zero(protected)
	if err != nil {
		Zero(key)
		return nil, fmt.Errorf("encode protected root key: %w", err)
	}
	if err := writePrivateFile(path, record); err != nil {
		Zero(key)
		if errors.Is(err, os.ErrExist) {
			content, readErr := os.ReadFile(path)
			if readErr == nil {
				defer Zero(content)
				return decodeRootKey(content, keyID)
			}
		}
		return nil, err
	}
	return key, nil
}

func decodeRootKey(content []byte, expectedKeyID string) ([]byte, error) {
	var record rootKeyFile
	if err := json.Unmarshal(content, &record); err != nil {
		return nil, errors.New("protected root key file is invalid")
	}
	if record.FormatVersion != "root-key-v1" || record.KeyID != expectedKeyID || len(record.ProtectedKey) == 0 {
		return nil, errors.New("protected root key identity is invalid")
	}
	key, err := unprotectRootKey(record.ProtectedKey)
	Zero(record.ProtectedKey)
	if err != nil || len(key) != 32 {
		Zero(key)
		return nil, errors.New("protected root key cannot be recovered")
	}
	return key, nil
}
