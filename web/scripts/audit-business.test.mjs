import assert from 'node:assert/strict'
import { test } from 'node:test'
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { spawnSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import { auditBusinessSources } from './audit-business.mjs'
import { readSources } from './architecture/sources.mjs'

const audit = entries => auditBusinessSources(new Map(entries))
test('所有业务模块及相对、alias、再导出、动态导入均不得反向依赖页面或路由', () => {
  for (const code of ["import { x } from '@/views/page'", "export * from '../../views/page'", "import('@/views/page')", "import(`@/views/page`)", "export type { X } from '@/router/index'"]) {
    assert.equal(audit([['src/workbench/tasks/a.ts', code]]).violations.length, 1)
  }
  assert.equal(audit([['src/workbench/tasks/a.ts', "import type { Task } from '@/api/browser'"]]).violations.length, 0)
})
test('公共能力、平台及 API 不依赖上层业务，外部依赖和纯类型端口合法', () => {
  for (const [file, dependency] of [
    ['src/components/a.vue', '@/workbench/tasks/a'], ['src/composables/a.ts', '@/views/A'],
    ['src/platform/a.ts', '@/api/browser'], ['src/api/a.ts', '@/workbench/tasks/a'],
    ['src/api/a.ts', '@/components/A'], ['src/platform/a.ts', '@/composables/a'],
  ]) {
    const script = `import type { X } from '${dependency}'`
    const code = file.endsWith('.vue') ? `<script setup lang="ts">${script}</script>` : script
    assert.equal(audit([[file, code]]).violations.length, 1)
  }
  assert.equal(audit([['src/workbench/tasks/a.ts', "import { ref } from 'vue'; import type { X } from '@/api/browser'"]]).violations.length, 0)
})
test('全图识别经 barrel、index、动态导入及跨模块的值循环，类型循环允许', () => {
  const result = audit([
    ['src/workbench/tasks/a.ts', "import { b } from '../nodes'"],
    ['src/workbench/nodes/index.ts', "export { b } from './b'"],
    ['src/workbench/nodes/b.ts', "const a = import('../tasks/a')"],
  ])
  assert.equal(result.violations.length, 1)
  assert.match(result.violations[0], /循环值依赖/)
  assert.equal(audit([
    ['src/workbench/tasks/a.ts', "export type { B } from './b'"],
    ['src/workbench/tasks/b.ts', "import { type A } from './a'"],
  ]).violations.length, 0)
})
test('Vue 普通和 setup script、空导入及混合类型默认导入正确区分', () => {
  for (const declaration of ["import api, { type X } from '@/api/browser'", "import {} from '@/api/browser'", "import '@/api/browser'", "export { api, type X } from '@/api/browser'"]) {
    assert.equal(audit([['src/workbench/import/normal/steps/A.vue', `<script lang="ts">${declaration}</script><script setup lang="ts">const x = 1</script>`]]).violations.length, 1)
  }
  assert.equal(audit([['src/workbench/export/steps/A.vue', `<script setup lang="ts">import { type X } from '@/api/browser'</script>`]]).violations.length, 0)
})
test('HTTP 与流客户端不得绕过 API，包括全局别名、属性和解构', () => {
  for (const code of ["fetch('/api/v1/tasks')", "new XMLHttpRequest()", "new EventSource('/api/v1/tasks')", "new WebSocket('synthetic')", "window.fetch('synthetic')", "const client = globalThis['fetch']; client('synthetic')", "const { fetch: client } = window; client('synthetic')", "import client from 'axios'"]) {
    assert.equal(audit([['src/workbench/tasks/a.ts', code]]).violations.length, 1, code)
  }
  assert.equal(audit([['src/api/browser.ts', "fetch('/api/v1/tasks'); new EventSource('synthetic')"]]).violations.length, 0)
  assert.equal(audit([['src/workbench/tasks/a.ts', "import { fetch } from '@/api/client'; fetch('synthetic')"]]).violations.length, 0)
  assert.equal(audit([['src/workbench/tasks/a.ts', "function f(fetch: () => void) { fetch() }; const object = { fetch: () => 1 }; object.fetch()"]]).violations.length, 0)
})
test('既有认证退出只保留准确文件与端点，不扩展网络豁免', () => {
  assert.equal(audit([['src/components/ProductHeader.vue', `<script setup>fetch('/logout', { method: 'POST' })</script>`]]).violations.length, 0)
  assert.equal(audit([['src/components/ProductHeader.vue', `<script setup>fetch('/api/v1/tasks')</script>`]]).violations.length, 1)
  assert.equal(audit([['src/views/A.vue', `<script setup>fetch('/logout')</script>`]]).violations.length, 1)
})
test('公共组件归属只作 REVIEW，间接的跨业务复用与外壳不误判', () => {
  const records = [
    ['src/components/A.vue', '<template><div /></template>'],
    ['src/components/B.vue', `<script setup>import A from './A.vue'</script>`],
    ['src/workbench/tasks/a.ts', "import B from '@/components/B.vue'"],
  ]
  const one = audit(records)
  assert.equal(one.violations.length, 0); assert.equal(one.reviews.length, 2)
  const two = audit([...records, ['src/workbench/nodes/a.ts', "import B from '@/components/B.vue'"]])
  assert.equal(two.reviews.length, 0)
  assert.equal(audit([['src/components/ProductShell.vue', '<template><div /></template>']]).reviews.length, 0)
})
test('多事务、动态依赖与巨型文件只发复核信号，简单页面不机械拆分', () => {
  const result = audit([['src/views/A.vue', `<script setup>api.list(); api.save(); api.remove(); import(modulePath)</script><template>${'\n'.repeat(420)}</template>`]])
  assert.equal(result.violations.length, 0); assert.equal(result.reviews.length, 3)
  assert.equal(audit([['src/views/A.vue', '<script setup>const x = 1</script>']]).reviews.length, 0)
  assert.equal(audit([['src/workbench/tasks/a.test.ts', "import { page } from '@/views/A'; fetch('synthetic')"]]).violations.length, 0)
})
test('空类型再导出按运行时边处理，require 值循环及扩展名解析有效', () => {
  assert.equal(audit([
    ['src/workbench/tasks/a.ts', "export {} from './b.js'"],
    ['src/workbench/tasks/b.js', "const a = require('./a')"],
  ]).violations.length, 1)
})
test('文件读取与 CLI 在 REVIEW 时成功，硬违规时非零退出', () => {
  const directory = mkdtempSync(join(tmpdir(), 'ob-architecture-test-'))
  try {
    mkdirSync(join(directory, 'src', 'workbench', 'tasks'), { recursive: true })
    const file = join(directory, 'src', 'workbench', 'tasks', 'a.ts')
    writeFileSync(file, '\n'.repeat(410))
    assert.equal(auditBusinessSources(readSources(directory)).violations.length, 0)
    // 真实 CLI 校验退出码，不能仅复述算法来证明门禁已经执行。
    const args = [fileURLToPath(new URL('./audit-business.mjs', import.meta.url)), '--root', directory]
    const review = spawnSync(process.execPath, args, { encoding: 'utf8' })
    assert.equal(review.status, 0)
    assert.match(review.stdout, /REVIEW/)
    writeFileSync(file, "import { page } from '@/views/A'")
    const failed = spawnSync(process.execPath, args, { encoding: 'utf8' })
    assert.equal(failed.status, 1)
    assert.match(failed.stderr, /FAIL/)
  } finally {
    rmSync(directory, { recursive: true })
  }
})
