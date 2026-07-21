# OB Loader/Dumper 4.3.5 安全文件兼容性验证

> 验证日期：2026-07-21
> 验证环境：Windows AMD64、Go 1.26.4、Java 1.8.0_491
> 验证对象：本地 `ob-loader-dumper-4.3.5-RELEASE.zip`
> 结论：Windows 格式兼容与无密码命令路径已通过；全平台、清理和正式 Agent 集成仍未通过

## 1. 目的与安全边界

本次验证只回答一个窄问题：平台能否在不把数据库密码写入明文临时文件或 OBDUMPER 参数的前提下，生成 4.3.5 可以读取的官方安全文件格式。

- 全程使用运行时随机生成的合成密码，不读取或复用真实数据库凭据；
- 数据库目标固定为不可用的 `127.0.0.1:1`，只用来证明工具已经越过参数和密码解析并进入连接阶段；
- 验证程序和运行材料位于被 `.gitignore` 排除的 `tmp/secure-gen-spike/`；
- 证据只记录布尔结果和非秘密产物摘要，不记录合成密码原值；
- 本次没有编写产品业务代码，也没有把验证程序加入正式源码目录。

## 2. 包内格式事实

### 2.1 `tools/secure-gen`

包内脚本的实际流程为：

1. 读取敏感属性明文；
2. 以“输入字节数 × 8 + 1024”计算 RSA 位数；
3. 生成 RSA 密钥；
4. 将私钥转换为未加密 PKCS#8 PEM；
5. 使用 RSA 公钥和 OpenSSL `rsautl -encrypt` 生成 `secure.rsa`；
6. 由 `security.properties` 指向密文与私钥。

脚本依赖 Bash、OpenSSL 和用户目录，包内没有 Windows 生成脚本；`-n` 模式还要求先准备明文属性文件。因此平台不能直接把该脚本作为跨平台任务实现。

### 2.2 4.3.5 Java 读取端

对包内类进行只读字节码检查，确认：

- `SecurityUtils` 从系统属性 `security.configurationFile` 读取配置；
- 配置键为 `encrypt.filePath`、`secretKey.filePath` 和可选 `security.className`；
- `OpenSslDecryptor` 使用 `PKCS8EncodedKeySpec` 读取私钥；
- `Cipher.getInstance("RSA")` 解密密文，在当前 Java 8 提供方上对应 PKCS#1 v1.5 填充；
- 解密结果按 Java Properties 读取，其中业务密码键为 `oceanbase.jdbc.password`；
- CLI 中显式密码优先于安全文件值，因此平台必须从实际 argv 中完全省略 `--password`，不能同时生成两份来源。

## 3. 验证方法

验证用 Go 程序只使用标准库：

```text
crypto/rand
crypto/rsa
crypto/x509
encoding/pem
```

它在内存中生成合成密码和属性内容，写出任务级 PKCS#8 私钥、RSA/PKCS#1 v1.5 密文及 `security.properties`。没有创建明文属性输入文件。Windows 配置路径使用完整绝对路径，并以 `/` 作为分隔符，避免 Java Properties 把反斜杠当作转义符。

Java 兼容探针直接调用包内 `SecurityUtils.decrypt()`，只比较密码摘要；随后以同一安全配置启动真实 `com.oceanbase.tools.loaddump.cmd.Obdumper`，命令不带 `--password`。

## 4. 结果

