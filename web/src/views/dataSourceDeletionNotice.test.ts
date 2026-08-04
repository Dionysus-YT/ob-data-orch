import { describe, expect, it } from 'vitest'

import { dataSourceDeletionNotice } from './dataSourceDeletionNotice'

describe('数据源删除结果提示', () => {
  it('说明物理删除后可复用名称', () => {
    expect(dataSourceDeletionNotice({ outcome: 'DELETED', revision: 0 })).toBe('数据源已物理删除，可使用原名称新建。')
  })

  it('说明归档仍保留名称的原因', () => {
    expect(dataSourceDeletionNotice({ outcome: 'ARCHIVED', revision: 2 })).toBe('数据源已归档，因历史引用仍保留名称。')
  })
})
