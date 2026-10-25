<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Alert as AAlert, Button as AButton, Descriptions as ADescriptions, DescriptionsItem as ADescriptionsItem, Space as ASpace, Spin as ASpin } from 'ant-design-vue'
import { browserApi } from '@/api/browser'
import { usePageEntryPosition } from '@/composables/usePageEntryPosition'
import { useToolCompatibility } from '@/workbench/tools/useToolCompatibility'

const content = ref<HTMLElement>(), state = useToolCompatibility(browserApi()), route = useRoute()
usePageEntryPosition(content)
watch(() => route.fullPath, () => { state.reset(); void state.read() }, { flush: 'sync' })
onMounted(() => { void state.read() })
const moduleName = { OBDUMPER_EXPORT: '导出', OBLOADER_NORMAL_IMPORT: '普通导入', OBLOADER_DIRECT_LOAD: '旁路导入' } as const
</script>

<template>
  <div ref="content" class="orch-ui">
    <header class="orch-page-header"><div class="orch-page-title"><h1>工具与兼容</h1><p class="orch-context-label">查看工具目标、模块参数目录及草稿和模板的兼容边界。</p></div></header>
    <section class="settings-main settings-sections" aria-label="工具与兼容内容" :aria-busy="state.loading.value">
      <AAlert type="info" show-icon message="工具与兼容当前为只读状态" description="当前目标与参数目录来自控制面内置基线；具体能力仍需满足对应任务条件，节点实际版本以环境检查结果为准。" />
      <AAlert v-if="state.failure.value" type="error" show-icon :message="state.failure.value" />
      <div v-if="state.loading.value" role="status"><ASpin size="small" /> 正在读取工具状态…</div>
      <template v-if="state.baseline.value">
        <section v-for="tool in state.baseline.value.tools" :key="tool.tool" :aria-labelledby="`tools-${tool.tool}-title`">
          <h2 :id="`tools-${tool.tool}-title`">{{ tool.tool }}</h2>
          <ADescriptions :column="1">
            <ADescriptionsItem label="受控目标版本">{{ tool.supportedVersions.join('、') }}</ADescriptionsItem>
            <ADescriptionsItem label="新建默认目标">{{ tool.defaultTargetVersion }} · 模块内置目标</ADescriptionsItem>
          </ADescriptions>
          <template v-for="module in tool.modules" :key="module.taskType">
            <h3>{{ moduleName[module.taskType] }}参数元数据</h3>
            <ADescriptions :column="1">
              <ADescriptionsItem label="目录选择">{{ module.metadataSelection === 'FIXED' ? '使用本模块固定目录' : '按任务配置选择，不设全平台单一活动版本' }}</ADescriptionsItem>
              <ADescriptionsItem label="当前配置目录"><ASpace direction="vertical" size="small"><span v-for="catalog in module.catalogs.filter(item => item.usage === 'TASK_CONFIGURATION')" :key="catalog.metadataVersion">{{ catalog.metadataVersion }}</span></ASpace></ADescriptionsItem>
              <ADescriptionsItem v-if="module.catalogs.some(item => item.usage === 'HISTORICAL_REPLAY')" label="仅历史重放"><ASpace direction="vertical" size="small"><span v-for="catalog in module.catalogs.filter(item => item.usage === 'HISTORICAL_REPLAY')" :key="catalog.metadataVersion">{{ catalog.metadataVersion }}</span></ASpace></ADescriptionsItem>
            </ADescriptions>
          </template>
        </section>
        <section aria-labelledby="tools-compatibility-title">
          <h2 id="tools-compatibility-title">兼容与历史对象</h2>
          <ADescriptions :column="1">
            <ADescriptionsItem label="节点实际版本">在执行节点中查看环境检查结果；平台目标不代表节点已安装或可执行。</ADescriptionsItem>
            <ADescriptionsItem label="草稿与模板">新建、重新编辑或使用时按对应模块规则核对；未知或不兼容时阻断后续操作。</ADescriptionsItem>
            <ADescriptionsItem label="历史任务">已提交任务保留原参数版本、配置和命令快照，不随基线变化改写。</ADescriptionsItem>
            <ADescriptionsItem label="当前来源">控制面内置工具与模块参数目录</ADescriptionsItem>
            <ADescriptionsItem label="本次载入时间">{{ state.baseline.value.loadedAt }} · UTC</ADescriptionsItem>
            <ADescriptionsItem label="本次读取时间">{{ state.baseline.value.capturedAt }} · UTC</ADescriptionsItem>
          </ADescriptions>
          <p class="orch-context-label">载入时间不代表目录发布时间。历史重放目录不作为新建候选；普通导入与旁路导入分别校验，本页不安装工具或激活其他目录。</p>
        </section>
      </template>
      <ASpace><AButton :loading="state.loading.value" @click="state.read">刷新工具状态</AButton></ASpace>
    </section>
  </div>
</template>
