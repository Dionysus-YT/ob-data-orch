package credential

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAESGCMEnvelopeBindsReferenceAndRejectsTampering(t *testing.T) {
	key := randomBytes(t, 32)
	keyring, err := NewKeyring(map[string][]byte{"key-v1": key})
	if err != nil {
		t.Fatalf("NewKeyring(): %v", err)
	}
	secret := randomSecret(t)
	defer Zero(secret)
	reference := Reference{CredentialID: "credential-1", Revision: 1, SecretType: DatabasePassword, DataSourceID: "source-1"}
	first, err := keyring.Encrypt("key-v1", reference, secret)
	if err != nil {
		t.Fatalf("Encrypt(first): %v", err)
	}
	second, err := keyring.Encrypt("key-v1", reference, secret)
	if err != nil {
		t.Fatalf("Encrypt(second): %v", err)
	}
	if bytes.Equal(first.Nonce, second.Nonce) || bytes.Equal(first.Ciphertext, second.Ciphertext) {
		t.Fatal("same secret was encrypted with repeated material")
	}
	plaintext, err := keyring.Decrypt(first)
	if err != nil {
		t.Fatalf("Decrypt(): %v", err)
	}
	defer Zero(plaintext)
	if !bytes.Equal(plaintext, secret) {
		t.Fatal("decrypted secret does not match")
	}

	tests := []struct {
		name string
		edit func(*Envelope)
	}{
		{name: "credential ID", edit: func(value *Envelope) { value.Reference.CredentialID = "credential-2" }},
		{name: "revision", edit: func(value *Envelope) { value.Reference.Revision = 2 }},
		{name: "data source", edit: func(value *Envelope) { value.Reference.DataSourceID = "source-2" }},
		{name: "nonce", edit: func(value *Envelope) { value.Nonce[0] ^= 0x01 }},
		{name: "ciphertext", edit: func(value *Envelope) { value.Ciphertext[0] ^= 0x01 }},
		{name: "format", edit: func(value *Envelope) { value.FormatVersion = "other" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tampered := cloneEnvelope(first)
			test.edit(&tampered)
			if _, err := keyring.Decrypt(tampered); !errors.Is(err, ErrDecryptDenied) {
				t.Fatalf("Decrypt() error = %v, want ErrDecryptDenied", err)
			}
		})
	}
	unknown := cloneEnvelope(first)
	unknown.KeyID = "unknown-key"
	if _, err := keyring.Decrypt(unknown); !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("unknown key error = %v, want ErrUnknownKey", err)
	}
	Zero(key)
}

func TestProtectedRootKeyRoundTripAndCorruptionFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "security", "root-key.json")
	first, err := LoadOrCreateRootKey(path, "key-v1")
	if err != nil {
		t.Fatalf("LoadOrCreateRootKey(first): %v", err)
	}
	defer Zero(first)
	second, err := LoadOrCreateRootKey(path, "key-v1")
	if err != nil {
		t.Fatalf("LoadOrCreateRootKey(second): %v", err)
	}
	defer Zero(second)
	if !bytes.Equal(first, second) {
		t.Fatal("root key changed after reload")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read protected root key: %v", err)
	}
	if runtime.GOOS == "windows" && bytes.Contains(content, first) {
		t.Fatal("Windows DPAPI root key file contains raw root key")
	}
	if err := os.WriteFile(path, []byte(`{"formatVersion":"broken"}`), 0o600); err != nil {
		t.Fatalf("corrupt protected root key: %v", err)
	}
	if _, err := LoadOrCreateRootKey(path, "key-v1"); err == nil {
		t.Fatal("corrupt root key unexpectedly loaded")
	}
}

