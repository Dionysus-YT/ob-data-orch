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
  if (readFileSync(path, 'utf8') !== output) throw new Error('Canonical Token CSS 未同步，请运行 npm run tokens:generate')
} else writeFileSync(path, output)
