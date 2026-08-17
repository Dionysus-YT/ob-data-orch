-- 0017: 对象存储凭据（EX-I6 后续切片：存储凭据槽位）
--
-- 对象存储（OSS/S3/COS/OBS）的 access-key/secret-key 绝不进入 URI、argv、日志或快照。
-- 凭据由创建者主体管理（独立资源，不绑定数据源），access-key 与 secret-key 分别使用
-- 与数据库密码同机制的 AES-GCM 加密信封（AAD 绑定 credentialId/revision/secretType/
-- storageCredentialId），任务提交时快照只引用 storage_credential_id + revision。
--
-- storage_credentials 是主体级凭据主表；storage_credential_revisions 是独立信封表
-- （与 sys_credential_revisions 同模式，避免重建 credential_revisions 历史表约束）。

CREATE TABLE storage_credentials (
    storage_credential_id TEXT PRIMARY KEY,
    owner_subject_id TEXT NOT NULL REFERENCES auth_subjects(subject_id),
    display_name TEXT NOT NULL CHECK (length(display_name) > 0),
    provider TEXT NOT NULL CHECK (provider IN ('OSS', 'S3', 'COS', 'OBS')),
    current_revision INTEGER NOT NULL CHECK (current_revision > 0),
    revision INTEGER NOT NULL CHECK (revision > 0),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
) STRICT, WITHOUT ROWID;

CREATE TABLE storage_credential_revisions (
    credential_id TEXT NOT NULL,
    revision INTEGER NOT NULL CHECK (revision > 0),
    storage_credential_id TEXT NOT NULL REFERENCES storage_credentials(storage_credential_id),
    secret_type TEXT NOT NULL CHECK (secret_type IN ('STORAGE_ACCESS_KEY', 'STORAGE_SECRET_KEY')),
    key_id TEXT NOT NULL CHECK (length(key_id) > 0),
    nonce BLOB NOT NULL CHECK (length(nonce) > 0),
    ciphertext BLOB NOT NULL CHECK (length(ciphertext) > 0),
    aad_json TEXT NOT NULL CHECK (json_valid(aad_json)),
    status TEXT NOT NULL CHECK (status IN ('ACTIVE', 'SUPERSEDED', 'REVOKED')),
    created_at TEXT NOT NULL,
    retired_at TEXT,
    PRIMARY KEY (credential_id, revision),
    UNIQUE (storage_credential_id, revision, secret_type)
) STRICT, WITHOUT ROWID;

-- 任务快照只引用存储凭据标识与修订（与数据库凭据列同口径），
-- 供受控 Agent 在 execution 私有目录生成 core-site.xml 时解析秘密槽位。
ALTER TABLE tasks ADD COLUMN storage_credential_id TEXT;
ALTER TABLE tasks ADD COLUMN storage_credential_revision INTEGER CHECK (storage_credential_revision IS NULL OR storage_credential_revision > 0);
