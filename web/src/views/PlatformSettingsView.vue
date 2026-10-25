<script setup lang="ts">
import { ref } from 'vue'
import { Alert as AAlert, Descriptions as ADescriptions, DescriptionsItem as ADescriptionsItem } from 'ant-design-vue'
import { usePageEntryPosition } from '@/composables/usePageEntryPosition'
import LogRetentionSettings from '@/workbench/platform-settings/LogRetentionSettings.vue'

const content = ref<HTMLElement>()
usePageEntryPosition(content)
</script>

<template>
  <div ref="content" class="orch-ui">
    <header class="orch-page-header"><div class="orch-page-title"><h1>平台设置</h1><p class="orch-context-label">管理平台展示默认、任务名称规则及任务与普通日志的保留策略。</p></div></header>
    <section class="settings-main settings-sections" aria-label="平台设置内容">
      <AAlert type="info" show-icon message="当前开放普通任务日志保留策略" description="基础配置、任务记录保留和任务名称规则尚未开放修改。" />
      <section aria-labelledby="platform-basics-title">
        <h2 id="platform-basics-title">基础配置</h2>
        <ADescriptions :column="1">
          <ADescriptionsItem label="管理范围">平台名称、默认时区、界面语言和列表默认分页大小。</ADescriptionsItem>
          <ADescriptionsItem label="生效边界">展示默认仅影响后续页面和会话，不改写原始时间或历史数据。</ADescriptionsItem>
        </ADescriptions>
      </section>
      <section aria-labelledby="platform-retention-title">
        <h2 id="platform-retention-title">保留策略</h2>
        <LogRetentionSettings />
      </section>
      <section aria-labelledby="platform-naming-title">
        <h2 id="platform-naming-title">任务名称规则</h2>
        <ADescriptions :column="1">
          <ADescriptionsItem label="适用范围">仅用于新建和编辑草稿的名称校验，不进入工具命令，不回写历史任务。</ADescriptionsItem>
          <ADescriptionsItem label="配置方式">使用受控的长度、字符和前缀规则；不接受脚本或任意正则表达式。</ADescriptionsItem>
        </ADescriptions>
      </section>
    </section>
  </div>
</template>
