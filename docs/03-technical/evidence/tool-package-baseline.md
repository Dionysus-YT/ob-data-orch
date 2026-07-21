# OB Loader/Dumper 4.3.5 本地工具包证据

> 证据状态：VS-P0-01 部分完成
> 采集方式：本地只读哈希、压缩包目录检查、主类 `--version` 与 `--help`
> 初次采集日期：2026-07-20
> 最近复核日期：2026-07-21
> 限制：未独立证明该文件来自 OceanBase 官方下载通道

官方参考：

- [OceanBase 导数工具 V4.3.5 文档概览](https://www.oceanbase.com/docs/common-oceanbase-dumper-loader-1000000004997287)
- [OceanBase 导数工具 V4.3.5 快速入门](https://www.oceanbase.com/docs/common-oceanbase-dumper-loader-1000000004997304)
- [OceanBase Loader/Dumper Security Features](https://en.oceanbase.com/docs/common-oceanbase-dumper-loader-10000000002265507)

## 1. 文件身份

| 项目 | 结果 |
|---|---|
| 文件名 | `ob-loader-dumper-4.3.5-RELEASE.zip` |
| 文件大小 | `153197121` 字节 |
| SHA-256 | `C1A5D5EE053106803263015F00EC7F0B94B73EA4015A2E53CF1AFFF598983493` |
| 本地修改时间 | `2026-07-15 17:50:25` |
| Git 处置 | 根目录 `.gitignore` 排除，不提交二进制包 |

压缩包包含 Linux 与 Windows 启动脚本、`conf/`、`lib/`、内置 README、CHANGELOG、LICENSE 和 NOTICE；未发现第二层嵌套安装包。

## 2. 工具身份

使用压缩包内依赖和 `com.oceanbase.tools.loaddump.cmd.Obdumper` 主类执行只读版本查询：

```text
Version: 4.3.5-RELEASE
```

内置中文 CHANGELOG 的首个版本标题为 `4.3.5 (2026-01-06)`，与工具版本相符。

## 3. 帮助参数集合

| 工具 | 退出码 | 唯一长参数数量 | 与现有基线 |
|---|---:|---:|---|
| OBDUMPER | 0 | 109 | 与导出参数映射基线数量一致 |
| OBLOADER | 0 | 103 | 与普通导入/旁路参数映射基线数量一致 |

本次只确认参数名称集合数量，没有把 `--help` 出现解释为参数在所有模式下均可运行。POS、`--block-size`、检查点、旁路适用性等行为仍按原 P0 计划验证。

## 4. Windows 启动环境发现

2026-07-20 初次采集时，显式发现的 JDK 为 `1.8.0_261`：

```text
java version "1.8.0_261"
Java HotSpot(TM) 64-Bit Server VM (build 25.261-b33)
```

工具内置 README 说明 Java 8+，并强烈建议 `1.8.0_3xx` 以上。2026-07-21 复核时，当前活动 Java 已为：

```text
java version "1.8.0_491"
Java HotSpot(TM) 64-Bit Server VM (build 25.491-b10)
```

该版本满足补丁级别建议。后续真实导出发现 Windows 本地库搜索问题并已修复；Windows 实际参数按用户确认的完整盘符绝对路径执行，早期自动化 `file://null` 结果作为环境差异保留。不能仅因 Java 版本满足要求就判定执行节点可用，详见首条切片 P0 证据。

OceanBase V4.3.5 快速入门同样要求 Java 8+、配置 `JAVA_HOME`，并强烈建议 JDK `1.8.0_3xx` 及以后版本；因此本地包内说明与官网环境要求一致。

Windows `obdumper.bat` 还表现出以下行为：

1. 未设置 `JAVA_HOME` 或 `JRE_HOME` 时打印环境错误；
2. 脚本仍返回退出码 `0`；
3. 仅凭脚本退出码无法判断工具是否真正启动；
4. 初次采集只完成版本和帮助核对；2026-07-21 已执行数据库连接和导出尝试，详见[首条纵向切片 P0 执行记录](first-vertical-slice-p0-2026-07-21.md)。

### 4.1 官方敏感信息文件能力

本地 4.3.5 压缩包包含：

```text
ob-loader-dumper-4.3.5-RELEASE/tools/secure-gen
ob-loader-dumper-4.3.5-RELEASE/conf/security.properties
```

`security.properties` 提供 `encrypt.filePath`、`secretKey.filePath` 和 `security.className` 配置键。包内 `secure-gen` 是依赖 Bash、OpenSSL 和用户目录的 Shell 脚本，没有对应 Windows `.bat/.cmd` 生成器；其 `-n` 模式需要明文属性文件输入。

官网说明 V4.2.0 起可以使用该机制加密业务密码、sys 密码和对象存储密钥，并在工具命令中省略 `--password`/`--sys-password`。2026-07-21 已证明平台在内存中生成的任务级材料可被 Windows 上的 4.3.5 解密器和 OBDUMPER 读取，命令不带 `--password`；错误私钥会被拒绝。Linux 目标读取、官方脚本产物交叉核对、正式并发隔离和终态清理仍未通过，因此 VS-P0-09 仍不能整体判定通过。详见[安全文件兼容性验证](secure-gen-compatibility-spike-2026-07-21.md)和[凭据、权限与安全最小契约](../credential-access-security-contract.md)。

## 5. 技术影响

- Agent 环境检查必须独立验证 Java 路径、版本、工具主类和版本输出；
- 不能只以启动脚本退出码为工具可用判断；
- 启动成功至少需要同时满足进程创建、预期版本输出或后续受控握手证据；
- 正式验证节点应使用满足项目技术基线且优先符合官方建议的 Java 版本，并实际验证本地库、输出路径和结果文件；
- `secure-gen` 官方安全文件路线应优先于明文命令参数；Windows 核心格式已通过，但跨平台读取、任务隔离和清理实测通过前，VS-P0-09 继续阻断；
- Windows 直接启动包内 Java 主类的隔离负例已通过，能够取得真实 Java 退出码并使用任务级安全配置；该结果只支持进入[启动入口契约](../tool-launch-isolation-contract.md)评审，Linux 和成功执行仍待验证；
- 工具包来源仍需通过官方发布页、发布校验值或可信交付记录补齐，之后才能将 VS-P0-01 标记为通过。

### 5.1 国产 Linux 部署事实

2026-07-21，用户明确确认三个目标国产 Linux 环境已具备兼容的 OB Loader/Dumper 4.3.5、Java 8 和相关本地库。本项目据此不再把官方工具的 CPU 架构能力作为平台技术选型阻断。用户提供的 `nkvers` 与 `lscpu` 摘要形成以下目标清单：

- Kylin Linux Advanced Server V10 SP1（Tercel），Kunpeng 920，`aarch64`；
- Kylin Linux Advanced Server V11 2503（Swan25），Kunpeng 920，`aarch64`；
- Kylin Linux Advanced Server V10 SP3 2403（Halberd），Hygon C86-4G，`x86_64`。

海光 C86 使用 `linux/amd64` 平台产物，两套鲲鹏系统使用 `linux/arm64` 平台产物。该确认不替代平台集成测试；控制面、Agent、SQLite 驱动、服务安装、环境事实、绝对路径、工具启动、日志采集、取消和终态核对仍需在三个目标系统分别执行。具体版本和门禁见[技术路线与部署选型基线](../technology-stack.md)。

## 6. 当前结论

VS-P0-01 状态为“部分完成”：本地文件摘要、目录结构、工具版本和 109/103 参数集合已经固化；Java 补丁版本已满足建议，但官方来源证明和可成功导出 CSV 的正式执行节点仍未完成。
