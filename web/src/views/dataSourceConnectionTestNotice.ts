import type { DataSourceConnectionTest } from '../api/browser'

export function dataSourceConnectionTestNotice(result: DataSourceConnectionTest) {
  if (result.status === 'SUCCEEDED' && result.verificationSource === 'G2_SYNTHETIC') {
    return 'G2 合成测试已结束。该结果不代表数据源已连通，不能据此启用数据源或选择导出任务。'
  }
  if (result.status === 'SUCCEEDED') {
    return '所选节点已回写基础连接成功。该结果只证明该节点在本次测试时可达，不代表其他节点、对象权限、性能或任务可行性。'
  }
  if (result.status === 'FAILED' && result.resultCode === 'DATABASE_TCP_UNREACHABLE') {
    return '所选节点无法建立到目标数据库监听端口的 TCP 连接。请检查该节点到目标端口的网络路由、防火墙和监听服务；本次尚未进入 JDBC 认证，不能据此判断账号或密码。'
  }
  if (result.status === 'FAILED' && result.resultCode === 'DATABASE_HOST_UNRESOLVABLE') {
    return '所选节点无法解析登记的数据库主机名。请先修复该节点的 DNS 或数据源地址；本次未进入 TCP 连接和 JDBC 认证。'
  }
  if (result.status === 'FAILED' && result.resultCode === 'DATABASE_TCP_REFUSED') {
    return '目标端口明确拒绝了所选节点的 TCP 连接。请检查 ODP 监听服务是否启动、是否监听登记端口，以及目标侧防火墙规则；本次未进入 JDBC 认证。'
  }
  if (result.status === 'FAILED' && result.resultCode === 'DATABASE_TCP_TIMEOUT') {
    return '所选节点在 5 秒内未能建立 TCP 连接。请检查到目标端口的路由与防火墙放通；本次未进入 JDBC 认证。'
  }
  if (result.status === 'FAILED' && result.resultCode === 'DATABASE_CONNECTION_FAILED') {
    return '所选节点已建立 TCP 连接，但固定 JDBC 探针未能完成基础连接。请核对 ODP 服务与连接身份；页面不会展示地址、账号、密码或异常原文。'
  }
  if (result.status === 'FAILED') return '所选节点已回写基础连接失败。页面不会根据失败码推断连接细节。'
  if (result.status === 'UNKNOWN') return '所选节点未能形成可验证的基础连接结论。'
  if (result.status === 'EXPIRED') return '连接测试租约已过期，不能作为数据源可用性依据。'
  return '连接测试绑定的配置、凭据、节点或节点事实已变化，结果已失效。'
}

export function sysCredentialVerificationNotice(result: DataSourceConnectionTest) {
  if (!result.sysCredentialConfigured) return ''
  if (result.sysVerificationStatus === 'SUCCEEDED') {
    return 'sys 租户凭据验证成功（固定 JDBC 探针已建立 sys 租户连接），可作为依赖 sys 凭据的导出能力依据。'
  }
  if (result.sysVerificationStatus === 'FAILED') {
    return `sys 租户凭据验证失败（${result.sysResultCode ?? 'SYS_CONNECTION_FAILED'}）。数据库连接不受影响，但依赖 sys 凭据的导出能力不可用。`
  }
  if (result.sysVerificationStatus === 'UNKNOWN') {
    return 'sys 租户凭据未能形成可验证的结论，依赖 sys 凭据的导出能力保持不可用。'
  }
  return ''
}
