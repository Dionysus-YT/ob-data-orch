-- 保留认证主体与既有授权事实，只为本轮权限编辑增加模板范围和独立版本。
CREATE TABLE subject_object_scopes_replacement (
    subject_id TEXT NOT NULL REFERENCES auth_subjects(subject_id),
    scope_type TEXT NOT NULL CHECK (scope_type IN (
        'DATA_SOURCE_USE', 'DATA_SOURCE_MANAGE', 'NODE_USE',
        'NODE_OPERATE', 'NODE_MANAGE', 'TASK_OPERATE_BY_DATA_SOURCE',
        'TEMPLATE_USE', 'TEMPLATE_MANAGE'
    )),
    object_id TEXT NOT NULL CHECK (length(object_id) > 0 AND object_id <> '*'),
    granted_by TEXT NOT NULL REFERENCES auth_subjects(subject_id),
    granted_at TEXT NOT NULL,
    PRIMARY KEY (subject_id, scope_type, object_id)
) STRICT, WITHOUT ROWID;
INSERT INTO subject_object_scopes_replacement SELECT * FROM subject_object_scopes;
DROP TABLE subject_object_scopes;
ALTER TABLE subject_object_scopes_replacement RENAME TO subject_object_scopes;

CREATE TABLE subject_permission_versions (
    subject_id TEXT PRIMARY KEY REFERENCES auth_subjects(subject_id),
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
    deployment_owner INTEGER NOT NULL DEFAULT 0 CHECK (deployment_owner IN (0, 1)),
    modified_by TEXT REFERENCES auth_subjects(subject_id),
    modified_at TEXT
) STRICT, WITHOUT ROWID;
-- 初始身份标记只防止重复引导，不由该标记授予普通角色或业务对象。
CREATE UNIQUE INDEX idx_single_permission_deployment_owner ON subject_permission_versions(deployment_owner) WHERE deployment_owner = 1;
INSERT INTO subject_permission_versions(subject_id) SELECT subject_id FROM auth_subjects;

-- 历史任务不回填权限；新任务只冻结本任务引用的范围摘要，执行仍复验当前授权。
ALTER TABLE tasks ADD COLUMN submitted_permission_revision INTEGER CHECK (submitted_permission_revision IS NULL OR submitted_permission_revision > 0);
ALTER TABLE tasks ADD COLUMN submitted_permission_scope_json TEXT CHECK (submitted_permission_scope_json IS NULL OR json_valid(submitted_permission_scope_json));
