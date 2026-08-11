package credential

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
)

func NewKeyring(keys map[string][]byte) (*Keyring, error) {
	if len(keys) == 0 {
		return nil, errors.New("credential keyring is empty")
	}
	result := &Keyring{keys: make(map[string][]byte, len(keys))}
	for keyID, key := range keys {
		if keyID == "" || len(key) != 32 {
			return nil, errors.New("credential keyring contains an invalid key")
		}
		result.keys[keyID] = append([]byte(nil), key...)
	}
	return result, nil
}

func NewRandomRootKey(reader io.Reader) ([]byte, error) {
	if reader == nil {
		return nil, errors.New("root key random source is nil")
	}
	key := make([]byte, 32)
	if _, err := io.ReadFull(reader, key); err != nil {
		return nil, fmt.Errorf("generate root key: %w", err)
	}
	return key, nil
}

func (k *Keyring) Encrypt(keyID string, reference Reference, plaintext []byte) (Envelope, error) {
	if err := validateReference(reference); err != nil {
		return Envelope{}, err
	}
	if len(plaintext) == 0 {
		return Envelope{}, errors.New("credential plaintext is empty")
	}
	key, ok := k.lookup(keyID)
	if !ok {
		return Envelope{}, ErrUnknownKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return Envelope{}, fmt.Errorf("create credential cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return Envelope{}, fmt.Errorf("create credential GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return Envelope{}, fmt.Errorf("generate credential nonce: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, aad(reference))
	return Envelope{
		FormatVersion: FormatVersion,
		KeyID:         keyID,
		Reference:     reference,
		Nonce:         nonce,
		Ciphertext:    ciphertext,
	}, nil
}

func (k *Keyring) Decrypt(envelope Envelope) ([]byte, error) {
	if envelope.FormatVersion != FormatVersion || validateReference(envelope.Reference) != nil || len(envelope.Nonce) == 0 || len(envelope.Ciphertext) == 0 {
		return nil, ErrDecryptDenied
	}
	key, ok := k.lookup(envelope.KeyID)
	if !ok {
		return nil, ErrUnknownKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrDecryptDenied
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(envelope.Nonce) != gcm.NonceSize() {
		return nil, ErrDecryptDenied
	}
	plaintext, err := gcm.Open(nil, envelope.Nonce, envelope.Ciphertext, aad(envelope.Reference))
	if err != nil {
		return nil, ErrDecryptDenied
	}
	return plaintext, nil
}

func (k *Keyring) KeyIDs() []string {
	result := make([]string, 0, len(k.keys))
	for keyID := range k.keys {
		result = append(result, keyID)
	}
	sort.Strings(result)
	return result
}

func Zero(value []byte) {
	for index := range value {
		value[index] = 0
	}
}

func (k *Keyring) lookup(keyID string) ([]byte, bool) {
	if k == nil || keyID == "" {
		return nil, false
	}
	key, ok := k.keys[keyID]
	return key, ok
}

func validateReference(reference Reference) error {
	// 数据库密码与可选的 sys 凭据都走同一加密信封；其他秘密类型一律拒绝。
	if reference.CredentialID == "" || reference.Revision < 1 || (reference.SecretType != DatabasePassword && reference.SecretType != SysPassword) || reference.DataSourceID == "" {
		return ErrInvalidReference
	}
	return nil
}

func aad(reference Reference) []byte {
	value, err := json.Marshal(struct {
		FormatVersion string `json:"formatVersion"`
		CredentialID  string `json:"credentialId"`
		Revision      int64  `json:"revision"`
		SecretType    string `json:"secretType"`
		DataSourceID  string `json:"dataSourceId"`
	}{
		FormatVersion: FormatVersion,
		CredentialID:  reference.CredentialID,
		Revision:      reference.Revision,
		SecretType:    reference.SecretType,
		DataSourceID:  reference.DataSourceID,
	})
	if err != nil {
		panic("credential AAD serialization is fixed and cannot fail")
	}
	return value
}
