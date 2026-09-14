<script setup lang="ts">
import { computed } from 'vue'
import { CircleHelp } from '@lucide/vue'

import FoundationButton from './FoundationButton.vue'

const props = defineProps<{ active: 'overview' | 'sources' | 'builder' | 'detail' }>()

const breadcrumb = computed(() => ({
  overview: '运行概览 / 首页',
  sources: '任务配置 / 数据源管理',
  builder: '任务配置 / 导出任务',
  detail: '运行与支撑 / 任务中心',
}[props.active]))

const groups = [
  { label: '运行概览', items: [{ label: '首页', page: 'overview' }] },
  { label: '任务配置', items: [{ label: '数据源管理', page: 'sources' }, { label: '导出任务', page: 'builder' }, { label: '普通导入' }, { label: '旁路导入' }] },
  { label: '运行与支撑', items: [{ label: '任务中心', page: 'detail' }, { label: '模板中心' }, { label: '执行节点' }, { label: '日志中心' }] },
  { label: '平台设置', items: [{ label: '系统设置' }, { label: '存储凭据' }] },
]
</script>

<template>
  <div class="vf-root">
    <aside class="vf-nav" aria-label="Visual Foundation Lab 主导航">
      <div class="vf-brand"><b>OB</b><strong>OB Data Orch</strong></div>
      <nav v-for="group in groups" :key="group.label" class="vf-nav-group">
        <p>{{ group.label }}</p>
        <button v-for="item in group.items" :key="item.label" :class="{ active: item.page === active }">{{ item.label }}</button>
      </nav>
    </aside>
    <main class="vf-main">
      <header class="vf-top">
        <span>{{ breadcrumb }}</span>
        <div><FoundationButton variant="utility" icon aria-label="帮助"><CircleHelp :size="16" /></FoundationButton><span class="vf-account">管理员</span></div>
      </header>
      <slot />
    </main>
  </div>
</template>
