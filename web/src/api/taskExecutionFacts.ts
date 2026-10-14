// 任务执行的增量安全事实在传输层严格裁剪；不接受原始事件或命令载荷。
export function parseTaskExecutionFacts(value: Record<string, unknown>) {
  const facts: {
    cancelAvailable?: boolean; cancelRequested?: boolean; cancelDeadline?: string; exitCode?: number; elapsedMs?: number; errorCode?: string; errorSummary?: string;
    plannedActualMatch?: 'MATCHED' | 'MISMATCHED' | 'UNAVAILABLE';
    outputLocation?: { kind: 'LOCAL' | 'OSS' | 'S3' | 'COS' | 'OBS' | 'UNAVAILABLE'; value?: string; redaction: 'FULL' | 'KIND_ONLY' | 'UNAVAILABLE' }
  } = {}
  const fail = () => { throw new Error('任务执行事实不符合响应契约。') }
  for (const key of ['cancelAvailable', 'cancelRequested'] as const) {
    if (value[key] !== undefined) { if (typeof value[key] !== 'boolean') fail(); facts[key] = value[key] as boolean }
  }
  for (const key of ['cancelDeadline', 'errorCode', 'errorSummary'] as const) {
    if (value[key] !== undefined) { if (typeof value[key] !== 'string') fail(); facts[key] = value[key] as string }
  }
  for (const key of ['exitCode', 'elapsedMs'] as const) {
    if (value[key] !== undefined) { if (!Number.isSafeInteger(value[key]) || key === 'elapsedMs' && (value[key] as number) < 0) fail(); facts[key] = value[key] as number }
  }
  if (value['plannedActualMatch'] !== undefined) {
    if (!['MATCHED', 'MISMATCHED', 'UNAVAILABLE'].includes(String(value['plannedActualMatch']))) fail()
    facts.plannedActualMatch = value['plannedActualMatch'] as NonNullable<typeof facts.plannedActualMatch>
  }
  if (value['outputLocation'] !== undefined) {
    const location = value['outputLocation']
    if (!location || typeof location !== 'object' || Array.isArray(location)) fail()
    const raw = location as Record<string, unknown>
    if (!['LOCAL', 'OSS', 'S3', 'COS', 'OBS', 'UNAVAILABLE'].includes(String(raw['kind'])) || !['FULL', 'KIND_ONLY', 'UNAVAILABLE'].includes(String(raw['redaction']))) fail()
    if ((raw['kind'] === 'UNAVAILABLE') !== (raw['redaction'] === 'UNAVAILABLE')) fail()
    if (raw['value'] !== undefined && (typeof raw['value'] !== 'string' || raw['redaction'] !== 'FULL')) fail()
    facts.outputLocation = { kind: raw['kind'] as NonNullable<typeof facts.outputLocation>['kind'], redaction: raw['redaction'] as NonNullable<typeof facts.outputLocation>['redaction'], ...(raw['value'] !== undefined ? { value: raw['value'] as string } : {}) }
  }
  return facts
}
