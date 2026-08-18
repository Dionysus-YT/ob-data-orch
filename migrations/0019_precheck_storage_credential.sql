-- 0019: 预检查冻结存储凭据引用（EX-V1 STORAGE_AUTH 真实探测前置）
--
-- STORAGE_AUTH 凭据有效性探测需要 Agent 在预检查阶段短时解析对象存储凭据。
-- 与任务提交同口径：precheck_runs 只冻结引用标识与修订（绝不存密钥明文），
-- Agent 在同一租约秘密槽位解析内读取对应信封，AAD 绑定与执行路径一致。
-- 本地输出草稿与未绑定存储凭据的对象存储草稿保持 NULL；它们不得请求 STORAGE_CREDENTIAL 槽位。

ALTER TABLE precheck_runs ADD COLUMN storage_credential_id TEXT;
ALTER TABLE precheck_runs ADD COLUMN storage_credential_revision INTEGER CHECK (storage_credential_revision IS NULL OR storage_credential_revision > 0);