| ID | 验证项 | 结果 | 证据与限制 |
|---|---|---|---|
| SG-P01 | Go 产物被 4.3.5 Java 解密器读取 | 通过 | `loaded=true`，Java 读取值与内存生成值摘要一致 |
| SG-P02 | OBDUMPER 不带 `--password` 启动 | 通过（Windows） | 未出现解密错误或密码缺失，工具进入 JDBC 连接并按预期在 `127.0.0.1:1` 拒绝连接 |
| SG-P03 | 错误私钥负例 | 通过 | A 任务密文配 B 任务私钥得到 `BadPaddingException`，未容错放行 |
| SG-P04 | 任务材料随机隔离 | 部分通过 | 两次生成的私钥和密文摘要均不同；尚未完成正式 Agent 并发任务测试 |
| SG-P05 | 明文属性文件 | 通过（验证程序） | 两个任务目录只生成私钥、密文和安全配置，没有明文属性输入文件 |
| SG-P06 | Windows ACL | 通过（受控目录） | 先关闭继承并仅授权当前模拟服务账户、SYSTEM、Administrators 后，工具仍可读取；默认继承目录曾允许普通本机用户读取，因此正式实现必须先建安全目录再写材料 |
| SG-P07 | Windows 启动脚本注入 | 通过但有既有风险 | `JAVA_OPTS` 只传安全配置路径时 `.bat` 能读取；`.bat` 仍会把 Java 失败退出码掩盖为 0 |
| SG-P08 | 直接 Java 入口退出码 | 通过（验证用） | 同一拒绝连接用例直接运行主类返回 1；是否作为正式跨平台进程适配方式仍待评审和 Linux 验证 |
| SG-P09 | 三目标构建 | 通过（仅构建） | 同一纯 Go 生成器成功构建 Windows AMD64、Linux AMD64、Linux ARM64；不等于两个 Linux 目标已运行通过 |
| SG-P10 | 官方脚本产物交叉读取 | 未执行 | 当前 Windows 环境无可用 Bash/OpenSSL；仍需在 Linux 目标用包内 `secure-gen` 生成合成材料并交叉核对 |
| SG-P11 | Linux 工具读取与权限 | 未执行 | 麒麟 C86/ARM64 正式节点尚未运行；`0700/0600` 和工具读取必须在目标机验证 |
| SG-P12 | 终态清理与崩溃恢复 | 未执行 | 尚无正式 Agent 生命周期、异常终止和残留目录监管实现 |

三目标构建产物摘要：

| 目标 | SHA-256 |
|---|---|
| Windows AMD64 | `872e57ed5bbe72dd1143dfcb04201c1608f575b7c822d5981531c4ea04945715` |
| Linux AMD64 | `cb3f14339e4f6a6faaff66a4518a4e289066cd3101018c595c4ae2ef06e2f03e` |
| Linux ARM64 | `6b96f28523d7b88ce0be20d8e4b591b1791c550e5aa7cbb3b2f0e6c93fde7950` |

这些摘要只标识本次临时验证构建，不是未来正式发布摘要。

## 5. 启动与隔离发现

### 5.1 Windows

4.3.5 的 `bin/windows/obdumper.bat` 没有像 Linux 脚本一样显式设置 `security.configurationFile`。本次通过每进程继承的 `JAVA_OPTS` 注入任务级配置路径后读取成功；该环境值只含配置路径，不含密码原值。

这条路线仍不能直接定版，因为官方批处理会掩盖 Java 失败退出码。Agent 已确认的终态契约要求同时核对进程、工具终态日志和输出事实，不能仅依赖批处理返回值。

### 5.2 Linux

Linux 启动脚本把 `security.configurationFile` 固定为发布包共享的 `conf/security.properties`。这与“每个 execution 使用独立配置、禁止修改共享配置”的安全要求冲突。后续必须在目标 Linux 上验证以下受控入口之一，再单独评审定版：

- Agent 直接启动包内 Java 主类，并完整复制、验证官方脚本所需 JVM/本地库参数；
- 为 execution 建立隔离工具运行视图，使脚本看到独立 `conf/security.properties`，而依赖仍来自受控发布包。

在证明路径、本地库、退出码、日志与升级兼容性之前，不选定其中任何一种。

## 6. 对现有契约的影响

1. 4.3.5 官方安全文件路线在 Windows 上已证明可行，不再只是文档推测；
2. 平台生成材料的密码不需要进入 OBDUMPER argv，PC-R04 的兼容修订具备实证前提；
3. PC-R04 暂不修改：CS-R01～CS-R18 已确认，但 Linux 运行、隔离入口和清理尚未通过；
4. VS-P0-09 从“完全没有安全注入证据”推进为“Windows 核心格式部分通过”，但整体仍阻断业务实现；
5. 正式 Agent 必须在写文件之前建立严格目录权限。仅调用 Go `0600` 文件模式不能替代 Windows ACL；
6. 私钥和 `secure.rsa` 组合可以恢复密码，任务级材料仍按秘密处理，不能进入结果下载、普通备份或日志。

## 7. 下一步门禁

按风险优先级继续：

1. 在 Linux AMD64/ARM64 目标分别运行包内 `secure-gen` 与平台生成器交叉读取；
2. 定版 Windows/Linux 统一的隔离启动入口，并复核官方 JVM 参数、本地库和退出状态；
3. 用两个并发 execution 验证配置、私钥、密文和日志不串用；
4. 验证正常终态、启动失败、Agent 崩溃和机器重启后的材料清理/残留监管；
5. 搜索 argv、父子进程、stdout、stderr、工具日志、崩溃文件和任务事件，确认合成秘密不可见；
6. CS-R01～CS-R18 已确认；上述剩余结果通过后，再正式修订 PC-R04 并开放实现。
