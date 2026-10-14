# MySQL 本地导出参数追踪

参数目录：`obdumper-4.3.5-slice-v11-expert-parameters`。共 79 个定义，按真实授权范围区分活动参数、身份派生、秘密槽位、Oracle/云端排除及历史门控。命令取证只保存参数名，不保存原始命令；“发射观察”不等于该功能实测通过。

| 参数 | 本轮适用与条件 | 正例 / 负例 | 命令发射观察 |
| --- | --- | --- | --- |
| `--host` | 由授权数据源派生；主链路冻结快照、argv 一致性和脱敏核对 | — / — | 102 个任务 |
| `--port` | 由授权数据源派生；主链路冻结快照、argv 一致性和脱敏核对 | — / — | 102 个任务 |
| `--user` | 由授权数据源派生；主链路冻结快照、argv 一致性和脱敏核对 | — / — | 102 个任务 |
| `--password` | 短时安全文件；真实秘密核对与清理见 MX-132；sys 正例依赖已配置 sys | — / — | 102 个任务 |
| `--database` | MySQL / 本地；逐项结果见实测表 | MX-001、MX-001-LOG、MX-002、MX-003、MX-004、MX-005、MX-006、MX-006-MULTILINE、MX-006-LONG、MX-007、MX-008、MX-010、MX-011-CSV、MX-011-CUT、MX-011-SQL、MX-012-VIEW、MX-012-FUNCTION、MX-012-PROCEDURE、MX-012-SEQUENCE、MX-013、MX-020、MX-021、MX-022、MX-023、MX-024、MX-025、MX-025-ALL-NOT-NULL、MX-025-NONE、MX-026、MX-027、MX-028、MX-029、MX-030、MX-031、MX-032、MX-033、MX-034、MX-035-UTF-8、MX-035-GBK、MX-035-GB2312、MX-035-GB18030、MX-035-Big5、MX-035-ISO-8859-1、MX-035-UTF-16、MX-035-UTF-32、MX-035-US-ASCII、MX-036-CSV、MX-037-CSV、MX-036-CUT、MX-037-CUT、MX-036-SQL、MX-037-SQL、MX-040、MX-041、MX-042、MX-043、MX-044、MX-045、MX-051、MX-060-dateValueFormat、MX-060-timeValueFormat、MX-060-datetimeValueFormat、MX-060-timestampValueFormat、MX-061、MX-061-CUT、MX-062、MX-062-CUT、MX-063、MX-070、MX-071、MX-072、MX-073、MX-074、MX-075、MX-076、MX-077-PRESERVE、MX-077-RESTART、MX-078、MX-090、MX-091、MX-092、MX-093、MX-094、MX-095、MX-096、MX-101、MX-110-zstd、MX-110-zlib、MX-110-gzip、MX-110-snappy、MX-146、MX-150-zstd-1、MX-150-zstd-22、MX-150-zlib--1、MX-150-zlib-9 / MX-014、MX-015、MX-016、MX-017、MX-018、MX-019、MX-046、MX-047、MX-048、MX-049、MX-050、MX-052、MX-053、MX-054、MX-055、MX-056、MX-064-timestampTzValueFormat、MX-064-timestampLtzValueFormat、MX-064-nlsDateFormat、MX-064-nlsTimestampFormat、MX-064-nlsTimestampTzFormat、MX-065、MX-066、MX-073-NO-SYS、MX-079、MX-080、MX-081、MX-082、MX-097、MX-098、MX-099、MX-100、MX-102、MX-103、MX-104、MX-105、MX-106、MX-111、MX-112、MX-113、MX-114、MX-115、MX-116、MX-117、MX-118、MX-118-NONE-ESCAPE、MX-140、MX-141、MX-142、MX-143、MX-144、MX-145、MX-147、MX-148、MX-149 | 102 个任务 |
| `--table` | MySQL / 本地；逐项结果见实测表 | MX-001、MX-001-LOG、MX-002、MX-003、MX-010、MX-011-CSV、MX-011-CUT、MX-011-SQL、MX-013、MX-020、MX-021、MX-022、MX-023、MX-024、MX-025、MX-025-ALL-NOT-NULL、MX-025-NONE、MX-026、MX-027、MX-028、MX-029、MX-030、MX-031、MX-032、MX-033、MX-034、MX-035-UTF-8、MX-035-GBK、MX-035-GB2312、MX-035-GB18030、MX-035-Big5、MX-035-ISO-8859-1、MX-035-UTF-16、MX-035-UTF-32、MX-035-US-ASCII、MX-036-CSV、MX-037-CSV、MX-036-CUT、MX-037-CUT、MX-036-SQL、MX-037-SQL、MX-040、MX-041、MX-042、MX-043、MX-044、MX-045、MX-051、MX-060-dateValueFormat、MX-060-timeValueFormat、MX-060-datetimeValueFormat、MX-060-timestampValueFormat、MX-061、MX-061-CUT、MX-062、MX-062-CUT、MX-063、MX-070、MX-071、MX-072、MX-073、MX-074、MX-075、MX-076、MX-078、MX-090、MX-091、MX-092、MX-093、MX-094、MX-095、MX-096、MX-101、MX-110-zstd、MX-110-zlib、MX-110-gzip、MX-110-snappy、MX-146、MX-150-zstd-1、MX-150-zstd-22、MX-150-zlib--1、MX-150-zlib-9 / MX-015、MX-046、MX-047、MX-048、MX-049、MX-050、MX-052、MX-053、MX-056、MX-064-timestampTzValueFormat、MX-064-timestampLtzValueFormat、MX-064-nlsDateFormat、MX-064-nlsTimestampFormat、MX-064-nlsTimestampTzFormat、MX-065、MX-066、MX-073-NO-SYS、MX-079、MX-080、MX-082、MX-097、MX-098、MX-099、MX-100、MX-102、MX-103、MX-104、MX-105、MX-106、MX-111、MX-112、MX-113、MX-114、MX-115、MX-116、MX-117、MX-118、MX-118-NONE-ESCAPE、MX-143、MX-144、MX-145、MX-147、MX-148、MX-149 | 88 个任务 |
| `--csv` | MySQL / 本地；逐项结果见实测表 | MX-001、MX-001-LOG、MX-002、MX-003、MX-004、MX-005、MX-006、MX-006-MULTILINE、MX-006-LONG、MX-007、MX-008、MX-011-CSV、MX-020、MX-021、MX-022、MX-023、MX-024、MX-025、MX-025-ALL-NOT-NULL、MX-025-NONE、MX-026、MX-027、MX-028、MX-029、MX-035-UTF-8、MX-035-GBK、MX-035-GB2312、MX-035-GB18030、MX-035-Big5、MX-035-ISO-8859-1、MX-035-UTF-16、MX-035-UTF-32、MX-035-US-ASCII、MX-036-CSV、MX-037-CSV、MX-040、MX-041、MX-042、MX-043、MX-044、MX-045、MX-051、MX-060-dateValueFormat、MX-060-timeValueFormat、MX-060-datetimeValueFormat、MX-060-timestampValueFormat、MX-061、MX-062、MX-063、MX-070、MX-071、MX-072、MX-078、MX-090、MX-091、MX-092、MX-093、MX-094、MX-095、MX-096、MX-101、MX-110-zstd、MX-110-zlib、MX-110-gzip、MX-110-snappy、MX-146、MX-150-zstd-1、MX-150-zstd-22、MX-150-zlib--1、MX-150-zlib-9 / MX-014、MX-015、MX-016、MX-017、MX-018、MX-019、MX-046、MX-047、MX-048、MX-049、MX-050、MX-052、MX-053、MX-054、MX-055、MX-056、MX-064-timestampTzValueFormat、MX-064-timestampLtzValueFormat、MX-064-nlsDateFormat、MX-064-nlsTimestampFormat、MX-064-nlsTimestampTzFormat、MX-065、MX-066、MX-079、MX-080、MX-081、MX-097、MX-098、MX-099、MX-100、MX-102、MX-103、MX-104、MX-105、MX-106、MX-111、MX-112、MX-113、MX-114、MX-115、MX-116、MX-117、MX-118-NONE-ESCAPE、MX-140、MX-141、MX-142、MX-145、MX-147、MX-148、MX-149 | 78 个任务 |
| `--skip-header` | MySQL / 本地；逐项结果见实测表 | MX-021 / MX-118 | 1 个任务 |
| `--column-separator` | MySQL / 本地；逐项结果见实测表 | MX-022 / MX-114 | 1 个任务 |
| `--column-quote` | MySQL / 本地；逐项结果见实测表 | MX-022 / MX-115 | 1 个任务 |
| `--column-quote-mode` | MySQL / 本地；逐项结果见实测表 | MX-023、MX-024、MX-025、MX-025-ALL-NOT-NULL、MX-025-NONE / MX-118-NONE-ESCAPE | 5 个任务 |
| `--escape-character` | MySQL / 本地；逐项结果见实测表 | MX-025-NONE、MX-026 / MX-116 | 2 个任务 |
| `--line-separator` | MySQL / 本地；逐项结果见实测表 | MX-027、MX-034 / — | 2 个任务 |
| `--null-string` | MySQL / 本地；逐项结果见实测表 | MX-028 / MX-117 | 1 个任务 |
| `--file-encoding` | MySQL / 本地；逐项结果见实测表 | MX-034、MX-035-UTF-8、MX-035-GBK、MX-035-GB2312、MX-035-GB18030、MX-035-Big5、MX-035-ISO-8859-1、MX-035-UTF-16、MX-035-UTF-32、MX-035-US-ASCII / — | 10 个任务 |
| `--file-path` | MySQL / 本地；逐项结果见实测表 | MX-001、MX-001-LOG、MX-002、MX-003、MX-004、MX-005、MX-006、MX-006-MULTILINE、MX-006-LONG、MX-007、MX-008、MX-010、MX-011-CSV、MX-011-CUT、MX-011-SQL、MX-012-VIEW、MX-012-FUNCTION、MX-012-PROCEDURE、MX-012-SEQUENCE、MX-013、MX-020、MX-021、MX-022、MX-023、MX-024、MX-025、MX-025-ALL-NOT-NULL、MX-025-NONE、MX-026、MX-027、MX-028、MX-029、MX-030、MX-031、MX-032、MX-033、MX-034、MX-035-UTF-8、MX-035-GBK、MX-035-GB2312、MX-035-GB18030、MX-035-Big5、MX-035-ISO-8859-1、MX-035-UTF-16、MX-035-UTF-32、MX-035-US-ASCII、MX-036-CSV、MX-037-CSV、MX-036-CUT、MX-037-CUT、MX-036-SQL、MX-037-SQL、MX-040、MX-041、MX-042、MX-043、MX-044、MX-045、MX-051、MX-060-dateValueFormat、MX-060-timeValueFormat、MX-060-datetimeValueFormat、MX-060-timestampValueFormat、MX-061、MX-061-CUT、MX-062、MX-062-CUT、MX-063、MX-070、MX-071、MX-072、MX-073、MX-074、MX-075、MX-076、MX-077-PRESERVE、MX-077-RESTART、MX-078、MX-090、MX-091、MX-092、MX-093、MX-094、MX-095、MX-096、MX-101、MX-110-zstd、MX-110-zlib、MX-110-gzip、MX-110-snappy、MX-146、MX-150-zstd-1、MX-150-zstd-22、MX-150-zlib--1、MX-150-zlib-9 / MX-014、MX-015、MX-016、MX-017、MX-018、MX-019、MX-046、MX-047、MX-048、MX-049、MX-050、MX-052、MX-053、MX-054、MX-055、MX-056、MX-064-timestampTzValueFormat、MX-064-timestampLtzValueFormat、MX-064-nlsDateFormat、MX-064-nlsTimestampFormat、MX-064-nlsTimestampTzFormat、MX-065、MX-066、MX-073-NO-SYS、MX-079、MX-080、MX-081、MX-082、MX-097、MX-098、MX-099、MX-100、MX-102、MX-103、MX-104、MX-105、MX-106、MX-111、MX-112、MX-113、MX-114、MX-115、MX-116、MX-117、MX-118、MX-118-NONE-ESCAPE、MX-140、MX-141、MX-142、MX-143、MX-144、MX-145、MX-147、MX-148、MX-149 | 102 个任务 |
| `--log-path` | MySQL / 本地；逐项结果见实测表 | MX-001-LOG、MX-002、MX-003、MX-004、MX-005、MX-006、MX-006-MULTILINE、MX-006-LONG、MX-007、MX-008、MX-010、MX-011-CSV、MX-011-CUT、MX-011-SQL、MX-012-VIEW、MX-012-FUNCTION、MX-012-PROCEDURE、MX-012-SEQUENCE、MX-013、MX-020、MX-021、MX-022、MX-023、MX-024、MX-025、MX-025-ALL-NOT-NULL、MX-025-NONE、MX-026、MX-027、MX-028、MX-029、MX-030、MX-031、MX-032、MX-033、MX-034、MX-035-UTF-8、MX-035-GBK、MX-035-GB2312、MX-035-GB18030、MX-035-Big5、MX-035-ISO-8859-1、MX-035-UTF-16、MX-035-UTF-32、MX-035-US-ASCII、MX-036-CSV、MX-037-CSV、MX-036-CUT、MX-037-CUT、MX-036-SQL、MX-037-SQL、MX-040、MX-041、MX-042、MX-043、MX-044、MX-045、MX-051、MX-060-dateValueFormat、MX-060-timeValueFormat、MX-060-datetimeValueFormat、MX-060-timestampValueFormat、MX-061、MX-061-CUT、MX-062、MX-062-CUT、MX-063、MX-070、MX-071、MX-072、MX-073、MX-074、MX-075、MX-076、MX-077-PRESERVE、MX-077-RESTART、MX-078、MX-090、MX-091、MX-092、MX-093、MX-094、MX-095、MX-096、MX-101、MX-110-zstd、MX-110-zlib、MX-110-gzip、MX-110-snappy、MX-146、MX-150-zstd-1、MX-150-zstd-22、MX-150-zlib--1、MX-150-zlib-9 / MX-014、MX-015、MX-016、MX-017、MX-018、MX-019、MX-046、MX-047、MX-048、MX-049、MX-050、MX-052、MX-053、MX-054、MX-055、MX-056、MX-064-timestampTzValueFormat、MX-064-timestampLtzValueFormat、MX-064-nlsDateFormat、MX-064-nlsTimestampFormat、MX-064-nlsTimestampTzFormat、MX-065、MX-066、MX-073-NO-SYS、MX-079、MX-080、MX-081、MX-082、MX-097、MX-098、MX-099、MX-100、MX-102、MX-103、MX-104、MX-105、MX-106、MX-111、MX-112、MX-113、MX-114、MX-115、MX-116、MX-117、MX-118、MX-118-NONE-ESCAPE、MX-140、MX-141、MX-142、MX-143、MX-144、MX-145、MX-147、MX-148、MX-149 | 101 个任务 |
| `--skip-check-dir` | MySQL / 本地；逐项结果见实测表 | MX-101 / — | 1 个任务 |
| `--all` | MySQL / 本地；逐项结果见实测表 | MX-004、MX-005 / — | 2 个任务 |
| `--view` | MySQL / 本地；逐项结果见实测表 | MX-012-VIEW、MX-013 / MX-014 | 2 个任务 |
| `--ddl` | MySQL / 本地；逐项结果见实测表 | MX-010、MX-011-CSV、MX-011-CUT、MX-011-SQL、MX-012-VIEW、MX-012-FUNCTION、MX-012-PROCEDURE、MX-012-SEQUENCE、MX-013、MX-073、MX-074、MX-075、MX-076、MX-077-PRESERVE、MX-077-RESTART / MX-066、MX-073-NO-SYS、MX-082、MX-143 | 14 个任务 |
| `--cut` | MySQL / 本地；逐项结果见实测表 | MX-011-CUT、MX-030、MX-031、MX-032、MX-036-CUT、MX-037-CUT、MX-061-CUT、MX-062-CUT / — | 8 个任务 |
| `--pos` | 当前普通向导门控；不启动历史能力 | — / MX-119-POS | 未观察 |
| `--sql` | MySQL / 本地；逐项结果见实测表 | MX-011-SQL、MX-033、MX-034、MX-036-SQL、MX-037-SQL / MX-118、MX-144 | 5 个任务 |
| `--par` | 当前普通向导门控；不启动历史能力 | — / MX-119-PARQUET | 未观察 |
| `--orc` | 当前普通向导门控；不启动历史能力 | — / MX-119-ORC | 未观察 |
| `--avro` | 当前普通向导门控；不启动历史能力 | — / MX-119-AVRO | 未观察 |
| `--exclude-table` | MySQL / 本地；逐项结果见实测表 | MX-005 / — | 1 个任务 |
| `--with-trim` | MySQL / 本地；逐项结果见实测表 | MX-029 / — | 1 个任务 |
| `--column-splitter` | MySQL / 本地；逐项结果见实测表 | MX-030、MX-031、MX-032 / — | 3 个任务 |
| `--trail-delimiter` | MySQL / 本地；逐项结果见实测表 | MX-031 / — | 1 个任务 |
| `--remove-newline` | MySQL / 本地；逐项结果见实测表 | MX-032 / — | 1 个任务 |
| `--compress` | MySQL / 本地；逐项结果见实测表 | MX-110-zstd、MX-110-zlib、MX-110-gzip、MX-110-snappy、MX-150-zstd-1、MX-150-zstd-22、MX-150-zlib--1、MX-150-zlib-9 / MX-111、MX-112、MX-113 | 8 个任务 |
| `--compression-algo` | MySQL / 本地；逐项结果见实测表 | MX-110-zstd、MX-110-zlib、MX-110-gzip、MX-110-snappy、MX-150-zstd-1、MX-150-zstd-22、MX-150-zlib--1、MX-150-zlib-9 / MX-111、MX-112、MX-113、MX-149 | 8 个任务 |
| `--no-nested-dir` | MySQL / 本地；逐项结果见实测表 | MX-091 / — | 1 个任务 |
| `--ctl-path` | 当前普通向导门控；不启动历史能力 | — / MX-145 | 未观察 |
| `--tmp-path` | MySQL / 本地；逐项结果见实测表 | MX-146 / — | 1 个任务 |
| `--max-file-size` | MySQL / 本地；逐项结果见实测表 | MX-094 / MX-148 | 1 个任务 |
| `--retain-empty-files` | MySQL / 本地；逐项结果见实测表 | MX-008、MX-036-CSV、MX-036-CUT、MX-036-SQL / — | 4 个任务 |
| `--query-sql` | MySQL / 本地；逐项结果见实测表 | MX-006、MX-006-MULTILINE、MX-006-LONG、MX-007、MX-008 / MX-017、MX-018、MX-019、MX-054、MX-055、MX-081、MX-140、MX-141、MX-142 | 6 个任务 |
| `--include-column-names` | MySQL / 本地；逐项结果见实测表 | MX-025-NONE、MX-035-UTF-8、MX-035-GBK、MX-035-GB2312、MX-035-GB18030、MX-035-Big5、MX-035-ISO-8859-1、MX-035-UTF-16、MX-035-UTF-32、MX-035-US-ASCII、MX-041、MX-150-zstd-1、MX-150-zstd-22、MX-150-zlib--1、MX-150-zlib-9 / MX-047、MX-048、MX-052、MX-053、MX-056、MX-142 | 15 个任务 |
| `--exclude-column-names` | MySQL / 本地；逐项结果见实测表 | MX-042 / MX-052 | 1 个任务 |
| `--exclude-virtual-columns` | MySQL / 本地；逐项结果见实测表 | MX-043 / — | 1 个任务 |
| `--flashback-scn` | MySQL / 本地；逐项结果见实测表 | MX-078 / MX-079 | 1 个任务 |
| `--flashback-timestamp` | Oracle 专属，正例不在本轮；MySQL 越界负例 | — / — | 未观察 |
| `--thread` | MySQL / 本地；逐项结果见实测表 | MX-001、MX-001-LOG、MX-002、MX-003、MX-004、MX-005、MX-006、MX-006-MULTILINE、MX-006-LONG、MX-007、MX-008、MX-011-CSV、MX-011-CUT、MX-011-SQL、MX-020、MX-021、MX-022、MX-023、MX-024、MX-025、MX-025-ALL-NOT-NULL、MX-025-NONE、MX-026、MX-027、MX-028、MX-029、MX-030、MX-031、MX-032、MX-033、MX-034、MX-035-UTF-8、MX-035-GBK、MX-035-GB2312、MX-035-GB18030、MX-035-Big5、MX-035-ISO-8859-1、MX-035-UTF-16、MX-035-UTF-32、MX-035-US-ASCII、MX-036-CSV、MX-037-CSV、MX-036-CUT、MX-037-CUT、MX-036-SQL、MX-037-SQL、MX-040、MX-041、MX-042、MX-043、MX-044、MX-045、MX-051、MX-060-dateValueFormat、MX-060-timeValueFormat、MX-060-datetimeValueFormat、MX-060-timestampValueFormat、MX-061、MX-061-CUT、MX-062、MX-062-CUT、MX-063、MX-070、MX-071、MX-072、MX-078、MX-090、MX-091、MX-092、MX-093、MX-094、MX-095、MX-096、MX-101、MX-110-zstd、MX-110-zlib、MX-110-gzip、MX-110-snappy、MX-146、MX-150-zstd-1、MX-150-zstd-22、MX-150-zlib--1、MX-150-zlib-9 / MX-014、MX-015、MX-016、MX-017、MX-018、MX-019、MX-046、MX-047、MX-048、MX-049、MX-050、MX-052、MX-053、MX-054、MX-055、MX-056、MX-064-timestampTzValueFormat、MX-064-timestampLtzValueFormat、MX-064-nlsDateFormat、MX-064-nlsTimestampFormat、MX-064-nlsTimestampTzFormat、MX-065、MX-079、MX-080、MX-081、MX-097、MX-098、MX-099、MX-100、MX-102、MX-103、MX-104、MX-105、MX-106、MX-111、MX-112、MX-113、MX-114、MX-115、MX-116、MX-117、MX-118、MX-118-NONE-ESCAPE、MX-140、MX-141、MX-142、MX-144、MX-145、MX-147、MX-148、MX-149 | 91 个任务 |
| `--page-size` | MySQL / 本地；逐项结果见实测表 | MX-095 / MX-147 | 3 个任务 |
| `--parallel-macro` | MySQL / 本地；逐项结果见实测表 | MX-096 / — | 1 个任务 |
| `--fetch-size` | Oracle 专属，正例不在本轮；MySQL 越界负例 | — / — | 未观察 |
| `--mem` | MySQL / 本地；逐项结果见实测表 | MX-001、MX-001-LOG、MX-002、MX-003、MX-004、MX-005、MX-006、MX-006-MULTILINE、MX-006-LONG、MX-007、MX-008、MX-011-CSV、MX-011-CUT、MX-011-SQL、MX-020、MX-021、MX-022、MX-023、MX-024、MX-025、MX-025-ALL-NOT-NULL、MX-025-NONE、MX-026、MX-027、MX-028、MX-029、MX-030、MX-031、MX-032、MX-033、MX-034、MX-035-UTF-8、MX-035-GBK、MX-035-GB2312、MX-035-GB18030、MX-035-Big5、MX-035-ISO-8859-1、MX-035-UTF-16、MX-035-UTF-32、MX-035-US-ASCII、MX-036-CSV、MX-037-CSV、MX-036-CUT、MX-037-CUT、MX-036-SQL、MX-037-SQL、MX-040、MX-041、MX-042、MX-043、MX-044、MX-045、MX-051、MX-060-dateValueFormat、MX-060-timeValueFormat、MX-060-datetimeValueFormat、MX-060-timestampValueFormat、MX-061、MX-061-CUT、MX-062、MX-062-CUT、MX-063、MX-070、MX-071、MX-072、MX-078、MX-090、MX-091、MX-092、MX-093、MX-094、MX-095、MX-096、MX-101、MX-110-zstd、MX-110-zlib、MX-110-gzip、MX-110-snappy、MX-146、MX-150-zstd-1、MX-150-zstd-22、MX-150-zlib--1、MX-150-zlib-9 / MX-014、MX-015、MX-016、MX-017、MX-018、MX-019、MX-046、MX-047、MX-048、MX-049、MX-050、MX-052、MX-053、MX-054、MX-055、MX-056、MX-064-timestampTzValueFormat、MX-064-timestampLtzValueFormat、MX-064-nlsDateFormat、MX-064-nlsTimestampFormat、MX-064-nlsTimestampTzFormat、MX-065、MX-079、MX-080、MX-081、MX-097、MX-098、MX-099、MX-100、MX-102、MX-103、MX-104、MX-105、MX-106、MX-111、MX-112、MX-113、MX-114、MX-115、MX-116、MX-117、MX-118、MX-118-NONE-ESCAPE、MX-140、MX-141、MX-142、MX-144、MX-145、MX-147、MX-148、MX-149 | 91 个任务 |
| `--drop-object` | MySQL / 本地；逐项结果见实测表 | MX-074 / — | 1 个任务 |
| `--retain-schema` | MySQL / 本地；逐项结果见实测表 | MX-075 / — | 1 个任务 |
| `--block-size` | MySQL / 本地；逐项结果见实测表 | MX-092、MX-093 / MX-106 | 3 个任务 |
| `--compression-level` | MySQL / 本地；逐项结果见实测表 | MX-110-zstd、MX-110-zlib、MX-150-zstd-1、MX-150-zstd-22、MX-150-zlib--1、MX-150-zlib-9 / MX-112、MX-113 | 6 个任务 |
| `--compact-schema` | MySQL / 本地；逐项结果见实测表 | MX-076 / — | 1 个任务 |
| `--where` | MySQL / 本地；逐项结果见实测表 | MX-040 / MX-054 | 1 个任务 |
| `--snapshot` | MySQL / 本地；逐项结果见实测表 | MX-070 / MX-079 | 1 个任务 |
| `--weak-read` | MySQL / 本地；逐项结果见实测表 | MX-071 / MX-143 | 2 个任务 |
| `--retry` | 目录状态 VALIDATION_GATED；保持原门禁 | — / — | 未观察 |
| `--date-value-format` | MySQL / 本地；逐项结果见实测表 | MX-060-dateValueFormat、MX-061、MX-061-CUT / MX-065、MX-066 | 3 个任务 |
| `--time-value-format` | MySQL / 本地；逐项结果见实测表 | MX-060-timeValueFormat、MX-061、MX-061-CUT / MX-144 | 4 个任务 |
| `--datetime-value-format` | MySQL / 本地；逐项结果见实测表 | MX-060-datetimeValueFormat、MX-061、MX-061-CUT / — | 3 个任务 |
| `--timestamp-value-format` | MySQL / 本地；逐项结果见实测表 | MX-060-timestampValueFormat、MX-061、MX-061-CUT / — | 4 个任务 |
| `--timestamp-tz-value-format` | Oracle 专属，正例不在本轮；MySQL 越界负例 | — / — | 未观察 |
| `--timestamp-ltz-value-format` | Oracle 专属，正例不在本轮；MySQL 越界负例 | — / — | 未观察 |
| `--nls-date-format` | Oracle 专属，正例不在本轮；MySQL 越界负例 | — / — | 未观察 |
| `--nls-timestamp-format` | Oracle 专属，正例不在本轮；MySQL 越界负例 | — / — | 未观察 |
| `--nls-timestamp-tz-format` | Oracle 专属，正例不在本轮；MySQL 越界负例 | — / — | 未观察 |
| `--partition` | MySQL / 本地；逐项结果见实测表 | MX-044、MX-045 / MX-046、MX-049、MX-055 | 2 个任务 |
| `--exclude-data-types` | MySQL / 本地；逐项结果见实测表 | MX-051 / — | 1 个任务 |
| `--enable-hidden-pk` | MySQL / 本地；逐项结果见实测表 | MX-072 / MX-081 | 未观察 |
| `--add-extra-message` | MySQL / 本地；逐项结果见实测表 | MX-073 / MX-073-NO-SYS | 未观察 |
| `--function` | MySQL / 本地；逐项结果见实测表 | MX-012-FUNCTION / — | 1 个任务 |
| `--procedure` | MySQL / 本地；逐项结果见实测表 | MX-012-PROCEDURE / — | 1 个任务 |
| `--sequence` | MySQL / 本地；逐项结果见实测表 | MX-012-SEQUENCE、MX-077-PRESERVE、MX-077-RESTART / — | 3 个任务 |
| `--sequence-policy` | MySQL / 本地；逐项结果见实测表 | MX-077-PRESERVE、MX-077-RESTART / MX-082 | 2 个任务 |
| `--preserve-zero-datetime` | MySQL / 本地；逐项结果见实测表 | MX-062、MX-062-CUT / — | 2 个任务 |
| `--sys-user` | 附加表定义；sys 凭据未配置 | MX-073（受阻） / MX-073-NO-SYS | 未观察 |
| `--sys-password` | 短时安全文件；真实秘密核对与清理见 MX-132；sys 正例依赖已配置 sys | — / — | 未观察 |

连接参数在表中按目录长名归一化；真实生成器可能使用官方短名。Oracle 格式、对象存储、真实检查点与 sys 正例不得由其他通过项外推为可用。
