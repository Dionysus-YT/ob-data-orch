<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

import EmptyState from '@/components/EmptyState.vue'

const route = useRoute()
const title = computed(() => route.meta.title as string)
const description = computed(() => route.meta.description as string)
const sections = computed(() => (route.meta.sections as string[] | undefined) ?? [])
</script>

<template>
  <section class="page-heading"><div><h1>{{ title }}</h1><p>{{ description }}</p></div></section>
  <section v-if="sections.length" class="module-sections"><article v-for="section in sections" :key="section" class="content-card section-placeholder"><h2>{{ section }}</h2><p>页面结构已按已确认低保真基线建立；该模块尚未接入业务数据，不展示示例对象或虚构状态。</p></article></section>
  <EmptyState v-else :title="`${title}暂无数据`" description="本页面只在对应模块接入后展示当前身份可见的真实数据。" />
</template>
