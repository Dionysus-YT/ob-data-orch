import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { analyzeSource, isTest, readSources, valueCycles } from './architecture/sources.mjs'

// 向导专属检查复用全前端依赖解析；规模只发出复核信号。
export function auditWizardSources(records) {
  const violations = []
  const reviews = []
  const graph = new Map()
  for (const [file, text] of records) {
    if (isTest(file)) continue
    const feature = /^src\/workbench\/(export|import)\//.test(file)
    const entry = /^src\/views\/(Export|NormalImport|DirectLoad)WizardView\.vue$/.test(file)
    if (!feature && !entry) continue
    const source = analyzeSource(file, text, records)
    if (source.totalLines > 400 || source.scriptLines > 300) reviews.push(`${file}: total=${source.totalLines}, script=${source.scriptLines}；复核职责、状态所有者与异步边界`)
    graph.set(file, source.dependencies.filter(dep => !dep.typeOnly && dep.resolved).map(dep => dep.resolved))
    for (const { typeOnly, target, specifier } of source.dependencies) {
      if (typeOnly) continue
      if (feature && target.startsWith('src/views/')) violations.push(`${file}: 业务能力不得反向依赖 views (${specifier})`)
      if (/\/steps\/.*\.vue$/.test(file) && target.startsWith('src/api/')) violations.push(`${file}: 步骤组件不得创建 API 客户端 (${specifier})`)
    }
  }
  violations.push(...valueCycles(graph).map(chain => `循环值依赖: ${chain.join(' → ')}`))
  return { violations, reviews }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
  const result = auditWizardSources(readSources(root))
  result.reviews.forEach(item => console.info(`REVIEW ${item}`))
  result.violations.forEach(item => console.error(`FAIL ${item}`))
  console.info(`Wizard architecture: ${result.violations.length} violations, ${result.reviews.length} review signals`)
  if (result.violations.length) process.exitCode = 1
}
