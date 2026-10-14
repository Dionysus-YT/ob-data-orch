package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"ob-data-orch/internal/credential"
)

// EncryptedSysCredential 是冻结 sys 槽位的加密材料；只在控制面内部短时持有。
type EncryptedSysCredential struct {
	CredentialID string
	Revision     int64
	Username     []byte
	KeyID        string
	Nonce        []byte
	Ciphertext   []byte
}

// Destroy 清除加密材料和短时身份缓冲，避免后续响应或日志意外持有。
func (c *EncryptedSysCredential) Destroy() {
	if c == nil {
		return
	}
	credential.Zero(c.Username)
	credential.Zero(c.Nonce)
	credential.Zero(c.Ciphertext)
	c.Username, c.Nonce, c.Ciphertext = nil, nil, nil
}

type frozenSysCredential struct {
	CredentialID string `json:"credentialId"`
	Revision     int64  `json:"revision"`
}

// FreezeTaskSysCredential 只向任务内部快照添加引用，不向草稿或浏览器投影添加 sys 标识。
// 任务读取使用既有最小安全快照；从头执行与模板只复用产品配置，再复核当前凭据。
func FreezeTaskSysCredential(snapshot string, credentialID string, revision int64) (string, error) {
	if credentialID == "" || revision < 1 {
		return "", errors.New("task sys credential reference is invalid")
	}
	var payload map[string]json.RawMessage
	if json.Unmarshal([]byte(snapshot), &payload) != nil || payload == nil {
		return "", errors.New("task snapshot is invalid")
	}
	reference, err := json.Marshal(frozenSysCredential{CredentialID: credentialID, Revision: revision})
	if err != nil {
		return "", err
	}
	payload["_sysCredential"] = reference
	content, err := json.Marshal(payload)
	return string(content), err
}

func snapshotSysCredential(snapshot string) (frozenSysCredential, bool) {
	var payload struct {
		SysCredential *frozenSysCredential `json:"_sysCredential"`
		Config        *ExportConfig        `json:"config"`
	}
	if json.Unmarshal([]byte(snapshot), &payload) != nil {
		return frozenSysCredential{}, false
	}
	requiresSys := payload.Config != nil && payload.Config.DDLBehavior.AddExtraMessage != nil && *payload.Config.DDLBehavior.AddExtraMessage
	if !requiresSys {
		return frozenSysCredential{}, payload.SysCredential == nil
	}
	if payload.SysCredential == nil || payload.SysCredential.CredentialID == "" || payload.SysCredential.Revision < 1 {
		return frozenSysCredential{}, false
	}
	return *payload.SysCredential, true
}

func readFrozenSysCredential(ctx context.Context, tx *sql.Tx, dataSourceID string, reference frozenSysCredential) (*EncryptedSysCredential, error) {
	if reference.CredentialID == "" && reference.Revision == 0 {
		return nil, nil
	}
	var username, clusterName string
	material := &EncryptedSysCredential{CredentialID: reference.CredentialID, Revision: reference.Revision}
	err := tx.QueryRowContext(ctx, `
		SELECT ds.sys_user, ds.cluster_name, cr.key_id, cr.nonce, cr.ciphertext
		FROM data_sources AS ds JOIN sys_credential_revisions AS cr
		  ON cr.credential_id = ds.sys_credential_id AND cr.revision = ds.sys_credential_revision
		 AND cr.data_source_id = ds.data_source_id
		WHERE ds.data_source_id = ? AND ds.state = 'ENABLED'
		  AND ds.sys_credential_id = ? AND ds.sys_credential_revision = ? AND cr.status = 'ACTIVE'
	`, dataSourceID, reference.CredentialID, reference.Revision).Scan(&username, &clusterName, &material.KeyID, &material.Nonce, &material.Ciphertext)
	if err != nil {
		material.Destroy()
		return nil, ErrPrecheckLeaseRejected
	}
	var ok bool
	material.Username, ok = composePrivateODPJDBCIdentity(username, "sys", clusterName)
	if !ok || material.KeyID == "" || len(material.Nonce) == 0 || len(material.Ciphertext) == 0 {
		material.Destroy()
		return nil, ErrPrecheckLeaseRejected
	}
	return material, nil
}
