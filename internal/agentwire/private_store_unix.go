//go:build !windows

package agentwire

import (
	"errors"
	"fmt"
	"os"
)

// securePrivatePath 仅允许本机服务账户读写身份状态。
// Linux 目标不依赖桌面密钥服务，目录必须为 0700、文件必须为 0600，且不接受符号链接。
func securePrivatePath(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect private Agent path: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || info.IsDir() != directory {
		return errors.New("private Agent path type is invalid")
	}
	want := os.FileMode(0o600)
	if directory {
		want = 0o700
	}
	if err := os.Chmod(path, want); err != nil {
		return fmt.Errorf("restrict private Agent path permissions: %w", err)
	}
	info, err = os.Stat(path)
	if err != nil || info.Mode().Perm() != want {
		return errors.New("private Agent path permissions cannot be verified")
	}
	return nil
}
