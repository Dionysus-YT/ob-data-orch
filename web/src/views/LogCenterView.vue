<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Alert as AAlert, Button as AButton, ConfigProvider, Spin as ASpin, Tabs as ATabs, TabPane as ATabPane } from 'ant-design-vue'
import { browserApi } from '@/api/browser'
import { managementTheme } from '@/platform/theme'
import { usePageEntryPosition } from '@/composables/usePageEntryPosition'
import { useLogPresets } from '@/workbench/log-query/useLogPresets'
import TaskLogWorkspace from '@/workbench/log-query/TaskLogWorkspace.vue'
import AuditLogWorkspace from '@/workbench/audit-logs/AuditLogWorkspace.vue'
import '@/workbench/log-query/logQuery.css'
const route = useRoute(), router = useRouter(), root = ref<HTMLElement>(), access = useLogPresets(browserApi())
usePageEntryPosition(root)
const preset = computed(() => route.query.preset === 'audit' && access.allowed.value ? 'audit' : 'task')
watch(() => route.fullPath, () => { void access.load() }, { immediate: true, flush: 'sync' })
function selectPreset(key: string | number) { if (key === 'audit' && !access.allowed.value) return; void router.replace({ name: 'logs', query: key === 'audit' ? { preset: 'audit' } : {} }) }
</script>
<template>
  <ConfigProvider :theme="managementTheme">
    <div ref="root" class="orch-ui log-query-workspace">
      <header class="orch-page-header"><div class="orch-page-title"><h1>日志中心</h1><span v-if="!access.allowed.value" class="orch-context-label">任务日志</span></div></header>
      <AAlert v-if="access.failure.value" class="orch-feedback" type="error" show-icon :message="access.failure.value"><template #action><AButton :loading="access.loading.value" @click="access.load">重试确认资格</AButton></template></AAlert>
      <AAlert v-else-if="route.query.preset === 'audit' && !access.loading.value && !access.allowed.value" class="orch-feedback" type="info" show-icon message="审计日志需单独授予“审计访问”能力，系统管理员角色不会自动获得。" />
      <ATabs v-if="access.allowed.value" :active-key="preset" @change="selectPreset"><ATabPane key="task" tab="任务日志" /><ATabPane key="audit" tab="审计日志" /></ATabs>
      <ASpin v-if="route.query.preset === 'audit' && access.loading.value" aria-label="正在确认审计访问资格" />
      <AuditLogWorkspace v-else-if="preset === 'audit'" :key="route.fullPath" @denied="access.revoke" />
      <TaskLogWorkspace v-else />
    </div>
  </ConfigProvider>
</template>
