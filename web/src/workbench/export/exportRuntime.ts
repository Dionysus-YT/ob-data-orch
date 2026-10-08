import type { ComputedRef } from 'vue'
import type { BrowserApi } from '@/api/browser'
import type { RouteLocationNormalizedLoaded, Router } from 'vue-router'

// 运行边界由入口注入；导出业务不创建路由或第二个 API 客户端。
export interface ExportRuntime {
  api: BrowserApi
  route: RouteLocationNormalizedLoaded
  router: Router
  fieldPrefix: string
  submitCancelID: string
  activeStep: ComputedRef<number>
  moveToStep(step: number): void
}
