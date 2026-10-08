import assert from 'node:assert/strict'
import { test } from 'node:test'
import { auditWizardSources } from './audit-wizards.mjs'

test('步骤 API 值导入被阻断，类型导入允许', () => {
  for (const prefix of ['', 'type ']) {
    const result = auditWizardSources(new Map([['src/workbench/export/steps/Step.vue', `<script setup lang="ts">import ${prefix}{ BrowserApi } from '@/api/browser'</script>`]]))
    assert.equal(result.violations.length, prefix ? 0 : 1)
  }
  for (const declaration of ["import api, { type BrowserApi } from '@/api/browser'", "import {} from '@/api/browser'"]) {
    const result = auditWizardSources(new Map([['src/workbench/export/steps/Step.vue', `<script lang="ts">${declaration}</script>`]]))
    assert.equal(result.violations.length, 1)
  }
})

test('禁止反向依赖页面并识别值依赖循环，类型循环不阻断', () => {
  const result = auditWizardSources(new Map([
    ['src/workbench/export/a.ts', "import { b } from './b'; import { page } from '@/views/page'"],
    ['src/workbench/export/b.ts', "import { a } from './a'"],
  ]))
  assert.equal(result.violations.length, 2)
  const types = auditWizardSources(new Map([
    ['src/workbench/export/a.ts', "import type { B } from './b'"],
    ['src/workbench/export/b.ts', "import type { A } from './a'"],
  ]))
  assert.equal(types.violations.length, 0)
})

test('规模超限只提示人工复核，不能机械阻断', () => {
  const result = auditWizardSources(new Map([['src/views/ExportWizardView.vue', '<template>\n' + '\n'.repeat(410) + '</template>']]))
  assert.equal(result.violations.length, 0)
  assert.equal(result.reviews.length, 1)
})
