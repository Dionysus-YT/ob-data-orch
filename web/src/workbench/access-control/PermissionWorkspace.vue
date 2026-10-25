<script setup lang="ts">
import { computed, nextTick, onMounted, watch } from 'vue'
import { Alert as AAlert, Badge as ABadge, Button as AButton, ConfigProvider, Input as AInput, Select as ASelect, Tag as ATag, type TableColumnsType } from 'ant-design-vue'
import { LeftOutlined, ReloadOutlined, RightOutlined, SearchOutlined } from '@ant-design/icons-vue'
import { browserApi } from '@/api/browser'
import { permissionCapabilities, permissionRoles, type PermissionProfile } from '@/api/accessControl'
import OrchOperationalTable from '@/components/OrchOperationalTable.vue'
import { useOperationMessage } from '@/composables/useOperationMessage'
import { managementTheme } from '@/platform/theme'
import PermissionEditor from './PermissionEditor.vue'
import { accountLabels, capabilityLabels, permissionTime, roleLabels, scopeSummary } from './permissionRules'
import { usePermissionList } from './usePermissionList'
import { usePermissionEditor } from './usePermissionEditor'

const api = browserApi()
const { MessageContext, success, clear } = useOperationMessage()
let invoker: HTMLElement | undefined
const list = usePermissionList(api, { onDenied: () => { editor.reset(); clear() }, onIdentityChanged: () => { editor.reset(); clear() } })
const editor = usePermissionEditor(api, { canManage: () => list.granted.value, currentSubjectId: () => list.currentSubjectId.value, onDenied: () => list.invalidate(), onSaved: result => { success(result.replayed ? '已确认原请求的保存回执，当前权限以重新读取结果为准。' : result.changed ? '权限已保存，后续请求按新授权校验。' : '权限配置没有变化。'); void list.read(true) } })
watch(editor.session, () => clear(), { flush: 'sync' })
// 保存后的目录复验会临时禁用行操作，等新事实就绪再恢复触发控件焦点。
async function restoreFocus() { if (editor.open.value || list.loading.value || !list.granted.value) return; await nextTick(); if (!editor.open.value && invoker?.isConnected && !invoker.matches(':disabled')) invoker.focus({ preventScroll: true }) }
watch(list.granted, granted => { if (granted) void restoreFocus() }, { flush: 'post' })
function openSubject(id: string, event: MouseEvent) { invoker = event.currentTarget instanceof HTMLElement ? event.currentTarget : undefined; void editor.openSubject(id) }
const columns: TableColumnsType<PermissionProfile> = [
  { key: 'subject', title: '用户', width: 190 }, { key: 'status', title: '账号状态', width: 100 },
  { key: 'roles', title: '固定角色', width: 210 }, { key: 'scopes', title: '对象范围', width: 240 },
  { key: 'capabilities', title: '独立能力', width: 140 }, { key: 'modified', title: '最近修改 · UTC', width: 165 },
  { key: 'actions', title: '操作', width: 110, fixed: 'right', className: 'orch-row-actions' },
]
const roleOptions = [{ value: '', label: '全部角色' }, ...permissionRoles.map(value => ({ value, label: roleLabels[value] }))]
const statusOptions = [{ value: '', label: '全部账号状态' }, ...Object.entries(accountLabels).map(([value, label]) => ({ value, label }))]
const capOptions = [{ value: '', label: '全部独立能力' }, ...permissionCapabilities.map(value => ({ value, label: capabilityLabels[value] }))]
const rows = computed(() => list.items.value.map(item => ({ ...item, id: item.subject.id })))
function candidateDenied(session: number) { if (session === editor.session.value && editor.open.value) list.invalidate() }
onMounted(() => { void list.read() })
</script>

