import { createRouter, createWebHistory } from 'vue-router'

// 页面按路由加载，登录后的首页不等待向导、详情及其编辑器依赖下载和初始化。
export const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    { path: '/workbench/data-sources', name: 'workbench-data-sources', redirect: (to) => ({ name: 'data-sources', query: to.query }) },
    { path: '/', name: 'home', component: () => import('@/views/HomeView.vue'), meta: { title: '首页' } },
    { path: '/data-sources', name: 'data-sources', component: () => import('@/workbench/sources/SourceWorkspace.vue'), meta: { title: '数据源管理', contentWidth: 'wide' } },
    { path: '/data-sources/:id', redirect: (to) => ({ name: 'data-sources', query: { edit: String(to.params.id) } }) },
    { path: '/exports/new', name: 'new-export', component: () => import('@/views/ExportWizardView.vue'), meta: { title: '导出任务' } },
    { path: '/imports/normal/new', name: 'new-normal-import', component: () => import('@/views/NormalImportWizardView.vue'), meta: { title: '普通导入' } },
    { path: '/imports/direct/new', name: 'new-direct-import', component: () => import('@/views/DirectLoadWizardView.vue'), meta: { title: '旁路导入' } },
    { path: '/tasks', name: 'tasks', component: () => import('@/views/TaskCenterView.vue'), meta: { title: '任务中心' } },
    { path: '/tasks/:id', name: 'task-detail', component: () => import('@/views/TaskDetailView.vue'), meta: { title: '任务详情' } },
    { path: '/templates', name: 'templates', component: () => import('@/views/TemplateCenterView.vue'), meta: { title: '模板中心' } },
    { path: '/nodes', name: 'nodes', component: () => import('@/views/ExecutionNodeView.vue'), meta: { title: '执行节点' }, children: [
      { path: 'new', name: 'node-new', component: () => import('@/views/ExecutionNodeFormView.vue'), meta: { title: '注册执行节点' } },
      { path: ':id/edit', name: 'node-edit', component: () => import('@/views/ExecutionNodeFormView.vue'), meta: { title: '编辑执行节点' } },
    ] },
    { path: '/nodes/:id', name: 'node-detail', component: () => import('@/views/ExecutionNodeDetailView.vue'), meta: { title: '执行节点详情' } },
    { path: '/logs', name: 'logs', component: () => import('@/views/LogCenterView.vue'), meta: { title: '日志中心' } },
    { path: '/settings', name: 'settings', component: () => import('@/views/PlatformSettingsView.vue'), redirect: (to) => ({ name: 'platform-settings', query: to.query, hash: to.hash }), children: [
      { path: 'platform', name: 'platform-settings', component: () => import('@/views/GeneralSettingsView.vue'), meta: { title: '平台设置 · 常规设置' } },
      { path: 'scheduling', name: 'scheduling-policy', component: () => import('@/views/SchedulingPolicyView.vue'), meta: { title: '平台设置 · 调度策略' } },
      { path: 'security', name: 'security-policy', component: () => import('@/views/SecurityPolicyView.vue'), meta: { title: '平台设置 · 安全策略' } },
      { path: 'tools', name: 'tool-compatibility', component: () => import('@/views/ToolCompatibilityView.vue'), meta: { title: '平台设置 · 工具与兼容' } },
    ] },
    { path: '/settings/access-control', name: 'access-control', component: () => import('@/views/AccessControlView.vue'), meta: { title: '权限配置' } },
    { path: '/settings/storage-credentials', name: 'storage-credentials', component: () => import('@/views/StorageCredentialsView.vue'), meta: { title: '存储凭据' } },
    { path: '/:pathMatch(.*)*', name: 'not-found', component: () => import('@/views/NotFoundView.vue'), meta: { title: '页面不存在' } },
  ],
})
