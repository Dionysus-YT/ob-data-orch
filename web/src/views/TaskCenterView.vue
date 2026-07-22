<template>
  <section class="page-heading task-heading">
    <div><h1>任务中心</h1><p>统一查看导出、普通导入和旁路导入任务。列表只用于定位与进入详情，不在单行展开命令、完整参数或日志。</p></div>
    <div class="task-create-actions">
      <RouterLink class="button button-secondary" to="/exports/new">新建导出</RouterLink>
      <RouterLink class="button button-secondary" to="/imports/normal/new">新建普通导入</RouterLink>
      <RouterLink class="button button-primary" to="/imports/direct/new">新建旁路导入</RouterLink>
    </div>
  </section>

  <section class="filter-bar task-filter-bar" aria-label="任务筛选">
    <label>任务名称 / ID 关键字<input disabled placeholder="功能接入后搜索有权任务" /></label>
    <label>任务类型<select disabled><option>全部</option></select></label>
    <label>任务状态<select disabled><option>全部</option></select></label>
    <label>创建时间<input disabled placeholder="开始日期 ～ 结束日期" /></label>
    <button type="button" class="button button-secondary" disabled>展开筛选</button>
    <button type="button" class="button button-primary" disabled>查询</button>
  </section>

  <section class="scope-bar"><span>默认筛选：全部有权限的任务</span><span>高级条件将包括数据源、环境、执行节点、创建人、开始时间和现有标签。</span></section>

  <section class="content-card table-card">
    <table>
      <thead><tr><th>任务</th><th>类型</th><th>数据源 / 对象</th><th>状态 / 阶段</th><th>进度</th><th>执行节点</th><th>创建人</th><th>时间</th><th>操作</th></tr></thead>
      <tbody><tr><td colspan="9"><div class="empty-state"><div class="empty-mark">□</div><h2>尚无可展示任务</h2><p>任务列表功能尚未接入。接入后只显示当前用户有权访问的任务，并按可靠执行证据展示状态、阶段、进度和结果。</p><div class="task-create-actions"><RouterLink class="button button-primary" to="/exports/new">创建首个导出任务</RouterLink></div></div></td></tr></tbody>
    </table>
  </section>
  <p class="section-hint">任务状态固定为草稿、等待调度、启动中、运行中、取消中、成功、失败、已取消；未知事实将显示“状态核对中”，不伪造失败或进度。</p>
</template>
