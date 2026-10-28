<script setup lang="ts">
import { ref } from 'vue'
import { Collapse as ACollapse, CollapsePanel as ACollapsePanel } from 'ant-design-vue'
import { usePageEntryPosition } from '@/composables/usePageEntryPosition'
import LogRetentionSettings from '@/workbench/platform-settings/LogRetentionSettings.vue'

const content = ref<HTMLElement>()
usePageEntryPosition(content)
</script>

<template>
  <div ref="content" class="settings-domain" aria-label="常规设置内容">
    <header class="settings-domain-header"><h2>常规设置</h2></header>
    <section class="settings-group" aria-labelledby="platform-retention-title">
      <div class="settings-group-label"><h3 id="platform-retention-title">保留策略</h3><p>自动清理已结束任务的普通日志，任务记录和审计日志不在此范围。</p></div>
      <LogRetentionSettings />
    </section>
    <ACollapse ghost class="settings-details">
      <ACollapsePanel key="unavailable" header="未开放设置">
        <p>平台名称、默认时区、界面语言、默认分页、任务记录保留和任务名称规则暂不支持修改。</p>
      </ACollapsePanel>
    </ACollapse>
  </div>
</template>
