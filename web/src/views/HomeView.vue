<script setup lang="ts">
import EmptyState from '@/components/EmptyState.vue'
</script>

<template>
  <section class="page-heading"><div><h1>首页</h1><p>在授权范围内快速了解任务和执行节点的当前运行情况。</p></div><button type="button" class="button button-secondary">刷新快照</button></section>
  <section class="scope-bar"><span>授权范围：当前身份可查看的对象</span><span>显示时区：待系统设置接入</span><span>快照状态：尚未采集</span></section>
  <EmptyState title="尚无可展示的运行快照" description="任务、执行节点和异常数据接入后，首页只展示授权范围内的真实运行事实，不以示例数据替代。" />
</template>
