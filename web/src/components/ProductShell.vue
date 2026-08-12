<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink, useRoute } from 'vue-router'

const route = useRoute()

const navGroups = [
  { label: '运行概览', items: [{ label: '首页', to: '/', icon: '⌂' }] },
  { label: '任务配置', items: [{ label: '数据源管理', to: '/data-sources', icon: '◌' }, { label: '导出任务', to: '/exports/new', icon: '↗' }, { label: '普通导入', to: '/imports/normal/new', icon: '↙' }, { label: '旁路导入', to: '/imports/direct/new', icon: '⇣' }] },
  { label: '运行与支撑', items: [{ label: '任务中心', to: '/tasks', icon: '□' }, { label: '模板中心', to: '/templates', icon: '▤' }, { label: '执行节点', to: '/nodes', icon: '◇' }, { label: '日志中心', to: '/logs', icon: '≡' }] },
  { label: '平台设置', items: [{ label: '系统设置', to: '/settings', icon: '⚙' }] },
]

const pageTitle = computed(() => typeof route.meta.title === 'string' ? route.meta.title : 'OB Data Orch')
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
            <span aria-hidden="true">{{ item.icon }}</span>{{ item.label }}
          </RouterLink>
        </section>
      </nav>
      <p class="nav-stage">产品页面基线<br><span>功能逐模块接入</span></p>
    </aside>
    <div class="page-frame">
      <header class="topbar">
        <div><span class="breadcrumb">OB Data Orch /</span> {{ pageTitle }}</div>
        <div class="topbar-actions"><button type="button" class="text-button">帮助</button><span class="user-avatar">管</span><span>管理员</span></div>
      </header>
      <main class="page-content" :class="{ 'page-content-wide': wideContent }"><slot /></main>
    </div>
  </div>
</template>
