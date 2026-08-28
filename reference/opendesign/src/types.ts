export type Tone = 'neutral' | 'info' | 'success' | 'warning' | 'danger'

export interface SummaryMetric {
  key: string
  label: string
  value: number | string
  note: string
  progress: number
  tone: Tone
}

export type ConnectionStatus =
  | 'SUCCEEDED'
  | 'FAILED'
  | 'INVALIDATED'
  | 'EXPIRED'
  | 'UNTESTED'

export interface DataSource {
  id: string
  name: string
  environment: 'DEVELOPMENT' | 'TEST' | 'STAGING' | 'PRODUCTION'
  mode: 'MYSQL' | 'ORACLE'
  host: string
  port: number
  cluster: string
  tenant: string
  database: string
  credential: 'AVAILABLE' | 'UNAVAILABLE'
  state: 'ENABLED' | 'DISABLED'
  testStatus: ConnectionStatus
  testedAt: string
}

export type TaskStatus = 'running' | 'success' | 'failed' | 'waiting' | 'cancelled'

export interface Task {
  id: string
  title: string
  status: string
  statusKey: TaskStatus
  tone: Tone
  source: string
  node: string
  updatedAt: string
  evidence: string
  type: 'DATA_EXPORT' | 'DDL_EXPORT'
  typeLabel: string
  format: 'CSV' | 'DDL' | 'CUT'
  stage: string
  progress: number
}

export interface NodeEvent {
  time: string
  level: 'INFO' | 'WARN' | 'ERROR'
  source: string
  title: string
  detail: string
}

export interface ExecutionNode {
  id: string
  displayName: string
  description: string
  platform: string
  managementState: 'ENABLED' | 'MAINTENANCE' | 'DISABLED'
  runtime: 'online' | 'stale'
  environment: 'normal' | 'expired'
  schedule: 'schedulable' | 'blocked'
  tool: string
  java: string
  capacity: number
  activeTasks: number
  lastHeartbeat: string
  outputRoot: string
  logRoot: string
  reason: string
  events: NodeEvent[]
}

export interface LogEntry extends NodeEvent {
  id: string
  node: string
}

export interface WizardDraft {
  sourceId: string
  content: 'STRUCTURE_AND_DATA' | 'DATA_ONLY' | 'STRUCTURE_ONLY'
  scope: 'SELECTED' | 'DATABASE'
  objectNames: string
  format: 'CSV' | 'DDL' | 'CUT'
  outputPath: string
  logPath: string
  skipCheckDir: boolean
  confirmed: boolean
}

export type ViewKey =
  | 'dashboard'
  | 'sources'
  | 'wizard'
  | 'tasks'
  | 'task'
  | 'nodes'
  | 'node'
  | 'logs'
