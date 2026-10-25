import assert from 'node:assert/strict'
import { test } from 'node:test'
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { basename, dirname, join, resolve } from 'node:path'
import { auditPlatform } from './audit-platform.mjs'

function audit(entries) {
  const root = mkdtempSync(join(tmpdir(), 'ob-platform-audit-'))
  try {
    const sources = { 'main.ts': Object.keys(entries).map(file => `import './${file}'`).join('\n'), ...entries }
    for (const [file, text] of Object.entries(sources)) {
      const path = join(root, 'src', file)
      mkdirSync(dirname(path), { recursive: true })
      writeFileSync(path, text)
    }
    return auditPlatform(root)
  } finally {
    assert.equal(dirname(root), resolve(tmpdir()))
    assert.ok(basename(root).startsWith('ob-platform-audit-'))
    rmSync(root, { recursive: true, force: true })
  }
}

const component = (css, template = '<div class="probe"></div>') => `<template>${template}</template><style>${css}</style>`

test('未引用选择器保留 REVIEW，不冒充已确认失效或视觉失败', () => {
  const result = audit({ 'View.vue': component('.missing { display: grid; }') })
  assert.deepEqual(result.violations, [])
  assert.equal(result.reviews.length, 1)
  assert.match(result.reviews[0], /\.missing.*需人工核对/)
  assert.equal(result.declarations[0].category, 'Component Exception')
  const referenced = audit({ 'View.vue': component('.probe { display: grid; }') })
  assert.deepEqual(referenced.reviews, [])
})

test('已登记的 Monaco 与额度动态类按文件识别，未知或复制类仍提示复核', () => {
  const result = audit({
    'workbench/export/components/SqlQueryEditor.vue': component('.probe:has(.find-widget.visible.replaceToggled) { display: block; }'),
    'workbench/nodes/NodeExecutionQuota.vue': component('.quota-used, .quota-reserved, .quota-free { display: block; }'),
  })
  assert.deepEqual(result.violations, [])
  assert.deepEqual(result.reviews, [])
  assert.equal(result.runtimeReferences.length, 2)
  assert.equal(audit({ 'View.vue': component('.find-widget { display: block; }') }).reviews.length, 1)
  assert.equal(audit({ 'workbench/nodes/NodeExecutionQuota.vue': component('.quota-unregistered { display: block; }') }).reviews.length, 1)
})

test('只接受精确登记的现役视觉声明，改值、换元素、跨文件或媒体覆盖仍阻断', () => {
  const result = audit({
    'platform/shell.css': '.nav-icon { font-size: 16px; }',
    'workbench/export/components/SqlQueryEditor.vue': component('.sql-query-in-icon { font-size: 8px; }'),
    'workbench/sources/SourceEditor.vue': component('.orch-environment-chip { font-size: 12px; } .orch-environment-chip--default { background: #f0f0f0; }'),
  })
  assert.deepEqual(result.violations, [])
  assert.ok(result.declarations.every(decl => decl.category === 'Component Exception'))
  for (const entries of [
    { 'platform/shell.css': '.nav-icon { font-size: 17px; }' },
    { 'platform/shell.css': '.body { font-size: 16px; }' },
    { 'Copy.vue': component('.sql-query-in-icon { font-size: 8px; }') },
    { 'workbench/sources/SourceEditor.vue': component('.orch-environment-chip--default { color: #f0f0f0; }') },
    { 'platform/shell.css': '@media (max-width: 500px) { .nav-icon { font-size: 16px; } }' },
  ]) assert.ok(audit(entries).violations.some(item => item.includes('Legacy')))
})

test('只读额度 Tooltip 触发器允许，缺少名称或新增动作及其他原生控件仍阻断', () => {
  const trigger = '<button type="button" :class="`quota-${entry.key}`" :aria-label="entry.label"><span>{{ entry.label }}</span></button>'
  const file = 'workbench/nodes/NodeExecutionQuota.vue'
  assert.deepEqual(audit({ [file]: component('', trigger) }).violations, [])
  for (const template of [
    trigger.replace(':aria-label="entry.label"', ''),
    trigger.replace('type="button"', 'type="submit"'),
    trigger.replace('type="button"', 'type="button" @click="changeQuota"'),
    trigger.replace('type="button"', 'type="button" v-on:click="changeQuota"'),
    trigger.replace('type="button"', 'type="button" v-on="handlers"'),
    trigger.replace('type="button"', 'type="button" v-bind="actions"'),
    trigger.replace('type="button"', 'type="button" :type="actionType"'),
    `${trigger}<input />`,
    `${trigger}<table></table>`,
    '<button type="button">保存</button>',
  ]) assert.ok(audit({ [file]: component('', template) }).violations.some(item => item.includes('自研 Primitive')))
  assert.ok(audit({ 'Other.vue': component('', trigger) }).violations.some(item => item.includes('自研 Primitive')))
})

test('未定义 Token、第二值源、重复覆盖和未登记 important 继续阻断', () => {
  for (const [css, reason] of [
    ['.probe { color: var(--ob-missing); }', '未定义 Token'],
    [':root { --ob-local: 1px; }', '第二个 Token 定义来源'],
    ['.probe { display: block; display: grid; }', 'Duplicate'],
    ['.probe { display: block !important; }', '未登记 important'],
    ['.probe { color: #123456; }', 'Legacy'],
    ['.probe { font-size: 15px; }', 'Legacy'],
  ]) assert.ok(audit({ 'View.vue': component(css) }).violations.some(item => item.includes(reason)), reason)
  assert.ok(audit({ 'platform/shell.css': '.nav-icon { font-size: 16px !important; }' }).violations.some(item => item.includes('未登记 important')))
  assert.ok(audit({ 'platform/shell.css': '.nav-icon { font-size: 16px; font-size: 16px; }' }).violations.some(item => item.includes('Duplicate')))
})

test('原生通用控件和内联裸视觉值继续阻断，Ant 组件与规范 Token 正常通过', () => {
  for (const template of ['<button>保存</button>', '<input />', '<select></select>', '<textarea></textarea>', '<table></table>', '<OrchButton />', '<WorkbenchInput />']) {
    assert.ok(audit({ 'View.vue': component('', template) }).violations.some(item => item.includes('自研 Primitive')))
  }
  assert.ok(audit({ 'View.vue': component('', '<div style="color: #123456"></div>') }).violations.some(item => item.includes('内联视觉值')))
  const result = audit({
    'platform/tokens.css': ':root { --ob-color-text: #132039; }',
    'View.vue': component('.probe { color: var(--ob-color-text); }', '<AButton class="probe">保存</AButton>'),
  })
  assert.deepEqual(result.violations, [])
  assert.deepEqual(result.reviews, [])
})
