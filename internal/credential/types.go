// Package credential contains the isolated secret-handling primitives for the
// first vertical slice. It intentionally has no HTTP, database, task, or tool
// execution entry points.
package credential

import "errors"

const (
	FormatVersion       = "credential-aes-gcm-v1"
	DatabasePassword    = "DATABASE_PASSWORD"
	SecurityPropertyKey = "oceanbase.jdbc.password"
)

var (
	ErrInvalidReference = errors.New("credential reference is invalid")
	ErrUnknownKey       = errors.New("credential key is unavailable")
	ErrDecryptDenied    = errors.New("credential decryption was rejected")
	ErrWorkspaceExists  = errors.New("execution security workspace already exists")
)

type Reference struct {
	CredentialID string
	Revision     int64
	SecretType   string
	DataSourceID string
}

// Envelope is safe to persist in credential_revisions. It never carries a
// plaintext secret or a root key.
type Envelope struct {
	FormatVersion string
	KeyID         string
	Reference     Reference
	Nonce         []byte
	Ciphertext    []byte
}

type Keyring struct {
	keys map[string][]byte
}

type Workspace struct {
	executionRoot string
	securityDir   string
	runtimeDir    string
	rawLogDir     string
	evidenceDir   string
}

// EvidenceDirectory 返回 execution 私有的无秘密证据目录。
// 调用方只能把已验证的非敏感事实写入此目录，绝不能把凭据或原始日志放入其中。
func (w Workspace) EvidenceDirectory() string {
	return w.evidenceDir
}

// RawLogDirectory 返回 execution 私有的原始工具日志目录。
// 该目录只能由受控 Agent 适配写入；完成证据后必须随工作区清理，不能暴露给浏览器或控制面。
func (w Workspace) RawLogDirectory() string {
	return w.rawLogDir
}

type MaterialPaths struct {
	SecurityConfiguration string
}

// SecurityMaterial keeps key and ciphertext bytes private to the package so
// callers cannot accidentally format them into logs or API responses.
type SecurityMaterial struct {
	privateKeyPEM []byte
	ciphertext    []byte
}

func (m *SecurityMaterial) Destroy() {
	if m == nil {
		return
	}
	Zero(m.privateKeyPEM)
	Zero(m.ciphertext)
	m.privateKeyPEM = nil
	m.ciphertext = nil
}
