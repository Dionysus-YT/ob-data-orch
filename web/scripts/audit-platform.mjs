import { readFileSync, readdirSync, writeFileSync, mkdirSync } from 'node:fs'
import { resolve, relative, dirname, extname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { parse as parseSFC } from '@vue/compiler-sfc'
import postcss from 'postcss'

// 例外只对应 DESIGN §8 登记的现役元素；文件、选择器、属性、值和媒体作用域必须同时匹配。
const visualExceptions = [
  { file: 'src/platform/shell.css', selector: '.nav-icon', property: 'font-size', value: '16px', reason: '导航 SVG 的 16px 图标几何，不是 UI 正文字号。' },
  { file: 'src/workbench/export/components/SqlQueryEditor.vue', selector: '.sql-query-in-icon', property: 'font-size', value: '8px', reason: 'SQL 工具栏 IN 图形的内部字形，不用于普通文本。' },
  { file: 'src/workbench/sources/SourceEditor.vue', selector: '.orch-environment-chip', property: 'font-size', value: '12px', reason: 'Ant Select 选项内部的现役环境标记密度。' },
  { file: 'src/workbench/sources/SourceEditor.vue', selector: '.orch-environment-chip--default', property: 'background', value: '#f0f0f0', reason: '仅禁选的默认提示背景，不是已保存环境或通用状态色。' },
]
const runtimeClasses = new Map([
  ['src/workbench/export/components/SqlQueryEditor.vue', new Set(['find-widget', 'visible', 'replaceToggled'])],
  ['src/workbench/nodes/NodeExecutionQuota.vue', new Set(['quota-used', 'quota-reserved', 'quota-free'])],
])

function visualException(decl) {
  return decl.context === '' ? visualExceptions.find(item => ['file', 'selector', 'property', 'value'].every(key => item[key] === decl[key])) : undefined
}

function quotaTooltipTrigger(file, tag, opening) {
  // 只读额度的键盘 Tooltip 触发器；新增动作或其他原生输入仍按 Primitive 违规处理。
  return file === 'src/workbench/nodes/NodeExecutionQuota.vue' && tag === 'button'
    && /(?:^|\s)type="button"/.test(opening)
    && /:class="\x60quota-\$\{entry\.key\}\x60"/.test(opening)
    && /:aria-label=/.test(opening)
    && !/\s(?:@|v-on\b|v-bind\b|:type\b)/.test(opening)
}

export function auditPlatform(root) {
  const source = resolve(root, 'src')
  const files = []
  function walk(path) {
    for (const entry of readdirSync(path, { withFileTypes: true })) {
      const full = resolve(path, entry.name)
      if (entry.isDirectory()) walk(full)
      else if (/\.(vue|ts|css)$/.test(entry.name)) files.push(full)
    }
  }
  walk(source)
  const name = (path) => relative(root, path).replaceAll('\\', '/')
  const graph = new Map()
  for (const file of files) {
    const text = readFileSync(file, 'utf8')
    const imports = [...text.matchAll(/(?:from\s*|import\s*\(?\s*)['"]([^'"]+)['"]/g)].map((match) => match[1])
    graph.set(file, imports.flatMap((specifier) => {
      if (!specifier.startsWith('.') && !specifier.startsWith('@/')) return []
      const path = specifier.startsWith('@/') ? resolve(source, specifier.slice(2)) : resolve(dirname(file), specifier)
      const found = [path, `${path}.ts`, `${path}.vue`, resolve(path, 'index.ts')].find((candidate) => files.includes(candidate))
      return found ? [found] : []
    }))
  }
  const active = new Set()
  function visit(file) {
    if (active.has(file) || file.includes('visual-foundation')) return
    active.add(file)
    for (const dependency of graph.get(file) ?? []) visit(dependency)
  }
  visit(resolve(source, 'main.ts'))
  const status = (file) => file.endsWith('.test.ts') || /UiFixture/.test(file) ? 'FIXTURE' : file.includes('visual-foundation') ? 'REFERENCE' : active.has(file) ? 'ACTIVE' : 'DEAD'
  const declarations = []
  const templateCorpus = files.filter(file => active.has(file) && /\.(vue|ts)$/.test(file)).map(file => {
    const text = readFileSync(file, 'utf8')
    if (file.endsWith('.ts')) return text
    const descriptor = parseSFC(text).descriptor
    return [descriptor.template?.content, descriptor.scriptSetup?.content, descriptor.script?.content].join('\n')
  }).join('\n')
  const selectorReviews = []
  const runtimeReferences = []
  const inlineStyles = []
  for (const file of files) {
    const text = readFileSync(file, 'utf8')
    if (file.endsWith('.vue')) {
      const template = parseSFC(text).descriptor.template?.content ?? ''
      for (const match of template.matchAll(/\s((?::)?(?:[\w-]+-)?style)="([^"]*)"/g)) {
        inlineStyles.push({ file: name(file), attribute: match[1], value: match[2], category: 'Component Exception', reason: '产品组件通过框架 style API 设置区域布局，或页面业务输入占宽；不定义色板或基础控件主题。' })
      }
    }
    const blocks = extname(file) === '.css' ? [{ content: text, start: 0 }] : extname(file) === '.vue' ? parseSFC(text).descriptor.styles.map((style) => ({ content: style.content, start: style.loc.start.line - 1 })) : []
    for (const block of blocks) {
      const ast = postcss.parse(block.content)
      if (status(file) === 'ACTIVE') ast.walkRules(rule => {
        for (const match of rule.selector.matchAll(/\.([\w-]+)/g)) {
          const className = match[1]
          if (/^(ant-|anticon$|is-|orch-status--)/.test(className)) continue
          if (new RegExp(`(?<![\\w-])${className}(?![\\w-])`).test(templateCorpus)) continue
          if (runtimeClasses.get(name(file))?.has(className)) {
            runtimeReferences.push(`${name(file)}: ${rule.selector} 已登记的运行时类名引用`)
          } else {
            // 静态字符串搜索不能证明选择器已失效，保留可复核信号，不把它伪装成视觉失败或已清理。
            selectorReviews.push(`${name(file)}: ${rule.selector} 静态未发现完整类名引用，需人工核对入口、动态拼接和运行状态后处置`)
          }
        }
      })
      ast.walkDecls((decl) => {
        const context = []
        for (let parent = decl.parent; parent && parent.type !== 'root'; parent = parent.parent) if (parent.type === 'atrule') context.unshift(`${parent.name} ${parent.params}`)
        declarations.push({ file: name(file), line: block.start + decl.source.start.line, context: context.join(' | '), selector: decl.parent.selector ?? decl.parent.name, property: decl.prop, value: decl.value, important: !!decl.important, status: status(file) })
      })
    }
  }
  const keys = new Map()
  for (const decl of declarations) {
    const key = `${decl.file}|${decl.context}|${decl.selector}|${decl.property}`
    keys.set(key, (keys.get(key) ?? 0) + 1)
  }
  // 分类是迁移处置，不把重复出现的任意历史数值自动升级为产品规范。
  for (const decl of declarations) {
    decl.category = decl.status === 'DEAD' || decl.status === 'REFERENCE' ? 'Dead'
      : decl.file === 'src/platform/tokens.css' && ['--ob-component-control-height', '--ob-component-table-header-height', '--ob-component-table-row-height'].includes(decl.property) ? 'Confirmed Product Decision'
      : decl.file === 'src/platform/tokens.css' || /var\(--ob-/.test(decl.value) ? 'Canonical Token'
      : /(?:height|size-control)/.test(decl.property) && ['40px', '47px', '48px'].includes(decl.value) ? 'Confirmed Product Decision'
      : keys.get(`${decl.file}|${decl.context}|${decl.selector}|${decl.property}`) > 1 ? 'Duplicate'
      : visualException(decl) ? 'Component Exception'
      : /#[\da-f]{3,8}\b|rgba?\(|hsla?\(/i.test(decl.value) || decl.property === 'font-size' && /\d+px/.test(decl.value) ? 'Legacy'
      : 'Component Exception'
    decl.reason = decl.category === 'Component Exception' && visualException(decl) ? visualException(decl).reason : decl.category === 'Component Exception' ? '布局、比例、定位、边框/形状、状态可见性或 CSS 关键字；不是 Primitive 视觉值，不提升为全局 Token。'
      : decl.category === 'Canonical Token' ? '消费唯一 tokens.ts 生成的变量或其生成声明。'
      : decl.category === 'Duplicate' ? '同文件、同媒体作用域、同选择器同属性重复声明，需要人工核验覆盖意图。'
      : decl.category === 'Dead' ? '无现役入口引用。' : decl.category === 'Confirmed Product Decision' ? 'P0/P1 冻结几何。' : '未归一的裸视觉值。'
  }
  const violations = []
  for (const style of inlineStyles) {
    if (/#[\da-f]{3,8}\b|rgba?\(|hsla?\(|font-?size/i.test(style.value)) violations.push(`${style.file} 内联视觉值必须消费 Canonical Token`)
  }
  const variables = new Set(declarations.filter(d => d.property.startsWith('--ob-')).map(d => d.property))
  for (const decl of declarations.filter(d => d.status === 'ACTIVE')) {
    for (const variable of decl.value.matchAll(/var\((--ob-[\w-]+)/g)) if (!variables.has(variable[1])) violations.push(`${decl.file}:${decl.line} 未定义 Token ${variable[1]}`)
    if (decl.property.startsWith('--') && decl.file !== 'src/platform/tokens.css' && decl.property !== '--ob-product-shell-current-navigation-width') violations.push(`${decl.file}:${decl.line} 第二个 Token 定义来源`)
    if (decl.category === 'Legacy' || decl.category === 'Duplicate') violations.push(`${decl.file}:${decl.line} ${decl.category}: ${decl.selector} ${decl.property}`)
    if (decl.important && !(decl.file === 'src/platform/components.css' && decl.context.includes('prefers-reduced-motion'))) violations.push(`${decl.file}:${decl.line} 未登记 important`)
  }
  for (const file of files.filter(file => active.has(file) && file.endsWith('.vue'))) {
    const template = parseSFC(readFileSync(file, 'utf8')).descriptor.template?.content ?? ''
    const primitives = [...template.matchAll(/<(button|input|select|textarea|table|OrchButton|Workbench\w+)\b[^>]*>/g)]
    if (primitives.some(match => !quotaTooltipTrigger(name(file), match[1], match[0]))) violations.push(`${name(file)} 现役自研 Primitive`)
  }
  const result = {
    scope: '入口静态 import 图与完整 CSS 声明；未引用选择器只作 REVIEW；最终 computed style 必须由浏览器测试验证',
    files: files.map((file) => ({ file: name(file), status: status(file), imports: (graph.get(file) ?? []).map(name) })),
    summary: {
      files: files.length,
      active: active.size,
      declarations: declarations.length,
      important: declarations.filter((decl) => decl.important).length,
      violations: violations.length,
      reviewSignals: new Set(selectorReviews).size,
      runtimeReferences: new Set(runtimeReferences).size,
      categories: Object.fromEntries(['Confirmed Product Decision', 'Canonical Token', 'Component Exception', 'Legacy', 'Duplicate', 'Dead'].map((category) => [category, declarations.filter((decl) => decl.category === category).length])),
    },
    declarations,
    inlineStyles,
    violations,
    reviews: [...new Set(selectorReviews)],
    runtimeReferences: [...new Set(runtimeReferences)],
  }
  return result
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
  const result = auditPlatform(root)
  const output = resolve(root, process.argv[2] ?? '../artifacts/frontend-platform-audit.json')
  mkdirSync(dirname(output), { recursive: true })
  writeFileSync(output, `${JSON.stringify(result, null, 2)}\n`)
  process.stdout.write(`${JSON.stringify(result.summary, null, 2)}\n${output}\n`)
  result.reviews.forEach(item => process.stdout.write(`REVIEW ${item}\n`))
  process.stdout.write(`Platform styles: ${result.violations.length} violations, ${result.reviews.length} review signals, ${result.runtimeReferences.length} runtime references\n`)
  if (result.violations.length) { process.stderr.write(`${result.violations.join('\n')}\n`); process.exitCode = 1 }
}
