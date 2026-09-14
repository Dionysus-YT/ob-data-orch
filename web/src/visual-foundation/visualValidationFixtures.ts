import type { ExecutionNodeSummary, TaskCommandEvidence, TaskExecution, TaskLog, TaskOverview, TaskSnapshot } from '@/api/browser'

export const VISUAL_VALIDATION_MARKER = 'VISUAL VALIDATION ONLY · NON-AUTHORITATIVE BUSINESS DATA · SCHEMA-BOUND'

export interface VisualOverviewFixture {
  readonly id: string
  readonly state: 'RUNNING' | 'WAITING_SCHEDULE' | 'FAILED'
  readonly category: 'RUNNING' | 'PENDING' | 'NEEDS_ATTENTION' | 'RECENT_ACTIVITY'
  readonly occurredAt: string
}

export const VISUAL_OVERVIEW_FIXTURES: readonly VisualOverviewFixture[] = [
  { id: 'visual-validation-task-running', state: 'RUNNING', category: 'RUNNING', occurredAt: '2026-09-02T10:20:00.000Z' },
  { id: 'visual-validation-task-pending', state: 'WAITING_SCHEDULE', category: 'PENDING', occurredAt: '2026-09-02T10:18:00.000Z' },
  { id: 'visual-validation-task-failed', state: 'FAILED', category: 'NEEDS_ATTENTION', occurredAt: '2026-09-02T10:15:00.000Z' },
  { id: 'visual-validation-activity-failed', state: 'FAILED', category: 'RECENT_ACTIVITY', occurredAt: '2026-09-02T10:15:00.000Z' },
]

export interface VisualRuntimeFixture {
  readonly node: ExecutionNodeSummary
  readonly selection: 'SELECTED' | 'AVAILABLE' | 'BLOCKED'
  readonly toolVersion: '4.3.5-RELEASE'
  readonly versionCompatibility: 'COMPATIBLE' | 'INCOMPATIBLE'
}

export const VISUAL_RUNTIME_FIXTURES: readonly VisualRuntimeFixture[] = [
  {
    node: {
      id: 'visual-validation-runtime-selected', displayName: 'Visual validation runtime · selected', platform: 'WINDOWS_AMD64', managementState: 'ENABLED', agentAssociationStatus: 'ASSOCIATED', heartbeatStatus: 'ONLINE', lastHeartbeatAt: '2026-09-02T10:20:00.000Z', environmentStatus: 'NORMAL', capacityStatus: 'AVAILABLE', acceptsNewTasks: true, unavailableReasons: [], revision: 1, updatedAt: '2026-09-02T10:20:00.000Z',
      agentFacts: { os: 'Windows', arch: 'amd64', agentVersion: 'visual-validation-agent', bootId: 'visual-validation-boot', observedAt: '2026-09-02T10:20:00.000Z', capacityTotal: 1_073_741_824_000, capacityUsed: 322_122_547_200 },
    },
    selection: 'SELECTED', toolVersion: '4.3.5-RELEASE', versionCompatibility: 'COMPATIBLE',
  },
  {
    node: {
      id: 'visual-validation-runtime-blocked', displayName: 'Visual validation runtime · blocked', platform: 'LINUX_AMD64', managementState: 'ENABLED', agentAssociationStatus: 'ASSOCIATED', heartbeatStatus: 'OFFLINE', lastHeartbeatAt: '2026-09-02T10:12:00.000Z', environmentStatus: 'ABNORMAL', capacityStatus: 'BUSY', acceptsNewTasks: false, unavailableReasons: [], revision: 1, updatedAt: '2026-09-02T10:20:00.000Z',
      agentFacts: { os: 'Linux', arch: 'amd64', agentVersion: 'visual-validation-agent', bootId: 'visual-validation-boot', observedAt: '2026-09-02T10:12:00.000Z', capacityTotal: 1_073_741_824_000, capacityUsed: 1_073_741_824_000 },
    },
    selection: 'BLOCKED', toolVersion: '4.3.5-RELEASE', versionCompatibility: 'INCOMPATIBLE',
  },
]

export interface VisualTaskFixture {
  readonly overview: TaskOverview
  readonly execution: TaskExecution
  readonly snapshot: TaskSnapshot
  readonly command: TaskCommandEvidence
  readonly logs: readonly TaskLog[]
  readonly failureCode: 'EXECUTION_EVIDENCE_UNAVAILABLE'
  readonly failureSummary: '执行证据不完整，需要人工核对。'
  readonly operations: readonly { readonly state: 'WAITING_SCHEDULE' | 'STARTING' | 'FAILED'; readonly occurredAt: string }[]
}

export const VISUAL_TASK_FIXTURE: VisualTaskFixture = {
  overview: { id: 'visual-validation-task-failed', type: 'OBDUMPER_EXPORT', dataSourceId: 'ui-fixture-data-mart-sales', nodeId: 'visual-validation-runtime-selected', precheckId: 'visual-validation-precheck', submittedAt: '2026-09-02T10:10:00.000Z' },
  execution: { state: 'FAILED', executionId: 'visual-validation-execution-failed', reconciliationRequired: true, stageEvidence: 'UNAVAILABLE', progressEvidence: 'UNAVAILABLE', startedAt: '2026-09-02T10:12:00.000Z', finishedAt: '2026-09-02T10:13:00.000Z', updatedAt: '2026-09-02T10:13:00.000Z', resultSummary: { result: 'FAILED', fileCount: 0, totalBytes: 0, files: [], checkpointPresent: false, observedAt: '2026-09-02T10:13:00.000Z' } },
  snapshot: { type: 'OBDUMPER_EXPORT', snapshotVersion: 'v2', dataSourceId: 'ui-fixture-data-mart-sales', nodeId: 'visual-validation-runtime-selected', precheckId: 'visual-validation-precheck', objectSummary: 'visual_validation_schema.visual_validation_table', format: 'CSV', configFingerprint: 'visual-validation-config-fingerprint', toolVersion: '4.3.5-RELEASE', metadataVersion: 'visual-validation-metadata', capabilityVersion: 'visual-validation-capability' },
  command: { kind: 'PLANNED', command: 'obdumper --csv --host 192.0.2.18 --port 2883 --user visual_validation_user -p ******', redaction: 'PASSWORD_ONLY' },
  logs: [
    { sourceSeq: 1, kind: 'LOG', message: '[ERROR] OBDUMPER execution evidence is unavailable.', integrityCode: 'NONE', receivedAt: '2026-09-02T10:13:00.000Z' },
    { sourceSeq: 2, kind: 'GAP', message: '日志缺口：执行证据不完整，需要人工核对。', integrityCode: 'LOCAL_SPOOL_LIMIT', receivedAt: '2026-09-02T10:13:01.000Z' },
  ],
  failureCode: 'EXECUTION_EVIDENCE_UNAVAILABLE', failureSummary: '执行证据不完整，需要人工核对。',
  operations: [
    { state: 'WAITING_SCHEDULE', occurredAt: '2026-09-02T10:10:00.000Z' },
    { state: 'STARTING', occurredAt: '2026-09-02T10:12:00.000Z' },
    { state: 'FAILED', occurredAt: '2026-09-02T10:13:00.000Z' },
  ],
}
