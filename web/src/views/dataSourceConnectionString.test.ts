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

  it('允许 -p 空值，以便用户稍后补填结构化密码', () => {
    const connectionString = ['mysql', '-h192.0.2.53', '-P2883', '-uroot@test#cluster_a', '-p'].join(' ')
    expect(parseDataSourceConnectionString(connectionString)).toMatchObject({
      compatibilityMode: 'MYSQL', host: '192.0.2.53', port: 2883, username: 'root', tenantName: 'test', clusterName: 'cluster_a', password: '',
    })
  })

  it('解析服务名形式的用户@租户:集群且不改写原始连接串', () => {
    const connectionString = ['mysql', '-hsynthetic-service.example', '-P2883', '-uroot@SERVICE:cluster_service', '-p'].join(' ')
    expect(parseDataSourceConnectionString(connectionString)).toMatchObject({
      compatibilityMode: 'MYSQL', host: 'synthetic-service.example', port: 2883, username: 'root', tenantName: 'SERVICE', clusterName: 'cluster_service', password: '',
    })
    expect(connectionString).toContain('@SERVICE:cluster_service')
  })

  it('拒绝非 ODP、未知客户端和不完整的参数', () => {
    const passwordOption = `-p${'synthetic-value'}`
    expect(parseDataSourceConnectionString(['mysql', '-h127.0.0.1', '-P2883', '-uapp@tenant', passwordOption].join(' '))).toBeUndefined()
    expect(parseDataSourceConnectionString(['psql', '-h127.0.0.1', '-P2883', '-uapp@tenant#cluster', passwordOption].join(' '))).toBeUndefined()
    expect(parseDataSourceConnectionString(['mysql', '-h127.0.0.1', '-P2883', '-uapp@tenant#cluster'].join(' '))).toBeUndefined()
  })
})
