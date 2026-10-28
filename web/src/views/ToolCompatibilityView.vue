<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Alert as AAlert, Button as AButton, Collapse as ACollapse, CollapsePanel as ACollapsePanel, Descriptions as ADescriptions, DescriptionsItem as ADescriptionsItem, Space as ASpace, Spin as ASpin } from 'ant-design-vue'
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
  <div ref="content" class="settings-domain" aria-label="工具与兼容内容" :aria-busy="state.loading.value">
    <header class="settings-domain-header"><h2>工具与兼容</h2><AButton :loading="state.loading.value" @click="state.read">刷新工具状态</AButton></header>
    <AAlert type="info" show-icon message="工具与兼容当前为只读状态" description="目标版本不代表节点安装与执行资格，实际版本以执行节点环境检查为准。" />
    <AAlert v-if="state.failure.value" type="error" show-icon :message="state.failure.value" />
    <div v-if="state.loading.value" role="status"><ASpin size="small" /> 正在读取工具状态…</div>
    <template v-if="state.baseline.value">
      <section v-for="tool in state.baseline.value.tools" :key="tool.tool" class="settings-group" :aria-labelledby="`tools-${tool.tool}-title`">
        <div class="settings-group-label"><h3 :id="`tools-${tool.tool}-title`">{{ tool.tool }}</h3></div>
        <div class="settings-group-body">
          <ADescriptions :column="1" size="small" :colon="false" :label-style="{ width: 'min(168px, 40%)', flexShrink: 0, color: 'var(--ob-color-form-secondary)' }" :content-style="{ minWidth: 0 }">
            <ADescriptionsItem :label="tool.supportedVersions.length === 1 && tool.supportedVersions.includes(tool.defaultTargetVersion) ? '目标版本（新建默认）' : '受控目标版本'"><strong>{{ tool.supportedVersions.join('、') }}</strong></ADescriptionsItem>
            <ADescriptionsItem v-if="tool.supportedVersions.length !== 1 || !tool.supportedVersions.includes(tool.defaultTargetVersion)" label="新建默认目标">{{ tool.defaultTargetVersion }}</ADescriptionsItem>
          </ADescriptions>
          <section v-for="module in tool.modules" :key="module.taskType" class="settings-catalog" :aria-label="`${moduleName[module.taskType]}参数元数据`">
            <h3>{{ moduleName[module.taskType] }}参数元数据</h3>
            <ADescriptions :column="1" size="small" :colon="false" :label-style="{ width: 'min(168px, 40%)', flexShrink: 0, color: 'var(--ob-color-form-secondary)' }" :content-style="{ minWidth: 0 }">
              <ADescriptionsItem label="目录选择">{{ module.metadataSelection === 'FIXED' ? '使用本模块固定目录' : '按任务配置选择，不设全平台单一活动版本' }}</ADescriptionsItem>
            </ADescriptions>
            <ACollapse ghost class="settings-details">
              <ACollapsePanel key="catalogs" header="参数目录">
                <ADescriptions :column="1" size="small">
                  <ADescriptionsItem label="当前配置目录"><ASpace direction="vertical" size="small"><span v-for="catalog in module.catalogs.filter(item => item.usage === 'TASK_CONFIGURATION')" :key="catalog.metadataVersion">{{ catalog.metadataVersion }}</span></ASpace></ADescriptionsItem>
                  <ADescriptionsItem v-if="module.catalogs.some(item => item.usage === 'HISTORICAL_REPLAY')" label="仅历史重放"><ASpace direction="vertical" size="small"><span v-for="catalog in module.catalogs.filter(item => item.usage === 'HISTORICAL_REPLAY')" :key="catalog.metadataVersion">{{ catalog.metadataVersion }}</span></ASpace></ADescriptionsItem>
                </ADescriptions>
              </ACollapsePanel>
            </ACollapse>
          </section>
        </div>
      </section>
      <section class="settings-group" aria-labelledby="tools-compatibility-title">
        <div class="settings-group-label"><h3 id="tools-compatibility-title">兼容与历史对象</h3></div>
        <div class="settings-group-body">
          <ADescriptions :column="1" size="small" :colon="false" :label-style="{ width: 'min(168px, 40%)', flexShrink: 0, color: 'var(--ob-color-form-secondary)' }" :content-style="{ minWidth: 0 }">
            <ADescriptionsItem label="草稿与模板">新建、重新编辑或使用时按对应模块规则核对；未知或不兼容时阻断后续操作。</ADescriptionsItem>
            <ADescriptionsItem label="历史任务">已提交任务保留原参数版本、配置和命令快照，不随基线变化改写。</ADescriptionsItem>
          </ADescriptions>
        </div>
      </section>
      <ACollapse ghost class="settings-details">
        <ACollapsePanel key="details" header="配置详情">
          <ADescriptions :column="1" size="small">
            <ADescriptionsItem label="当前来源">控制面内置工具与模块参数目录</ADescriptionsItem>
            <ADescriptionsItem label="本次载入时间">{{ state.baseline.value.loadedAt }} · UTC</ADescriptionsItem>
            <ADescriptionsItem label="本次读取时间">{{ state.baseline.value.capturedAt }} · UTC</ADescriptionsItem>
            <ADescriptionsItem label="载入时间口径">控制面载入时间，不代表目录发布时间。</ADescriptionsItem>
          </ADescriptions>
        </ACollapsePanel>
      </ACollapse>
    </template>
  </div>
</template>
