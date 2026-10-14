package controlplane

import (
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/store"
)

// decryptExportSysCredential 只能在调用方已复验所有者与租约后使用，不保存明文。
func (s *Server) decryptExportSysCredential(material *store.EncryptedSysCredential, dataSourceID string) ([]byte, error) {
	if material == nil {
		return nil, nil
	}
	return s.decryptor.Decrypt(credential.Envelope{
		FormatVersion: credential.FormatVersion, KeyID: material.KeyID,
		Reference: credential.Reference{CredentialID: material.CredentialID, Revision: material.Revision, SecretType: credential.SysPassword, DataSourceID: dataSourceID},
		Nonce:     material.Nonce, Ciphertext: material.Ciphertext,
	})
}

// exportConnectionPayload 在响应写出期间短时引用秘密；无 sys 参数时保持原协议形状。
func exportConnectionPayload(host string, port int, username, password []byte, sys *store.EncryptedSysCredential, sysPassword []byte) map[string]any {
	payload := map[string]any{"host": host, "port": port, "username": username, "password": password}
	if sys != nil {
		payload["sysUsername"], payload["sysPassword"] = sys.Username, sysPassword
	}
	return payload
}
