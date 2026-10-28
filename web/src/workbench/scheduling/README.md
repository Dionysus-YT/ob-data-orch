# 调度策略维护入口

- `SchedulingPolicySettings.vue`：带单位的分组 Form、只读/编辑切换、配置详情折叠、影响确认和页面离开/退出确认；无全局配置树。
- `useSchedulingPolicy.ts`：唯一策略事实、表单、冻结确认、写事务、幂等重试及会话代，取消/卸载不能恢复旧响应。
- `api/schedulingPolicy.ts`：严格安全投影与回执解码，传输复用既有浏览器安全边界。
- `SchedulingTimingSettings.vue` / `useSchedulingTiming.ts`：时间策略独立视图与唯一读取、表单、冻结差异、保存和迟到响应所有者；不复制并发域状态，两域分别保存与确认放弃。
- `api/schedulingTiming.ts`：实际心跳来源、空覆盖、固定显式选择与严格回执投影；不把控制面检查间隔当成 Agent 网络周期。
- `SchedulingCandidatesSettings.vue` / `useSchedulingCandidates.ts`：候选顺序独立事实、表单、影响核对及写事务，保持真实来源、未知回执重试和路由会话隔离；不持有任务节点选择。
- 2026-10-26 表单整改：三项时间字段宽屏并排、窄屏单列；候选顺序用原生 Radio.Group，非编辑态锁定。所有输入显示均从现有策略或当前编辑表单派生，不自动开始编辑，不把未知结果的表单值当作已生效事实。来源、版本、修改与采样记录按需展开，复用原事务与失败关闭边界。
- `api/schedulingCandidates.ts`：`EXISTING/ONLINE_FIRST` 严格投影与白名单写入；保存后复读，候选顺序改变不自动选中或改派。
- `internal/store/scheduling_policy.go` 负责原子策略保存，`scheduling_admission.go` 负责与领取共事务的准入。业务边界见 `docs/03-technical/scheduling-policy-contract.md`。
- `internal/store/scheduling_timing*.go` / `scheduling_monitor.go`：时间事务、领取时最新心跳/检查间隔、冻结启动期限及待核对投影；监视器的固定生命周期由组合根装配，预算只由既有可靠证据释放。
- `internal/schedulingpolicy/candidates.go` 负责稳定分组与心跳证据边界；`internal/store/scheduling_candidates.go` 保存独立策略并按原 NODE_MANAGE 范围读取普通/旁路候选快照与预算。控制面在已授权集合内读取最小心跳、排序并返回前复验；导出保持 NODE_USE 精简投影。管理列表与连接测试不使用任务候选顺序。
- 全平台数量无权时为空，不以本人数量或 0 代替。SS-NODE-02 已按用户方案 1 确认：节点沿用 JVM 内存预算，预算在节点域维护，取消旧的新节点任务数默认项；Agent Worker 技术上界仍有效，不把预算换算为固定任务数。无自动选节点或推荐算法。
