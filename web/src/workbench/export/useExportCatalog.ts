import { ref, reactive, computed, watch, onScopeDispose } from 'vue'
import { type ExportObjectType, exportCatalogErrorMessage, type ExportObjectCatalogQuery } from '@/api/browser'
import { objectCategories } from './exportPresentation'
import type { ExportForm } from './exportForm'
import type { ExportReferences } from './useExportReferences'
import type { ExportRuntime } from './exportRuntime'

// useExportCatalog 管理数据库及对象目录缓存、查询和过期响应。
export function useExportCatalog(deps: Pick<ExportForm, 'objectType' | 'applicableSelectedObjects' | 'contentKind' | 'selectedObjectsByType' | 'database' | 'hydratingDerivedDraft' | 'objectNames' | 'selectedNodeID' | 'scopeKind' | 'selectedDataSourceID' | 'clearSelectedObjects'> & Pick<ExportReferences, 'selectedSource' | 'selectedSourceRevision'> & Pick<ExportRuntime, 'activeStep' | 'api'>) {
  const {
    objectType, applicableSelectedObjects, contentKind, selectedObjectsByType, database,
    hydratingDerivedDraft, objectNames, selectedNodeID, scopeKind, selectedDataSourceID,
    clearSelectedObjects, selectedSource, selectedSourceRevision, activeStep, api,
  } = deps
  const databaseCatalogNames = ref<string[]>([])
  const completeDatabaseCatalogNames = ref<string[] | null>(null)

  const databaseCatalogKeyword = ref('')
  const databaseCatalogLoading = ref(false)

  const databaseCatalogLoaded = ref(false)
  const databaseCatalogTruncated = ref(false)

  const databaseCatalogFailure = ref('')
  const databaseDropdownOpen = ref(false)

  const manualDatabaseOpen = ref(false)
  const manualDatabaseInput = ref('')

  const manualDatabaseError = ref('')

  let databaseCatalogEpoch = 0

  let databaseCatalogTimer: ReturnType<typeof setTimeout> | undefined

  let databaseCatalogLoadTimer: ReturnType<typeof setTimeout> | undefined

  let databaseCatalogWaitResolve: (() => void) | undefined

  type ObjectCatalogState = { names: string[]; baseNames: string[]; keyword: string; loading: boolean; loaded: boolean; baseLoaded: boolean; truncated: boolean; baseTruncated: boolean; failure: string }

  function emptyObjectCatalog(): ObjectCatalogState { return { names: [], baseNames: [], keyword: '', loading: false, loaded: false, baseLoaded: false, truncated: false, baseTruncated: false, failure: '' } }

  const catalogByType = reactive<Record<ExportObjectType, ObjectCatalogState>>({ TABLE: emptyObjectCatalog(), VIEW: emptyObjectCatalog(), FUNCTION: emptyObjectCatalog(), PROCEDURE: emptyObjectCatalog(), SEQUENCE: emptyObjectCatalog() })
  const candidateObjectNames = computed({ get: () => catalogByType[objectType.value].names, set: (names: string[]) => {
    const catalog = catalogByType[objectType.value]
    catalog.names = names
    if (!catalog.keyword) catalog.baseNames = names
  } })

  const candidateGroupExpanded = ref(false)
  const selectedGroupExpanded = reactive<Record<ExportObjectType, boolean>>({ TABLE: true, VIEW: true, FUNCTION: true, PROCEDURE: true, SEQUENCE: true })

  const candidateObjectKeyword = ref('')
  const catalogKeywordTooLong = computed(() => new TextEncoder().encode(candidateObjectKeyword.value.trim()).length > 100)

  const catalogLoading = computed(() => catalogByType[objectType.value].loading)
  const catalogLoaded = computed(() => catalogByType[objectType.value].loaded)

  const catalogTruncated = computed(() => catalogByType[objectType.value].truncated)
  const catalogFailure = computed(() => catalogByType[objectType.value].failure)

  const catalogEpoch: Record<ExportObjectType, number> = { TABLE: 0, VIEW: 0, FUNCTION: 0, PROCEDURE: 0, SEQUENCE: 0 }
  const catalogTimers = new Map<ExportObjectType, ReturnType<typeof setTimeout>>()

  const catalogWaitResolves = new Map<ExportObjectType, () => void>()

  let catalogLoadTimer: ReturnType<typeof setTimeout> | undefined

  let batchCatalogEpoch = 0

  let batchCatalogTimer: ReturnType<typeof setTimeout> | undefined

  let batchCatalogWaitResolve: (() => void) | undefined

  let batchCatalogLoading = false

  const selectedObjectKeyword = ref('')
  const visibleSelectedGroups = computed(() => {
    const keyword = selectedObjectKeyword.value.trim().toLocaleLowerCase()
    const groups = visibleObjectCategories.value.map((category) => ({ ...category, rows: [] as typeof applicableSelectedObjects.value }))
    const byType = new Map(groups.map((group) => [group.type, group]))
    for (const item of applicableSelectedObjects.value) if (!keyword || item.name.toLocaleLowerCase().includes(keyword)) byType.get(item.type)?.rows.push(item)
    return groups.filter((group) => group.rows.length)
  })

  const visibleCandidateObjectNames = computed(() => {
    const keyword = candidateObjectKeyword.value.trim().toLocaleLowerCase()
    return keyword ? candidateObjectNames.value.filter((name) => name.toLocaleLowerCase().includes(keyword)) : candidateObjectNames.value
  })
  const visibleObjectCategories = computed(() => contentKind.value === 'DATA_ONLY' ? objectCategories.filter((category) => category.type === 'TABLE') : objectCategories)

  const candidateTotalCount = computed(() => visibleObjectCategories.value.reduce((total, category) => total + catalogByType[category.type].baseNames.length, 0))

  function catalogCount(type: ExportObjectType): string | number {
    const catalog = catalogByType[type]
    if (catalog.baseLoaded) return catalog.baseNames.length
    if (catalog.loading) return '加载中'
    if (catalog.failure) return '加载失败'
    return selectedObjectsByType[type].length || '待加载'
  }

  const databaseOptions = computed(() => [...new Set([
    ...databaseCatalogNames.value,
    selectedSource.value?.defaultDatabase ?? '',
    database.value,
  ].filter(Boolean))].filter((name) => name.toLocaleLowerCase().includes(databaseCatalogKeyword.value.trim().toLocaleLowerCase())))

  watch(objectType, (_type, previousType) => {
    if (hydratingDerivedDraft.value) return
    // 分类目录已按数据库预加载；切换时仅恢复上一个分类的完整候选列表。
    catalogByType[previousType].names = catalogByType[previousType].baseNames
    catalogByType[previousType].keyword = ''
    catalogByType[previousType].truncated = catalogByType[previousType].baseTruncated
    candidateObjectKeyword.value = ''
    selectedObjectKeyword.value = ''
  })

  watch(contentKind, (kind) => {
    if (hydratingDerivedDraft.value) return
    if (kind === 'DATA_ONLY' && objectType.value !== 'TABLE') {
      objectType.value = 'TABLE'
      candidateGroupExpanded.value = false
    }
  })

  watch(objectNames, (names) => {
    // 恢复历史草稿时，将已选对象并入候选区，使两栏状态保持一致。
    const candidates = new Set(candidateObjectNames.value)
    const missing = names.map((name) => name.trim()).filter((name) => name && !candidates.has(name))
    if (missing.length > 0) {
      candidateObjectNames.value = [...candidateObjectNames.value, ...missing]
      catalogByType[objectType.value].baseNames = [...new Set([...catalogByType[objectType.value].baseNames, ...missing])]
    }
  }, { deep: true })

  let disposed = false
  onScopeDispose(() => {
    disposed = true
    stopCatalogQuery()
    stopDatabaseCatalogQuery()
  })

  function stopCatalogQuery() {
    batchCatalogEpoch++
    if (batchCatalogTimer) clearTimeout(batchCatalogTimer)
    batchCatalogWaitResolve?.()
    batchCatalogWaitResolve = undefined
    batchCatalogLoading = false
    if (catalogLoadTimer) clearTimeout(catalogLoadTimer)
    catalogLoadTimer = undefined
    for (const category of objectCategories) {
      stopCatalogType(category.type)
      Object.assign(catalogByType[category.type], emptyObjectCatalog())
    }
  }

  // 首轮固定查询一次读取五类；分类展开和内容模式切换只使用本地缓存。
  async function loadAllCatalogs() {
    if (disposed) return
    if (batchCatalogLoading || objectCategories.every((category) => catalogByType[category.type].baseLoaded)) return
    const source = selectedSource.value
    const nodeID = selectedNodeID.value
    const schema = database.value.trim()
    if (activeStep.value !== 2 || scopeKind.value !== 'SPECIFIED' || !source || !nodeID || !schema) return
    batchCatalogLoading = true
    const epoch = batchCatalogEpoch
    const typeEpochs = { ...catalogEpoch }
    for (const category of objectCategories) {
      catalogByType[category.type].loading = true
      catalogByType[category.type].failure = ''
    }
    try {
      let query = await api.searchExportObjects(source.id, source.revision, { nodeId: nodeID, database: schema, objectType: 'ALL', keyword: '' })
      if (epoch !== batchCatalogEpoch) return
      while (query.status === 'PENDING' || query.status === 'LEASED') {
        await new Promise<void>((resolve) => { batchCatalogWaitResolve = resolve; batchCatalogTimer = setTimeout(resolve, 1000) })
        batchCatalogWaitResolve = undefined
        if (epoch !== batchCatalogEpoch) return
        query = await api.getExportObjectCatalogQuery(query.id)
        if (epoch !== batchCatalogEpoch) return
      }
      if (epoch !== batchCatalogEpoch) return
      if (query.dataSourceId !== source.id || query.nodeId !== nodeID || query.database !== schema || query.objectType !== 'ALL' || query.keyword !== '') throw new Error('对象目录与当前查询条件不一致。')
      if (query.status !== 'SUCCEEDED') throw new Error(query.status === 'EXPIRED' ? '执行节点未及时完成对象查询，请重试。' : '执行节点未能读取对象目录，请重试。')
      for (const group of query.groups) {
        if (catalogEpoch[group.objectType] !== typeEpochs[group.objectType]) continue
        const catalog = catalogByType[group.objectType]
        if (group.unavailable) { catalog.failure = `${objectCategories.find((item) => item.type === group.objectType)?.label ?? '对象'}目录不可用，请重试。`; continue }
        const names = [...new Set([...group.objects, ...selectedObjectsByType[group.objectType]])]
        catalog.baseNames = names
        catalog.baseLoaded = true
        catalog.baseTruncated = group.truncated
        catalog.loaded = true
        if (!catalog.keyword) { catalog.names = names; catalog.truncated = group.truncated }
      }
    } catch (error) {
      if (epoch === batchCatalogEpoch) for (const category of objectCategories) {
        if (catalogEpoch[category.type] === typeEpochs[category.type]) catalogByType[category.type].failure = error instanceof Error ? error.message : exportCatalogErrorMessage(error)
      }
    } finally {
      if (epoch === batchCatalogEpoch) {
        batchCatalogLoading = false
        for (const category of objectCategories) if (catalogEpoch[category.type] === typeEpochs[category.type]) catalogByType[category.type].loading = false
      }
    }
  }

  function stopCatalogType(type: ExportObjectType) {
    catalogEpoch[type]++
    const timer = catalogTimers.get(type)
    if (timer) clearTimeout(timer)
    catalogTimers.delete(type)
    catalogWaitResolves.get(type)?.()
    catalogWaitResolves.delete(type)
    catalogByType[type].loading = false
  }

  function stopDatabaseCatalogQuery() {
    databaseCatalogEpoch++
    if (databaseCatalogTimer) clearTimeout(databaseCatalogTimer)
    databaseCatalogWaitResolve?.()
    databaseCatalogWaitResolve = undefined
    if (databaseCatalogLoadTimer) clearTimeout(databaseCatalogLoadTimer)
    databaseCatalogTimer = undefined
    databaseCatalogLoadTimer = undefined
    databaseCatalogLoading.value = false
    databaseCatalogFailure.value = ''
  }

  async function loadDatabaseCatalog() {
    if (disposed) return
    if (databaseCatalogLoading.value) return
    stopDatabaseCatalogQuery()
    const epoch = databaseCatalogEpoch
    const source = selectedSource.value
    const nodeID = selectedNodeID.value
    const keyword = databaseCatalogKeyword.value.trim()
    if (new TextEncoder().encode(keyword).length > 100) {
      databaseCatalogFailure.value = '数据库关键字最多 100 字节。'
      return
    }
    if (activeStep.value !== 2 || !source || !nodeID) return
    databaseCatalogLoading.value = true
    try {
      let query = await api.searchExportObjects(source.id, source.revision, { nodeId: nodeID, database: '', objectType: 'DATABASE', keyword })
      if (epoch !== databaseCatalogEpoch) return
      while (query.status === 'PENDING' || query.status === 'LEASED') {
        await new Promise<void>((resolve) => { databaseCatalogWaitResolve = resolve; databaseCatalogTimer = setTimeout(resolve, 2000) })
        databaseCatalogWaitResolve = undefined
        if (epoch !== databaseCatalogEpoch) return
        query = await api.getExportObjectCatalogQuery(query.id)
        if (epoch !== databaseCatalogEpoch) return
      }
      if (query.dataSourceId !== source.id || query.nodeId !== nodeID || query.database !== '' || query.objectType !== 'DATABASE' || query.keyword !== keyword) {
        databaseCatalogFailure.value = '数据库查询结果与当前条件不一致，请重新加载。'
        return
      }
      if (query.status !== 'SUCCEEDED') {
        databaseCatalogFailure.value = query.status === 'EXPIRED' ? '执行节点未及时完成数据库查询。可重新加载或手动输入。' : '执行节点未能读取数据库目录，可手动输入并由预检查确认。'
        return
      }
      databaseCatalogNames.value = [...query.objects]
      databaseCatalogLoaded.value = true
      databaseCatalogTruncated.value = query.truncated
      if (!keyword && !query.truncated) completeDatabaseCatalogNames.value = [...query.objects]
    } catch (error) {
      if (epoch === databaseCatalogEpoch) databaseCatalogFailure.value = exportCatalogErrorMessage(error)
    } finally {
      if (epoch === databaseCatalogEpoch) databaseCatalogLoading.value = false
    }
  }

  function scheduleDatabaseCatalog(keyword: string) {
    databaseCatalogKeyword.value = keyword
    if (completeDatabaseCatalogNames.value) {
      databaseCatalogNames.value = completeDatabaseCatalogNames.value
      databaseCatalogLoaded.value = true
      databaseCatalogTruncated.value = false
      return
    }
    stopDatabaseCatalogQuery()
    databaseCatalogNames.value = []
    databaseCatalogLoaded.value = false
    databaseCatalogTruncated.value = false
    if (activeStep.value !== 2 || !selectedSource.value || !selectedNodeID.value) return
    databaseCatalogLoadTimer = setTimeout(() => { void loadDatabaseCatalog() }, 500)
  }

  function onDatabaseDropdownVisibleChange(open: boolean) {
    databaseDropdownOpen.value = open
    if (open && selectedNodeID.value && !databaseCatalogLoaded.value && !databaseCatalogLoading.value && !databaseCatalogFailure.value) {
      void loadDatabaseCatalog()
    }
  }

  function selectDatabaseOption(value: unknown) {
    if (typeof value !== 'string') return
    databaseDropdownOpen.value = false
    if (value === '__manual__') {
      manualDatabaseInput.value = database.value
      manualDatabaseError.value = ''
      manualDatabaseOpen.value = true
      return
    }
    database.value = value
    databaseCatalogKeyword.value = ''
  }

  function confirmManualDatabase() {
    const name = manualDatabaseInput.value.trim()
    if (!name || new TextEncoder().encode(name).length > 256 || /[*,\r\n\0]/.test(name)) {
      manualDatabaseError.value = '请输入不含通配符和逗号、最多 256 字节的数据库或 Schema 名称。'
      return
    }
    database.value = name
    databaseDropdownOpen.value = false
    databaseCatalogKeyword.value = ''
    manualDatabaseOpen.value = false
    manualDatabaseError.value = ''
  }

  watch([selectedDataSourceID, selectedSourceRevision, selectedNodeID], () => {
    stopDatabaseCatalogQuery()
    databaseCatalogNames.value = []
    completeDatabaseCatalogNames.value = null
    databaseCatalogLoaded.value = false
    databaseCatalogTruncated.value = false
    databaseCatalogKeyword.value = ''
    databaseDropdownOpen.value = false
    if (activeStep.value === 2 && selectedSource.value && selectedNodeID.value) {
      databaseCatalogLoadTimer = setTimeout(() => { void loadDatabaseCatalog() }, 500)
    }
  }, { flush: 'sync' })

  watch(activeStep, (step) => {
    if (step === 2 && selectedSource.value && selectedNodeID.value && !databaseCatalogLoaded.value && !databaseCatalogLoading.value && !databaseCatalogFailure.value) void loadDatabaseCatalog()
  }, { flush: 'post' })

  function catalogMatches(query: ExportObjectCatalogQuery, sourceID: string, nodeID: string, schema: string, type: ExportObjectType, keyword: string) {
    return query.dataSourceId === sourceID && query.nodeId === nodeID && query.database === schema && query.objectType === type && query.keyword === keyword
  }

  async function loadCatalog(type: ExportObjectType = objectType.value, keyword = candidateObjectKeyword.value.trim(), refresh = true) {
    if (disposed) return
    const catalog = catalogByType[type]
    if (catalog.loading && !refresh) return
    if (catalog.baseLoaded && !keyword && !refresh) return
    stopCatalogType(type)
    const epoch = catalogEpoch[type]
    const source = selectedSource.value
    const nodeID = selectedNodeID.value
    const schema = database.value.trim()
    if (new TextEncoder().encode(keyword).length > 100) { catalog.failure = '名称关键字最多 100 字节，请缩短后重试。'; return }
    if (activeStep.value !== 2 || scopeKind.value !== 'SPECIFIED' || !source || !nodeID || !schema) return
    catalog.loading = true
    catalog.failure = ''
    let succeeded = false
    try {
      let query = await api.searchExportObjects(source.id, source.revision, { nodeId: nodeID, database: schema, objectType: type, keyword })
      if (epoch !== catalogEpoch[type] || !catalogMatches(query, source.id, nodeID, schema, type, keyword)) return
      while (query.status === 'PENDING' || query.status === 'LEASED') {
        await new Promise<void>((resolve) => {
          catalogWaitResolves.set(type, resolve)
          catalogTimers.set(type, setTimeout(resolve, 2000))
        })
        catalogWaitResolves.delete(type)
        catalogTimers.delete(type)
        if (epoch !== catalogEpoch[type]) return
        query = await api.getExportObjectCatalogQuery(query.id)
        if (epoch !== catalogEpoch[type] || !catalogMatches(query, source.id, nodeID, schema, type, keyword)) return
      }
      if (query.status !== 'SUCCEEDED') {
        catalog.failure = query.status === 'EXPIRED' ? '执行节点未及时领取或完成对象查询。请确认 Agent 在线后重试。' : '节点未能读取对象元数据，请检查数据源连接与节点状态后重试。'
        return
      }
      catalog.loaded = true
      succeeded = true
      const selected = selectedObjectsByType[type].map((name) => name.trim()).filter(Boolean)
      const names = [...new Set([...query.objects, ...selected])]
      if (!keyword) {
        catalog.baseNames = names
        catalog.baseLoaded = true
        catalog.baseTruncated = query.truncated
      }
      if (!keyword || (type === objectType.value && keyword === candidateObjectKeyword.value.trim())) {
        catalog.names = names
        catalog.keyword = keyword
        catalog.truncated = query.truncated
      }
    } catch (error) {
      if (epoch === catalogEpoch[type]) catalog.failure = exportCatalogErrorMessage(error)
    } finally {
      if (epoch === catalogEpoch[type]) {
        catalog.loading = false
        // 用户在首轮加载完成前搜索时，仍补齐该分类的完整目录缓存。
        if (succeeded && keyword && !catalog.baseLoaded) void loadCatalog(type, '', true)
      }
    }
  }

  function scheduleCatalogLoad() {
    if (catalogLoadTimer) clearTimeout(catalogLoadTimer)
    if (activeStep.value !== 2 || scopeKind.value !== 'SPECIFIED' || !selectedSource.value || !selectedNodeID.value || !database.value.trim()) return
    catalogLoadTimer = setTimeout(() => {
      catalogLoadTimer = undefined
      void loadAllCatalogs()
    }, 300)
  }

  watch([selectedDataSourceID, selectedSourceRevision, selectedNodeID, database], () => {
    if (hydratingDerivedDraft.value) return
    stopCatalogQuery()
    candidateGroupExpanded.value = false
    candidateObjectKeyword.value = ''
    selectedObjectKeyword.value = ''
    scheduleCatalogLoad()
  }, { flush: 'sync' })

  watch([activeStep, scopeKind], scheduleCatalogLoad, { flush: 'post' })

  watch(candidateObjectKeyword, (keyword) => {
    const catalog = catalogByType[objectType.value]
    if (catalog.keyword && catalog.keyword !== keyword.trim()) {
      catalog.names = catalog.baseNames
      catalog.keyword = ''
      catalog.truncated = catalog.baseTruncated
    }
  })

  function toggleCandidate(name: string) {
    const selected = new Set(objectNames.value.map((value) => value.trim()).filter(Boolean))
    if (selected.has(name)) selected.delete(name)
    else selected.add(name)
    objectNames.value = selected.size > 0 ? [...selected] : ['']
  }

  function selectableCategoryNames(type: ExportObjectType): string[] {
    return type === objectType.value ? visibleCandidateObjectNames.value : catalogByType[type].baseNames
  }

  function categorySelectedCount(type: ExportObjectType): number {
    const selected = new Set(selectedObjectsByType[type])
    return selectableCategoryNames(type).filter((name) => selected.has(name)).length
  }

  function toggleCategorySelection(type: ExportObjectType) {
    const names = selectableCategoryNames(type)
    if (names.length === 0) return
    const selected = new Set(selectedObjectsByType[type])
    const allSelected = names.every((name) => selected.has(name))
    for (const name of names) {
      if (allSelected) selected.delete(name)
      else selected.add(name)
    }
    selectedObjectsByType[type] = [...selected]
  }

  function chooseObjectCategory(type: string) {
    if (type !== 'TABLE' && type !== 'VIEW' && type !== 'FUNCTION' && type !== 'PROCEDURE' && type !== 'SEQUENCE') return
    if (type === objectType.value) {
      candidateGroupExpanded.value = !candidateGroupExpanded.value
      return
    }
    objectType.value = type
    candidateGroupExpanded.value = true
    selectedGroupExpanded[type] = true
  }

  function clearObjectCategory(type: ExportObjectType) {
    selectedObjectsByType[type] = []
  }

  function removeObjectNameRow(index: number, type: ExportObjectType = objectType.value) {
    selectedObjectsByType[type] = selectedObjectsByType[type].filter((_, position) => position !== index)
  }

  function clearObjectNameRows() {
    clearSelectedObjects()
    selectedObjectKeyword.value = ''
  }

  return {
    databaseCatalogNames, completeDatabaseCatalogNames, databaseCatalogKeyword, databaseCatalogLoading,
    databaseCatalogLoaded, databaseCatalogTruncated, databaseCatalogFailure, databaseDropdownOpen,
    manualDatabaseOpen, manualDatabaseInput, manualDatabaseError, emptyObjectCatalog, catalogByType,
    candidateObjectNames, candidateGroupExpanded, selectedGroupExpanded, candidateObjectKeyword,
    catalogKeywordTooLong, catalogLoading, catalogLoaded, catalogTruncated, catalogFailure,
    selectedObjectKeyword, visibleSelectedGroups, visibleCandidateObjectNames, visibleObjectCategories,
    candidateTotalCount, catalogCount, databaseOptions, stopCatalogQuery, loadAllCatalogs, stopCatalogType,
    stopDatabaseCatalogQuery, loadDatabaseCatalog, scheduleDatabaseCatalog,
    onDatabaseDropdownVisibleChange, selectDatabaseOption, confirmManualDatabase, catalogMatches,
    loadCatalog, scheduleCatalogLoad, toggleCandidate, selectableCategoryNames, categorySelectedCount,
    toggleCategorySelection, chooseObjectCategory, clearObjectCategory, removeObjectNameRow,
    clearObjectNameRows,
  }
}

export type ExportCatalog = ReturnType<typeof useExportCatalog>
