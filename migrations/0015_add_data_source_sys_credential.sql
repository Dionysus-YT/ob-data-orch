-- 0015: 数据源可选 sys 凭据（--sys-user/--sys-password）
--
-- 参考 ODC 数据源实现：sys 租户账号/密码是高级设置中的可选项（拥有 sys 租户视图查看
-- 权限的账号，如 root；非必填）。本项目把 sys 凭据作为数据源可选字段，与数据库密码
-- 共用同一加密信封机制（AES-GCM + AAD 绑定 secretType），绝不进入命令行、日志、审计、
-- 响应或快照。
--
-- 迁移在事务内执行且要求 foreign_keys=ON，重建 credential_revisions（改 CHECK 约束）
-- 会触发外键失败；因此 sys 凭据使用独立表 sys_credential_revisions（无其他表引用它），
-- 历史表结构保持不变。data_sources 增加 sys_user / sys_credential_id /
-- sys_credential_revision（均可空，未配置表示该数据源没有 sys 凭据）。

CREATE TABLE sys_credential_revisions (
    credential_id TEXT NOT NULL,
    revision INTEGER NOT NULL CHECK (revision > 0),
    data_source_id TEXT NOT NULL REFERENCES data_sources(data_source_id),
    key_id TEXT NOT NULL CHECK (length(key_id) > 0),
    nonce BLOB NOT NULL CHECK (length(nonce) > 0),
    ciphertext BLOB NOT NULL CHECK (length(ciphertext) > 0),
    aad_json TEXT NOT NULL CHECK (json_valid(aad_json)),
    status TEXT NOT NULL CHECK (status IN ('ACTIVE', 'SUPERSEDED', 'REVOKED')),
    created_at TEXT NOT NULL,
    retired_at TEXT,
    PRIMARY KEY (credential_id, revision)
) STRICT, WITHOUT ROWID;

ALTER TABLE data_sources ADD COLUMN sys_user TEXT;
ALTER TABLE data_sources ADD COLUMN sys_credential_id TEXT;
ALTER TABLE data_sources ADD COLUMN sys_credential_revision INTEGER CHECK (sys_credential_revision IS NULL OR sys_credential_revision > 0);
