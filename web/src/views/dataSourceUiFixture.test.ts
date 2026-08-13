import { describe, expect, it } from 'vitest'

import { DATA_SOURCE_UI_FIXTURES, dataSourceUiFixtureForSearch, isDataSourceUiFixtureEnabled } from './dataSourceUiFixture'

describe('数据源列表开发界面样本', () => {
  it('提供 16 条具有稳定且唯一标识的安全合成数据', () => {
    expect(DATA_SOURCE_UI_FIXTURES).toHaveLength(16)
    expect(DATA_SOURCE_UI_FIXTURES.map((source) => source.id)).toEqual([
      'ui-fixture-data-source-01', 'ui-fixture-data-source-02', 'ui-fixture-data-source-03', 'ui-fixture-data-source-04',
      'ui-fixture-data-source-05', 'ui-fixture-data-source-06', 'ui-fixture-data-source-07', 'ui-fixture-data-source-08',
      'ui-fixture-data-source-09', 'ui-fixture-data-source-10', 'ui-fixture-data-source-11', 'ui-fixture-data-source-12',
      'ui-fixture-data-source-13', 'ui-fixture-data-source-14', 'ui-fixture-data-source-15', 'ui-fixture-data-source-16',
    ])
    expect(new Set(DATA_SOURCE_UI_FIXTURES.map((source) => source.id)).size).toBe(16)
    expect(DATA_SOURCE_UI_FIXTURES.every((source) => source.connectionKind === 'ODP')).toBe(true)
    expect(DATA_SOURCE_UI_FIXTURES.every((source) => source.host.endsWith('.test') || /^\d{1,3}(?:\.\d{1,3}){3}$/.test(source.host))).toBe(true)
    expect(DATA_SOURCE_UI_FIXTURES.every((source) => source.host.length <= 15)).toBe(true)
    expect(DATA_SOURCE_UI_FIXTURES.every((source) => source.tenantName.length <= 8)).toBe(true)
    expect(DATA_SOURCE_UI_FIXTURES.every((source) => source.username.length > 0)).toBe(true)
    expect(new Set(DATA_SOURCE_UI_FIXTURES.map((source) => source.sysCredentialState))).toEqual(new Set(['AVAILABLE', 'UNAVAILABLE']))
  })

  it('覆盖列表布局和筛选所需的数据状态', () => {
    expect(new Set(DATA_SOURCE_UI_FIXTURES.map((source) => source.environment))).toEqual(new Set(['DEVELOPMENT', 'TEST', 'STAGING', 'PRODUCTION']))
    expect(new Set(DATA_SOURCE_UI_FIXTURES.map((source) => source.compatibilityMode))).toEqual(new Set(['MYSQL', 'ORACLE']))
    expect(new Set(DATA_SOURCE_UI_FIXTURES.map((source) => source.state))).toEqual(new Set(['ENABLED', 'DISABLED']))
    expect(new Set(DATA_SOURCE_UI_FIXTURES.map((source) => source.port))).toEqual(new Set([2881, 2882, 2883, 2884]))
    expect(DATA_SOURCE_UI_FIXTURES.filter((source) => source.lastTestStatus === 'SUCCEEDED')).toHaveLength(4)
    expect(DATA_SOURCE_UI_FIXTURES.filter((source) => source.lastTestStatus === 'FAILED')).toHaveLength(4)
    expect(DATA_SOURCE_UI_FIXTURES.filter((source) => source.lastTestStatus === 'INVALIDATED')).toHaveLength(4)
    expect(DATA_SOURCE_UI_FIXTURES.filter((source) => !source.lastTestStatus)).toHaveLength(4)
    expect(DATA_SOURCE_UI_FIXTURES.some((source) => source.displayName === '开发源')).toBe(true)
    expect(DATA_SOURCE_UI_FIXTURES.some((source) => source.displayName.length > 40 && /[\u4E00-\u9FFF]/u.test(source.displayName))).toBe(true)
    expect(DATA_SOURCE_UI_FIXTURES.some((source) => source.displayName.length > 80 && /^[a-z-]+$/u.test(source.displayName))).toBe(true)
    expect(DATA_SOURCE_UI_FIXTURES.some((source) => source.host === 'db.test')).toBe(true)
    expect(Math.max(...DATA_SOURCE_UI_FIXTURES.map((source) => source.host.length))).toBe(15)
    expect(DATA_SOURCE_UI_FIXTURES.some((source) => source.tenantName.length === 1)).toBe(true)
    expect(Math.max(...DATA_SOURCE_UI_FIXTURES.map((source) => source.tenantName.length))).toBe(8)
    expect(new Set(DATA_SOURCE_UI_FIXTURES.map((source) => source.username)).size).toBeGreaterThan(8)
  })

  it('仅在开发环境且唯一显式参数精确匹配时启用', () => {
    expect(isDataSourceUiFixtureEnabled('?uiFixture=data-sources', true)).toBe(true)
    expect(isDataSourceUiFixtureEnabled('?uiFixture=other', true)).toBe(false)
    expect(isDataSourceUiFixtureEnabled('?uiFixture=data-sources&uiFixture=other', true)).toBe(false)
    expect(isDataSourceUiFixtureEnabled('?other=data-sources', true)).toBe(false)
    expect(isDataSourceUiFixtureEnabled('', true)).toBe(false)
  })

  it('非开发环境始终失败关闭，并且命中时返回独立副本', () => {
    expect(isDataSourceUiFixtureEnabled('?uiFixture=data-sources', false)).toBe(false)
    expect(dataSourceUiFixtureForSearch('?uiFixture=data-sources', false)).toBeUndefined()
    expect(dataSourceUiFixtureForSearch('', true)).toBeUndefined()

    const first = dataSourceUiFixtureForSearch('?uiFixture=data-sources', true)
    const second = dataSourceUiFixtureForSearch('?uiFixture=data-sources', true)
    expect(first).toEqual(DATA_SOURCE_UI_FIXTURES)
    expect(second).toEqual(DATA_SOURCE_UI_FIXTURES)
    expect(first).not.toBe(second)
  })
})
