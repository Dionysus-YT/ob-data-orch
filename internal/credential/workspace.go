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

// storageConfigurationPropertyNames 返回各 provider 在 Hadoop core-site.xml 中的官方密钥属性名。
// 属性名只接受四种受控 provider；未知 provider 失败关闭。
func storageConfigurationPropertyNames(provider string) (accessKeyProperty, secretKeyProperty string, ok bool) {
	switch provider {
	case "OSS":
		return "fs.oss.accessKeyId", "fs.oss.accessKeySecret", true
	case "S3":
		return "fs.s3a.access.key", "fs.s3a.secret.key", true
	case "COS":
		return "fs.cosn.userinfo.secretId", "fs.cosn.userinfo.secretKey", true
	case "OBS":
		return "fs.obs.access.key", "fs.obs.secret.key", true
	default:
		return "", "", false
	}
}

// escapeXMLValue 转义 Hadoop XML 属性值中的保留字符，防止密钥内容破坏配置文件结构。
func escapeXMLValue(value []byte) []byte {
	escaped := make([]byte, 0, len(value)+16)
	for _, char := range value {
		switch char {
		case '&':
			escaped = append(escaped, "&amp;"...)
		case '<':
			escaped = append(escaped, "&lt;"...)
		case '>':
			escaped = append(escaped, "&gt;"...)
		case '"':
			escaped = append(escaped, "&quot;"...)
		case '\'':
			escaped = append(escaped, "&apos;"...)
		default:
			escaped = append(escaped, char)
		}
	}
	return escaped
}

// GenerateStorageConfiguration 生成 Hadoop core-site.xml 内容（EX-I6 存储凭据槽位）。
// access-key 与 secret-key 只进入该 XML 值区（转义后），绝不进入 argv、日志或环境变量。
// NUL、回车与换行会破坏 XML 结构或注入伪行，一律失败关闭。
func GenerateStorageConfiguration(provider string, accessKey, secretKey []byte) ([]byte, error) {
	if len(accessKey) == 0 || len(secretKey) == 0 || bytesContainLineBreak(accessKey) || bytesContainLineBreak(secretKey) || bytesContainNul(accessKey) || bytesContainNul(secretKey) {
		return nil, errors.New("storage configuration secret is invalid")
	}
	accessKeyProperty, secretKeyProperty, ok := storageConfigurationPropertyNames(provider)
	if !ok {
		return nil, errors.New("storage configuration provider is unsupported")
	}
	content := make([]byte, 0, len(accessKey)+len(secretKey)+256)
	content = append(content, []byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<configuration>\n  <property><name>")...)
	content = append(content, []byte(accessKeyProperty)...)
	content = append(content, []byte("</name><value>")...)
	content = append(content, escapeXMLValue(accessKey)...)
	content = append(content, []byte("</value></property>\n  <property><name>")...)
	content = append(content, []byte(secretKeyProperty)...)
	content = append(content, []byte("</name><value>")...)
	content = append(content, escapeXMLValue(secretKey)...)
	content = append(content, []byte("</value></property>\n</configuration>\n")...)
	return content, nil
}

// WriteStorageConfiguration 把 core-site.xml 写入 execution 私有目录并返回 HADOOP_CONF_DIR 目标目录。
func (w Workspace) WriteStorageConfiguration(content []byte) (string, error) {
	if !w.valid() || len(content) == 0 {
		return "", errors.New("storage configuration workspace is invalid")
	}
	confDir := filepath.Join(w.securityDir, "hadoop-conf")
	if err := os.Mkdir(confDir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", errors.New("create storage configuration directory failed")
	}
	if err := writePrivateFile(filepath.Join(confDir, "core-site.xml"), content); err != nil {
		return "", err
	}
	return confDir, nil
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
	// 原始工具日志和运行时资产都只能位于 execution 私有目录；终态证据完成后的清理必须先移除其内容，
	// 否则目录非空会永久阻断材料回收，并留下原始输出或可替换的受控子进程资产。
	if err := clearPrivateDirectory(w.rawLogDir, "raw log cleanup failed"); err != nil {
		return err
	}
	if err := clearPrivateDirectory(w.runtimeDir, "runtime asset cleanup failed"); err != nil {
		return err
	}
	for _, directory := range []string{w.securityDir, w.runtimeDir, w.rawLogDir, w.evidenceDir, w.executionRoot} {
		if err := os.Remove(directory); err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.New("security workspace cleanup failed")
		}
	}
	return nil
}

func clearPrivateDirectory(directory, failureMessage string) error {
	entries, err := os.ReadDir(directory)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New(failureMessage)
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(directory, entry.Name())); err != nil {
			return errors.New(failureMessage)
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

// bytesContainNul 检测字节流中的 NUL 字符；XML 1.0 值区不允许出现 NUL。
func bytesContainNul(value []byte) bool {
	for _, character := range value {
		if character == 0 {
			return true
		}
	}
	return false
}
