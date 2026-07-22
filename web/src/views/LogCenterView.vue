<template>
  <section class="page-heading"><div><h1>日志中心</h1><p>跨任务、节点、调度和审计的脱敏日志查询入口；不提供任意节点文件访问、原始未脱敏日志或日志分析平台能力。</p></div></section>
  <section class="log-tabs" aria-label="日志查询预设"><button type="button" class="active" disabled>任务日志</button><button type="button" disabled>节点 / Agent 日志</button><button type="button" disabled>调度日志</button><button type="button" disabled>审计日志</button></section>
  <section class="filter-bar log-filter-bar" aria-label="日志筛选">
    <label>时间范围 <input disabled value="最近 1 小时" /></label><label>任务 ID / 名称 <input disabled placeholder="可选" /></label><label>节点 <select disabled><option>全部</option></select></label><label>日志级别 <select disabled><option>全部（含未标级别）</option></select></label><label>关键字 <input disabled placeholder="脱敏文本包含查询" /></label>
    <button type="button" class="button button-secondary" disabled>更多条件</button><button type="button" class="button button-primary" disabled>查询</button>
  </section>
  <section class="scope-bar"><span>首次进入已预设“任务日志 + 最近 1 小时”，但不会自动执行宽范围查询。</span><span>原始来源、任务类型、状态、用户 / 操作人、事件与跟踪标识作为展开条件。</span></section>
  <section class="content-card log-result-shell">
    <div class="log-result-header"><div><strong>查询结果</strong><span>尚未查询</span></div><div><button class="link-button" disabled>手动刷新</button><button class="link-button" disabled>下载脱敏日志</button></div></div>
    <div class="empty-state"><div class="empty-mark">▤</div><h2>等待日志查询</h2><p>查询功能接入后，结果会保留日志原始来源、来源时间、接收时间、原始级别与采集完整性；stdout 不会自动标为 INFO，stderr 也不会自动标为 ERROR。</p></div>
  </section>
  <p class="section-hint">采集延迟、日志缺口与无匹配分别表达，均不直接改变任务终态；页面、复制与下载采用同一脱敏规则。</p>
</template>