func TestSecurityWorkspaceContainsOnlyEncryptedMaterialAndCleansUp(t *testing.T) {
	root := filepath.Join(t.TempDir(), "agent-security")
	workspace, err := CreateWorkspace(root, "execution-1")
	if err != nil {
		t.Fatalf("CreateWorkspace(): %v", err)
	}
	if _, err := CreateWorkspace(root, "execution-1"); !errors.Is(err, ErrWorkspaceExists) {
		t.Fatalf("workspace reuse error = %v, want ErrWorkspaceExists", err)
	}
	if _, err := CreateWorkspace(root, "../escape"); err == nil {
		t.Fatal("path traversal workspace unexpectedly succeeded")
	}

	secret := randomSecret(t)
	defer Zero(secret)
	material, err := GenerateSecurityMaterial(secret)
	if err != nil {
		t.Fatalf("GenerateSecurityMaterial(): %v", err)
	}
	second, err := GenerateSecurityMaterial(secret)
	if err != nil {
		t.Fatalf("GenerateSecurityMaterial(second): %v", err)
	}
	if bytes.Equal(material.privateKeyPEM, second.privateKeyPEM) || bytes.Equal(material.ciphertext, second.ciphertext) {
		t.Fatal("two execution materials were not independently randomized")
	}
	paths, err := workspace.WriteSecurityMaterial(&material)
	if err != nil {
		t.Fatalf("WriteSecurityMaterial(): %v", err)
	}
	if strings.Contains(paths.SecurityConfiguration, string(secret)) {
		t.Fatal("security configuration path contains a secret")
	}
	assertSecurityMaterialDecrypts(t, material, secret)
	for _, path := range []string{
		filepath.Join(workspace.securityDir, "key.pem"),
		filepath.Join(workspace.securityDir, "secure.rsa"),
		paths.SecurityConfiguration,
	} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read security material %q: %v", path, err)
		}
		if bytes.Contains(content, secret) {
			t.Fatalf("security material %q contains plaintext secret", path)
		}
		if runtime.GOOS != "windows" {
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0o600 {
				t.Fatalf("private file permissions for %q are not 0600", path)
			}
		}
	}
	if err := workspace.Cleanup(); err != nil {
		t.Fatalf("Cleanup(): %v", err)
	}
	material.Destroy()
	if material.privateKeyPEM != nil || material.ciphertext != nil {
		t.Fatal("security material retained bytes after destruction")
	}
	if _, err := os.Stat(workspace.executionRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("execution workspace still exists after cleanup: %v", err)
	}
}

func TestSecurityMaterialRejectsLineBreakSecret(t *testing.T) {
	if _, err := GenerateSecurityMaterial([]byte{'a', '\n', 'b'}); err == nil {
		t.Fatal("line-break secret unexpectedly accepted")
	}
}

func assertSecurityMaterialDecrypts(t *testing.T, material SecurityMaterial, secret []byte) {
	t.Helper()
	block, _ := pem.Decode(material.privateKeyPEM)
	if block == nil {
		t.Fatal("private key PEM cannot be decoded")
	}
	privateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		t.Fatalf("parse private key: %v", err)
	}
	rsaKey, ok := privateKey.(*rsa.PrivateKey)
	if !ok {
		t.Fatal("private key is not RSA")
	}
	payload, err := rsa.DecryptPKCS1v15(rand.Reader, rsaKey, material.ciphertext)
	if err != nil {
		t.Fatalf("decrypt security material: %v", err)
	}
	defer Zero(payload)
	want := append([]byte(SecurityPropertyKey+"="), secret...)
	want = append(want, '\n')
	if !bytes.Equal(payload, want) {
		t.Fatal("security material payload does not match")
	}
}

func randomBytes(t *testing.T, size int) []byte {
	t.Helper()
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		t.Fatalf("random bytes: %v", err)
	}
	return value
}

func randomSecret(t *testing.T) []byte {
	t.Helper()
	return []byte(hex.EncodeToString(randomBytes(t, 24)))
}

func cloneEnvelope(input Envelope) Envelope {
	result := input
	result.Nonce = append([]byte(nil), input.Nonce...)
	result.Ciphertext = append([]byte(nil), input.Ciphertext...)
	return result
}
