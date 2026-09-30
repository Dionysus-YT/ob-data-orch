<script setup lang="ts">
import { LockOutlined, InfoCircleOutlined } from '@ant-design/icons-vue'
import OrchOperationalTable from '@/components/OrchOperationalTable.vue'
const columns = [{"key":"c0","title":"登录标识"},{"key":"c1","title":"账号状态"},{"key":"c2","title":"固定角色"},{"key":"c3","title":"对象范围"},{"key":"c4","title":"独立能力"},{"key":"c5","title":"最近修改"},{"key":"c6","title":"操作"}]

import { Button as AButton, Input as AInput, Select as ASelect, SelectOption as ASelectOption } from 'ant-design-vue'
</script>
<template>
  <section class="page-heading">
    <div>
      <h1>权限配置</h1>
      <p>位于系统设置的安全分区。固定角色、对象范围和独立高风险能力共同决定有效权限；本页不创建认证账号、修改身份或重置密码。</p>
    </div>
    <RouterLink v-slot="{ href, navigate }" to="/settings" custom><AButton :href="href" @click="navigate">返回系统设置</AButton></RouterLink>
  </section>

  <section class="context-note"><span><InfoCircleOutlined class="product-icon" aria-hidden="true" /></span><span>权限变更、生产任务与敏感命令均由服务端重新校验。系统管理员不天然获得业务对象、生产任务或敏感命令访问权。</span></section>

  <section class="filter-bar" aria-label="已认证用户筛选">
    <label>登录标识 / 显示名称<AInput aria-label="登录标识 / 显示名称" disabled placeholder="身份目录接入后搜索" /></label>
    <label>固定角色<ASelect default-value="全部" aria-label="固定角色" disabled><ASelectOption value="全部">全部</ASelectOption></ASelect></label>
    <label>账号状态<ASelect default-value="全部" aria-label="账号状态" disabled><ASelectOption value="全部">全部</ASelectOption></ASelect></label>
    <label>高风险能力<ASelect default-value="全部" aria-label="高风险能力" disabled><ASelectOption value="全部">全部</ASelectOption></ASelect></label>
    <AButton disabled type="primary">查询</AButton>
  </section>

  <section class="content-card table-card">
    <OrchOperationalTable :rows="[]" :columns="columns" row-key="id" label="权限列表">
      <template #empty><div><div class="empty-state"><div class="empty-mark"><LockOutlined class="product-icon" aria-hidden="true" /></div><h2>尚无可展示的已认证用户</h2><p>身份和权限服务接入后，仅展示当前操作者有权查看的用户与授权摘要。无权限、无匹配、身份同步失败和未知状态将分别表达，且不泄露数量。</p></div></div></template>
    </OrchOperationalTable>
  </section>

  <section class="module-sections access-control-guides">
    <article class="content-card section-placeholder"><h2>固定角色与有效权限</h2><p>普通用户、运维人员、数据源管理员、节点管理员和系统管理员可多选；多角色只合并功能能力，不自动扩大对象范围或安全条件。</p></article>
    <article class="content-card section-placeholder"><h2>对象范围与独立能力</h2><p>数据源、节点、任务和模板范围分别配置。生产任务、专家配置、敏感命令和审计日志均需独立能力及相应依赖。</p></article>
    <article class="content-card section-placeholder"><h2>原子保存与并发冲突</h2><p>保存前比较新增、保留和移除项，变更原因必填。出现版本冲突时阻断覆盖，刷新后重新比较；不提供部分保存。</p></article>
    <article class="content-card section-placeholder"><h2>运行时高风险门禁</h2><p>生产任务需对象范围、预检查、脱敏命令和二次确认共同通过；敏感命令还需总开关、独立能力和审计，未脱敏日志永不开放。</p></article>
  </section>
</template>
