package agentwire

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ensurePrivateDirectory 建立并复验机器身份目录的受保护权限。
// 关联状态和根密钥均位于该目录内，目录为符号链接或权限无法收紧时必须失败关闭。
func ensurePrivateDirectory(directory string) error {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create private Agent directory: %w", err)
	}
	return securePrivatePath(directory, true)
}

// writePrivateFileAtomically 使用同目录临时文件和原子替换保存加密状态。
// 在关联成功前的任何中断都保留原状态，从而可以继续使用原 requestId 重试而不丢失机器凭据。
func writePrivateFileAtomically(target string, content []byte) error {
	if err := ensurePrivateDirectory(filepath.Dir(target)); err != nil {
		return err
	}
	if info, err := os.Lstat(target); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || info.IsDir() {
			return errors.New("Agent identity target is invalid")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect Agent identity target: %w", err)
	}
	temporaryName, err := privateTemporaryName(target)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(temporaryName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create Agent identity temporary file: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
		_ = os.Remove(temporaryName)
	}()
	if err := securePrivatePath(temporaryName, false); err != nil {
		return err
	}
	if _, err := file.Write(content); err != nil {
		return errors.New("write Agent identity failed")
	}
	if err := file.Sync(); err != nil {
		return errors.New("sync Agent identity failed")
	}
	if err := file.Close(); err != nil {
		return errors.New("close Agent identity failed")
	}
	closed = true
	if err := os.Rename(temporaryName, target); err != nil {
		return errors.New("replace Agent identity failed")
	}
	if err := securePrivatePath(target, false); err != nil {
		return err
	}
	return nil
}

func privateTemporaryName(target string) (string, error) {
	randomBytes := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, randomBytes); err != nil {
		return "", errors.New("generate Agent identity temporary name failed")
	}
	defer zeroBytes(randomBytes)
	return target + ".tmp-" + fmt.Sprintf("%x", randomBytes), nil
}

func zeroBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
