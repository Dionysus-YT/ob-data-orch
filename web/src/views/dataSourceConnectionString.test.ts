import { describe, expect, it } from 'vitest'

import { parseDataSourceConnectionString } from './dataSourceConnectionString'

describe('ODC 连接串解析', () => {
  it('只解析 mysql 开头的 OceanBase MySQL ODP 连接串', () => {
    const passwordOption = `-p${'synthetic-value'}`
    const connectionString = ['mysql', '-h', '127.0.0.1', '-P', '2883', '-u', 'app@tenant_a#cluster_a', '-D', 'app_db', passwordOption].join(' ')
    expect(parseDataSourceConnectionString(connectionString)).toEqual({
      compatibilityMode: 'MYSQL', host: '127.0.0.1', port: 2883, username: 'app', tenantName: 'tenant_a', clusterName: 'cluster_a', defaultDatabase: 'app_db', password: 'synthetic-value',
    })
  })

  it('只解析 obclient 开头的 OceanBase Oracle ODP 连接串', () => {
    const passwordOption = `-p${'synthetic-value'}`
    const connectionString = ['obclient', '-h127.0.0.2', '-P2883', '-uora_user@tenant_b#cluster_b', passwordOption, '-A'].join(' ')
    expect(parseDataSourceConnectionString(connectionString)).toEqual({
      compatibilityMode: 'ORACLE', host: '127.0.0.2', port: 2883, username: 'ora_user', tenantName: 'tenant_b', clusterName: 'cluster_b', defaultDatabase: undefined, password: 'synthetic-value',
    })
  })

  it('拒绝非 ODP、未知客户端和不完整的参数', () => {
    const passwordOption = `-p${'synthetic-value'}`
    expect(parseDataSourceConnectionString(['mysql', '-h127.0.0.1', '-P2883', '-uapp@tenant', passwordOption].join(' '))).toBeUndefined()
    expect(parseDataSourceConnectionString(['psql', '-h127.0.0.1', '-P2883', '-uapp@tenant#cluster', passwordOption].join(' '))).toBeUndefined()
    expect(parseDataSourceConnectionString(['mysql', '-h127.0.0.1', '-P2883', '-uapp@tenant#cluster'].join(' '))).toBeUndefined()
  })
})
