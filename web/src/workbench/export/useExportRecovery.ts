import { type ExportContentKind, type ExportScopeKind, type ExportObjectType } from '@/api/browser'
import { watch, nextTick, onScopeDispose } from 'vue'
import { objectCategories } from './exportPresentation'
import type { ExportForm } from './exportForm'
import type { ExportReferences } from './useExportReferences'
import type { ExportRuntime } from './exportRuntime'
import type { ExportCatalog } from './useExportCatalog'
import type { ExportLifecycle } from './useExportDraftLifecycle'

// useExportRecovery 管理当前历史项内受会话约束的步骤恢复。
export function useExportRecovery(deps: Pick<ExportForm, 'selectedDataSourceID' | 'selectedNodeID' | 'database' | 'contentKind' | 'scopeKind' | 'objectType' | 'selectedObjectsByType' | 'querySql' | 'queryResultLimit' | 'hydratingDerivedDraft'> & Pick<ExportReferences, 'selectedSourceRevision' | 'selectedSource' | 'eligibleSources' | 'nodes'> & Pick<ExportRuntime, 'route' | 'activeStep'> & Pick<ExportCatalog, 'scheduleCatalogLoad'> & Pick<ExportLifecycle, 'currentDraft'>) {
  const {
    selectedDataSourceID, selectedNodeID, database, contentKind, scopeKind, objectType,
    selectedObjectsByType, querySql, queryResultLimit, hydratingDerivedDraft, selectedSourceRevision,
    selectedSource, eligibleSources, nodes, route, activeStep, scheduleCatalogLoad,
  } = deps
  let disposed = false
  onScopeDispose(() => { disposed = true })
  const step2RecoveryKey = 'obDataOrchExportStep2Recovery'
  const savedDraftRecoveryKey = 'obDataOrchExportSavedDraftRecovery'

  type Step2Recovery = {
    version: 1
    sessionFingerprint: string
    sourceId: string
    sourceRevision: number
    nodeId: string
    database: string
    contentKind: ExportContentKind
    scopeKind: ExportScopeKind
    querySql?: string
    queryResultLimit?: string
    objectType: ExportObjectType
    selectedObjects: Record<ExportObjectType, string[]>
  }

  let recoverySessionFingerprint = ''

  let recoveryReady = false

  watch([selectedDataSourceID, selectedSourceRevision, selectedNodeID, database, contentKind, scopeKind, objectType, selectedObjectsByType, querySql, queryResultLimit], persistStep2Recovery, { deep: true, flush: 'post' })
  watch(deps.currentDraft, persistSavedDraft, { flush: 'post' })

  // 历史项只保留草稿标识和会话指纹；配置、权限与修订必须重新从服务端读取。
  function persistSavedDraft() {
    if (disposed || !recoveryReady || !recoverySessionFingerprint || route.query.draft) return
    const state = recoveryHistoryState()
    if (deps.currentDraft.value) state[savedDraftRecoveryKey] = { version: 1, sessionFingerprint: recoverySessionFingerprint, draftId: deps.currentDraft.value.id }
    else delete state[savedDraftRecoveryKey]
    try { window.history.replaceState(state, '') } catch { /* 历史项不可写时不影响当前表单和服务端保存。 */ }
  }

  function savedDraftID(fingerprint: string): string {
    const saved: unknown = recoveryHistoryState()[savedDraftRecoveryKey]
    if (!saved || typeof saved !== 'object' || !fingerprint) return ''
    const reference = saved as { version?: unknown; sessionFingerprint?: unknown; draftId?: unknown }
    if (reference.version !== 1 || reference.sessionFingerprint !== fingerprint || typeof reference.draftId !== 'string' || reference.draftId.length > 256) return ''
    return reference.draftId
  }

  // 只保存会话指纹，不把 CSRF 原值写入浏览器历史；换登录会话后拒绝回填旧选择。
  async function currentSessionFingerprint(): Promise<string> {
    const token = document.querySelector('meta[name="ob-data-orch-csrf-token"]')?.getAttribute('content')?.trim()
    if (!token || !globalThis.crypto?.subtle) return ''
    try {
      const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(token))
      return [...new Uint8Array(digest)].map((part) => part.toString(16).padStart(2, '0')).join('')
    } catch {
      return ''
    }
  }

  function recoveryHistoryState(): Record<string, unknown> {
    const state: unknown = window.history.state
    return state && typeof state === 'object' ? { ...state } : {}
  }

  function persistStep2Recovery() {
    if (disposed || !recoveryReady || !recoverySessionFingerprint || route.query.draft) return
    const state = recoveryHistoryState()
    const source = selectedSource.value
    if (!source) delete state[step2RecoveryKey]
    else {
      state[step2RecoveryKey] = {
        version: 1,
        sessionFingerprint: recoverySessionFingerprint,
        sourceId: source.id,
        sourceRevision: source.revision,
        nodeId: selectedNodeID.value,
        database: database.value,
        contentKind: contentKind.value,
        scopeKind: scopeKind.value,
        querySql: scopeKind.value === 'QUERY_RESULT' ? querySql.value : undefined,
        queryResultLimit: scopeKind.value === 'QUERY_RESULT' ? queryResultLimit.value : undefined,
        objectType: objectType.value,
        selectedObjects: {
          TABLE: [...selectedObjectsByType.TABLE],
          VIEW: [...selectedObjectsByType.VIEW],
          FUNCTION: [...selectedObjectsByType.FUNCTION],
          PROCEDURE: [...selectedObjectsByType.PROCEDURE],
          SEQUENCE: [...selectedObjectsByType.SEQUENCE],
        },
      } satisfies Step2Recovery
    }
    try { window.history.replaceState(state, '') } catch { /* 浏览器拒绝保存大型选择时，当前表单仍可继续使用。 */ }
  }

  // 仅在当前登录会话、数据源修订和可用节点仍匹配时恢复数据库及对象选择。
  async function restoreStep2Recovery() {
    const recovery: unknown = recoveryHistoryState()[step2RecoveryKey]
    if (!recovery || typeof recovery !== 'object') return
    const saved = recovery as Partial<Step2Recovery>
    if (saved.version !== 1 || saved.sessionFingerprint !== recoverySessionFingerprint || typeof saved.sourceId !== 'string') {
      const state = recoveryHistoryState()
      delete state[step2RecoveryKey]
      window.history.replaceState(state, '')
      return
    }
    const source = eligibleSources.value.find((item) => item.id === saved.sourceId)
    if (!source) return
    const node = nodes.value.find((item) => item.id === saved.nodeId)
    hydratingDerivedDraft.value = true
    try {
      selectedDataSourceID.value = source.id
      selectedNodeID.value = node?.id ?? ''
      if (node && source.revision === saved.sourceRevision && typeof saved.database === 'string' && saved.database.length <= 256) {
        database.value = saved.database
        contentKind.value = saved.contentKind === 'DDL_ONLY' || saved.contentKind === 'DDL_AND_DATA' ? saved.contentKind : 'DATA_ONLY'
        scopeKind.value = saved.scopeKind === 'ALL' || saved.scopeKind === 'QUERY_RESULT' ? saved.scopeKind : 'SPECIFIED'
        querySql.value = scopeKind.value === 'QUERY_RESULT' && typeof saved.querySql === 'string' && new TextEncoder().encode(saved.querySql).length <= 60 * 1024 ? saved.querySql : ''
        queryResultLimit.value = scopeKind.value === 'QUERY_RESULT' && typeof saved.queryResultLimit === 'string' && saved.queryResultLimit.length <= 10 ? saved.queryResultLimit : ''
        objectType.value = objectCategories.some((category) => category.type === saved.objectType) ? saved.objectType! : 'TABLE'
        for (const category of objectCategories) {
          const names = saved.selectedObjects?.[category.type]
          selectedObjectsByType[category.type] = Array.isArray(names) ? [...new Set(names.filter((name): name is string => typeof name === 'string' && name.length > 0 && name.length <= 256))] : []
        }
      }
      await nextTick()
    } finally {
      hydratingDerivedDraft.value = false
    }
    if (activeStep.value === 2) scheduleCatalogLoad()
  }

  async function restoreAfterReferences(fingerprint: string) {
    if (disposed) return ''
    recoverySessionFingerprint = fingerprint
    const draftID = savedDraftID(fingerprint)
    if (!draftID && fingerprint && activeStep.value >= 2 && activeStep.value <= 4) await restoreStep2Recovery()
    recoveryReady = true
    if (!draftID) persistStep2Recovery()
    return draftID
  }

  return { restoreAfterReferences, currentSessionFingerprint, persistStep2Recovery }
}

export type ExportRecovery = ReturnType<typeof useExportRecovery>
