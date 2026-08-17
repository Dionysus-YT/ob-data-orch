import { describe, expect, it } from 'vitest'

import { createBrowserApi, dataSourceErrorMessage, executionNodeErrorMessage, exportDraftErrorMessage, taskDetailErrorMessage, type FetchLike } from './browser'

function apiWith(response: Response, csrfToken = 'synthetic-csrf-token') {
  const calls: Array<{ path: string; init: RequestInit }> = []
  const fetcher = (async (path: string | URL | Request, init?: RequestInit) => {
    calls.push({ path: String(path), init: init ?? {} })
    return response
  }) as FetchLike
  return {
    api: createBrowserApi({ fetcher, csrfToken: () => csrfToken, idempotencyKey: () => 'synthetic-idempotency-key' }),
    calls,
  }
}

describe('浏览器 API 客户端', () => {
  it('读取数据源列表中的普通业务用户名且忽略敏感字段', async () => {
    const { api } = apiWith(Response.json({
      items: [{
        id: 'source-1',
        displayName: '合成数据源',
        environment: 'TEST',
        connectionKind: 'ODP',
        compatibilityMode: 'MYSQL',
        host: '127.0.0.1',
        port: 2881,
        clusterName: 'synthetic-cluster',
        tenantName: 'synthetic-tenant',
        username: 'synthetic-user',
        state: 'ENABLED',
        revision: 1,
        credentialRevision: 1,
        lastTestStatus: 'SUCCEEDED',
        lastTestedAt: '2026-07-24T03:00:00Z',
        password: 'must-not-be-read',
        sysUser: 'must-not-be-read',
        combinedUsername: 'must-not-be-read',
      }],
    }))

    const [source] = await api.listDataSources()

    expect(source).toEqual({
      id: 'source-1',
      displayName: '合成数据源',
      environment: 'TEST',
      connectionKind: 'ODP',
      compatibilityMode: 'MYSQL',
      host: '127.0.0.1',
      port: 2881,
      clusterName: 'synthetic-cluster',
      tenantName: 'synthetic-tenant',
      username: 'synthetic-user',
      state: 'ENABLED',
      revision: 1,
      credentialRevision: 1,
      lastTestStatus: 'SUCCEEDED',
      lastTestedAt: '2026-07-24T03:00:00Z',
    })
    expect(source).not.toHaveProperty('password')
    expect(source).not.toHaveProperty('sysUser')
    expect(source).not.toHaveProperty('combinedUsername')
  })

  it('数据源详情继续按授权投影普通业务用户名', async () => {
    const { api, calls } = apiWith(Response.json({ item: {
      id: 'source-1', displayName: '合成数据源', environment: 'TEST', connectionKind: 'ODP', compatibilityMode: 'MYSQL',
      host: '127.0.0.1', port: 2881, clusterName: 'synthetic-cluster', tenantName: 'synthetic-tenant', username: 'synthetic-user',
      state: 'DISABLED', revision: 1, credentialRevision: 1, password: 'must-not-be-read', sysUser: 'must-not-be-read',
    } }))

    await expect(api.getDataSource('source/1')).resolves.toMatchObject({ id: 'source-1', username: 'synthetic-user' })
    expect(calls[0]?.path).toBe('/api/v1/data-sources/source%2F1')
  })

  it('以独立函数调用注入的 fetch，避免原生浏览器 fetch 收到错误 this', async () => {
    const observation: { receiver: unknown } = { receiver: null }
    const fetcher = function (this: unknown): Promise<Response> {
      observation.receiver = this
      return Promise.resolve(Response.json({ items: [] }))
    } as FetchLike
    const api = createBrowserApi({ fetcher, csrfToken: () => 'synthetic-csrf-token', idempotencyKey: () => 'synthetic-idempotency-key' })

    await expect(api.listDataSources()).resolves.toEqual([])

    expect(observation.receiver).toBeUndefined()
  })

  it('只读取导出草稿选择节点所需的安全候选字段', async () => {
    const { api, calls } = apiWith(Response.json({
      items: [{ id: 'node-1', displayName: '合成节点', platform: 'WINDOWS_AMD64', agentCredential: 'must-not-be-read' }],
    }))

    await expect(api.listExportNodeCandidates()).resolves.toEqual([{ id: 'node-1', displayName: '合成节点', platform: 'WINDOWS_AMD64' }])

    expect(calls[0]?.path).toBe('/api/v1/execution-nodes?eligibleFor=OBDUMPER_EXPORT')
  })

  it('读取节点管理列表时投影明确的失败关闭状态', async () => {
    const { api, calls } = apiWith(Response.json({
      items: [{
        id: 'node-1',
        displayName: 'Windows 节点',
        platform: 'WINDOWS_AMD64',
        managementState: 'DISABLED',
        agentAssociationStatus: 'PENDING',
        heartbeatStatus: 'NEVER_CONNECTED',
        environmentStatus: 'NOT_CHECKED',
        capacityStatus: 'UNKNOWN',
        acceptsNewTasks: false,
        unavailableReasons: ['AGENT_ASSOCIATION_REQUIRED', 'ENVIRONMENT_CHECK_REQUIRED'],
        revision: 1,
        updatedAt: '2026-07-24T03:00:00Z',
        agentCredential: 'must-not-be-read',
      }],
    }))

    await expect(api.listExecutionNodes()).resolves.toEqual([{
      id: 'node-1',
      displayName: 'Windows 节点',
      platform: 'WINDOWS_AMD64',
      managementState: 'DISABLED',
      agentAssociationStatus: 'PENDING',
      heartbeatStatus: 'NEVER_CONNECTED',
      lastHeartbeatAt: null,
      environmentStatus: 'NOT_CHECKED',
      capacityStatus: 'UNKNOWN',
      acceptsNewTasks: false,
      unavailableReasons: ['AGENT_ASSOCIATION_REQUIRED', 'ENVIRONMENT_CHECK_REQUIRED'],
      revision: 1,
      updatedAt: '2026-07-24T03:00:00Z',
    }])

    expect(calls[0]?.path).toBe('/api/v1/execution-nodes')
    expect(calls[0]?.init).toMatchObject({ method: 'GET' })
  })

  it('投影受认证 Agent 的心跳与只读环境事实，不读取敏感或路径字段', async () => {
    const { api } = apiWith(Response.json({
      items: [{
        id: 'node-associated',
        displayName: '已关联 Windows 节点',
        platform: 'WINDOWS_AMD64',
        managementState: 'ENABLED',
        agentAssociationStatus: 'ASSOCIATED',
        heartbeatStatus: 'ONLINE',
        lastHeartbeatAt: '2026-07-27T03:00:00Z',
        environmentStatus: 'NORMAL',
        capacityStatus: 'AVAILABLE',
        acceptsNewTasks: true,
        unavailableReasons: [],
        revision: 3,
        updatedAt: '2026-07-27T03:00:00Z',
        agentFacts: {
          os: 'windows',
          arch: 'amd64',
          agentVersion: '0.1.0',
          bootId: 'synthetic-boot-id',
          observedAt: '2026-07-27T03:00:00Z',
          capacityTotal: 1,
          capacityUsed: 0,
          cpuUsagePercent: null,
          memoryUsagePercent: null,
          disks: [{ path: 'E:\\private\\agent-data', availableBytes: 1 }],
          machineCredential: 'must-not-be-read',
        },
      }],
    }))

    await expect(api.listExecutionNodes()).resolves.toMatchObject([{
      id: 'node-associated',
      agentAssociationStatus: 'ASSOCIATED',
      heartbeatStatus: 'ONLINE',
      lastHeartbeatAt: '2026-07-27T03:00:00Z',
      environmentStatus: 'NORMAL',
      capacityStatus: 'AVAILABLE',
      acceptsNewTasks: true,
      agentFacts: {
        os: 'windows',
        arch: 'amd64',
        agentVersion: '0.1.0',
        bootId: 'synthetic-boot-id',
        observedAt: '2026-07-27T03:00:00Z',
        capacityTotal: 1,
        capacityUsed: 0,
      },
    }])
  })

  it('拒绝将未检查环境投影为可接收新任务', async () => {
    const { api } = apiWith(Response.json({
      items: [{
        id: 'node-unchecked',
        displayName: '未检查节点',
        platform: 'WINDOWS_AMD64',
        managementState: 'ENABLED',
        agentAssociationStatus: 'ASSOCIATED',
        heartbeatStatus: 'ONLINE',
        lastHeartbeatAt: '2026-07-27T03:00:00Z',
        environmentStatus: 'NOT_CHECKED',
        capacityStatus: 'AVAILABLE',
        acceptsNewTasks: true,
        unavailableReasons: [],
        revision: 1,
        updatedAt: '2026-07-27T03:00:00Z',
      }],
    }))

    await expect(api.listExecutionNodes()).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
  })

  it('保留历史已启用节点的只读管理状态', async () => {
    const { api } = apiWith(Response.json({
      items: [{
        id: 'node-enabled',
        displayName: '历史已启用节点',
        platform: 'WINDOWS_AMD64',
        managementState: 'ENABLED',
        agentAssociationStatus: 'PENDING',
        heartbeatStatus: 'NEVER_CONNECTED',
        environmentStatus: 'NOT_CHECKED',
        capacityStatus: 'UNKNOWN',
        acceptsNewTasks: false,
        unavailableReasons: ['AGENT_ASSOCIATION_REQUIRED', 'ENVIRONMENT_CHECK_REQUIRED'],
        revision: 4,
        updatedAt: '2026-07-24T03:00:00Z',
      }],
    }))

    await expect(api.listExecutionNodes()).resolves.toMatchObject([{ id: 'node-enabled', managementState: 'ENABLED', acceptsNewTasks: false }])
  })

  it('读取节点详情时保留允许根目录并对节点标识编码', async () => {
    const { api, calls } = apiWith(Response.json({
      item: {
        id: 'node/1',
        displayName: 'Linux 节点',
        platform: 'LINUX_ARM64',
        managementState: 'MAINTENANCE',
        agentAssociationStatus: 'PENDING',
        heartbeatStatus: 'NEVER_CONNECTED',
        environmentStatus: 'NOT_CHECKED',
        capacityStatus: 'UNKNOWN',
        acceptsNewTasks: false,
        unavailableReasons: ['AGENT_ASSOCIATION_REQUIRED'],
        revision: 2,
        updatedAt: '2026-07-24T03:00:00Z',
        allowedRoots: ['/var/lib/ob-data-orch/exports'],
        toolHome: '/opt/ob-loader-dumper-4.3.5',
        javaPath: '/usr/lib/jvm/java-8/bin/java',
        createdAt: '2026-07-24T02:00:00Z',
        toolPath: 'must-not-be-read',
      },
    }))

    await expect(api.getExecutionNode('node/1')).resolves.toMatchObject({
      id: 'node/1',
      platform: 'LINUX_ARM64',
      allowedRoots: ['/var/lib/ob-data-orch/exports'],
      revision: 2,
    })

    expect(calls[0]?.path).toBe('/api/v1/execution-nodes/node%2F1')
    expect(calls[0]?.init).toMatchObject({ method: 'GET' })
  })

  it('创建节点使用 CSRF 与幂等键', async () => {
    const { api, calls } = apiWith(Response.json({ id: 'node-1', revision: 1, managementState: 'DISABLED' }))
    const input = {
      displayName: 'Windows 节点',
      platform: 'WINDOWS_AMD64' as const,
      allowedRoots: ['E:\\ob-data\\exports'],
      toolHome: 'E:\\tools\\ob-loader-dumper-4.3.5',
      javaPath: 'C:\\Program Files\\Java\\jdk8\\bin\\java.exe',
    }

    await expect(api.createExecutionNode(input)).resolves.toBe('node-1')

    expect(calls[0]?.path).toBe('/api/v1/execution-nodes')
    expect(calls[0]?.init).toMatchObject({ method: 'POST' })
    expect(calls[0]?.init.headers).toMatchObject({
      'X-CSRF-Token': 'synthetic-csrf-token',
      'Idempotency-Key': 'synthetic-idempotency-key',
    })
    expect(calls[0]?.init.body).toBe(JSON.stringify(input))
  })

  it('更新节点使用版本条件并返回受控详情', async () => {
    const { api, calls } = apiWith(Response.json({
      item: {
        id: 'node-1',
        displayName: 'Windows 节点（已更新）',
        platform: 'WINDOWS_AMD64',
        managementState: 'DISABLED',
        agentAssociationStatus: 'PENDING',
        heartbeatStatus: 'NEVER_CONNECTED',
        environmentStatus: 'NOT_CHECKED',
        capacityStatus: 'UNKNOWN',
        acceptsNewTasks: false,
        unavailableReasons: ['AGENT_ASSOCIATION_REQUIRED', 'ENVIRONMENT_CHECK_REQUIRED'],
        revision: 2,
        updatedAt: '2026-07-24T03:00:00Z',
        allowedRoots: ['E:\\ob-data\\exports'],
        toolHome: 'E:\\tools\\ob-loader-dumper-4.3.5',
        javaPath: 'C:\\Program Files\\Java\\jdk8\\bin\\java.exe',
        createdAt: '2026-07-24T02:00:00Z',
      },
    }))
    const input = {
      displayName: 'Windows 节点（已更新）',
      platform: 'WINDOWS_AMD64' as const,
      allowedRoots: ['E:\\ob-data\\exports'],
      toolHome: 'E:\\tools\\ob-loader-dumper-4.3.5',
      javaPath: 'C:\\Program Files\\Java\\jdk8\\bin\\java.exe',
    }

    await expect(api.updateExecutionNode('node-1', 1, input)).resolves.toMatchObject({ id: 'node-1', revision: 2, allowedRoots: input.allowedRoots })

    expect(calls[0]?.path).toBe('/api/v1/execution-nodes/node-1')
    expect(calls[0]?.init).toMatchObject({ method: 'PATCH' })
    expect(calls[0]?.init.headers).toMatchObject({ 'X-CSRF-Token': 'synthetic-csrf-token', 'If-Match': '"rev-1"' })
    expect(calls[0]?.init.headers).not.toHaveProperty('Idempotency-Key')
  })

  it('签发节点一次性关联材料时使用 CSRF 与幂等键', async () => {
    const { api, calls } = apiWith(Response.json({
      requestId: 'request-1',
      enrollmentId: 'enrollment-1',
      nodeId: 'node-1',
      enrollmentMaterial: 'synthetic-one-time-material',
      expiresAt: '2026-07-27T04:00:00Z',
      displayedOnce: true,
      machineCredential: 'must-not-be-read',
    }))

    await expect(api.issueExecutionNodeEnrollment('node-1')).resolves.toEqual({
      requestId: 'request-1',
      enrollmentId: 'enrollment-1',
      nodeId: 'node-1',
      enrollmentMaterial: 'synthetic-one-time-material',
      expiresAt: '2026-07-27T04:00:00Z',
      displayedOnce: true,
    })

    expect(calls[0]?.path).toBe('/api/v1/execution-nodes/node-1:enrollments')
    expect(calls[0]?.init).toMatchObject({ method: 'POST' })
    expect(calls[0]?.init.body).toBeUndefined()
    expect(calls[0]?.init.headers).toMatchObject({
      'X-CSRF-Token': 'synthetic-csrf-token',
      'Idempotency-Key': 'synthetic-idempotency-key',
    })
  })

  it('关联材料响应未明确一次性展示或节点不一致时失败关闭', async () => {
    const notDisplayedOnce = apiWith(Response.json({
      requestId: 'request-1', enrollmentId: 'enrollment-1', nodeId: 'node-1', enrollmentMaterial: 'synthetic-one-time-material', expiresAt: '2026-07-27T04:00:00Z', displayedOnce: false,
    })).api
    const wrongNode = apiWith(Response.json({
      requestId: 'request-1', enrollmentId: 'enrollment-1', nodeId: 'node-2', enrollmentMaterial: 'synthetic-one-time-material', expiresAt: '2026-07-27T04:00:00Z', displayedOnce: true,
    })).api

    await expect(notDisplayedOnce.issueExecutionNodeEnrollment('node-1')).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
    await expect(wrongNode.issueExecutionNodeEnrollment('node-1')).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
  })

  it('签发关联材料缺少 CSRF 时不发送请求', async () => {
    const { api, calls } = apiWith(Response.json({}), '')

    await expect(api.issueExecutionNodeEnrollment('node-1')).rejects.toMatchObject({ code: 'CSRF_TOKEN_UNAVAILABLE' })

    expect(calls).toHaveLength(0)
  })

  it('保留节点字段错误，拒绝不安全的节点状态响应', async () => {
    const errorResponse = Response.json({
      requestId: 'synthetic-request',
      code: 'EXECUTION_NODE_FIELDS_INVALID',
      message: '执行节点字段不符合要求。',
      retryable: false,
      fieldErrors: [{ field: 'allowedRoots', code: 'EXECUTION_NODE_ROOTS_INVALID', message: '允许根目录必须是绝对路径。' }],
      safeDetails: {},
    }, { status: 422 })
    const { api } = apiWith(errorResponse)
    const input = {
      displayName: 'Linux 节点', platform: 'LINUX_AMD64' as const, allowedRoots: ['/var/lib/ob-data-orch/exports'],
      toolHome: '/opt/ob-loader-dumper-4.3.5', javaPath: '/usr/lib/jvm/java-8/bin/java',
    }

    await expect(api.createExecutionNode(input)).rejects.toMatchObject({
      code: 'EXECUTION_NODE_FIELDS_INVALID',
      fieldErrors: [{ field: 'allowedRoots', code: 'EXECUTION_NODE_ROOTS_INVALID', message: '允许根目录必须是绝对路径。' }],
    })

    const invalidState = apiWith(Response.json({
      items: [{
        id: 'node-1', displayName: '不安全节点', platform: 'WINDOWS_AMD64', managementState: 'ARCHIVED',
        agentAssociationStatus: 'PENDING', heartbeatStatus: 'NEVER_CONNECTED', environmentStatus: 'NOT_CHECKED', capacityStatus: 'UNKNOWN',
        acceptsNewTasks: false, unavailableReasons: [], revision: 1, updatedAt: '2026-07-24T03:00:00Z',
      }],
    })).api

    await expect(invalidState.listExecutionNodes()).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
  })

  it('为创建草稿添加 CSRF 与幂等键', async () => {
    const { api, calls } = apiWith(Response.json({ id: 'draft-1' }))

    await api.createExportDraft({
      configVersion: 'v6',
      dataSourceId: 'source-1',
      nodeId: 'node-1',
      config: {
        objectScope: { database: 'synthetic_db', scopeKind: 'SPECIFIED', objectTypes: ['TABLE'], expressions: [{ name: 'synthetic_table' }] },
        contentSelection: { contentKind: 'DATA_ONLY' },
        dataFormat: { formatKind: 'CSV' },
        outputConfig: { outputKind: 'LOCAL', filePath: '/E:/tmp/output' },
      },
    })

    expect(calls[0]?.path).toBe('/api/v1/export-drafts')
    expect(calls[0]?.init.headers).toMatchObject({
      'X-CSRF-Token': 'synthetic-csrf-token',
      'Idempotency-Key': 'synthetic-idempotency-key',
    })
  })

  it('读取预检查时只接受完整固定六项安全结果', async () => {
    const { api, calls } = apiWith(Response.json({
      item: {
        id: 'precheck-1',
        draftId: 'draft-1',
        draftRevision: 3,
        configFingerprint: 'synthetic-fingerprint',
        nodeId: 'node-1',
        status: 'PENDING',
        integrityStatus: 'UNKNOWN',
        results: [
          { check: 'DATABASE_CONNECTIVITY', status: 'UNKNOWN', evidenceCode: 'DATABASE_CONNECTION_UNAVAILABLE' },
          { check: 'OBJECT_ACCESS', status: 'UNKNOWN', evidenceCode: 'OBJECT_ACCESS_UNAVAILABLE' },
          { check: 'TOOL_ENVIRONMENT', status: 'UNKNOWN', evidenceCode: 'TOOL_RUNTIME_UNAVAILABLE' },
          { check: 'OUTPUT_PATH', status: 'UNKNOWN', evidenceCode: 'OUTPUT_PATH_UNAVAILABLE' },
          { check: 'OUTPUT_EMPTY', status: 'UNKNOWN', evidenceCode: 'OUTPUT_PATH_UNAVAILABLE' },
          { check: 'AVAILABLE_SPACE', status: 'UNKNOWN', evidenceCode: 'OUTPUT_SPACE_UNAVAILABLE' },
        ],
        validUntil: '2026-07-30T09:00:00Z',
      },
    }))

    const precheck = await api.getPrecheck('precheck-1')

    expect(precheck.id).toBe('precheck-1')
    expect(precheck.status).toBe('PENDING')
    expect(precheck.results).toHaveLength(6)
    expect(precheck.results[0]).toMatchObject({ check: 'DATABASE_CONNECTIVITY', status: 'UNKNOWN' })

    expect(calls[0]?.path).toBe('/api/v1/prechecks/precheck-1')
  })

  it('读取任务日志时只接受浏览器契约的 lowerCamelCase 脱敏投影', async () => {
    const { api, calls } = apiWith(Response.json({
      items: [
        { sourceSeq: 6, kind: 'LOG', message: '', integrityCode: '', receivedAt: '2026-07-31T02:59:00Z' },
        { sourceSeq: 7, kind: 'LOG', message: '2026-07-31 [ERROR] synthetic tool failure', integrityCode: '', receivedAt: '2026-07-31T03:00:00Z', password: 'must-not-be-read' },
      ],
    }))

    await expect(api.getTaskLogs('task/1')).resolves.toEqual({
      items: [
        { sourceSeq: 6, kind: 'LOG', message: '', integrityCode: 'NONE', receivedAt: '2026-07-31T02:59:00Z' },
        { sourceSeq: 7, kind: 'LOG', message: '2026-07-31 [ERROR] synthetic tool failure', integrityCode: 'NONE', receivedAt: '2026-07-31T03:00:00Z' },
      ],
      integrity: 'UNKNOWN',
      lastReliableCursor: undefined,
      nextCursor: undefined,
    })

    expect(calls[0]?.path).toBe('/api/v1/tasks/task%2F1/logs')
  })

  it('分别读取任务概览、冻结快照、命令证据与执行事实', async () => {
    const overview = apiWith(Response.json({
      item: { id: 'task-1', type: 'OBDUMPER_EXPORT', dataSourceId: 'source-1', nodeId: 'node-1', precheckId: 'precheck-1', submittedAt: '2026-07-31T03:00:00Z', plannedCommand: 'must-not-be-read' },
    }))
    await expect(overview.api.getTaskOverview('task/1')).resolves.toEqual({
      id: 'task-1', type: 'OBDUMPER_EXPORT', dataSourceId: 'source-1', nodeId: 'node-1', precheckId: 'precheck-1', submittedAt: '2026-07-31T03:00:00Z',
    })
    expect(overview.calls[0]?.path).toBe('/api/v1/tasks/task%2F1')

    const snapshot = apiWith(Response.json({
      item: {
        type: 'OBDUMPER_EXPORT', snapshotVersion: 'v2', dataSourceId: 'source-1', nodeId: 'node-1', precheckId: 'precheck-1', objectSummary: 'synthetic_db.synthetic_table',
        format: 'DDL_CSV', configFingerprint: 'synthetic-fingerprint', toolVersion: '4.3.5-RELEASE', metadataVersion: 'metadata-v1', capabilityVersion: 'capability-v1', filePath: 'must-not-be-read',
      },
    }))
    await expect(snapshot.api.getTaskSnapshot('task/1')).resolves.toEqual({
      type: 'OBDUMPER_EXPORT', snapshotVersion: 'v2', dataSourceId: 'source-1', nodeId: 'node-1', precheckId: 'precheck-1', objectSummary: 'synthetic_db.synthetic_table',
      format: 'DDL_CSV', configFingerprint: 'synthetic-fingerprint', toolVersion: '4.3.5-RELEASE', metadataVersion: 'metadata-v1', capabilityVersion: 'capability-v1',
    })
    expect(snapshot.calls[0]?.path).toBe('/api/v1/tasks/task%2F1/snapshot')

    const command = apiWith(Response.json({ item: { kind: 'PLANNED', command: 'obdumper --user ****** -p ******', redaction: 'PASSWORD_ONLY', argv: 'must-not-be-read' } }))
    await expect(command.api.getTaskCommandEvidence('task/1')).resolves.toEqual({ kind: 'PLANNED', command: 'obdumper --user ****** -p ******', redaction: 'PASSWORD_ONLY' })
    expect(command.calls[0]?.path).toBe('/api/v1/tasks/task%2F1/command-evidence')

    const execution = apiWith(Response.json({
      item: {
        state: 'RUNNING', executionId: 'execution-1', reconciliationRequired: true, stageEvidence: 'UNAVAILABLE', progressEvidence: 'UNAVAILABLE',
        startedAt: '2026-07-31T03:01:00Z', updatedAt: '2026-07-31T03:02:00Z', processEvidence: 'must-not-be-read',
      },
    }))
    await expect(execution.api.getTaskExecution('task/1')).resolves.toEqual({
      state: 'RUNNING', executionId: 'execution-1', reconciliationRequired: true, stageEvidence: 'UNAVAILABLE', progressEvidence: 'UNAVAILABLE',
      startedAt: '2026-07-31T03:01:00Z', finishedAt: undefined, updatedAt: '2026-07-31T03:02:00Z',
    })
    expect(execution.calls[0]?.path).toBe('/api/v1/tasks/task%2F1/execution')

    const unsafe = apiWith(Response.json({ item: { kind: 'PLANNED', command: 'obdumper -p synthetic-password', redaction: 'PASSWORD_ONLY' } }))
    await expect(unsafe.api.getTaskCommandEvidence('task-1')).rejects.toMatchObject({ code: 'UNSAFE_COMMAND_RESPONSE' })
  })

  it('读取授权任务页时只保留列表安全字段并编码游标', async () => {
    const { api, calls } = apiWith(Response.json({
      items: [{
        id: 'task-1', type: 'OBDUMPER_EXPORT', dataSourceId: 'source-1', objectSummary: 'synthetic_db.synthetic_table',
        state: 'RUNNING', stageEvidence: 'UNAVAILABLE', progressEvidence: 'UNAVAILABLE', reconciliationRequired: true,
        nodeId: 'node-1', ownedByCurrentUser: true, submittedAt: '2026-07-31T03:00:00Z', updatedAt: '2026-07-31T03:01:00Z',
        plannedCommand: 'must-not-be-read', error: 'must-not-be-read',
      }],
      nextCursor: 'cursor/next',
      totalPages: 2,
    }))

    await expect(api.listTasks('cursor/current', 20)).resolves.toEqual({
      items: [{
        id: 'task-1', type: 'OBDUMPER_EXPORT', dataSourceId: 'source-1', objectSummary: 'synthetic_db.synthetic_table',
        state: 'RUNNING', stageEvidence: 'UNAVAILABLE', progressEvidence: 'UNAVAILABLE', reconciliationRequired: true,
        nodeId: 'node-1', ownedByCurrentUser: true, submittedAt: '2026-07-31T03:00:00Z', updatedAt: '2026-07-31T03:01:00Z',
      }],
      nextCursor: 'cursor/next',
      totalPages: 2,
    })
    expect(calls[0]?.path).toBe('/api/v1/tasks?limit=20&cursor=cursor%2Fcurrent')
  })

  it('任务列表默认请求每页 10 条', async () => {
    const { api, calls } = apiWith(Response.json({ items: [], totalPages: 0 }))

    await expect(api.listTasks()).resolves.toEqual({ items: [], nextCursor: undefined, totalPages: 0 })

    expect(calls[0]?.path).toBe('/api/v1/tasks?limit=10')
  })

  it('拒绝无效任务总页数', async () => {
    const { api } = apiWith(Response.json({ items: [], totalPages: -1 }))

    await expect(api.listTasks()).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
  })

  it('拒绝任务列表伪造的阶段或进度证据', async () => {
    const { api } = apiWith(Response.json({
      items: [{
        id: 'task-1', type: 'OBDUMPER_EXPORT', dataSourceId: 'source-1', state: 'RUNNING',
        stageEvidence: 'RELIABLE', progressEvidence: 'PERCENT', reconciliationRequired: false,
        nodeId: 'node-1', ownedByCurrentUser: true, submittedAt: '2026-07-31T03:00:00Z', updatedAt: '2026-07-31T03:01:00Z',
      }], totalPages: 1,
    }))

    await expect(api.listTasks()).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
  })

  it('拒绝缺项的预检查结果，避免空结果被误判为通过', async () => {
    const { api } = apiWith(Response.json({
      item: {
        id: 'precheck-1', draftId: 'draft-1', draftRevision: 3, configFingerprint: 'synthetic-fingerprint', nodeId: 'node-1',
        status: 'SUCCEEDED', integrityStatus: 'COMPLETE',
        results: [{ check: 'DATABASE_CONNECTIVITY', status: 'PASSED', evidenceCode: 'DATABASE_CONNECTED' }],
        validUntil: '2026-07-30T09:00:00Z',
      },
    }))

    await expect(api.getPrecheck('precheck-1')).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
  })

  it('读取导出草稿时只解析服务端返回的固定 CSV 配置', async () => {
    const { api, calls } = apiWith(Response.json({
      item: {
        id: 'draft-1',
        dataSourceId: 'source-1',
        nodeId: 'node-1',
        revision: 3,
        configVersion: 'v6',
        config: {
          configVersion: 'v6',
          dataSourceId: 'source-1',
          nodeId: 'node-1',
          database: 'synthetic_db',
          scopeKind: 'SPECIFIED',
          table: 'synthetic_table',
          contentKind: 'DATA_ONLY',
          format: 'CSV',
          filePath: '/E:/tmp/output',
          logPath: '',
          skipCheckDir: false,
          config: {
            objectScope: { database: 'synthetic_db', scopeKind: 'SPECIFIED', objectTypes: ['TABLE'], expressions: [{ name: 'synthetic_table' }], username: 'must-not-be-read' },
            contentSelection: { contentKind: 'DATA_ONLY' },
            dataFormat: { formatKind: 'CSV' },
            outputConfig: { outputKind: 'LOCAL', filePath: '/E:/tmp/output' },
          },
        },
        configFingerprint: 'synthetic-fingerprint',
      },
    }))

    await expect(api.getExportDraft('draft-1')).resolves.toEqual({
      id: 'draft-1',
      dataSourceId: 'source-1',
      nodeId: 'node-1',
      revision: 3,
      configVersion: 'v6',
      config: {
        objectScope: { database: 'synthetic_db', scopeKind: 'SPECIFIED', objectTypes: ['TABLE'], expressions: [{ name: 'synthetic_table' }], excludeTables: undefined },
        contentSelection: { contentKind: 'DATA_ONLY' },
        dataFormat: { formatKind: 'CSV' },
        outputConfig: { outputKind: 'LOCAL', filePath: '/E:/tmp/output', logPath: '', skipCheckDir: false, controlFilePath: '', tmpPath: '' },
      },
      configFingerprint: 'synthetic-fingerprint',
    })

    expect(calls[0]?.path).toBe('/api/v1/export-drafts/draft-1')
    expect(calls[0]?.init).toMatchObject({ method: 'GET' })
  })

  it('解析全部对象草稿时把 null 可选数组视为缺省', async () => {
    const { api } = apiWith(Response.json({
      item: {
        id: 'draft-all',
        dataSourceId: 'source-1',
        nodeId: 'node-1',
        revision: 1,
        configVersion: 'v6',
        config: {
          configVersion: 'v6',
          dataSourceId: 'source-1',
          nodeId: 'node-1',
          database: 'synthetic_db',
          scopeKind: 'ALL',
          table: '',
          contentKind: 'DDL_AND_DATA',
          format: 'DDL_CSV',
          filePath: '/E:/tmp/output',
          logPath: '',
          skipCheckDir: false,
          config: {
            objectScope: { database: 'synthetic_db', scopeKind: 'ALL', objectTypes: null, expressions: null, excludeTables: null },
            contentSelection: { contentKind: 'DDL_AND_DATA' },
            dataFormat: { formatKind: 'CSV' },
            outputConfig: { outputKind: 'LOCAL', filePath: '/E:/tmp/output' },
            performanceConfig: { retry: false },
            filterConfig: {},
            ddlBehavior: {},
          },
        },
        configFingerprint: 'synthetic-fingerprint',
      },
    }))

    await expect(api.getExportDraft('draft-all')).resolves.toMatchObject({
      configVersion: 'v6',
      config: {
        objectScope: { database: 'synthetic_db', scopeKind: 'ALL', objectTypes: undefined, expressions: undefined, excludeTables: undefined },
        contentSelection: { contentKind: 'DDL_AND_DATA' },
        dataFormat: { formatKind: 'CSV' },
      },
    })
  })

  it('解析 EX-I3 全量选项的标准文档', async () => {
    const { api } = apiWith(Response.json({
      item: {
        id: 'draft-exi3',
        dataSourceId: 'source-1',
        nodeId: 'node-1',
        revision: 1,
        configVersion: 'v6',
        config: {
          configVersion: 'v6',
          dataSourceId: 'source-1',
          nodeId: 'node-1',
          database: 'synthetic_db',
          scopeKind: 'SPECIFIED',
          table: 'synthetic_table',
          contentKind: 'DATA_ONLY',
          format: 'CSV',
          filePath: '/E:/tmp/output',
          logPath: '',
          skipCheckDir: false,
          config: {
            objectScope: { database: 'synthetic_db', scopeKind: 'SPECIFIED', objectTypes: ['TABLE'], expressions: [{ name: 'synthetic_table' }] },
            contentSelection: { contentKind: 'DATA_ONLY' },
            dataFormat: { formatKind: 'CSV', csvOptions: { skipHeader: true, columnSeparator: '|', columnQuoteMode: 'minimal', escapeCharacter: '\\', withTrim: true } },
            outputConfig: { outputKind: 'LOCAL', filePath: '/E:/tmp/output', noNestedDir: true, maxFileSize: 1048576, retainEmptyFiles: true, compress: true, compressionAlgo: 'zstd' },
            filterConfig: { includeColumnNames: ['col_a', 'col_b'], excludeVirtualColumns: true, flashbackScn: 100 },
            performanceConfig: { thread: 4, pageSize: 1000, jvmMemory: '4G' },
          },
        },
        configFingerprint: 'synthetic-fingerprint',
      },
    }))

    await expect(api.getExportDraft('draft-exi3')).resolves.toMatchObject({
      configVersion: 'v6',
      config: {
        dataFormat: { formatKind: 'CSV', csvOptions: { skipHeader: true, columnSeparator: '|', columnQuoteMode: 'minimal', escapeCharacter: '\\', withTrim: true } },
        outputConfig: { noNestedDir: true, maxFileSize: 1048576, retainEmptyFiles: true, compress: true, compressionAlgo: 'zstd' },
        filterConfig: { includeColumnNames: ['col_a', 'col_b'], excludeVirtualColumns: true, flashbackScn: 100 },
        performanceConfig: { thread: 4, pageSize: 1000, jvmMemory: '4G' },
      },
    })
  })

  it('解析仅 DDL 草稿时把空 formatKind 视为无数据格式', async () => {
    const { api } = apiWith(Response.json({
      item: {
        id: 'draft-ddl',
        dataSourceId: 'source-1',
        nodeId: 'node-1',
        revision: 1,
        configVersion: 'v6',
        config: {
          configVersion: 'v6',
          dataSourceId: 'source-1',
          nodeId: 'node-1',
          database: 'synthetic_db',
          scopeKind: 'SPECIFIED',
          table: 'table_one',
          contentKind: 'DDL_ONLY',
          format: 'DDL',
          filePath: '/E:/tmp/output',
          logPath: '',
          skipCheckDir: false,
          config: {
            objectScope: { database: 'synthetic_db', scopeKind: 'SPECIFIED', objectTypes: ['TABLE'], expressions: [{ name: 'table_one' }] },
            contentSelection: { contentKind: 'DDL_ONLY' },
            dataFormat: { formatKind: '' },
            outputConfig: { outputKind: 'LOCAL', filePath: '/E:/tmp/output' },
          },
        },
        configFingerprint: 'synthetic-fingerprint',
      },
    }))

    await expect(api.getExportDraft('draft-ddl')).resolves.toMatchObject({
      configVersion: 'v6',
      config: {
        objectScope: { database: 'synthetic_db', scopeKind: 'SPECIFIED', objectTypes: ['TABLE'], expressions: [{ name: 'table_one' }] },
        contentSelection: { contentKind: 'DDL_ONLY' },
        dataFormat: undefined,
      },
    })
  })

  it('命令预览使用 CSRF 与草稿版本，且允许展示非密码参数', async () => {
    const { api, calls } = apiWith(Response.json({
      command: 'obdumper -h127.0.0.1 -P2883 -usynthetic-user@synthetic-tenant#synthetic-cluster -p ****** --database synthetic_db --table synthetic_table --csv --file-path /E:/tmp/output',
      configFingerprint: 'synthetic-fingerprint',
    }))
    const draft = {
      id: 'draft-1',
      dataSourceId: 'source-1',
      nodeId: 'node-1',
      revision: 3,
      configVersion: 'v6' as const,
      config: {
        objectScope: { database: 'synthetic_db', scopeKind: 'SPECIFIED' as const, objectTypes: ['TABLE' as const], expressions: [{ name: 'synthetic_table' }] },
        contentSelection: { contentKind: 'DATA_ONLY' as const },
        dataFormat: { formatKind: 'CSV' as const },
        outputConfig: { outputKind: 'LOCAL' as const, filePath: '/E:/tmp/output' },
      },
      configFingerprint: 'synthetic-fingerprint',
    }

    await expect(api.previewExportCommand(draft)).resolves.toEqual({
      command: 'obdumper -h127.0.0.1 -P2883 -usynthetic-user@synthetic-tenant#synthetic-cluster -p ****** --database synthetic_db --table synthetic_table --csv --file-path /E:/tmp/output',
      configFingerprint: 'synthetic-fingerprint',
    })

    expect(calls[0]?.path).toBe('/api/v1/export-drafts/draft-1:preview-command')
    expect(calls[0]?.init.headers).toMatchObject({
      'X-CSRF-Token': 'synthetic-csrf-token',
      'If-Match': '"rev-3"',
    })
    expect(calls[0]?.init.headers).not.toHaveProperty('Idempotency-Key')
  })

  it('命令预览发现未隐藏密码时拒绝展示', async () => {
    const { api } = apiWith(Response.json({
      command: 'obdumper --user synthetic-user --password synthetic-password',
      configFingerprint: 'synthetic-fingerprint',
    }))
    const draft = {
      id: 'draft-1',
      dataSourceId: 'source-1',
      nodeId: 'node-1',
      revision: 3,
      configVersion: 'v6' as const,
      config: {
        objectScope: { database: 'synthetic_db', scopeKind: 'SPECIFIED' as const, objectTypes: ['TABLE' as const], expressions: [{ name: 'synthetic_table' }] },
        contentSelection: { contentKind: 'DATA_ONLY' as const },
        dataFormat: { formatKind: 'CSV' as const },
        outputConfig: { outputKind: 'LOCAL' as const, filePath: '/E:/tmp/output' },
      },
      configFingerprint: 'synthetic-fingerprint',
    }

    await expect(api.previewExportCommand(draft)).rejects.toMatchObject({ code: 'UNSAFE_COMMAND_RESPONSE' })
  })

  it('命令预览发现短密码参数的原值时拒绝展示', async () => {
    const { api } = apiWith(Response.json({
      command: 'obdumper -usynthetic-user@synthetic-tenant#synthetic-cluster -p synthetic-password',
      configFingerprint: 'synthetic-fingerprint',
    }))
    const draft = {
      id: 'draft-1',
      dataSourceId: 'source-1',
      nodeId: 'node-1',
      revision: 3,
      configVersion: 'v6' as const,
      config: {
        objectScope: { database: 'synthetic_db', scopeKind: 'SPECIFIED' as const, objectTypes: ['TABLE' as const], expressions: [{ name: 'synthetic_table' }] },
        contentSelection: { contentKind: 'DATA_ONLY' as const },
        dataFormat: { formatKind: 'CSV' as const },
        outputConfig: { outputKind: 'LOCAL' as const, filePath: '/E:/tmp/output' },
      },
      configFingerprint: 'synthetic-fingerprint',
    }

    await expect(api.previewExportCommand(draft)).rejects.toMatchObject({ code: 'UNSAFE_COMMAND_RESPONSE' })
  })

  it('连接测试候选只读取当前身份可使用的安全节点投影', async () => {
    const { api, calls } = apiWith(Response.json({
      items: [{ id: 'node-1', displayName: '合成测试节点', platform: 'WINDOWS_AMD64', agentCredential: 'must-not-be-read' }],
    }))

    await expect(api.listDataSourceConnectionTestNodeCandidates()).resolves.toEqual([
      { id: 'node-1', displayName: '合成测试节点', platform: 'WINDOWS_AMD64' },
    ])

    expect(calls[0]?.path).toBe('/api/v1/execution-nodes?eligibleFor=DATA_SOURCE_CONNECTION_TEST')
    expect(calls[0]?.init).toMatchObject({ method: 'GET' })
  })

  it('连接测试提交节点、数据源版本、CSRF 与幂等键', async () => {
    const { api, calls } = apiWith(Response.json({ item: { id: 'connection-test-1', status: 'PENDING', nodeId: 'node-1' } }))

    await expect(api.startDataSourceConnectionTest('source-1', 4, 'node-1')).resolves.toEqual({
      id: 'connection-test-1', status: 'PENDING', nodeId: 'node-1',
    })

    expect(calls[0]?.path).toBe('/api/v1/data-sources/source-1:test-connection')
    expect(calls[0]?.init).toMatchObject({ method: 'POST', body: JSON.stringify({ nodeId: 'node-1' }) })
    expect(calls[0]?.init.headers).toMatchObject({
      'X-CSRF-Token': 'synthetic-csrf-token',
      'If-Match': '"rev-4"',
      'Idempotency-Key': 'synthetic-idempotency-key',
    })
    expect(calls[0]?.init.body).not.toContain('password')
  })

  it('只读取连接测试的安全终态投影', async () => {
    const { api, calls } = apiWith(Response.json({
      item: {
        id: 'connection-test/1',
        status: 'SUCCEEDED',
        nodeId: 'node-1',
        nodeDisplayName: '合成测试节点',
        agentId: 'agent-1',
        nodeFactsRevision: 3,
        code: 'DATABASE_CONNECTED',
        completedAt: '2026-07-27T03:00:00Z',
        verificationSource: 'AGENT_JDBC',
        realConnectionVerified: true,
        sysCredentialConfigured: false,
        password: 'must-not-be-read',
        connectionString: 'must-not-be-read',
      },
    }))

    await expect(api.getDataSourceConnectionTest('connection-test/1')).resolves.toEqual({
      id: 'connection-test/1',
      status: 'SUCCEEDED',
      nodeId: 'node-1',
      nodeDisplayName: '合成测试节点',
      agentId: 'agent-1',
      factsRevision: 3,
      resultCode: 'DATABASE_CONNECTED',
      completedAt: '2026-07-27T03:00:00Z',
      verificationSource: 'AGENT_JDBC',
      realConnectionVerified: true,
      sysCredentialConfigured: false,
    })

    expect(calls[0]?.path).toBe('/api/v1/data-source-connection-tests/connection-test%2F1')
    expect(calls[0]?.init).toMatchObject({ method: 'GET' })
  })

  it('拒绝缺少终态证据或使用未知来源的连接测试响应', async () => {
    const missingEvidence = apiWith(Response.json({
      item: { id: 'connection-test-1', status: 'SUCCEEDED', nodeId: 'node-1', verificationSource: 'AGENT_JDBC' },
    })).api
    const unknownSource = apiWith(Response.json({
      item: {
        id: 'connection-test-1', status: 'FAILED', nodeId: 'node-1', resultCode: 'DATABASE_CONNECTION_FAILED',
        completedAt: '2026-07-27T03:00:00Z', verificationSource: 'UNTRUSTED_SOURCE',
        realConnectionVerified: false,
        sysCredentialConfigured: false,
      },
    })).api

    await expect(missingEvidence.getDataSourceConnectionTest('connection-test-1')).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
    await expect(unknownSource.getDataSourceConnectionTest('connection-test-1')).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
  })

  it('拒绝连接测试来源、终态与实际连接验证标记不一致的响应', async () => {
    const api = apiWith(Response.json({
      item: {
        id: 'connection-test-1', status: 'SUCCEEDED', nodeId: 'node-1', resultCode: 'SYNTHETIC_OK',
        completedAt: '2026-07-27T03:00:00Z', verificationSource: 'G2_SYNTHETIC', realConnectionVerified: true,
        sysCredentialConfigured: false,
      },
    })).api

    await expect(api.getDataSourceConnectionTest('connection-test-1')).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
  })

  it('数据源编辑使用版本条件，且空密码不会被发送', async () => {
    const { api, calls } = apiWith(Response.json({
      item: { id: 'source-1', displayName: '合成数据源', environment: 'TEST', connectionKind: 'ODP', compatibilityMode: 'MYSQL', host: '127.0.0.1', port: 2881, clusterName: 'synthetic-cluster', tenantName: 'synthetic-tenant', username: 'synthetic-user', state: 'ENABLED', revision: 2, credentialRevision: 1 },
    }))

    await api.updateDataSource('source-1', 1, { displayName: '合成数据源', environment: 'TEST', connectionKind: 'ODP', compatibilityMode: 'MYSQL', host: '127.0.0.1', port: 2881, clusterName: 'synthetic-cluster', tenantName: 'synthetic-tenant', username: 'synthetic-user' })

    expect(calls[0]?.path).toBe('/api/v1/data-sources/source-1')
    expect(calls[0]?.init.headers).toMatchObject({ 'X-CSRF-Token': 'synthetic-csrf-token', 'If-Match': '"rev-1"' })
    expect(calls[0]?.init.body).not.toContain('password')
  })

  it('保留受控字段错误供表单在对应位置展示', async () => {
    const { api } = apiWith(Response.json({
      requestId: 'synthetic-request',
      code: 'DATA_SOURCE_FIELD_INVALID',
      message: '数据源字段不符合要求。',
      retryable: false,
      fieldErrors: [{ field: 'host', code: 'HOST_INVALID', message: 'ODP 地址格式不正确。' }],
      safeDetails: {},
    }, { status: 422 }))

    await expect(api.createDataSource({
      displayName: '合成数据源', environment: 'TEST', connectionKind: 'ODP', compatibilityMode: 'MYSQL', host: 'invalid host', port: 2883, clusterName: 'synthetic-cluster', tenantName: 'synthetic-tenant', username: 'synthetic-user', password: 'synthetic-password',
    })).rejects.toMatchObject({
      code: 'DATA_SOURCE_FIELD_INVALID',
      fieldErrors: [{ field: 'host', code: 'HOST_INVALID', message: 'ODP 地址格式不正确。' }],
    })
  })

  it('数据源启停使用版本条件且不生成幂等键', async () => {
    const { api, calls } = apiWith(Response.json({ state: 'DISABLED', revision: 3 }))

    await expect(api.changeDataSourceState('source-1', 2, 'DISABLED')).resolves.toEqual({
      state: 'DISABLED',
      revision: 3,
    })

    expect(calls[0]?.path).toBe('/api/v1/data-sources/source-1:disable')
    expect(calls[0]?.init).toMatchObject({ method: 'POST' })
    expect(calls[0]?.init.headers).toMatchObject({
      'X-CSRF-Token': 'synthetic-csrf-token',
      'If-Match': '"rev-2"',
    })
    expect(calls[0]?.init.headers).not.toHaveProperty('Idempotency-Key')
  })

  it('数据源物理删除使用版本条件且返回明确结果', async () => {
    const { api, calls } = apiWith(Response.json({ outcome: 'DELETED', revision: 0 }))

    await expect(api.deleteOrArchiveDataSource('source-1', 2)).resolves.toEqual({ outcome: 'DELETED', revision: 0 })

    expect(calls[0]?.path).toBe('/api/v1/data-sources/source-1')
    expect(calls[0]?.init).toMatchObject({ method: 'DELETE' })
    expect(calls[0]?.init.headers).toMatchObject({
      'X-CSRF-Token': 'synthetic-csrf-token',
      'If-Match': '"rev-2"',
    })
    expect(calls[0]?.init.headers).not.toHaveProperty('Idempotency-Key')
  })

  it('数据源归档保留明确结果', async () => {
    const { api } = apiWith(Response.json({ outcome: 'ARCHIVED', revision: 3 }))

    await expect(api.deleteOrArchiveDataSource('source-1', 2)).resolves.toEqual({ outcome: 'ARCHIVED', revision: 3 })
  })

  it('执行节点删除使用版本条件且返回明确结果', async () => {
    const { api, calls } = apiWith(Response.json({ outcome: 'ARCHIVED', revision: 3, agentAccessRevoked: true }))

    await expect(api.deleteOrArchiveExecutionNode('node-1', 2)).resolves.toEqual({ outcome: 'ARCHIVED', revision: 3, agentAccessRevoked: true })

    expect(calls[0]?.path).toBe('/api/v1/execution-nodes/node-1')
    expect(calls[0]?.init).toMatchObject({ method: 'DELETE' })
    expect(calls[0]?.init.headers).toMatchObject({
      'X-CSRF-Token': 'synthetic-csrf-token',
      'If-Match': '"rev-2"',
    })
    expect(calls[0]?.init.headers).not.toHaveProperty('Idempotency-Key')
  })

  it('拒绝未知的执行节点删除结果', async () => {
    const { api } = apiWith(Response.json({ outcome: 'UNKNOWN', revision: 3 }))

    await expect(api.deleteOrArchiveExecutionNode('node-1', 2)).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
  })

  it('拒绝缺少 Agent 撤销事实的执行节点归档结果', async () => {
    const { api } = apiWith(Response.json({ outcome: 'ARCHIVED', revision: 3 }))

    await expect(api.deleteOrArchiveExecutionNode('node-1', 2)).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
  })

  it('拒绝未知的数据源删除结果', async () => {
    const { api } = apiWith(Response.json({ outcome: 'UNKNOWN', revision: 3 }))

    await expect(api.deleteOrArchiveDataSource('source-1', 2)).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
  })

  it('数据源启停与删除缺少 CSRF 时失败关闭', async () => {
    const { api, calls } = apiWith(Response.json({}), '')

    await expect(api.changeDataSourceState('source-1', 2, 'DISABLED')).rejects.toMatchObject({
      code: 'CSRF_TOKEN_UNAVAILABLE',
    })
    await expect(api.deleteOrArchiveDataSource('source-1', 2)).rejects.toMatchObject({
      code: 'CSRF_TOKEN_UNAVAILABLE',
    })

    expect(calls).toHaveLength(0)
  })

  it('缺少 CSRF 时失败关闭且不发送写请求', async () => {
    const { api, calls } = apiWith(Response.json({ id: 'draft-1' }), '')

    await expect(api.createExportDraft({
      configVersion: 'v6',
      dataSourceId: 'source-1', nodeId: 'node-1',
      config: {
        objectScope: { database: 'synthetic_db', scopeKind: 'SPECIFIED', objectTypes: ['TABLE'], expressions: [{ name: 'synthetic_table' }] },
        contentSelection: { contentKind: 'DATA_ONLY' },
        dataFormat: { formatKind: 'CSV' },
        outputConfig: { outputKind: 'LOCAL', filePath: '/E:/tmp/output' },
      },
    })).rejects.toMatchObject({ code: 'CSRF_TOKEN_UNAVAILABLE' })

    expect(calls).toHaveLength(0)
  })

  it('为数据源页面提供安全且可操作的错误反馈', () => {
    expect(dataSourceErrorMessage({ status: 401, message: 'unsafe' }, '请求失败。')).toContain('安全校验')
    expect(dataSourceErrorMessage({ code: 'DATA_SOURCE_NAME_UNAVAILABLE', message: 'unsafe' }, '请求失败。')).toContain('名称不可用')
    expect(dataSourceErrorMessage({ status: 404, message: 'unsafe' }, '请求失败。')).toContain('无权访问')
    expect(dataSourceErrorMessage({ status: 409, conflict: true }, '请求失败。')).toContain('刷新')
    expect(dataSourceErrorMessage({ status: 412, conflict: true }, '请求失败。')).toContain('刷新')
    expect(dataSourceErrorMessage({ status: 0, code: 'NETWORK_UNAVAILABLE' }, '请求失败。')).toContain('控制面')
    expect(dataSourceErrorMessage({ status: 0, code: 'RESPONSE_INVALID', message: '返回字段不符合契约。' }, '请求失败。')).toContain('返回字段')
    expect(exportDraftErrorMessage({ status: 404, message: 'unsafe' }, '请求失败。')).toContain('执行节点')
    expect(exportDraftErrorMessage({ code: 'CSRF_TOKEN_UNAVAILABLE' }, '请求失败。')).toContain('拒绝创建草稿')
    expect(exportDraftErrorMessage({ status: 0, code: 'RESPONSE_INVALID', message: '返回字段不符合契约。' }, '请求失败。')).toContain('返回字段')
    expect(taskDetailErrorMessage({ status: 404, message: 'unsafe' }, '请求失败。')).toContain('任务不存在')
    expect(executionNodeErrorMessage({ code: 'EXECUTION_NODE_NAME_UNAVAILABLE', message: 'unsafe' }, '请求失败。')).toContain('名称不可用')
    expect(executionNodeErrorMessage({ status: 404, message: 'unsafe' }, '请求失败。')).toContain('无权访问')
  })

  it('v6 对象存储、CUT 与 POS 草稿 get → update 往返不丢失已支持字段', async () => {
    const cases = [
      {
        name: 'OSS',
        item: {
          id: 'draft-oss', dataSourceId: 'source-1', nodeId: 'node-1', revision: 2, configVersion: 'v6',
          config: {
            configVersion: 'v6', dataSourceId: 'source-1', nodeId: 'node-1', database: 'synthetic_db', scopeKind: 'SPECIFIED', table: 'synthetic_table', contentKind: 'DATA_ONLY', format: 'CSV',
            filePath: 'oss://synthetic-bucket/exports?endpoint=oss-cn-hangzhou.aliyuncs.com', logPath: '', skipCheckDir: false,
            config: {
              objectScope: { database: 'synthetic_db', scopeKind: 'SPECIFIED', objectTypes: ['TABLE'], expressions: [{ name: 'synthetic_table' }] },
              contentSelection: { contentKind: 'DATA_ONLY' },
              dataFormat: { formatKind: 'CSV' },
              outputConfig: { outputKind: 'OSS', filePath: 'oss://synthetic-bucket/exports?endpoint=oss-cn-hangzhou.aliyuncs.com', tmpPath: '/E:/tmp/staging' },
            },
          },
          configFingerprint: 'synthetic-fingerprint',
        },
      },
      {
        name: 'CUT',
        item: {
          id: 'draft-cut', dataSourceId: 'source-1', nodeId: 'node-1', revision: 2, configVersion: 'v6',
          config: {
            configVersion: 'v6', dataSourceId: 'source-1', nodeId: 'node-1', database: 'synthetic_db', scopeKind: 'SPECIFIED', table: 'synthetic_table', contentKind: 'DATA_ONLY', format: 'CUT',
            filePath: '/E:/tmp/output', logPath: '', skipCheckDir: false,
            config: {
              objectScope: { database: 'synthetic_db', scopeKind: 'SPECIFIED', objectTypes: ['TABLE'], expressions: [{ name: 'synthetic_table' }] },
              contentSelection: { contentKind: 'DATA_ONLY' },
              dataFormat: { formatKind: 'CUT', csvOptions: { columnSplitter: '|', lineSeparator: '\\n', fileEncoding: 'UTF-8' }, cutOptions: { trailDelimiter: true } },
              outputConfig: { outputKind: 'LOCAL', filePath: '/E:/tmp/output' },
            },
          },
          configFingerprint: 'synthetic-fingerprint',
        },
      },
      {
        name: 'POS',
        item: {
          id: 'draft-pos', dataSourceId: 'source-1', nodeId: 'node-1', revision: 2, configVersion: 'v6',
          config: {
            configVersion: 'v6', dataSourceId: 'source-1', nodeId: 'node-1', database: 'synthetic_db', scopeKind: 'SPECIFIED', table: 'synthetic_table', contentKind: 'DATA_ONLY', format: 'POS',
            filePath: '/E:/tmp/output', logPath: '', skipCheckDir: false,
            config: {
              objectScope: { database: 'synthetic_db', scopeKind: 'SPECIFIED', objectTypes: ['TABLE'], expressions: [{ name: 'synthetic_table' }] },
              contentSelection: { contentKind: 'DATA_ONLY' },
              dataFormat: { formatKind: 'POS' },
              outputConfig: { outputKind: 'LOCAL', filePath: '/E:/tmp/output', controlFilePath: '/E:/tmp/controls' },
            },
          },
          configFingerprint: 'synthetic-fingerprint',
        },
      },
    ]
    for (const testCase of cases) {
      // 每次请求都返回新的 Response：同一 Response 的 body 只能消费一次。
      const fetcher = (async () => Response.json({ item: testCase.item })) as FetchLike
      const api = createBrowserApi({ fetcher, csrfToken: () => 'synthetic-csrf-token', idempotencyKey: () => 'synthetic-idempotency-key' })
      const draft = await api.getExportDraft(testCase.item.id)
      const updated = await api.updateExportDraft(draft)
      expect(updated.config.outputConfig).toEqual(draft.config.outputConfig)
      expect(updated.config.dataFormat).toEqual(draft.config.dataFormat)
      expect(updated.config.objectScope).toEqual(draft.config.objectScope)
    }
  })

  it('任务快照接受 POS/PARQUET/ORC/AVRO 格式并拒绝未知格式', async () => {
    for (const format of ['POS', 'PARQUET', 'ORC', 'AVRO'] as const) {
      const { api } = apiWith(Response.json({
        item: {
          type: 'OBDUMPER_EXPORT', snapshotVersion: 'v2', dataSourceId: 'source-1', nodeId: 'node-1', precheckId: 'precheck-1', format,
          configFingerprint: 'synthetic-fingerprint', toolVersion: '4.3.5-RELEASE', metadataVersion: 'metadata-v1', capabilityVersion: 'capability-v1',
        },
      }))
      await expect(api.getTaskSnapshot('task-1')).resolves.toMatchObject({ format })
    }
    const unknown = apiWith(Response.json({
      item: {
        type: 'OBDUMPER_EXPORT', snapshotVersion: 'v2', dataSourceId: 'source-1', nodeId: 'node-1', precheckId: 'precheck-1', format: 'MIX',
        configFingerprint: 'synthetic-fingerprint', toolVersion: '4.3.5-RELEASE', metadataVersion: 'metadata-v1', capabilityVersion: 'capability-v1',
      },
    }))
    await expect(unknown.api.getTaskSnapshot('task-1')).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
  })

  it('存储凭据列表按白名单解析且绝不回显密钥字段', async () => {
    const { api } = apiWith(Response.json({
      items: [{
        id: 'storage-1', displayName: '合成 OSS 凭据', provider: 'OSS', currentRevision: 2, revision: 2,
        updatedAt: '2026-08-14T00:00:00Z', accessKey: 'must-not-be-read', secretKey: 'must-not-be-read', ciphertext: 'must-not-be-read',
      }],
    }))
    const [credential] = await api.listStorageCredentials()
    expect(credential).toEqual({ id: 'storage-1', displayName: '合成 OSS 凭据', provider: 'OSS', currentRevision: 2, revision: 2, updatedAt: '2026-08-14T00:00:00Z' })
    expect(credential).not.toHaveProperty('accessKey')
    expect(credential).not.toHaveProperty('secretKey')
    expect(credential).not.toHaveProperty('ciphertext')
    const invalid = apiWith(Response.json({ items: [{ id: 'storage-2', displayName: '未知提供方', provider: 'FTP', currentRevision: 1, revision: 1, updatedAt: '2026-08-14T00:00:00Z' }] }))
    await expect(invalid.api.listStorageCredentials()).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
  })

  it('创建/轮换/删除存储凭据使用 CSRF、幂等键与 If-Match', async () => {
    const calls: Array<{ path: string; init: RequestInit }> = []
    // 每次调用都返回新的 Response：同一 Response 的 body 只能消费一次。
    const fetcher = (async (path: string | URL | Request, init?: RequestInit) => {
      calls.push({ path: String(path), init: init ?? {} })
      return Response.json({ item: { id: 'storage-1', displayName: '合成 OSS 凭据', provider: 'OSS', currentRevision: 1, revision: 1, updatedAt: '2026-08-14T00:00:00Z' } })
    }) as FetchLike
    const api = createBrowserApi({ fetcher, csrfToken: () => 'synthetic-csrf-token', idempotencyKey: () => 'synthetic-idempotency-key' })
    await api.createStorageCredential({ displayName: '合成 OSS 凭据', provider: 'OSS', accessKey: 'access', secretKey: 'secret' })
    expect(calls[0]?.path).toBe('/api/v1/storage-credentials')
    expect(calls[0]?.init.headers).toMatchObject({ 'X-CSRF-Token': 'synthetic-csrf-token', 'Idempotency-Key': 'synthetic-idempotency-key' })
    // 轮换同时携带 If-Match 与幂等键。
    await api.rotateStorageCredential('storage-1', 2, { displayName: '合成 OSS 凭据', provider: 'OSS', accessKey: 'rotated', secretKey: 'rotated' })
    expect(calls[1]?.path).toBe('/api/v1/storage-credentials/storage-1:rotate')
    expect(calls[1]?.init.headers).toMatchObject({ 'If-Match': '"rev-2"', 'Idempotency-Key': 'synthetic-idempotency-key' })
    // 删除携带 If-Match 但不携带幂等键（与数据源删除同口径）。
    await api.deleteStorageCredential('storage-1', 3)
    expect(calls[2]?.path).toBe('/api/v1/storage-credentials/storage-1')
    expect(calls[2]?.init.method).toBe('DELETE')
    expect(calls[2]?.init.headers).toMatchObject({ 'If-Match': '"rev-3"' })
  })

  it('草稿输出配置往返保留存储凭据引用', async () => {
    const { api } = apiWith(Response.json({
      item: {
        id: 'draft-storage', dataSourceId: 'source-1', nodeId: 'node-1', revision: 1, configVersion: 'v6',
        config: {
          config: {
            objectScope: { database: 'synthetic_db', scopeKind: 'SPECIFIED', objectTypes: ['TABLE'], expressions: [{ name: 'synthetic_table' }] },
            contentSelection: { contentKind: 'DATA_ONLY' },
            dataFormat: { formatKind: 'CSV' },
            outputConfig: { outputKind: 'OSS', filePath: 'oss://bucket/path?region=cn-hangzhou', storageCredential: { storageCredentialId: 'storage-1', revision: 2 } },
          },
        },
        configFingerprint: 'synthetic-fingerprint',
      },
    }))
    const draft = await api.getExportDraft('draft-storage')
    expect(draft.config.outputConfig.storageCredential).toEqual({ storageCredentialId: 'storage-1', revision: 2 })
  })

  it('派生操作使用 CSRF 与幂等键并校验响应一致性', async () => {
    const calls: Array<{ path: string; init: RequestInit }> = []
    const fetcher = (async (path: string | URL | Request, init?: RequestInit) => {
      calls.push({ path: String(path), init: init ?? {} })
      if (String(path).includes(':rebuild-draft')) {
        return Response.json({ draftId: 'draft-derived', sourceTaskId: 'task-1', derivation: 'REBUILD_FROM_CONFIG' })
      }
      return Response.json({ id: 'task-derived', parentTaskId: 'task-1', derivationKind: 'CHECKPOINT_RESUME' })
    }) as FetchLike
    const api = createBrowserApi({ fetcher, csrfToken: () => 'synthetic-csrf-token', idempotencyKey: () => 'synthetic-idempotency-key' })
    await expect(api.rebuildTaskDraft('task-1', 'REBUILD_FROM_CONFIG')).resolves.toEqual({ draftId: 'draft-derived', sourceTaskId: 'task-1', derivation: 'REBUILD_FROM_CONFIG' })
    expect(calls[0]?.path).toBe('/api/v1/tasks/task-1:rebuild-draft')
    expect(calls[0]?.init.headers).toMatchObject({ 'X-CSRF-Token': 'synthetic-csrf-token', 'Idempotency-Key': 'synthetic-idempotency-key' })
    await expect(api.resumeTaskFromCheckpoint('task-1')).resolves.toEqual({ id: 'task-derived', parentTaskId: 'task-1', derivationKind: 'CHECKPOINT_RESUME' })
    expect(calls[1]?.path).toBe('/api/v1/tasks/task-1:resume-checkpoint')
    expect(calls[1]?.init.headers).toMatchObject({ 'X-CSRF-Token': 'synthetic-csrf-token', 'Idempotency-Key': 'synthetic-idempotency-key' })
  })

  it('任务概览按白名单解析派生关系', async () => {
    const { api } = apiWith(Response.json({
      item: {
        id: 'task-derived', type: 'OBDUMPER_EXPORT', dataSourceId: 'source-1', nodeId: 'node-1',
        precheckId: 'precheck-1', submittedAt: '2026-08-14T08:00:00Z',
        parentTaskId: 'task-1', derivationKind: 'CHECKPOINT_RESUME', password: 'must-not-be-read',
      },
    }))
    await expect(api.getTaskOverview('task-derived')).resolves.toMatchObject({ id: 'task-derived', parentTaskId: 'task-1', derivationKind: 'CHECKPOINT_RESUME' })
    const inconsistent = apiWith(Response.json({ item: { id: 'task-bad', type: 'OBDUMPER_EXPORT', dataSourceId: 'source-1', nodeId: 'node-1', precheckId: 'p', submittedAt: '2026-08-14T08:00:00Z', derivationKind: 'CHECKPOINT_RESUME' } }))
    await expect(inconsistent.api.getTaskOverview('task-bad')).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
  })
})
