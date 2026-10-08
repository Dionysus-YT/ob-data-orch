import { readFileSync, writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { foundation, semantic, component, product } from '../src/platform/tokens.ts'

const kebab = (text) => text.replace(/([a-z0-9])([A-Z])/g, '$1-$2').toLowerCase()
function flatten(object, prefix) {
  return Object.entries(object).flatMap(([key, value]) => {
    const name = `${prefix}-${kebab(key)}`
    if (typeof value === 'object') return flatten(value, name)
    const unit = key === 'weight' ? '' : key.endsWith('Ms') ? 'ms' : 'px'
    return [`  --ob-${name}: ${typeof value === 'number' ? `${value}${unit}` : value};`]
  })
}
const output = `/* 由 scripts/generate-tokens.mjs 从 tokens.ts 生成；禁止手工修改。 */\n:root {\n${[
  ...flatten(foundation, 'foundation'), ...flatten(semantic, 'color'), ...flatten(component, 'component'), ...flatten(product, 'product'),
].join('\n')}\n}\n`
const path = fileURLToPath(new URL('../src/platform/tokens.css', import.meta.url))
if (process.argv.includes('--check')) {
  // Git 在 Windows 检出时可使用 CRLF；只忽略换行编码，值或声明漂移仍须阻断。
  if (readFileSync(path, 'utf8').replaceAll('\r\n', '\n') !== output) throw new Error('Canonical Token CSS 未同步，请运行 npm run tokens:generate')
} else writeFileSync(path, output)
