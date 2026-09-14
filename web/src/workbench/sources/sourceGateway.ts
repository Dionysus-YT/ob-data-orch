import { browserApi, type DataSourceSummary, type DataSourceUpdate } from '@/api/browser'
import { dataSourceUiFixtureForSearch } from '@/views/dataSourceUiFixture'

export type SourceGateway = Pick<ReturnType<typeof browserApi>, 'listDataSources' | 'getDataSource' | 'createDataSource' | 'updateDataSource' | 'changeDataSourceState' | 'deleteDataSource' | 'archiveDataSource' | 'listDataSourceConnectionTestNodeCandidates' | 'startDataSourceConnectionTest' | 'getDataSourceConnectionTest'>

export function sourceGateway(search: string): { api: SourceGateway; preview: boolean } {
  const fixture = dataSourceUiFixtureForSearch(search)
  return fixture ? { api: createVisualSourceGateway(fixture), preview: true } : { api: browserApi(), preview: false }
}

// 视觉样本只在开发入口的内存中读写；不调用真实 API，也不保存任何密码或原始连接串。
export function createVisualSourceGateway(fixtures: readonly DataSourceSummary[]): SourceGateway {
  let rows: DataSourceSummary[] = structuredClone([...fixtures])
  let sequence = 0
  const tests = new Map<string, { sourceId: string; revision: number; startedAt: number; nodeId: string }>()
  const get = (id: string) => {
    const row = rows.find((item) => item.id === id)
    if (!row) throw { status: 404, code: 'NOT_FOUND', message: '视觉样本不存在。' }
    return row
  }
  const current = (id: string, revision: number) => {
    const row = get(id)
    if (row.revision !== revision) throw { status: 412, code: 'REVISION_CONFLICT', conflict: true }
    return row
  }
  const change = (row: DataSourceSummary) => { rows = rows.map((item) => item.id === row.id ? row : item); return structuredClone(row) }
  const eligibility = (row: DataSourceSummary, action: 'enable' | 'disable' | 'delete' | 'archive') => {
    if (!row.lifecycleEligibility?.[action].allowed) throw { status: 409, code: 'LIFECYCLE_INELIGIBLE', message: '当前样本没有该操作的服务端资格投影。' }
  }
  return {
    async listDataSources() { return structuredClone(rows) },
    async getDataSource(id) { return structuredClone(get(id)) },
    async listDataSourceConnectionTestNodeCandidates() { return [{ id: 'visual-validation-runtime', displayName: 'Visual validation runtime', platform: 'WINDOWS_AMD64' }] },
    async createDataSource(input) {
      const id = `visual-validation-created-${++sequence}`
      rows = [...rows, { id, displayName: input.displayName, environment: input.environment, connectionKind: input.connectionKind, compatibilityMode: input.compatibilityMode, host: input.host, port: input.port, clusterName: input.clusterName, tenantName: input.tenantName, username: input.username, defaultDatabase: input.defaultDatabase, state: 'DISABLED', revision: 1, credentialRevision: 1, sysCredentialState: input.sysPassword ? 'AVAILABLE' : 'UNAVAILABLE', lifecycleEligibility: { enable: { allowed: false, reason: '视觉验证结果不具备真实连接资格。' }, disable: { allowed: false }, delete: { allowed: true }, archive: { allowed: false } } }]
      return id
    },
    async updateDataSource(id, revision, input) {
      const row = current(id, revision)
      const safe: { -readonly [K in keyof DataSourceUpdate]?: DataSourceUpdate[K] } = {}
      const { displayName, environment, connectionKind, compatibilityMode, host, port, clusterName, tenantName, username, defaultDatabase } = input
      Object.assign(safe, Object.fromEntries(Object.entries({ displayName, environment, connectionKind, compatibilityMode, host, port, clusterName, tenantName, username, defaultDatabase }).filter(([, value]) => value !== undefined)))
      const invalidates = Object.keys(input).some((key) => key !== 'displayName' && key !== 'environment')
      const result = { ...row, ...safe, revision: revision + 1, credentialRevision: row.credentialRevision + (input.password || input.sysPassword !== undefined ? 1 : 0), sysCredentialState: input.sysPassword === undefined ? row.sysCredentialState : input.sysPassword ? 'AVAILABLE' : 'UNAVAILABLE' }
      if (invalidates) result.lastTestStatus = 'INVALIDATED'
      return change(result)
    },
    async changeDataSourceState(id, revision, state) {
      const row = current(id, revision)
      eligibility(row, state === 'ENABLED' ? 'enable' : 'disable')
      change({ ...row, state, revision: revision + 1 })
      return { state, revision: revision + 1 }
    },
    async deleteDataSource(id, revision) { const row = current(id, revision); eligibility(row, 'delete'); rows = rows.filter((item) => item.id !== id); return { outcome: 'DELETED', revision: revision + 1 } },
    async archiveDataSource(id, revision) { const row = current(id, revision); eligibility(row, 'archive'); rows = rows.filter((item) => item.id !== id); return { outcome: 'ARCHIVED', revision: revision + 1 } },
    async startDataSourceConnectionTest(id, revision, nodeId) {
      current(id, revision)
      if (nodeId !== 'visual-validation-runtime') throw { status: 409, message: '请选择视觉验证执行节点。' }
      const testId = `visual-validation-test-${++sequence}`
      tests.set(testId, { sourceId: id, revision, startedAt: Date.now(), nodeId })
      change({ ...get(id), lastTestStatus: 'PENDING' })
      return { id: testId, status: 'PENDING', nodeId }
    },
    async getDataSourceConnectionTest(id) {
      const test = tests.get(id)
      if (!test) throw { status: 404, message: '视觉验证测试不存在。' }
      const row = get(test.sourceId)
      const status = row.revision !== test.revision ? 'INVALIDATED' as const : Date.now() - test.startedAt < 1800 ? 'PENDING' as const : 'FAILED' as const
      const completedAt = status === 'PENDING' ? undefined : new Date(test.startedAt + 1800).toISOString()
      change({ ...row, lastTestStatus: status, lastTestedAt: completedAt })
      return { id, status, nodeId: test.nodeId, nodeDisplayName: 'Visual validation runtime', verificationSource: 'G2_SYNTHETIC', realConnectionVerified: false, sysCredentialConfigured: row.sysCredentialState === 'AVAILABLE', resultCode: status === 'FAILED' ? 'DATABASE_TCP_TIMEOUT' : undefined, completedAt }
    },
  }
}
