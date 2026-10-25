<script setup lang="ts">
import { ref } from 'vue'
import { Alert as AAlert } from 'ant-design-vue'
import { usePageEntryPosition } from '@/composables/usePageEntryPosition'
import SchedulingPolicySettings from '@/workbench/scheduling/SchedulingPolicySettings.vue'
import SchedulingTimingSettings from '@/workbench/scheduling/SchedulingTimingSettings.vue'
import SchedulingCandidatesSettings from '@/workbench/scheduling/SchedulingCandidatesSettings.vue'

const content = ref<HTMLElement>()
usePageEntryPosition(content)
</script>

<template>
  <div ref="content" class="orch-ui">
    <header class="orch-page-header"><div class="orch-page-title"><h1>调度策略</h1><p class="orch-context-label">管理全平台并发、时间策略与节点候选顺序。</p></div></header>
    <section class="settings-main settings-sections" aria-label="调度策略内容">
      <AAlert type="info" show-icon message="并发、时间与候选顺序分别编辑和保存" description="节点并发由 JVM 内存预算约束，预算在执行节点中维护；候选顺序只影响列表，始终由用户显式选择节点。" />
      <section aria-labelledby="scheduling-concurrency-title">
        <h2 id="scheduling-concurrency-title">并发约束</h2>
        <SchedulingPolicySettings />
      </section>
      <section aria-labelledby="scheduling-timing-title">
        <h2 id="scheduling-timing-title">时间策略</h2>
        <SchedulingTimingSettings />
      </section>
      <section aria-labelledby="scheduling-candidates-title">
        <h2 id="scheduling-candidates-title">节点选择</h2>
        <SchedulingCandidatesSettings />
      </section>
    </section>
  </div>
</template>
