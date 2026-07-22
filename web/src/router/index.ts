import { createRouter, createWebHistory } from 'vue-router'

import DataSourceFormView from '@/views/DataSourceFormView.vue'
import DataSourceListView from '@/views/DataSourceListView.vue'
import ExportWizardView from '@/views/ExportWizardView.vue'
import HomeView from '@/views/HomeView.vue'
import ModulePlaceholderView from '@/views/ModulePlaceholderView.vue'
import NotFoundView from '@/views/NotFoundView.vue'
import TaskWizardView from '@/views/TaskWizardView.vue'

export const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    { path: '/', name: 'home', component: HomeView, meta: { title: '首页' } },
    { path: '/data-sources', name: 'data-sources', component: DataSourceListView, meta: { title: '数据源管理' } },
    { path: '/data-sources/:id', name: 'data-source-form', component: DataSourceFormView, meta: { title: '数据源管理' } },
    { path: '/exports/new', name: 'new-export', component: ExportWizardView, meta: { title: '导出任务' } },
    { path: '/imports/normal/new', name: 'new-normal-import', component: TaskWizardView, meta: { title: '普通导入', kind: 'normal' } },
    { path: '/imports/direct/new', name: 'new-direct-import', component: TaskWizardView, meta: { title: '旁路导入', kind: 'direct' } },
    { path: '/tasks', name: 'tasks', component: ModulePlaceholderView, meta: { title: '任务中心', description: '统一查看导出、普通导入和旁路导入任务的状态、快照、命令与日志。', sections: ['任务列表', '状态与筛选'] } },
    { path: '/tasks/:id', name: 'task-detail', component: ModulePlaceholderView, meta: { title: '任务详情', description: '在同一任务上下文中查看稳定状态、可靠阶段、不可变配置快照、脱敏命令与执行日志。', sections: ['运行概览', '配置快照', '命令与日志'] } },
    { path: '/templates', name: 'templates', component: ModulePlaceholderView, meta: { title: '模板中心', description: '复用已确认的任务配置；模板不保存凭据，也不会改写历史任务快照。', sections: ['模板列表', '兼容状态'] } },
    { path: '/nodes', name: 'nodes', component: ModulePlaceholderView, meta: { title: '执行节点', description: '展示执行节点可接收状态、工具环境、资源快照和任务关系。', sections: ['节点列表', '环境与状态'] } },
    { path: '/nodes/:id', name: 'node-detail', component: ModulePlaceholderView, meta: { title: '执行节点详情', description: '查看节点注册事实、环境检查、维护状态与关联任务；控制面不会直接操作节点文件或进程。', sections: ['节点概览', '工具环境', '关联任务与事件'] } },
    { path: '/logs', name: 'logs', component: ModulePlaceholderView, meta: { title: '日志中心', description: '在权限范围内查询已脱敏日志、来源上下文和采集完整性。', sections: ['查询条件', '日志结果'] } },
    { path: '/settings', name: 'settings', component: ModulePlaceholderView, meta: { title: '系统设置', description: '平台基础、调度、安全、工具与兼容四个分区保持在同一设置页面。', sections: ['平台基础', '调度', '安全', '工具与兼容'] } },
    { path: '/settings/access-control', name: 'access-control', component: ModulePlaceholderView, meta: { title: '权限与安全', description: '权限配置位于系统设置的安全分区，只管理已认证用户的授权，不创建身份账号。', sections: ['用户与权限列表', '固定角色与对象范围', '高风险能力'] } },
    { path: '/:pathMatch(.*)*', name: 'not-found', component: NotFoundView, meta: { title: '页面不存在' } },
  ],
})