<template>
  <ConfigProvider :theme="managementTheme">
    <div class="orch-ui">
      <MessageContext />
      <header class="orch-page-header"><div class="orch-page-title"><h1>权限配置</h1><p class="orch-context-label">配置已认证用户的固定角色与明确对象范围，保存前核对权限差异。</p></div></header>
      <section class="orch-management permission-workspace">
        <div class="orch-table-meta"><h2>用户权限</h2><AButton :loading="list.loading.value" @click="() => list.read()"><template #icon><ReloadOutlined class="product-icon" aria-hidden="true" /></template>刷新</AButton></div>
        <AAlert v-if="list.failure.value" type="error" show-icon :message="list.failure.value" />
        <form class="orch-toolbar permission-filters" aria-label="用户权限筛选" @submit.prevent="list.apply">
          <AInput v-model:value="list.form.search" aria-label="登录标识或显示名称" placeholder="搜索登录标识或显示名称" :maxlength="128" allow-clear />
          <ASelect v-model:value="list.form.role" aria-label="固定角色筛选" :options="roleOptions" />
          <ASelect v-model:value="list.form.status" aria-label="账号状态筛选" :options="statusOptions" />
          <ASelect v-model:value="list.form.capability" aria-label="独立能力筛选" :options="capOptions" />
          <AButton html-type="submit" type="primary" :loading="list.loading.value"><template #icon><SearchOutlined class="product-icon" aria-hidden="true" /></template>查询</AButton><AButton :disabled="!list.hasFilters.value" @click="list.clear">重置</AButton>
        </form>
        <p class="permission-directory-note">账号信息只读；仅可修改其他用户的授权，当前登录用户的权限只读。</p>
        <OrchOperationalTable :rows="rows" :columns="columns" row-key="id" :minimum-width="1155" :loading="list.loading.value" label="用户权限列表">
          <template #bodyCell="{ record, column }">
            <div v-if="column.key === 'subject'" class="permission-identity"><strong :title="record.subject.displayName">{{ record.subject.displayName }}</strong><span class="permission-login"><span :title="record.subject.login">{{ record.subject.login }}</span><ATag v-if="record.subject.id === list.currentSubjectId.value">当前用户</ATag></span></div>
            <ABadge v-else-if="column.key === 'status'" :status="record.subject.accountStatus === 'ACTIVE' ? 'success' : record.subject.accountStatus === 'DISABLED' ? 'default' : 'warning'" :text="accountLabels[record.subject.accountStatus]" />
            <div v-else-if="column.key === 'roles'" class="permission-tags"><ATag v-for="role in record.assignment.roles" :key="role">{{ roleLabels[role] }}</ATag><span v-if="!record.assignment.roles.length" class="permission-muted">未配置</span></div>
            <span v-else-if="column.key === 'scopes'">{{ scopeSummary(record.assignment) }}</span>
            <div v-else-if="column.key === 'capabilities'" class="permission-tags"><ATag v-for="capability in record.assignment.capabilities" :key="capability">{{ capabilityLabels[capability] }}</ATag><span v-if="!record.assignment.capabilities.length" class="permission-muted">未配置</span></div>
            <span v-else-if="column.key === 'modified'">{{ permissionTime(record.modifiedAt) }}</span>
            <AButton v-else-if="column.key === 'actions'" type="link" :disabled="!list.granted.value || editor.saving.value" :aria-label="`${record.editable ? '配置' : '查看'} ${record.subject.displayName} 的权限`" @click="event => openSubject(record.subject.id, event)">{{ record.editable ? '配置权限' : '查看权限' }}</AButton>
          </template>
          <template #empty><div class="empty-state"><h2>{{ list.loading.value ? '正在读取用户权限' : list.failure.value ? '权限目录暂不可用' : list.hasFilters.value ? '没有匹配的用户' : '暂无已认证用户' }}</h2><p>{{ list.failure.value ? '请重试读取并核对当前身份的访问资格。' : '列表仅包含服务端当前授权范围内的可信主体。' }}</p></div></template>
        </OrchOperationalTable>
        <footer class="orch-table-footer"><span>{{ list.granted.value ? `本页 ${list.items.value.length} 位用户` : '等待可信目录' }}</span><nav class="permission-pagination" aria-label="用户权限分页"><span>10 条/页</span><AButton type="text" aria-label="上一页" :disabled="!list.granted.value || !list.canPrevious.value" @click="list.previous"><template #icon><LeftOutlined class="product-icon" aria-hidden="true" /></template></AButton><span aria-live="polite">第 {{ list.pageNumber.value }} 页</span><AButton type="text" aria-label="下一页" :disabled="!list.granted.value || !list.canNext.value" @click="list.next"><template #icon><RightOutlined class="product-icon" aria-hidden="true" /></template></AButton></nav></footer>
      </section>
      <PermissionEditor :model="editor" :api="api" @denied="candidateDenied" @closed="restoreFocus" />
    </div>
  </ConfigProvider>
</template>

<style scoped>
.permission-workspace { min-inline-size: 0; }
.permission-directory-note { color: var(--ob-color-secondary); margin: 0; padding: 0 var(--ob-foundation-space-6) var(--ob-foundation-space-4); }
.permission-filters { display: flex; flex-wrap: wrap; gap: var(--ob-foundation-space-2); }
.permission-filters > .ant-input-affix-wrapper { flex: 1 1 230px; }
.permission-filters > .ant-select { flex: 0 1 165px; min-inline-size: 145px; }
.permission-identity { display: grid; gap: var(--ob-foundation-space-1); min-inline-size: 0; }
.permission-identity > strong, .permission-login > span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-inline-size: 0; }
.permission-login { display: flex; align-items: center; gap: var(--ob-foundation-space-1); min-inline-size: 0; }
.permission-login > span { flex: 0 1 auto; }
.permission-login > .ant-tag { flex: none; margin: 0; }
.permission-identity > span, .permission-muted { color: var(--ob-color-secondary); }
.permission-tags { display: flex; flex-wrap: wrap; gap: var(--ob-foundation-space-1); }
.permission-tags .ant-tag { margin: 0; }
.permission-pagination { display: flex; align-items: center; flex-wrap: wrap; gap: var(--ob-foundation-space-2); margin-inline-start: auto; }
</style>
