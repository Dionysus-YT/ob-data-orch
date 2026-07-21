//go:build !windows

package credential

import (
	"errors"
	"fmt"
	"os"
)

func protectRootKey(value []byte) ([]byte, error) {
	return append([]byte(nil), value...), nil
}

func unprotectRootKey(value []byte) ([]byte, error) {
	if len(value) != 32 {
		return nil, errors.New("protected root key is invalid")
	}
	return append([]byte(nil), value...), nil
}

func securePrivatePath(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect private path: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || info.IsDir() != directory {
		return errors.New("private path type is invalid")
	}
	want := os.FileMode(0o600)
	if directory {
		want = 0o700
	}
	if err := os.Chmod(path, want); err != nil {
		return fmt.Errorf("restrict private path permissions: %w", err)
	}
	info, err = os.Stat(path)
	if err != nil || info.Mode().Perm() != want {
		return errors.New("private path permissions cannot be verified")
	}
	return nil
}
