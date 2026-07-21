# OBDUMPER 4.3.5 直接 Java 隔离启动验证

> 验证日期：2026-07-21
> 验证环境：Windows AMD64、Java 1.8.0_491
> 验证对象：本地 OB Loader/Dumper 4.3.5 发布包
> 结论：Windows 候选入口通过最小负例；Linux AMD64/ARM64 仍待目标机验证

## 1. 验证目的

本次验证用于判断 Agent 是否可以不经过 `.bat`/Shell，直接以结构化参数启动包内 Java 主类，从而同时满足：

- 每个 execution 使用独立 `security.properties`；
- 实际 argv 不包含 `--password`；
- Agent 直接持有 Java 进程身份和真实退出码；
- Windows 完整绝对路径及包含空格的任务路径不被 shell 改写；
- 子进程不继承可注入 Java 参数的环境变量；
- 不修改发布包共享 `conf/`、脚本或 JAR。

只使用随机合成密码和不可连接端点 `127.0.0.1:1`。本次不接触真实数据库、不执行导出、不编写正式 Agent 代码。

## 2. 发布包启动脚本事实

| 项目 | Windows `bin/windows/obdumper.bat` | Linux `bin/obdumper` |
|---|---|---|
| Java 入口 | `com.oceanbase.tools.loaddump.cmd.Obdumper` | 相同 |
| 默认堆 | `-Xms4G -Xmx4G` | 相同；脚本另解析 `--mem` |
| Java 8 GC | 8u300 及以上 G1，以下 CMS | 相同原则 |
| 类路径 | `.;<tool>/lib/*` | 排序后的全部 JAR + `<tool>/conf/` |
| 安全配置 | 脚本未设置；可从继承 `JAVA_OPTS` 注入 | 固定为共享 `<tool>/conf/security.properties` |
| 日志目录 | 固定为共享 `<tool>/logs` | 固定为共享 `<tool>/logs` |
| Windows Hadoop | 设置 `<tool>/ext/windows/hadoop` | 不适用 |
| 退出码 | 已实测会把 Java 失败掩盖为批处理 0 | `eval` 最后一条 Java 命令，预计保留退出码，仍需目标机实测 |
| shell 风险 | cmd 参数拼接与额外进程层 | Bash `eval` 与额外进程层 |

脚本摘要：

| 文件 | SHA-256 |
|---|---|
| `bin/windows/obdumper.bat` | `836a0d2dcbf7d4fdcb8b1155d50eca7e08e155cc55b7833458b5e57033f49dd2` |
| `bin/obdumper` | `cc20579a66c682504bfd9c55d13b4a443f21d3dc4a3861c83a32597e4dcb2fec` |

两份脚本的共同业务相关系统属性基本一致，但存在平台差异：Windows 额外设置 obproxy、decrypt 和 Hadoop 路径；Linux 额外设置 `enable.definer=true`，并把安全配置写死为共享路径。不能拿一套未经核对的 JVM 参数覆盖所有平台。

## 3. 候选入口

验证入口采用以下结构，不生成命令字符串：

```text
已验证的 java 绝对路径
+ 版本化 JVM 选项令牌
+ 版本化系统属性令牌
+ -Dsecurity.configurationFile=<execution 私有配置>
+ -Dlog4j.output=<execution 私有日志目录>
+ -classpath <4.3.5 包内类路径>
+ com.oceanbase.tools.loaddump.cmd.Obdumper
+ 已确认的 OBDUMPER 业务参数令牌
```

进程工作目录、TEMP/TMP、私钥、密文、安全配置、工具原生日志与输出目录彼此区分。用户填写的导出 `--file-path` 仍按任务快照原样传入，不改到 Agent 工作目录下。

## 4. Windows 执行结果

### 4.1 完整候选配置负例

以包含空格的任务目录启动真实 OBDUMPER，使用与 Windows 官方脚本对应的堆、GC、系统属性、Hadoop 路径和包内类路径；安全配置由任务私有路径提供，命令不含 `--password`。

| 观察项 | 结果 |
|---|---|
| Java 进程创建 | 通过 |
| Java 真实退出码 | `1` |
| 进入 JDBC 连接阶段 | 通过 |
| `127.0.0.1:1` 按预期拒绝 | 通过 |
| 安全配置解密错误 | 未出现 |
| 密码缺失错误 | 未出现 |
| 含空格输出路径保持为单一参数 | 通过 |
| 子进程环境移除 `JAVA_TOOL_OPTIONS`、`_JAVA_OPTIONS`、`CLASSPATH` | 通过 |

这证明候选入口已越过 Java 启动、类路径、CLI 解析、安全文件读取和密码解析，失败发生在预期的网络连接阶段。

### 4.2 进程可观察性

进程运行期间通过 Windows 进程事实核对：

| 观察项 | 结果 |
|---|---|
| 观察到的执行文件 | 直接 Java 进程 |
| 父进程 | 当前受控验证进程 |
| 命令行包含 `--password` | 否 |
| 命令行包含任务级 `security.properties` 路径 | 是 |
| 最终退出码 | `1` |

配置路径对同机进程查看者可见，因此路径本身不得包含密码、用户名或其他秘密。能读取私钥和密文的同一服务账户/管理员仍在已确认威胁模型之外，不能宣称直接 Java 启动消除了管理员级风险。

## 5. 最小环境验证

验证进程没有继承当前会话的完整环境，而是显式提供 Windows 启动所需的最小集合：

- `SystemRoot`、`WINDIR`；
- execution 私有 `TEMP`、`TMP`；
- 已验证 Java 目录和必要系统目录组成的 `PATH`；
- 已验证的 `JAVA_HOME`/`JRE_HOME`。

以下注入变量不进入子进程：

```text
JAVA_OPTS
JAVA_TOOL_OPTIONS
JDK_JAVA_OPTIONS
_JAVA_OPTIONS
CLASSPATH
```

正式 Linux 配置还必须清除 `LD_PRELOAD` 等动态加载注入变量，并按目标本地库事实建立允许列表。不能简单复制 Agent 服务进程的全部环境。

## 6. 当前不能判定通过的内容

- 麒麟 V10 SP3 C86、V10 SP1 ARM64、V11 ARM64 上的直接 Java 启动；
- Linux 排序类路径与本地库加载；
- Linux 任务级 `security.properties` 读取；
- Windows/Linux 成功导出路径和真实 `System exit 0`；
- 两个并发 execution 的安全配置、TEMP、日志和工作目录隔离；
- Agent 崩溃后 Java 进程继续运行、重连和终态恢复；
- OOM、JVM 崩溃和工具原生日志中的敏感信息处理；
- 安全材料与工具原生日志的正常/异常清理。

因此本证据只支持进入启动契约评审，不能让 VS-P0-09、VS-P0-10 或 VS-P0-11 整体通过。

## 7. 结论

直接 Java 入口比继续包装官方脚本更符合当前已确认契约：

- 不需要修改共享发布包；
- Windows/Linux 可以使用同一进程模型，同时保留平台专属启动配置；
- 安全配置和日志可以按 execution 隔离；
- Agent 能直接取得 Java PID、开始时间和真实退出码；
- 结构化 argv 避免 cmd/Bash 再解析业务参数。

但它不是“直接拼一个 java 命令”即可完成。正式实现必须使用与精确工具版本绑定的启动配置、环境允许列表、路径和权限预检查、进程恢复证据及目标平台实测。详细规则见[跨平台隔离启动入口契约](../tool-launch-isolation-contract.md)。
