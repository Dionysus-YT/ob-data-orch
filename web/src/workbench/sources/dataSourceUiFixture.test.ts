import { describe, expect, it } from 'vitest'

import { DATA_SOURCE_UI_FIXTURES, dataSourceUiFixtureForSearch, isDataSourceUiFixtureEnabled } from './dataSourceUiFixture'

describe('数据源列表开发界面样本', () => {
  it('提供与冻结基线同态的 7 条安全合成数据', () => {
    expect(DATA_SOURCE_UI_FIXTURES).toHaveLength(7)
    expect(DATA_SOURCE_UI_FIXTURES.map((source) => source.id)).toEqual([
      'ui-fixture-production-finance-reporting',
      'ui-fixture-customer-analytics-source',
      'ui-fixture-data-mart-sales',
      'ui-fixture-internal-reporting-db',
      'ui-fixture-legacy-finance-archive',
      'ui-fixture-data-lake-external-source',
      'ui-fixture-dev-sandbox',
    ])
    expect(new Set(DATA_SOURCE_UI_FIXTURES.map((source) => source.id)).size).toBe(7)
    expect(DATA_SOURCE_UI_FIXTURES.every((source) => source.connectionKind === 'ODP')).toBe(true)
    expect(DATA_SOURCE_UI_FIXTURES.every((source) => source.host === '192.0.2.18')).toBe(true)
    expect(DATA_SOURCE_UI_FIXTURES.every((source) => source.username.length > 0)).toBe(true)
    expect(new Set(DATA_SOURCE_UI_FIXTURES.map((source) => source.sysCredentialState))).toEqual(new Set(['AVAILABLE', 'UNAVAILABLE']))
  })

  it('覆盖列表布局和筛选所需的数据状态', () => {
    expect(new Set(DATA_SOURCE_UI_FIXTURES.map((source) => source.environment))).toEqual(new Set(['DEVELOPMENT', 'TEST', 'STAGING', 'PRODUCTION']))
    expect(new Set(DATA_SOURCE_UI_FIXTURES.map((source) => source.compatibilityMode))).toEqual(new Set(['MYSQL', 'ORACLE']))
    expect(new Set(DATA_SOURCE_UI_FIXTURES.map((source) => source.state))).toEqual(new Set(['ENABLED', 'DISABLED']))
    expect(new Set(DATA_SOURCE_UI_FIXTURES.map((source) => source.port))).toEqual(new Set([2883]))
    expect(DATA_SOURCE_UI_FIXTURES.filter((source) => source.lastTestStatus === 'SUCCEEDED')).toHaveLength(2)
    expect(DATA_SOURCE_UI_FIXTURES.filter((source) => source.lastTestStatus === 'FAILED')).toHaveLength(1)
    expect(DATA_SOURCE_UI_FIXTURES.filter((source) => source.lastTestStatus === 'INVALIDATED')).toHaveLength(1)
    expect(DATA_SOURCE_UI_FIXTURES.filter((source) => source.lastTestStatus === 'UNKNOWN')).toHaveLength(1)
    expect(DATA_SOURCE_UI_FIXTURES.filter((source) => source.lastTestStatus === 'PENDING')).toHaveLength(1)
    expect(DATA_SOURCE_UI_FIXTURES.filter((source) => !source.lastTestStatus)).toHaveLength(1)
  })

  it('为有未完成任务的数据源提供服务端生命周期资格样本', () => {
    const source = DATA_SOURCE_UI_FIXTURES.find((item) => item.displayName === 'Production finance reporting')
    expect(source?.lifecycleEligibility).toMatchObject({
      delete: { allowed: false, reasonCode: 'UNFINISHED_TASKS_EXIST' },
      archive: { allowed: true, referenceCount: 3 },
    })
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
