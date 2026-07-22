package credential

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var executionIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

func CreateWorkspace(root, executionID string) (Workspace, error) {
	if root == "" || !executionIDPattern.MatchString(executionID) {
		return Workspace{}, errors.New("execution security workspace identity is invalid")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Workspace{}, fmt.Errorf("resolve security root: %w", err)
	}
	if err := ensurePrivateDirectory(absRoot); err != nil {
		return Workspace{}, err
	}
	executionsRoot := filepath.Join(absRoot, "executions")
	if err := ensurePrivateDirectory(executionsRoot); err != nil {
		return Workspace{}, err
	}
	executionRoot := filepath.Join(executionsRoot, executionID)
	if err := os.Mkdir(executionRoot, 0o700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return Workspace{}, ErrWorkspaceExists
		}
		return Workspace{}, fmt.Errorf("create execution security workspace: %w", err)
	}
	workspace := Workspace{
		executionRoot: executionRoot,
		securityDir:   filepath.Join(executionRoot, "security"),
		runtimeDir:    filepath.Join(executionRoot, "runtime"),
		rawLogDir:     filepath.Join(executionRoot, "tool-raw-log"),
		evidenceDir:   filepath.Join(executionRoot, "evidence"),
	}
	for _, directory := range []string{workspace.executionRoot, workspace.securityDir, workspace.runtimeDir, workspace.rawLogDir, workspace.evidenceDir} {
		if directory != workspace.executionRoot {
			if err := os.Mkdir(directory, 0o700); err != nil {
				_ = workspace.Cleanup()
				return Workspace{}, fmt.Errorf("create execution private directory: %w", err)
			}
		}
		if err := securePrivatePath(directory, true); err != nil {
			_ = workspace.Cleanup()
			return Workspace{}, err
		}
	}
	return workspace, nil
}

func GenerateSecurityMaterial(secret []byte) (SecurityMaterial, error) {
	if len(secret) == 0 || bytesContainLineBreak(secret) {
		return SecurityMaterial{}, errors.New("security material secret is invalid")
	}
	payload := append([]byte(SecurityPropertyKey+"="), secret...)
	payload = append(payload, '\n')
	keyBits := len(payload)*8 + 1024
	privateKey, err := rsa.GenerateKey(rand.Reader, keyBits)
	if err != nil {
		Zero(payload)
		return SecurityMaterial{}, fmt.Errorf("generate task RSA key: %w", err)
	}
	ciphertext, err := rsa.EncryptPKCS1v15(rand.Reader, &privateKey.PublicKey, payload)
	Zero(payload)
	if err != nil {
		return SecurityMaterial{}, fmt.Errorf("encrypt task security material: %w", err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		Zero(ciphertext)
		return SecurityMaterial{}, fmt.Errorf("encode task private key: %w", err)
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})
	Zero(pkcs8)
	if len(privatePEM) == 0 {
		Zero(ciphertext)
		return SecurityMaterial{}, errors.New("encode task private key failed")
	}
	return SecurityMaterial{privateKeyPEM: privatePEM, ciphertext: ciphertext}, nil
}

func (w Workspace) WriteSecurityMaterial(material *SecurityMaterial) (MaterialPaths, error) {
	if !w.valid() || material == nil || len(material.privateKeyPEM) == 0 || len(material.ciphertext) == 0 {
		return MaterialPaths{}, errors.New("security material workspace is invalid")
	}
	keyPath := filepath.Join(w.securityDir, "key.pem")
	cipherPath := filepath.Join(w.securityDir, "secure.rsa")
	configPath := filepath.Join(w.securityDir, "security.properties")
	if err := writePrivateFile(keyPath, material.privateKeyPEM); err != nil {
		return MaterialPaths{}, err
	}
	if err := writePrivateFile(cipherPath, material.ciphertext); err != nil {
		_ = os.Remove(keyPath)
		return MaterialPaths{}, err
	}
	config := "encrypt.filePath=" + javaPath(cipherPath) + "\n" +
		"secretKey.filePath=" + javaPath(keyPath) + "\n"
	if err := writePrivateFile(configPath, []byte(config)); err != nil {
		_ = os.Remove(cipherPath)
		_ = os.Remove(keyPath)
		return MaterialPaths{}, err
	}
	return MaterialPaths{SecurityConfiguration: configPath}, nil
}

// OwnsSecurityConfiguration 判断给定路径是否正好是当前 execution 的私有安全配置。
// 启动适配不能接受调用方提供的任意配置路径，否则会把共享或外部文件错误地交给工具进程。
func (w Workspace) OwnsSecurityConfiguration(path string) bool {
	if !w.valid() || path == "" {
		return false
	}
	return filepath.Clean(path) == filepath.Join(w.securityDir, "security.properties")
}

func (w Workspace) Cleanup() error {
	if !w.valid() {
		return errors.New("security workspace cleanup target is invalid")
	}
	for _, target := range []string{
		filepath.Join(w.securityDir, "security.properties"),
		filepath.Join(w.securityDir, "secure.rsa"),
		filepath.Join(w.securityDir, "key.pem"),
	} {
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.New("security material cleanup failed")
		}
	}
	// 原始工具日志只能位于 execution 私有目录；终态证据完成后的清理必须先移除其内容，
	// 否则目录非空会永久阻断材料回收并留下不应长期保留的原始输出。
	entries, err := os.ReadDir(w.rawLogDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("raw log cleanup failed")
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(w.rawLogDir, entry.Name())); err != nil {
			return errors.New("raw log cleanup failed")
		}
	}
	for _, directory := range []string{w.securityDir, w.runtimeDir, w.rawLogDir, w.evidenceDir, w.executionRoot} {
		if err := os.Remove(directory); err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.New("security workspace cleanup failed")
		}
	}
	return nil
}

func (w Workspace) valid() bool {
	if w.executionRoot == "" || w.securityDir == "" {
		return false
	}
	return filepath.Dir(w.securityDir) == w.executionRoot &&
		filepath.Dir(w.runtimeDir) == w.executionRoot &&
		filepath.Dir(w.rawLogDir) == w.executionRoot &&
		filepath.Dir(w.evidenceDir) == w.executionRoot
}

func ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create private directory: %w", err)
	}
	return securePrivatePath(path, true)
}

func writePrivateFile(path string, content []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create private security material: %w", err)
	}
	defer file.Close()
	if err := securePrivatePath(path, false); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return errors.New("write private security material failed")
	}
	if err := file.Sync(); err != nil {
		return errors.New("sync private security material failed")
	}
	return nil
}

func javaPath(path string) string {
	return strings.ReplaceAll(path, `\`, "/")
}

func bytesContainLineBreak(value []byte) bool {
	for _, character := range value {
		if character == '\r' || character == '\n' {
			return true
		}
	}
	return false
}
