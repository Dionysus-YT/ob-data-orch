import { readFileSync, readdirSync } from 'node:fs'
import { dirname, posix, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import ts from 'typescript'
import { parse } from '@vue/compiler-sfc'

// 检查实际依赖边界；规模仅发出人工复核信号，不据行数阻断开发。
export function auditWizardSources(records) {
  const violations = []
  const reviews = []
  const graph = new Map()
  for (const [file, text] of records) {
    if (/\.test\./.test(file)) continue
    const feature = /^src\/workbench\/(export|import)\//.test(file)
    const entry = /^src\/views\/(Export|NormalImport|DirectLoad)WizardView\.vue$/.test(file)
    if (!feature && !entry) continue
    const descriptor = file.endsWith('.vue') ? parse(text).descriptor : undefined
    const script = descriptor ? [descriptor.scriptSetup?.content, descriptor.script?.content].filter(Boolean).join('\n') : text
    const totalLines = text.split('\n').length
    const scriptLines = script.split('\n').length
    if (totalLines > 400 || scriptLines > 300) reviews.push(`${file}: total=${totalLines}, script=${scriptLines}；复核职责、状态所有者与异步边界`)
    const ast = ts.createSourceFile(file, script, ts.ScriptTarget.Latest, true)
    const edges = []
    for (const node of ast.statements) {
      if (!ts.isImportDeclaration(node) || !ts.isStringLiteral(node.moduleSpecifier)) continue
      const clause = node.importClause
      if (clause?.isTypeOnly || (!clause?.name && clause?.namedBindings && ts.isNamedImports(clause.namedBindings) && clause.namedBindings.elements.length > 0 && clause.namedBindings.elements.every(e => e.isTypeOnly))) continue
      const specifier = node.moduleSpecifier.text
      const target = specifier.startsWith('@/') ? `src/${specifier.slice(2)}` : specifier.startsWith('.') ? posix.normalize(posix.join(posix.dirname(file), specifier)) : ''
      if (!target) continue
      if (feature && target.startsWith('src/views/')) violations.push(`${file}: 业务能力不得反向依赖 views (${specifier})`)
      if (/\/steps\/.*\.vue$/.test(file) && target.startsWith('src/api/')) violations.push(`${file}: 步骤组件不得创建 API 客户端 (${specifier})`)
      const dependency = [target, `${target}.ts`, `${target}.vue`].find(candidate => records.has(candidate))
      if (dependency) edges.push(dependency)
    }
    graph.set(file, edges)
  }
  const visited = new Set()
  function visit(file, chain) {
    if (chain.includes(file)) { violations.push(`循环值依赖: ${[...chain.slice(chain.indexOf(file)), file].join(' → ')}`); return }
    if (visited.has(file)) return
    visited.add(file)
    for (const dependency of graph.get(file) ?? []) visit(dependency, [...chain, file])
  }
  for (const file of graph.keys()) visit(file, [])
  return { violations, reviews }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
  const records = new Map()
  function walk(directory) {
    for (const entry of readdirSync(resolve(root, directory), { withFileTypes: true })) {
      const file = `${directory}/${entry.name}`
      if (entry.isDirectory()) walk(file)
      else if (/\.(vue|ts)$/.test(file)) records.set(file, readFileSync(resolve(root, file), 'utf8'))
    }
  }
  walk('src')
  const result = auditWizardSources(records)
  result.reviews.forEach(review => console.info(`REVIEW ${review}`))
  result.violations.forEach(violation => console.error(`FAIL ${violation}`))
  console.info(`Wizard architecture: ${result.violations.length} violations, ${result.reviews.length} review signals`)
  if (result.violations.length) process.exitCode = 1
}
