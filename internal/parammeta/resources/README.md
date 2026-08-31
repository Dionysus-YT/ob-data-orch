# 参数元数据资源

本目录保存首条切片的不可变参数元数据及兼容修订：

- `obdumper-4.3.5-slice-v1.json`：DEV-02 发布的原始基线，不再原地修改；
- `obdumper-4.3.5-slice-v2.json`：历史修订清单，以 SHA-256 固定 `v1`，只把数据库密码的发射目标从 argv 改为官方安全文件属性 `oceanbase.jdbc.password`。
- `obdumper-4.3.5-slice-v3.json`：历史修订清单，以 SHA-256 固定 `v1`，保留 v2 的密码安全覆盖，并将首条切片能力版本切换为仅私有 ODP。
- `obdumper-4.3.5-slice-v4.json`：历史修订清单，保留私有 ODP 与密码安全文件边界，并以本机 4.3.5 帮助核验的短参数生成连接预览。
- `obdumper-4.3.5-slice-v5.json`～`v7.json`：当前 Export 泛化能力链，覆盖格式、对象范围、输出、过滤、DDL、性能和风险门禁的版本化映射；旧版本不可原地修改。
- `../drafts/obdumper-4.3.5-slice-v8.draft.json`：2026-08-22 六步向导与参数分层目标草案，仅用于契约评审，不进入运行时目录或 catalog。它收缩普通新建格式、移除 query-sql 敏感门禁并修正 retain-empty-files 活动范围；完成代码、schema 和兼容回归前不得发布为资源文件。

新增修订时必须创建新版本资源并保留历史文件。不得改写旧资源以改变历史任务语义，也不得在修订清单中放入任务值或秘密原值。
