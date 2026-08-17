import { describe, expect, it } from 'vitest'

import type { StorageCredentialListItem } from '@/api/browser'
import { filterStorageCredentials, isStorageCredentialProvider, storageCredentialProviderMatches, validateStorageCredentialWrite } from './storageCredentialList'

const credential = (id: string, provider: 'OSS' | 'S3' | 'COS' | 'OBS', displayName: string): StorageCredentialListItem => ({
  id,
  displayName,
  provider,
  currentRevision: 1,
  revision: 1,
  updatedAt: '2026-08-14T00:00:00Z',
})

describe('validateStorageCredentialWrite', () => {
  it('接受合法输入', () => {
    expect(validateStorageCredentialWrite({ displayName: '合成 OSS 凭据', provider: 'OSS', accessKey: 'access', secretKey: 'secret' })).toEqual({})
  })

  it('拒绝空名称、超长名称与未知 provider', () => {
    expect(validateStorageCredentialWrite({ displayName: '  ', provider: 'OSS', accessKey: 'a', secretKey: 's' }).displayName).toBeTruthy()
    expect(validateStorageCredentialWrite({ displayName: 'x'.repeat(257), provider: 'OSS', accessKey: 'a', secretKey: 's' }).displayName).toBeTruthy()
    expect(validateStorageCredentialWrite({ displayName: '名称', provider: 'FTP', accessKey: 'a', secretKey: 's' }).provider).toBeTruthy()
  })

  it('拒绝空密钥、超长密钥与含控制换行的密钥', () => {
    expect(validateStorageCredentialWrite({ displayName: '名称', provider: 'OSS', accessKey: '', secretKey: 's' }).accessKey).toBeTruthy()
    expect(validateStorageCredentialWrite({ displayName: '名称', provider: 'OSS', accessKey: 'a', secretKey: '' }).secretKey).toBeTruthy()
    expect(validateStorageCredentialWrite({ displayName: '名称', provider: 'OSS', accessKey: 'a', secretKey: 's'.repeat(4097) }).secretKey).toBeTruthy()
    expect(validateStorageCredentialWrite({ displayName: '名称', provider: 'OSS', accessKey: 'a\nb', secretKey: 's' }).accessKey).toBeTruthy()
    expect(validateStorageCredentialWrite({ displayName: '名称', provider: 'OSS', accessKey: 'a', secretKey: 's\rx' }).secretKey).toBeTruthy()
  })
})

describe('storageCredentialProviderMatches', () => {
  it('只允许同 provider 绑定输出类型', () => {
    const oss = credential('1', 'OSS', 'OSS 凭据')
    expect(storageCredentialProviderMatches(oss, 'OSS')).toBe(true)
    expect(storageCredentialProviderMatches(oss, 'S3')).toBe(false)
    expect(storageCredentialProviderMatches(oss, 'LOCAL')).toBe(false)
  })
})

describe('filterStorageCredentials', () => {
  const items = [credential('1', 'OSS', '生产 OSS'), credential('2', 'S3', '测试 S3'), credential('3', 'COS', '腾讯 COS')]

  it('按关键字过滤名称与标识', () => {
    expect(filterStorageCredentials(items, { keyword: '生产', provider: '' }).map((item) => item.id)).toEqual(['1'])
    expect(filterStorageCredentials(items, { keyword: '3', provider: '' }).map((item) => item.id)).toEqual(['2', '3'])
  })

  it('按 provider 过滤', () => {
    expect(filterStorageCredentials(items, { keyword: '', provider: 'OSS' }).map((item) => item.id)).toEqual(['1'])
    expect(filterStorageCredentials(items, { keyword: '', provider: 'OBS' })).toEqual([])
  })
})

describe('isStorageCredentialProvider', () => {
  it('白名单校验', () => {
    expect(isStorageCredentialProvider('OSS')).toBe(true)
    expect(isStorageCredentialProvider('LOCAL')).toBe(false)
  })
})
