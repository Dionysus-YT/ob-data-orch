<template>
  <section class="page-heading task-heading"><div><h1>模板中心</h1><p>复用三类任务的显式、非敏感配置；模板只是新草稿的起点，不能直接执行或复用预检查与风险确认。</p></div><button class="button button-primary" disabled>新建模板</button></section>
  <section class="filter-bar"><label>模板名称、描述或标签<input disabled placeholder="功能接入后搜索有权模板" /></label><label>模板类型<select disabled><option>全部</option></select></label><label>兼容状态<select disabled><option>全部</option></select></label><button class="button button-secondary" disabled>更多筛选</button><button class="button button-primary" disabled>查询</button></section>
  <section class="content-card table-card"><table><thead><tr><th>模板名称</th><th>类型</th><th>数据源策略</th><th>工具版本</th><th>兼容状态</th><th>高风险配置</th><th>来源</th><th>更新时间</th><th>操作</th></tr></thead><tbody><tr><td colspan="9"><div class="empty-state"><div class="empty-mark">□</div><h2>尚无可展示模板</h2><p>接入后可手工新建，或从成功任务保存模板；列表不会显示凭据、密钥、节点事实、预检查、命令、日志或结果。</p></div></td></tr></tbody></table></section>
  <p class="section-hint">使用模板一定创建新的可编辑草稿，并重新选择/确认数据源、节点、路径、权限与预检查。三类模板不可互相转换。</p>
</template>
