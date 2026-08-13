<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { ArrowDownToLine, ArrowUpFromLine, CircleHelp, Database, House, LayoutTemplate, ListTodo, ScrollText, Server, Settings, Zap } from '@lucide/vue'
import WorkbenchIconButton from './WorkbenchIconButton.vue'

const route = useRoute()

const navGroups = [
  { label: '运行概览', items: [{ label: '首页', to: '/', icon: House }] },
  { label: '任务配置', items: [{ label: '数据源管理', to: '/data-sources', icon: Database }, { label: '导出任务', to: '/exports/new', icon: ArrowUpFromLine }, { label: '普通导入', to: '/imports/normal/new', icon: ArrowDownToLine }, { label: '旁路导入', to: '/imports/direct/new', icon: Zap }] },
  { label: '运行与支撑', items: [{ label: '任务中心', to: '/tasks', icon: ListTodo }, { label: '模板中心', to: '/templates', icon: LayoutTemplate }, { label: '执行节点', to: '/nodes', icon: Server }, { label: '日志中心', to: '/logs', icon: ScrollText }] },
  { label: '平台设置', items: [{ label: '系统设置', to: '/settings', icon: Settings }] },
]

const wideContent = computed(() => route.meta.contentWidth === 'wide')
</script>

<template>
  <div class="product-layout">
    <aside class="side-nav" aria-label="主导航">
      <RouterLink class="brand" to="/">
        <span class="brand-mark">OB</span>
        <span>OB Data Orch</span>
      </RouterLink>
      <nav>
        <section v-for="group in navGroups" :key="group.label" class="nav-group">
          <p>{{ group.label }}</p>
          <RouterLink v-for="item in group.items" :key="item.to" :to="item.to" class="nav-item" :class="{ active: route.path === item.to || (item.to !== '/' && route.path.startsWith(item.to)) }">
            <component :is="item.icon" class="nav-icon" :size="16" :stroke-width="1.75" aria-hidden="true" />{{ item.label }}
          </RouterLink>
        </section>
      </nav>
    </aside>
    <div class="page-frame">
      <header class="topbar">
        <div class="topbar-actions">
          <WorkbenchIconButton label="帮助"><CircleHelp :size="16" :stroke-width="1.75" aria-hidden="true" /></WorkbenchIconButton>
          <span class="topbar-divider" aria-hidden="true" />
          <span class="user-avatar">管</span><span>管理员</span>
        </div>
      </header>
      <main class="page-content" :class="{ 'page-content-wide': wideContent }"><slot /></main>
    </div>
  </div>
</template>
