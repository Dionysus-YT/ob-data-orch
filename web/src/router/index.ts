import { createRouter, createWebHistory } from 'vue-router'

import AccessControlView from '@/views/AccessControlView.vue'
import DirectLoadWizardView from '@/views/DirectLoadWizardView.vue'
import ExecutionNodeDetailView from '@/views/ExecutionNodeDetailView.vue'
import ExecutionNodeFormView from '@/views/ExecutionNodeFormView.vue'
import ExportWizardView from '@/views/ExportWizardView.vue'
import ExecutionNodeView from '@/views/ExecutionNodeView.vue'
import HomeView from '@/views/HomeView.vue'
import LogCenterView from '@/views/LogCenterView.vue'
import NormalImportWizardView from '@/views/NormalImportWizardView.vue'
import NotFoundView from '@/views/NotFoundView.vue'
import StorageCredentialsView from '@/views/StorageCredentialsView.vue'
import TaskCenterView from '@/views/TaskCenterView.vue'
import TaskDetailView from '@/views/TaskDetailView.vue'
import TemplateCenterView from '@/views/TemplateCenterView.vue'
import SystemSettingsView from '@/views/SystemSettingsView.vue'

export const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    { path: '/workbench/data-sources', name: 'workbench-data-sources', redirect: (to) => ({ name: 'data-sources', query: to.query }) },
    { path: '/', name: 'home', component: HomeView, meta: { title: '首页' } },
    { path: '/data-sources', name: 'data-sources', component: () => import('@/workbench/sources/SourceWorkspace.vue'), meta: { title: '数据源管理', contentWidth: 'wide' } },
    { path: '/data-sources/:id', redirect: (to) => ({ name: 'data-sources', query: { edit: String(to.params.id) } }) },
    { path: '/exports/new', name: 'new-export', component: ExportWizardView, meta: { title: '导出任务' } },
    { path: '/imports/normal/new', name: 'new-normal-import', component: NormalImportWizardView, meta: { title: '普通导入' } },
    { path: '/imports/direct/new', name: 'new-direct-import', component: DirectLoadWizardView, meta: { title: '旁路导入' } },
    { path: '/tasks', name: 'tasks', component: TaskCenterView, meta: { title: '任务中心' } },
    { path: '/tasks/:id', name: 'task-detail', component: TaskDetailView, meta: { title: '任务详情' } },
    { path: '/templates', name: 'templates', component: TemplateCenterView, meta: { title: '模板中心' } },
    { path: '/nodes', name: 'nodes', component: ExecutionNodeView, meta: { title: '执行节点' } },
    { path: '/nodes/new', name: 'node-new', component: ExecutionNodeFormView, meta: { title: '注册执行节点' } },
    { path: '/nodes/:id/edit', name: 'node-edit', component: ExecutionNodeFormView, meta: { title: '编辑执行节点' } },
    { path: '/nodes/:id', name: 'node-detail', component: ExecutionNodeDetailView, meta: { title: '执行节点详情' } },
    { path: '/logs', name: 'logs', component: LogCenterView, meta: { title: '日志中心' } },
    { path: '/settings', name: 'settings', component: SystemSettingsView, meta: { title: '系统设置' } },
    { path: '/settings/access-control', name: 'access-control', component: AccessControlView, meta: { title: '权限配置' } },
    { path: '/settings/storage-credentials', name: 'storage-credentials', component: StorageCredentialsView, meta: { title: '存储凭据' } },
    { path: '/:pathMatch(.*)*', name: 'not-found', component: NotFoundView, meta: { title: '页面不存在' } },
  ],
})
