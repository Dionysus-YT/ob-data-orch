import { readFileSync, readdirSync } from 'node:fs'
import { posix, resolve } from 'node:path'
import ts from 'typescript'
import { parse } from '@vue/compiler-sfc'

export function readSources(root) {
  const records = new Map()
  function walk(directory) {
    for (const entry of readdirSync(resolve(root, directory), { withFileTypes: true })) {
      const file = `${directory}/${entry.name}`
      if (entry.isDirectory()) walk(file)
      else if (/\.(vue|[cm]?[jt]sx?)$/.test(file)) records.set(file, readFileSync(resolve(root, file), 'utf8'))
    }
  }
  walk('src')
  return records
}

export const isTest = file => /(?:\.(?:test|spec)\.|\/__tests__\/)/.test(file)

// Vue 两类 script 使用同一图；类型导入与运行时导入分别记录，类型边不参与循环判定。
export function analyzeSource(file, text, records) {
  const descriptor = file.endsWith('.vue') ? parse(text).descriptor : undefined
  const script = descriptor ? [descriptor.scriptSetup?.content, descriptor.script?.content].filter(Boolean).join('\n') : text
  const ast = ts.createSourceFile(file, script, ts.ScriptTarget.Latest, true, /\.tsx$/.test(file) ? ts.ScriptKind.TSX : ts.ScriptKind.TS)
  const dependencies = []
  const dynamic = []
  function add(specifier, typeOnly = false) {
    const clean = specifier.split('?')[0]
    const target = clean.startsWith('@/') ? `src/${clean.slice(2)}` : clean.startsWith('.') ? posix.normalize(posix.join(posix.dirname(file), clean)) : ''
    const resolved = target && [target, `${target}.ts`, `${target}.tsx`, `${target}.vue`, `${target}.js`, `${target}/index.ts`, `${target}/index.vue`, `${target}/index.js`].find(candidate => records.has(candidate))
    dependencies.push({ specifier, target, resolved, typeOnly })
  }
  function onlyTypes(clause) {
    return clause?.isTypeOnly || (!clause?.name && clause?.namedBindings && ts.isNamedImports(clause.namedBindings) && clause.namedBindings.elements.length > 0 && clause.namedBindings.elements.every(item => item.isTypeOnly))
  }
  function visit(node) {
    if (ts.isImportDeclaration(node) && ts.isStringLiteral(node.moduleSpecifier)) add(node.moduleSpecifier.text, Boolean(onlyTypes(node.importClause)))
    else if (ts.isExportDeclaration(node) && node.moduleSpecifier && ts.isStringLiteral(node.moduleSpecifier)) {
      const types = node.isTypeOnly || (node.exportClause && ts.isNamedExports(node.exportClause) && node.exportClause.elements.length > 0 && node.exportClause.elements.every(item => item.isTypeOnly))
      add(node.moduleSpecifier.text, Boolean(types))
    } else if (ts.isImportEqualsDeclaration(node) && ts.isExternalModuleReference(node.moduleReference) && node.moduleReference.expression && ts.isStringLiteral(node.moduleReference.expression)) add(node.moduleReference.expression.text, node.isTypeOnly)
    else if (ts.isCallExpression(node) && (node.expression.kind === ts.SyntaxKind.ImportKeyword || (ts.isIdentifier(node.expression) && node.expression.text === 'require'))) {
      const argument = node.arguments[0]
      if (argument && (ts.isStringLiteral(argument) || ts.isNoSubstitutionTemplateLiteral(argument))) add(argument.text)
      else dynamic.push(node.getText(ast))
    }
    ts.forEachChild(node, visit)
  }
  visit(ast)
  return { file, ast, script, dependencies, dynamic, totalLines: text.split('\n').length, scriptLines: script.split('\n').length }
}

export function valueCycles(graph) {
  const visited = new Set()
  const active = []
  const cycles = []
  function visit(file) {
    const index = active.indexOf(file)
    if (index !== -1) { cycles.push([...active.slice(index), file]); return }
    if (visited.has(file)) return
    visited.add(file)
    active.push(file)
    for (const target of graph.get(file) ?? []) visit(target)
    active.pop()
  }
  for (const file of graph.keys()) visit(file)
  return cycles
}
