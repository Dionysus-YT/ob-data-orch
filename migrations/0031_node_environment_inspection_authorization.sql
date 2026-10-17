-- 固定本机复验授权随当前机器身份保存，存量身份与旧注册码默认不扩大授权。
ALTER TABLE agents ADD COLUMN environment_inspection_authorized INTEGER NOT NULL DEFAULT 0
    CHECK (environment_inspection_authorized IN (0, 1));
ALTER TABLE agent_enrollment_tokens ADD COLUMN environment_inspection_authorized INTEGER NOT NULL DEFAULT 0
    CHECK (environment_inspection_authorized IN (0, 1));
