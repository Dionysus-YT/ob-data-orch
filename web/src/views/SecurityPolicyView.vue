<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Alert as AAlert, Button as AButton, Descriptions as ADescriptions, DescriptionsItem as ADescriptionsItem, Space as ASpace, Spin as ASpin } from 'ant-design-vue'
import { browserApi } from '@/api/browser'
import { usePageEntryPosition } from '@/composables/usePageEntryPosition'
import { useSecurityPolicy } from '@/workbench/security/useSecurityPolicy'

const content = ref<HTMLElement>(), state = useSecurityPolicy(browserApi()), route = useRoute()
usePageEntryPosition(content)
watch(() => route.fullPath, () => { state.reset(); void state.read() }, { flush: 'sync' })
onMounted(() => { void state.read() })
const enabled = (value: boolean) => value ? '开启' : '关闭'
</script>

<template>
  <div ref="content" class="orch-ui">
    <header class="orch-page-header"><div class="orch-page-title"><h1>安全策略</h1><p class="orch-context-label">查看控制面安全状态；用户角色及对象范围在权限配置中管理。</p></div></header>
    <section class="settings-main settings-sections" aria-label="安全策略内容" :aria-busy="state.loading.value">
      <AAlert type="info" show-icon message="安全策略当前为只读状态" description="以下事实来自本次控制面启动配置。配置状态不替代任务授权、预检查或目标环境验证。" />
      <AAlert v-if="state.failure.value" type="error" show-icon :message="state.failure.value" />
      <div v-if="state.loading.value" role="status"><ASpin size="small" /> 正在读取安全状态…</div>
      <template v-if="state.policy.value">
        <section aria-labelledby="security-baseline-title">
          <h2 id="security-baseline-title">安全基线</h2>
          <ADescriptions :column="1">
            <ADescriptionsItem label="内置脱敏">已启用且不可关闭；页面、复制、下载、错误、日志和审计保持保护规则。</ADescriptionsItem>
            <ADescriptionsItem label="凭据加密格式">{{ state.policy.value.credentialFormat }}</ADescriptionsItem>
            <ADescriptionsItem label="敏感命令原文">未开放；普通脱敏命令查看和复制不受影响。</ADescriptionsItem>
            <ADescriptionsItem label="生产任务确认">由任务提交规则执行，本页不提供关闭入口。</ADescriptionsItem>
          </ADescriptions>
        </section>
        <section aria-labelledby="security-key-title">
          <h2 id="security-key-title">密钥保护</h2>
          <ADescriptions :column="1">
            <ADescriptionsItem label="密钥载入状态">已载入</ADescriptionsItem>
            <ADescriptionsItem label="密钥版本标识">{{ state.policy.value.credentialKeyId }}</ADescriptionsItem>
            <ADescriptionsItem label="来源">本机受保护密钥载体</ADescriptionsItem>
            <ADescriptionsItem label="启动载入时间">{{ state.policy.value.loadedAt }} · UTC</ADescriptionsItem>
          </ADescriptions>
          <p class="orch-context-label">载入时间不代表轮换时间或当前文件权限检查时间；密钥材料不进入页面。</p>
        </section>
        <section aria-labelledby="security-controls-title">
          <h2 id="security-controls-title">执行能力与审计</h2>
          <ADescriptions :column="1">
            <ADescriptionsItem label="控制面真实执行">{{ enabled(state.policy.value.realExecutionEnabled) }}</ADescriptionsItem>
            <ADescriptionsItem label="普通导入执行">{{ enabled(state.policy.value.normalImportExecutionEnabled) }}</ADescriptionsItem>
            <ADescriptionsItem label="旁路导入探针">{{ enabled(state.policy.value.directLoadProbeEnabled) }}</ADescriptionsItem>
            <ADescriptionsItem label="旁路导入执行">{{ enabled(state.policy.value.directLoadExecutionEnabled) }}</ADescriptionsItem>
            <ADescriptionsItem label="审计日志保留">未配置期限，当前不自动清理；普通任务日志期限不适用于审计。</ADescriptionsItem>
            <ADescriptionsItem label="本次读取时间">{{ state.policy.value.capturedAt }} · UTC</ADescriptionsItem>
          </ADescriptions>
          <p class="orch-context-label">执行开关仅表示控制面许可；节点自身开关、用户授权和各任务执行门禁仍需满足。</p>
        </section>
      </template>
      <ASpace><AButton :loading="state.loading.value" @click="state.read">刷新安全状态</AButton></ASpace>
    </section>
  </div>
</template>
