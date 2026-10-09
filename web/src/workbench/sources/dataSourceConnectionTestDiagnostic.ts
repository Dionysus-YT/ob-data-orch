import type { DataSourceConnectionTest } from '@/api/browser'

export type DataSourceDiagnosticTarget = 'connection' | 'execution-node' | 'sys-credential'

export interface DataSourceConnectionTestDiagnostic {
  readonly title: string
  readonly stage: string
  readonly moduleLabel: string
  readonly target: DataSourceDiagnosticTarget
  readonly summary: string
  readonly checks: readonly string[]
}

// 诊断信息只能基于 Agent 回写的有限证据码收缩定位范围，不能从 JDBC 失败反推出密码、账号或租户等敏感细节。
export function dataSourceConnectionTestDiagnostic(result: DataSourceConnectionTest): DataSourceConnectionTestDiagnostic | undefined {
  if (result.status === 'EXPIRED') {
    return {
      title: '连接测试未完成', stage: '执行调度', moduleLabel: '执行节点连接测试', target: 'execution-node',
      summary: '测试租约在形成受控终态前已过期，当前结果不能证明数据源是否可连接。',
      checks: ['确认所选执行节点仍在线且空闲。', '刷新节点列表并重新选择节点后再次测试。'],
    }
  }
  if (result.status === 'INVALIDATED') {
    return {
      title: '连接测试结果已失效', stage: '配置一致性', moduleLabel: '连接信息', target: 'connection',
      summary: '数据源配置、凭据、执行节点或节点事实已变化，旧结果不能继续使用。',
      checks: ['保存当前连接配置。', '确认执行节点后重新发起测试。'],
    }
  }
  if (result.status === 'UNKNOWN') {
    return executionNodeDiagnostic('连接测试结果未知', '执行节点未能回写可验证的连接结论。')
  }
  if (result.status !== 'FAILED') return undefined

  switch (result.resultCode) {
    case 'DATABASE_HOST_UNRESOLVABLE':
      return {
        title: '数据库主机无法解析', stage: '主机名解析', moduleLabel: '连接信息 · ODP 地址', target: 'connection',
        summary: '所选执行节点无法解析当前登记的 ODP 域名，测试尚未进入端口连接和身份认证。',
        checks: ['确认 ODP 地址拼写正确，且不是仅控制面可解析的内部域名。', '确认所选执行节点的 DNS 配置能够解析该域名。'],
      }
    case 'DATABASE_TCP_REFUSED':
      return networkDiagnostic('目标端口拒绝连接', 'ODP 地址与 SQL 端口', '目标主机明确拒绝了 TCP 连接，测试尚未进入身份认证。', ['确认 ODP 服务已经启动并监听当前 SQL 端口。', '确认目标侧防火墙或访问控制没有主动拒绝该执行节点。'])
    case 'DATABASE_TCP_TIMEOUT':
      return networkDiagnostic('连接目标端口超时', 'ODP 地址与 SQL 端口', '所选执行节点未能在限定时间内建立 TCP 连接，测试尚未进入身份认证。', ['确认执行节点到 ODP 的网络路由可达。', '确认中间防火墙、安全组和目标端口已经放通。'])
    case 'DATABASE_TCP_UNREACHABLE':
      return networkDiagnostic('目标网络不可达', 'ODP 地址与 SQL 端口', '所选执行节点无法建立到目标监听端口的 TCP 连接，测试尚未进入身份认证。', ['确认 ODP 地址和 SQL 端口填写正确。', '确认执行节点所在网络可以访问目标网段和端口。'])
    case 'DATABASE_CONNECTION_FAILED':
      return {
        title: 'JDBC 基础连接失败', stage: 'JDBC 会话建立', moduleLabel: '连接信息 · 身份与租户', target: 'connection',
        summary: 'TCP 连接已经建立，但固定 JDBC 探针未能建立数据库会话。现有受控证据无法安全区分具体是身份、租户/集群、兼容模式还是 ODP 服务端拒绝。',
        checks: ['核对租户模式、集群名和租户名是否与目标实例一致。', '核对用户名；如凭据近期变更，请重新输入密码并先保存。', '确认 ODP 接受该连接身份，且租户白名单或访问策略允许所选执行节点。'],
      }
    default:
      return executionNodeDiagnostic('基础连接测试失败', '执行节点已回写失败，但当前证据码不足以继续收缩故障范围。')
  }
}

function networkDiagnostic(title: string, moduleLabel: string, summary: string, checks: readonly string[]): DataSourceConnectionTestDiagnostic {
  return { title, stage: 'TCP 网络连接', moduleLabel: `连接信息 · ${moduleLabel}`, target: 'connection', summary, checks }
}

function executionNodeDiagnostic(title: string, summary: string): DataSourceConnectionTestDiagnostic {
  return {
    title, stage: '执行节点探针', moduleLabel: '执行节点连接测试', target: 'execution-node', summary,
    checks: ['确认执行节点在线、空闲且环境检查正常。', '刷新节点列表，必要时选择其他合格节点重新测试。'],
  }
}

