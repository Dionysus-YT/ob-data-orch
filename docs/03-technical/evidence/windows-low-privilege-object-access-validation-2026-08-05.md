# Windows 低权限对象访问预检查验证（2026-08-05）

> 验证状态：已完成受控低权限负例；WI-03 的成功账号、低权限账号和不存在对象负例均已有现场证据
> 适用范围：已登记的非生产低权限数据源与 Windows `test` 执行节点
> 关联：F2、WI-03、WI-06、VS-P0-06

## 1. 授权范围与安全边界

- 用户明确授权使用已登记低权限账号、`test` 执行节点、固定数据库和固定单表执行一次 `EXPORT_PREFLIGHT` 复测。
- 本次只创建单表 CSV 草稿并执行固定六项预检查；没有点击“提交并启动导出”，没有创建任务或 execution，也没有启动 OBDUMPER。
- 数据库秘密仅由既有短时槽位交给 Agent 私有 JDBC 探针；浏览器、终端输出、SQLite 预检查结果、Agent 日志和本文均不保存或展示密码、原始数据库异常、SQLState、错误号或异常正文。
- 使用新的输出路径；预检查结束后该路径仍不存在，没有导出结果或工具日志产物需要清理。

## 2. 实际步骤与结果

1. 通过内置浏览器向导选择已登记低权限数据源、固定对象和 `test · WINDOWS_AMD64` 节点，创建新的 CSV 草稿。
2. 页面发起固定 `EXPORT_PREFLIGHT`，运行中的标准 Agent 完成 `claim → acknowledge → resolve secret → complete`，SQLite 预检查完整性为 `COMPLETE`。
3. 持久化六项结果如下：
   - `DATABASE_CONNECTIVITY = PASSED / DATABASE_CONNECTED`
   - `OBJECT_ACCESS = FAILED / OBJECT_NOT_ACCESSIBLE`
   - `TOOL_ENVIRONMENT = PASSED / TOOL_RUNTIME_READY`
   - `OUTPUT_PATH = PASSED / OUTPUT_PATH_WRITABLE`
   - `OUTPUT_EMPTY = PASSED / OUTPUT_PATH_EMPTY`
   - `AVAILABLE_SPACE = PASSED / OUTPUT_SPACE_SUFFICIENT`
4. 页面只汇总 1 项阻断结果，显示“数据库连接已完成，但当前账号无法访问所选表。可能是读取权限不足，或对象不存在、不可见；请核对库表名、兼容模式和对象授权。”并展示稳定原因码 `OBJECT_NOT_ACCESSIBLE`。
5. “提交并启动导出”保持禁用。SQLite 中该预检查关联任务数为 0；进程核对没有 OBDUMPER，输出路径不存在。
6. `result_json` 仅包含固定检查名、状态和稳定证据码；未发现 SQLState、JDBC 异常类型、数据库错误正文或权限错误原文。仓库秘密扫描通过。

## 3. 描述合理性与结论

本次已知输入是低权限账号，因此现场根因满足“读取权限不足”。产品响应没有直接断言对象存在，也没有返回数据库原始权限错误，而是把权限不足、对象不存在和对象不可见统一为 `OBJECT_NOT_ACCESSIBLE`。这一描述既明确告诉操作者当前账号无法访问所选表，也保持对象防枚举和错误脱敏边界，属于合理且可操作的安全描述。

结合 2026-08-04 已完成的成功账号路径和不存在对象负例，WI-03 的三类账号/对象场景已有现场证据，可以标记通过。本结论不替代 CSV 特殊值、无密码 argv、清理/恢复、跨目标平台或 WI-01～WI-12 其余门禁，G3 仍保持未通过。
