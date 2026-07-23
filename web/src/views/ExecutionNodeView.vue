<template>
  <section class="page-heading task-heading"><div><h1>执行节点</h1><p>只管理 Agent 注册、在线、环境、容量与任务支撑事实；平台不会通过 SSH、WinRM 或远程 Shell 安装、控制节点或执行任意命令。</p></div><button class="button button-primary" disabled>注册节点</button></section>
  <section class="filter-bar"><label>节点名称 / 主机名关键字<input disabled placeholder="功能接入后搜索有权节点" /></label><label>管理状态<select disabled><option>全部</option></select></label><label>心跳状态<select disabled><option>全部</option></select></label><label>接收新任务<select disabled><option>全部</option></select></label><button class="button button-secondary" disabled>展开筛选</button><button class="button button-primary" disabled>查询</button></section>
  <section class="content-card table-card"><table><thead><tr><th>节点名称</th><th>主机摘要</th><th>管理状态</th><th>在线状态</th><th>工具摘要</th><th>接收新任务</th><th>任务容量</th><th>资源摘要</th><th>操作</th></tr></thead><tbody><tr><td colspan="9"><div class="empty-state"><div class="empty-mark">□</div><h2>尚无已登记执行节点</h2><p>接入后节点通过 Agent 主动关联并上报心跳、工具与固定环境检查事实；离线不等于运行任务已失败。</p></div></td></tr></tbody></table></section>
  <p class="section-hint">维护或禁用只阻止接收新任务，不自动取消运行任务；任务实际输入/输出路径、数据库、对象与参数仍由具体任务预检查。</p>
</template>
