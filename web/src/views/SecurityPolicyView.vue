<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Alert as AAlert, Badge as ABadge, Button as AButton, Collapse as ACollapse, CollapsePanel as ACollapsePanel, Descriptions as ADescriptions, DescriptionsItem as ADescriptionsItem, Spin as ASpin } from 'ant-design-vue'
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
  <div ref="content" class="settings-domain" aria-label="安全策略内容" :aria-busy="state.loading.value">
    <header class="settings-domain-header"><h2>安全策略</h2><AButton :loading="state.loading.value" @click="state.read">刷新安全状态</AButton></header>
    <AAlert type="info" show-icon message="安全策略当前为只读状态" />
    <AAlert v-if="state.failure.value" type="error" show-icon :message="state.failure.value" />
    <div v-if="state.loading.value" role="status"><ASpin size="small" /> 正在读取安全状态…</div>
    <template v-if="state.policy.value">
      <section class="settings-group" aria-labelledby="security-baseline-title">
        <div class="settings-group-label"><h3 id="security-baseline-title">安全基线</h3></div>
        <ADescriptions :column="1" size="small" :colon="false" :label-style="{ width: 'min(168px, 40%)', flexShrink: 0, color: 'var(--ob-color-form-secondary)' }" :content-style="{ minWidth: 0 }">
          <ADescriptionsItem label="内置脱敏"><ABadge status="success" text="已启用 · 不可关闭" /></ADescriptionsItem>
          <ADescriptionsItem label="凭据加密格式">{{ state.policy.value.credentialFormat }}</ADescriptionsItem>
          <ADescriptionsItem label="敏感命令原文"><ABadge status="default" text="未开放" />；普通脱敏命令查看和复制不受影响。</ADescriptionsItem>
          <ADescriptionsItem label="生产任务确认"><ABadge status="processing" text="提交时执行 · 不可关闭" /></ADescriptionsItem>
        </ADescriptions>
      </section>
      <section class="settings-group" aria-labelledby="security-key-title">
        <div class="settings-group-label"><h3 id="security-key-title">密钥保护</h3></div>
        <div class="settings-group-body">
          <ADescriptions :column="1" size="small" :colon="false" :label-style="{ width: 'min(168px, 40%)', flexShrink: 0, color: 'var(--ob-color-form-secondary)' }" :content-style="{ minWidth: 0 }">
            <ADescriptionsItem label="密钥载入状态"><ABadge status="success" text="已载入" /></ADescriptionsItem>
            <ADescriptionsItem label="密钥版本标识">{{ state.policy.value.credentialKeyId }}</ADescriptionsItem>
          </ADescriptions>
        </div>
      </section>
      <section class="settings-group" aria-labelledby="security-controls-title">
        <div class="settings-group-label"><h3 id="security-controls-title">执行能力与审计</h3></div>
        <div class="settings-group-body">
          <ADescriptions :column="1" size="small" :colon="false" :label-style="{ width: 'min(168px, 40%)', flexShrink: 0, color: 'var(--ob-color-form-secondary)' }" :content-style="{ minWidth: 0 }">
            <ADescriptionsItem label="控制面真实执行"><ABadge :status="state.policy.value.realExecutionEnabled ? 'success' : 'default'" :text="enabled(state.policy.value.realExecutionEnabled)" /></ADescriptionsItem>
            <ADescriptionsItem label="普通导入执行"><ABadge :status="state.policy.value.normalImportExecutionEnabled ? 'success' : 'default'" :text="enabled(state.policy.value.normalImportExecutionEnabled)" /></ADescriptionsItem>
            <ADescriptionsItem label="旁路导入探针"><ABadge :status="state.policy.value.directLoadProbeEnabled ? 'success' : 'default'" :text="enabled(state.policy.value.directLoadProbeEnabled)" /></ADescriptionsItem>
            <ADescriptionsItem label="旁路导入执行"><ABadge :status="state.policy.value.directLoadExecutionEnabled ? 'success' : 'default'" :text="enabled(state.policy.value.directLoadExecutionEnabled)" /></ADescriptionsItem>
            <ADescriptionsItem label="审计日志保留">未配置期限，当前不自动清理；普通任务日志期限不适用于审计。</ADescriptionsItem>
          </ADescriptions>
          <p class="orch-context-label">开启仅表示控制面许可，执行仍需满足节点开关、用户授权及任务预检查。</p>
        </div>
      </section>
      <ACollapse ghost class="settings-details">
        <ACollapsePanel key="details" header="配置详情">
          <ADescriptions :column="1" size="small">
            <ADescriptionsItem label="配置来源">本次控制面启动配置</ADescriptionsItem>
            <ADescriptionsItem label="密钥来源">本机受保护密钥载体</ADescriptionsItem>
            <ADescriptionsItem label="启动载入时间">{{ state.policy.value.loadedAt }} · UTC</ADescriptionsItem>
            <ADescriptionsItem label="本次读取时间">{{ state.policy.value.capturedAt }} · UTC</ADescriptionsItem>
            <ADescriptionsItem label="载入时间边界">不代表轮换时间或当前文件权限检查时间；密钥材料不进入页面。</ADescriptionsItem>
          </ADescriptions>
        </ACollapsePanel>
      </ACollapse>
    </template>
  </div>
</template>
