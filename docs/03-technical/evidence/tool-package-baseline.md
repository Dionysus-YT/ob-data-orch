# OB Loader/Dumper 4.3.5 本地工具包证据

> 证据状态：VS-P0-01 部分完成  
> 采集方式：本地只读哈希、压缩包目录检查、主类 `--version` 与 `--help`  
> 采集日期：2026-07-20  
> 限制：未独立证明该文件来自 OceanBase 官方下载通道

官方参考：

- [OceanBase 导数工具 V4.3.5 文档概览](https://www.oceanbase.com/docs/common-oceanbase-dumper-loader-1000000004997287)
- [OceanBase 导数工具 V4.3.5 快速入门](https://www.oceanbase.com/docs/common-oceanbase-dumper-loader-1000000004997304)

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

当前机器的 `java -version` 为：

```text
java version "1.8.0_261"
Java HotSpot(TM) 64-Bit Server VM (build 25.261-b33)
```

工具内置 README 说明 Java 8+，并强烈建议 `1.8.0_3xx` 以上；当前环境低于建议补丁版本，只适合完成只读帮助和版本核对，不作为正式执行环境通过证据。

OceanBase V4.3.5 快速入门同样要求 Java 8+、配置 `JAVA_HOME`，并强烈建议 JDK `1.8.0_3xx` 及以后版本；因此本地包内说明与官网环境要求一致。

Windows `obdumper.bat` 还表现出以下行为：

1. 未设置 `JAVA_HOME` 或 `JRE_HOME` 时打印环境错误；
2. 脚本仍返回退出码 `0`；
3. 仅凭脚本退出码无法判断工具是否真正启动；
4. 本次通过直接调用压缩包主类完成版本和帮助核对，没有执行数据库连接或导出。

## 5. 技术影响

- Agent 环境检查必须独立验证 Java 路径、版本、工具主类和版本输出；
- 不能只以启动脚本退出码为工具可用判断；
- 启动成功至少需要同时满足进程创建、预期版本输出或后续受控握手证据；
- 正式验证节点应使用满足项目技术基线且优先符合官方建议的 Java 版本；
- 工具包来源仍需通过官方发布页、发布校验值或可信交付记录补齐，之后才能将 VS-P0-01 标记为通过。

## 6. 当前结论

VS-P0-01 状态为“部分完成”：本地文件摘要、目录结构、工具版本和 109/103 参数集合已经固化；官方来源证明和正式验证节点 Java 环境尚未完成。
