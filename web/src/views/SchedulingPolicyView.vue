<script setup lang="ts">
import { ref } from 'vue'
import { usePageEntryPosition } from '@/composables/usePageEntryPosition'
import SchedulingPolicySettings from '@/workbench/scheduling/SchedulingPolicySettings.vue'
import SchedulingTimingSettings from '@/workbench/scheduling/SchedulingTimingSettings.vue'
import SchedulingCandidatesSettings from '@/workbench/scheduling/SchedulingCandidatesSettings.vue'

const content = ref<HTMLElement>()
usePageEntryPosition(content)
</script>

<template>
  <div ref="content" class="settings-domain" aria-label="调度策略内容">
    <header class="settings-domain-header"><h2>调度策略</h2></header>
    <section class="settings-group" aria-labelledby="scheduling-concurrency-title">
      <div class="settings-group-label"><h3 id="scheduling-concurrency-title">并发约束</h3><p>限制全平台同时占用的执行数量。节点资源预算在执行节点中维护。</p></div>
      <SchedulingPolicySettings />
    </section>
    <section class="settings-group" aria-labelledby="scheduling-timing-title">
      <div class="settings-group-label"><h3 id="scheduling-timing-title">时间策略</h3></div>
      <SchedulingTimingSettings />
    </section>
    <section class="settings-group" aria-labelledby="scheduling-candidates-title">
      <div class="settings-group-label"><h3 id="scheduling-candidates-title">节点选择</h3></div>
      <SchedulingCandidatesSettings />
    </section>
  </div>
</template>
