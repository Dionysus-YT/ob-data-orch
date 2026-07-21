# SQLite 跨平台最小技术验证（2026-07-21）

> 证据状态：Windows AMD64 构建与运行通过；Linux AMD64/ARM64 构建通过、目标麒麟运行待执行
> 验证范围：纯 Go SQLite 驱动、关闭 CGO、三目标构建、Windows 基础事务与备份
> 非目标：业务数据模型、任务调度实现、性能压测和国产 Linux 认证

## 1. 验证目的

确认 SQLite 轻量路线不会破坏以下平台约束：

- 同一份 Go 代码能生成 Windows AMD64、Linux AMD64、Linux ARM64 产物；
- 国产 ARM Linux 不需要为平台 SQLite 额外安装 C 编译器或 SQLite 服务；
- 最小事务、WAL、外键、完整性检查和一致备份行为成立；
- 编译成本和产物大小处于轻量项目可接受范围。

本验证位于 Git 忽略的 `tmp/sqlite-cross-platform-spike/`，没有创建或提交业务代码、正式数据库或部署配置。

## 2. 验证输入

| 项目 | 值 |
|---|---|
| Go | `go1.26.4 windows/amd64` |
| CGO | 三个目标均为 `CGO_ENABLED=0` |
| SQLite 驱动候选 | `modernc.org/sqlite v1.54.0` |
| 驱动模块摘要 | `h1:JCxR4qwkJvOaqAoYcgDoO25Nc+ROg6EJ2LfBVzdrgog=` |
| 夹具 `main.go` SHA-256 | `38559B964104A4B648C36E0083EC34900626408E597D9DACE6A43414F7808975` |
| 夹具 `go.mod` SHA-256 | `1600C6AD15E0CD5E0EA81FD02BD4E8E472B69347450931038A488DBCBCC01CAC` |
| 夹具 `go.sum` SHA-256 | `66DB7CFA01866EB864E367E804B69EBAD9B716CB1C73B1D5558C081987C658BF` |

夹具只创建任务与事件两张最小关系表，用于验证外键、提交、回滚和备份；这些表不构成正式数据模型。

## 3. 构建结果

| 目标 | 结果 | 大小 | SHA-256 | 首次独立/并行构建耗时 |
|---|---|---:|---|---:|
| Windows AMD64 | 通过 | 10,248,192 字节 | `E0BB483A8E8A305D9AAC0BE65B0BFE542A73369861759142E56DB66483050582` | 35.67 秒 |
| Linux AMD64 | 通过 | 9,896,616 字节 | `85ACD22766A0F319D64E107C8EA99C683CFCD3001775305E10FF098889479592` | 95.96 秒 |
| Linux ARM64 | 通过 | 9,695,904 字节 | `6E821E872256ED827872739396A08379166CF21242103BA721D3C7F22B124F38` | 113.73 秒 |

两个 Linux 构建并行执行，因此耗时只用于识别首次编译成本，不能作为性能基准。构建信息已分别核对 `GOOS`、`GOARCH` 和 `CGO_ENABLED=0`。

纯 Go SQLite 会增加约 9～10 MB 二进制体积，首次跨架构编译相对较慢；这些成本发生在构建和发布阶段，不要求目标服务器安装 SQLite 服务或 C 工具链。当前成本可接受，后续 CI 应复用 Go 构建缓存。

## 4. Windows AMD64 运行结果

Windows 产物在本机实际运行并返回：

```json
{
  "goos": "windows",
  "goarch": "amd64",
  "sqlite_version": "3.53.3",
  "journal_mode": "wal",
  "foreign_keys": 1,
  "integrity_check": "ok",
  "committed_rows": 1,
  "backup_rows": 1
}
```

已验证：

- 数据库和关系表创建成功；
- WAL 生效；
- 外键约束已开启；
- 已提交事务保留一行，回滚事务未产生第二行；
- `PRAGMA integrity_check` 返回 `ok`；
- `VACUUM INTO` 生成一致备份，备份中的提交行数与源库一致；
- 运行退出码为 `0`。

## 5. 尚未证明

交叉编译通过只证明平台产物可以生成，不证明以下目标已经认证：

- `OS-KY10-ARM`：麒麟 V10 SP1 + Kunpeng 920；
- `OS-KY11-ARM`：麒麟 V11 2503 + Kunpeng 920；
- `OS-KY10-C86`：麒麟 V10 SP3 2403 + Hygon C86；
- systemd 安装、文件权限、服务用户、信号和崩溃恢复；
- 各目标系统上的 SQLite 初始化、WAL、备份恢复与升级迁移；
- Agent 与 OB Loader/Dumper 的进程、路径、日志和终态集成；
- 并发 Agent、日志写入和任务租约压力下是否出现不可接受的 `SQLITE_BUSY`。

上述项目必须在目标系统执行，不能用本次 Windows 运行或交叉编译替代。

## 6. 技术结论

| 判断 | 结论 |
|---|---|
| SQLite 是否破坏三架构交付 | 否；三目标关闭 CGO 均构建成功 |
| 是否需要目标机安装 SQLite 服务 | 否 |
| 纯 Go 驱动候选是否可继续 | 是；`modernc.org/sqlite v1.54.0` 可进入麒麟运行验证 |
| 是否已经最终锁定驱动 | 否；三套麒麟运行、升级和并发门禁通过后再锁定 |
| 是否可进入业务代码 | 否；技术路线整体确认和实现契约仍未完成 |

因此 TS-R03 的可构建性风险已经明显收敛，但技术栈仍应保持评审状态。下一步不是创建业务表，而是先确认 TS-R01～TS-R14，再设计首条切片最小实现契约。
