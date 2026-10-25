import type { UnwrapRef } from 'vue'
import type { DirectLoadWizard } from './useDirectLoadWizard'

/** 步骤消费组合能力的引用，不创建表单副本或持有 HTTP 客户端。 */
export type DirectLoadStepModel = UnwrapRef<DirectLoadWizard>
