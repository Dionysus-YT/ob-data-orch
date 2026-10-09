import ts from 'typescript'

export function dependencyViolation(file, dependency) {
  const { target, typeOnly, specifier } = dependency
  if (/^src\/workbench\//.test(file) && /^src\/(views|router)\//.test(target)) return `业务模块不得反向依赖页面或路由 (${specifier})`
  if (/^src\/(components|composables)\//.test(file) && /^src\/(workbench|views|router)\//.test(target)) return `公共能力不得依赖具体业务、页面或路由 (${specifier})`
  if (/^src\/platform\//.test(file) && /^src\/(workbench|views|router|api|components|composables)\//.test(target)) return `平台层不得依赖业务或装配层 (${specifier})`
  if (/^src\/api\//.test(file) && /^src\/(workbench|views|router|components|composables|platform)\//.test(target)) return `API 边界不得依赖业务/UI/平台实现 (${specifier})`
  if (!typeOnly && /\/steps\/.*\.vue$/.test(file) && target.startsWith('src/api/')) return `步骤组件不得创建 API 客户端 (${specifier})`
  if (!typeOnly && !file.startsWith('src/api/') && /^(axios|ky|ofetch|superagent|undici)(\/|$)/.test(specifier)) return `网络客户端只能用于 api 边界 (${specifier})`
}

// 权限退出是既有产品外壳的认证边界，仅保留准确文件与 /logout 调用，不豁免其他请求。
export function networkViolations(source) {
  if (source.file.startsWith('src/api/')) return []
  const ast = ts.createSourceFile('audit.ts', source.script, ts.ScriptTarget.Latest, true)
  const options = { noLib: true, noResolve: true }
  const host = { ...ts.createCompilerHost(options), getSourceFile: file => file === 'audit.ts' ? ast : undefined, fileExists: file => file === 'audit.ts', readFile: () => undefined }
  const checker = ts.createProgram(['audit.ts'], options, host).getTypeChecker()
  const violations = []
  const names = new Set(['fetch', 'XMLHttpRequest', 'EventSource', 'WebSocket'])
  const globals = new Set(['window', 'globalThis', 'self'])
  const unbound = node => ts.isIdentifier(node) && !checker.getSymbolAtLocation(node)?.declarations?.length
  function nameOf(expression) {
    if (ts.isIdentifier(expression) && unbound(expression)) return expression.text
    if ((ts.isPropertyAccessExpression(expression) || ts.isElementAccessExpression(expression)) && unbound(expression.expression) && globals.has(expression.expression.text)) {
      return ts.isPropertyAccessExpression(expression) ? expression.name.text : expression.argumentExpression && ts.isStringLiteral(expression.argumentExpression) ? expression.argumentExpression.text : ''
    }
    return ''
  }
  function visit(node) {
    if (ts.isTypeNode(node) || ts.isInterfaceDeclaration(node) || ts.isImportDeclaration(node) || ts.isExportDeclaration(node)) return
    const parent = node.parent
    const declarationName = parent?.name === node && !ts.isShorthandPropertyAssignment(parent)
    const name = !declarationName ? nameOf(node) : ''
    const logout = source.file === 'src/components/ProductHeader.vue' && name === 'fetch' && ts.isCallExpression(parent) && parent.expression === node && parent.arguments[0] && ts.isStringLiteral(parent.arguments[0]) && parent.arguments[0].text === '/logout'
    if (names.has(name) && !logout) violations.push(`请求/订阅必须经 api 边界 (${name})`)
    if (ts.isVariableDeclaration(node) && ts.isObjectBindingPattern(node.name) && node.initializer && unbound(node.initializer) && globals.has(node.initializer.text)) {
      for (const item of node.name.elements) {
        const property = item.propertyName ?? item.name
        if (ts.isIdentifier(property) && names.has(property.text)) violations.push(`请求/订阅必须经 api 边界 (${property.text})`)
      }
    }
    ts.forEachChild(node, visit)
  }
  visit(ast)
  return [...new Set(violations)]
}

// AST 只能发现职责风险信号，不能证明业务状态是否重复；此类结果只要求人工复核。
export function responsibilityReviews(source) {
  const reviews = []
  if (source.totalLines > 400 || source.scriptLines > 300) reviews.push(`total=${source.totalLines}, script=${source.scriptLines}；复核职责与状态所有者`)
  if (source.dynamic.length) reviews.push('非字面量动态依赖需要人工确认边界与循环')
  if (!source.file.startsWith('src/views/')) return reviews
  const calls = new Set()
  const lifecycles = new Set()
  function visit(node) {
    if (ts.isCallExpression(node)) {
      if (ts.isPropertyAccessExpression(node.expression) && ts.isIdentifier(node.expression.expression) && node.expression.expression.text === 'api') calls.add(node.expression.name.text)
      if (ts.isIdentifier(node.expression) && ['watch', 'setTimeout', 'setInterval', 'onBeforeUnmount', 'onScopeDispose'].includes(node.expression.text)) lifecycles.add(node.expression.text)
    }
    ts.forEachChild(node, visit)
  }
  visit(source.ast)
  if (calls.size >= 3) reviews.push(`${calls.size} 类直接 API 调用；复核页面是否接管多个业务事务`)
  if (calls.size && lifecycles.size >= 3) reviews.push('页面混合业务请求与多个异步生命周期；确认是否应迁入 feature')
  return reviews
}

export function ownershipReviews(sources, graph) {
  const reverse = new Map()
  for (const [file, dependencies] of graph) for (const target of dependencies) {
    if (!reverse.has(target)) reverse.set(target, [])
    reverse.get(target).push(file)
  }
  function owners(file, visited = new Set()) {
    if (visited.has(file)) return []
    visited.add(file)
    const feature = /^src\/workbench\/([^/]+)\//.exec(file)?.[1]
    if (feature) return [feature]
    if (file.startsWith('src/views/')) {
      const features = new Set((sources.get(file)?.dependencies ?? []).map(dep => /^src\/workbench\/([^/]+)\//.exec(dep.target)?.[1]).filter(Boolean))
      return features.size === 1 ? [...features] : [`page:${file}`]
    }
    return (reverse.get(file) ?? []).flatMap(parent => owners(parent, visited))
  }
  const reviews = []
  for (const file of sources.keys()) {
    if (!/^src\/(components|composables)\//.test(file) || /\/(ProductShell|ProductHeader)\.vue$/.test(file)) continue
    const consumers = new Set(owners(file))
    if (consumers.size < 2) reviews.push(`${file}: 公共归属仅发现 ${consumers.size} 个业务入口 (${[...consumers].join(', ') || '无'})；人工确认实际共享或迁回 feature`)
  }
  return reviews
}
