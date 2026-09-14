<script setup lang="ts">
import { ref } from 'vue'
import { RouterLink } from 'vue-router'
import { Archive, ArrowDownToLine, ArrowUpFromLine, ChevronRight, CircleHelp, Database, House, Layers3, ListChecks, Logs, PanelsTopLeft, Server, Settings2, UserRound } from '@lucide/vue'
import OrchButton from './OrchButton.vue'
import OrchDialog from './OrchDialog.vue'
const help = ref(false)
const groups = [
  { label: '运行概览', items: [{ label: '首页', to: '/', icon: House }] },
  { label: '任务配置', items: [
    { label: '数据源管理', to: '/data-sources', icon: Database },
    { label: '导出任务', to: '/exports/new', icon: ArrowUpFromLine },
    { label: '普通导入', to: '/imports/normal/new', icon: ArrowDownToLine },
    { label: '旁路导入', to: '/imports/direct/new', icon: Layers3 },
    { label: '模板中心', to: '/templates', icon: PanelsTopLeft },
  ] },
  { label: '运行与支持', items: [
    { label: '任务中心', to: '/tasks', icon: ListChecks },
    { label: '执行节点', to: '/nodes', icon: Server },
    { label: '日志中心', to: '/logs', icon: Logs },
  ] },
  { label: '平台设置', items: [
    { label: '系统设置', to: '/settings', icon: Settings2 },
    { label: '存储凭据', to: '/settings/storage-credentials', icon: Archive },
  ] },
]
</script>

<template>
  <div class="orch-ui orch-application">
    <a class="orch-skip" href="#orch-workspace">跳至工作区</a>
    <aside class="orch-side-nav" aria-label="产品导航">
      <RouterLink to="/data-sources" class="orch-wordmark" aria-label="OB Data Orch 数据源管理"><Database :size="25" class="orch-brand-mark" aria-hidden="true" /><span>OB Data Orch</span></RouterLink>
      <nav class="orch-side-nav-list"><section v-for="group in groups" :key="group.label" class="orch-side-group"><h2>{{ group.label }}</h2><RouterLink v-for="item in group.items" :key="item.label" :to="item.to" :title="item.label" :class="{ 'is-current': item.label === '数据源管理' }" :aria-current="item.label === '数据源管理' ? 'page' : undefined"><component :is="item.icon" :size="17" /><span>{{ item.label }}</span></RouterLink></section></nav>
      <div class="orch-side-footer"><span>OB Loader / Dumper</span><span class="orch-technical">4.3.5</span></div>
    </aside>
    <div class="orch-content-frame">
      <header class="orch-product-header"><div class="orch-header-context" aria-label="当前位置"><span>任务配置</span><ChevronRight :size="14" /><strong>数据源管理</strong></div><div class="orch-global-actions"><OrchButton variant="quiet" label="产品帮助" @click="help = true"><CircleHelp :size="16" /><span>帮助</span></OrchButton><span class="orch-header-divider" /><RouterLink to="/settings/access-control" class="orch-account" title="账户与权限配置" aria-label="账户与权限配置"><UserRound :size="16" /><span>账户</span></RouterLink></div></header>
      <main id="orch-workspace" class="orch-main" tabindex="-1"><slot /></main>
    </div>
    <OrchDialog :open="help" title="OB Data Orch" confirm-label="关闭" @confirm="help = false" @cancel="help = false"><p>OB Loader / Dumper 4.3.5 的轻量可视化编排平台。</p><p>数据源保存与连接测试相互独立。真实连接测试由已授权的执行节点完成，页面不会直接连接数据库。</p><RouterLink to="/nodes" class="orch-inline-link" @click="help = false">查看执行节点<ChevronRight :size="14" /></RouterLink></OrchDialog>
  </div>
</template>
