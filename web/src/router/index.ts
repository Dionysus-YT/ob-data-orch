import { createRouter, createWebHistory } from 'vue-router'

import AccessControlView from '@/views/AccessControlView.vue'
import DataSourceFormView from '@/views/DataSourceFormView.vue'
import DataSourceListView from '@/views/DataSourceListView.vue'
import DirectLoadWizardView from '@/views/DirectLoadWizardView.vue'
import ExecutionNodeDetailView from '@/views/ExecutionNodeDetailView.vue'
import ExportWizardView from '@/views/ExportWizardView.vue'
import ExecutionNodeView from '@/views/ExecutionNodeView.vue'
import HomeView from '@/views/HomeView.vue'
import LogCenterView from '@/views/LogCenterView.vue'
import NormalImportWizardView from '@/views/NormalImportWizardView.vue'
import NotFoundView from '@/views/NotFoundView.vue'
import TaskCenterView from '@/views/TaskCenterView.vue'
import TaskDetailView from '@/views/TaskDetailView.vue'
import TemplateCenterView from '@/views/TemplateCenterView.vue'
import SystemSettingsView from '@/views/SystemSettingsView.vue'

export const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    { path: '/', name: 'home', component: HomeView, meta: { title: '首页' } },
    { path: '/data-sources', name: 'data-sources', component: DataSourceListView, meta: { title: '数据源管理' } },
    { path: '/data-sources/:id', name: 'data-source-form', component: DataSourceFormView, meta: { title: '数据源管理' } },
    { path: '/exports/new', name: 'new-export', component: ExportWizardView, meta: { title: '导出任务' } },
    { path: '/imports/normal/new', name: 'new-normal-import', component: NormalImportWizardView, meta: { title: '普通导入' } },
    { path: '/imports/direct/new', name: 'new-direct-import', component: DirectLoadWizardView, meta: { title: '旁路导入' } },
    { path: '/tasks', name: 'tasks', component: TaskCenterView, meta: { title: '任务中心' } },
    { path: '/tasks/:id', name: 'task-detail', component: TaskDetailView, meta: { title: '任务详情' } },
    { path: '/templates', name: 'templates', component: TemplateCenterView, meta: { title: '模板中心' } },
    { path: '/nodes', name: 'nodes', component: ExecutionNodeView, meta: { title: '执行节点' } },
    { path: '/nodes/:id', name: 'node-detail', component: ExecutionNodeDetailView, meta: { title: '执行节点详情' } },
    { path: '/logs', name: 'logs', component: LogCenterView, meta: { title: '日志中心' } },
    { path: '/settings', name: 'settings', component: SystemSettingsView, meta: { title: '系统设置' } },
    { path: '/settings/access-control', name: 'access-control', component: AccessControlView, meta: { title: '权限配置' } },
    { path: '/:pathMatch(.*)*', name: 'not-found', component: NotFoundView, meta: { title: '页面不存在' } },
  ],
})
