<script setup lang="ts">
import { FileTextOutlined } from '@ant-design/icons-vue'
import { Button as AButton, Input as AInput, Select as ASelect, SelectOption as ASelectOption } from 'ant-design-vue'
</script>
<template>
  <section class="page-heading"><div><h1>日志中心</h1><p>跨任务、节点、调度和审计的脱敏日志查询入口；不提供任意节点文件访问、原始未脱敏日志或日志分析平台能力。</p></div></section>
  <section class="log-tabs" aria-label="日志查询预设"><AButton disabled class="active">任务日志</AButton><AButton disabled>节点 / Agent 日志</AButton><AButton disabled>调度日志</AButton><AButton disabled>审计日志</AButton></section>
  <section class="filter-bar log-filter-bar" aria-label="日志筛选">
    <label>时间范围 <AInput aria-label="时间范围" disabled value="最近 1 小时" /></label><label>任务 ID / 名称 <AInput aria-label="任务 ID / 名称" disabled placeholder="可选" /></label><label>节点 <ASelect default-value="全部" aria-label="节点" disabled><ASelectOption value="全部">全部</ASelectOption></ASelect></label><label>日志级别 <ASelect default-value="全部（含未标级别）" aria-label="日志级别" disabled><ASelectOption value="全部（含未标级别）">全部（含未标级别）</ASelectOption></ASelect></label><label>关键字 <AInput aria-label="关键字" disabled placeholder="脱敏文本包含查询" /></label>
    <AButton disabled>更多条件</AButton><AButton disabled type="primary">查询</AButton>
  </section>
  <section class="scope-bar"><span>首次进入已预设“任务日志 + 最近 1 小时”，但不会自动执行宽范围查询。</span><span>原始来源、任务类型、状态、用户 / 操作人、事件与跟踪标识作为展开条件。</span></section>
  <section class="content-card log-result-shell">
    <div class="log-result-header"><div><strong>查询结果</strong><span>尚未查询</span></div><div><AButton disabled type="text">手动刷新</AButton><AButton disabled type="text">下载脱敏日志</AButton></div></div>
    <div class="empty-state"><div class="empty-mark"><FileTextOutlined class="product-icon" aria-hidden="true" /></div><h2>等待日志查询</h2><p>查询功能接入后，结果会保留日志原始来源、来源时间、接收时间、原始级别与采集完整性；stdout 不会自动标为 INFO，stderr 也不会自动标为 ERROR。</p></div>
  </section>
  <p class="section-hint">采集延迟、日志缺口与无匹配分别表达，均不直接改变任务终态；页面、复制与下载采用同一脱敏规则。</p>
</template>
