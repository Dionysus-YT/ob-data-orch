import { createRouter, createWebHistory } from 'vue-router'

import DataSourceFormView from '@/views/DataSourceFormView.vue'
import DataSourceListView from '@/views/DataSourceListView.vue'
import DirectLoadWizardView from '@/views/DirectLoadWizardView.vue'
import ExportWizardView from '@/views/ExportWizardView.vue'
import ExecutionNodeView from '@/views/ExecutionNodeView.vue'
import HomeView from '@/views/HomeView.vue'
import LogCenterView from '@/views/LogCenterView.vue'
import ModulePlaceholderView from '@/views/ModulePlaceholderView.vue'
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
    { path: '/nodes/:id', name: 'node-detail', component: ModulePlaceholderView, meta: { title: '执行节点详情', description: '查看节点注册事实、环境检查、维护状态与关联任务；控制面不会直接操作节点文件或进程。', sections: ['节点概览', '工具环境', '关联任务与事件'] } },
    { path: '/logs', name: 'logs', component: LogCenterView, meta: { title: '日志中心' } },
    { path: '/settings', name: 'settings', component: SystemSettingsView, meta: { title: '系统设置' } },
    { path: '/settings/access-control', name: 'access-control', component: ModulePlaceholderView, meta: { title: '权限与安全', description: '权限配置位于系统设置的安全分区，只管理已认证用户的授权，不创建身份账号。', sections: ['用户与权限列表', '固定角色与对象范围', '高风险能力'] } },
    { path: '/:pathMatch(.*)*', name: 'not-found', component: NotFoundView, meta: { title: '页面不存在' } },
  ],
})
