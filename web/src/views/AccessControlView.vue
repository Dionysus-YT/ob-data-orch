<template>
  <section class="page-heading">
    <div>
      <h1>权限配置</h1>
      <p>位于系统设置的安全分区。固定角色、对象范围和独立高风险能力共同决定有效权限；本页不创建认证账号、修改身份或重置密码。</p>
    </div>
    <RouterLink class="button button-secondary" to="/settings">返回系统设置</RouterLink>
  </section>

  <section class="context-note"><span>ⓘ</span><span>权限变更、生产任务与敏感命令均由服务端重新校验。系统管理员不天然获得业务对象、生产任务或敏感命令访问权。</span></section>

  <section class="filter-bar" aria-label="已认证用户筛选">
    <label>登录标识 / 显示名称<input disabled placeholder="身份目录接入后搜索" /></label>
    <label>固定角色<select disabled><option>全部</option></select></label>
    <label>账号状态<select disabled><option>全部</option></select></label>
    <label>高风险能力<select disabled><option>全部</option></select></label>
    <button type="button" class="button button-primary" disabled>查询</button>
  </section>

  <section class="content-card table-card">
    <table>
      <thead><tr><th>登录标识</th><th>账号状态</th><th>固定角色</th><th>对象范围</th><th>独立能力</th><th>最近修改</th><th>操作</th></tr></thead>
      <tbody><tr><td colspan="7"><div class="empty-state"><div class="empty-mark">⌑</div><h2>尚无可展示的已认证用户</h2><p>身份和权限服务接入后，仅展示当前操作者有权查看的用户与授权摘要。无权限、无匹配、身份同步失败和未知状态将分别表达，且不泄露数量。</p></div></td></tr></tbody>
    </table>
  </section>

  <section class="module-sections access-control-guides">
    <article class="content-card section-placeholder"><h2>固定角色与有效权限</h2><p>普通用户、运维人员、数据源管理员、节点管理员和系统管理员可多选；多角色只合并功能能力，不自动扩大对象范围或安全条件。</p></article>
    <article class="content-card section-placeholder"><h2>对象范围与独立能力</h2><p>数据源、节点、任务和模板范围分别配置。生产任务、专家配置、敏感命令和审计日志均需独立能力及相应依赖。</p></article>
    <article class="content-card section-placeholder"><h2>原子保存与并发冲突</h2><p>保存前比较新增、保留和移除项，变更原因必填。出现版本冲突时阻断覆盖，刷新后重新比较；不提供部分保存。</p></article>
    <article class="content-card section-placeholder"><h2>运行时高风险门禁</h2><p>生产任务需对象范围、预检查、脱敏命令和二次确认共同通过；敏感命令还需总开关、独立能力和审计，未脱敏日志永不开放。</p></article>
  </section>
</template>
