import { afterEach, describe, expect, it, vi } from 'vitest'
import { effectScope, ref, shallowRef } from 'vue'
import type { ExportDraftListItem, ExportDraftListPage, TaskOperationPage } from '@/api/browser'
import type { TaskDetailSession } from './taskDetailSession'
import { useTaskDraftList } from './useTaskDraftList'
import { useTaskOperations } from './useTaskOperations'
import { taskDetailPrimary, taskLogIsError, taskResultText } from './taskDetailRules'

const scopes: ReturnType<typeof effectScope>[] = []
afterEach(() => { scopes.splice(0).forEach(scope => scope.stop()) })
function deferred<T>() { let resolve!: (value: T) => void; let reject!: (reason: unknown) => void; const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no }); return { promise, resolve, reject } }
async function settle() { for (let index = 0; index < 20; index++) await Promise.resolve() }
const operation = (id: string) => ({ id, action: 'TASK_SUBMITTED', actorType: 'SUBJECT' as const, actorName: '合成用户', result: 'SUCCEEDED' as const, occurredAt: '2026-10-14T00:00:00Z' })
const draft = (id: string): ExportDraftListItem => ({ id, type: 'OBDUMPER_EXPORT', displayName: id, dataSourceId: 'synthetic-source', nodeId: 'synthetic-node', revision: 1, createdAt: '2026-10-14T00:00:00Z', updatedAt: '2026-10-14T00:00:00Z', availableActions: ['CONTINUE_EDIT', 'SUBMIT', 'DELETE'] })

describe('操作记录与草稿会话', () => {
  it('操作记录只接纳当前会话，A → B → A 与卸载的迟到响应不可复活', async () => {
    const pending = deferred<TaskOperationPage>(), final = deferred<TaskOperationPage>()
    const reads = vi.fn().mockReturnValueOnce(pending.promise).mockResolvedValueOnce({ items: [operation('B')] }).mockReturnValueOnce(final.promise)
    const binding = (id: string): TaskDetailSession => ({ id, active: () => true, api: { getTaskOperations: reads } as unknown as TaskDetailSession['api'] })
    const session = shallowRef<TaskDetailSession | null>(binding('A')); const scope = effectScope(); scopes.push(scope)
    const model = scope.run(() => useTaskOperations(session))!
    void model.load(); session.value = binding('B'); await model.load(); session.value = binding('A'); void model.load()
    pending.resolve({ items: [operation('old-A')] }); await settle(); expect(model.operations.value).toEqual([])
    scope.stop(); final.resolve({ items: [operation('late-A')] }); await settle(); expect(model.operations.value).toEqual([])
  })
  it('加载更多防重并去除页内和跨页重复', async () => {
    const next = deferred<TaskOperationPage>()
    const reads = vi.fn().mockResolvedValueOnce({ items: [operation('one'), operation('one')], nextCursor: 'cursor-one' }).mockReturnValueOnce(next.promise)
    const session = shallowRef<TaskDetailSession | null>({ id: 'A', active: () => true, api: { getTaskOperations: reads } as unknown as TaskDetailSession['api'] })
    const scope = effectScope(); scopes.push(scope); const model = scope.run(() => useTaskOperations(session))!
    await model.load(); const first = model.load(true), second = model.load(true); expect(first).toBe(second); expect(reads).toHaveBeenCalledTimes(2)
    next.resolve({ items: [operation('one'), operation('two')] }); await first
    expect(model.operations.value.map(item => item.id)).toEqual(['one', 'two']); expect(model.nextCursor.value).toBeUndefined()
  })
  it('草稿条件切换隔离旧读取，分页传可靠游标，删除防重并在卸载后不提示', async () => {
    const old = deferred<ExportDraftListPage>(), deleting = deferred<void>()
    const reads = vi.fn().mockReturnValueOnce(old.promise).mockResolvedValue({ items: [draft('new')], totalPages: 2, nextCursor: 'page-two' })
    const remove = vi.fn(() => deleting.promise), navigate = vi.fn()
    const query = ref({ draftKeyword: 'old' }); const scope = effectScope(); scopes.push(scope)
    const model = scope.run(() => useTaskDraftList(query, vi.fn(), navigate, () => ({ listTaskDrafts: reads, deleteExportDraft: remove, deleteNormalImportDraft: vi.fn(async () => {}), deleteDirectLoadDraft: vi.fn(async () => {}) })))!
    query.value = { draftKeyword: 'new' }; await settle(); old.resolve({ items: [draft('old')], totalPages: 1 }); await settle()
    expect(model.items.value.map(item => item.id)).toEqual(['new'])
    model.next(); await settle(); expect(reads).toHaveBeenLastCalledWith(10, 'new', 'page-two')
    model.open(draft('new'), true); expect(navigate).toHaveBeenCalledWith({ path: '/exports/new', query: { draft: 'new', step: '5' } })
    model.requestDelete(draft('new')); const first = model.confirmDelete(); await model.confirmDelete(); expect(remove).toHaveBeenCalledTimes(1)
    scope.stop(); deleting.resolve(); await first; expect(model.notice.value).toBe(''); expect(reads).toHaveBeenCalledTimes(3)
  })
  it('草稿授权失效清空列表和确认，旧删除响应不污染新筛选', async () => {
    const deleting = deferred<void>(); let denied = false
    const reads = vi.fn(async () => { if (denied) throw { status: 403, code: 'DENIED', message: '无权读取' }; return { items: [draft('A')], totalPages: 1 } })
    const query = ref({ draftKeyword: 'A' }); const scope = effectScope(); scopes.push(scope)
    const model = scope.run(() => useTaskDraftList(query, vi.fn(), vi.fn(), () => ({ listTaskDrafts: reads, deleteExportDraft: () => deleting.promise, deleteNormalImportDraft: vi.fn(async () => {}), deleteDirectLoadDraft: vi.fn(async () => {}) })))!
    await settle(); model.requestDelete(draft('A')); const pending = model.confirmDelete(); query.value = { draftKeyword: 'B' }; await settle()
    deleting.resolve(); await pending; expect(model.notice.value).toBe(''); expect(model.confirmation.value).toBeUndefined(); expect(model.busyId.value).toBe('')
    denied = true; await model.load(true); expect(model.restricted.value).toBe(true); expect(model.items.value).toEqual([]); expect(model.totalPages.value).toBe(0)
  })
  it('主操作不推断资格，stderr 不冒充错误', () => {
    expect(taskDetailPrimary({ state: 'FAILED', availableActions: ['COPY_COMMAND'] })).toBeUndefined()
    expect(taskDetailPrimary({ state: 'FAILED', availableActions: ['REBUILD_FROM_CONFIG', 'CHECKPOINT_RESUME'] })).toBe('CHECKPOINT_RESUME')
    expect(taskLogIsError({ message: 'stderr text', kind: 'STDERR' } as Parameters<typeof taskLogIsError>[0])).toBe(false)
    expect(taskLogIsError({ message: '[ERROR] synthetic failure' } as Parameters<typeof taskLogIsError>[0])).toBe(true)
    expect(taskResultText(undefined, true)).toBe('尚无结果证据')
    expect(taskResultText({ result: 'FAILED' }, true)).toBe('结果未通过核验')
    expect(taskResultText({ result: 'VERIFIED' }, true)).toBe('远端对象未逐项核验')
  })
  it('统一草稿保持导入类型，恢复与删除使用导入入口', async () => {
    const item = { ...draft('normal'), type: 'OBLOADER_NORMAL_IMPORT' as const, availableActions: ['CONTINUE_EDIT', 'DELETE'] as const }
    const navigate = vi.fn(), removeImport = vi.fn(async () => {}), removeExport = vi.fn(async () => {})
    const scope = effectScope(); scopes.push(scope)
    const model = scope.run(() => useTaskDraftList(ref({}), vi.fn(), navigate, () => ({ listTaskDrafts: vi.fn(async () => ({ items: [item], totalPages: 1 })), deleteExportDraft: removeExport, deleteNormalImportDraft: removeImport, deleteDirectLoadDraft: vi.fn(async () => {}) })))!
    await settle(); model.open(item)
    expect(navigate).toHaveBeenCalledWith({ path: '/imports/normal/new', query: { draft: 'normal', step: '1' } })
    model.requestDelete(item); await model.confirmDelete()
    expect(removeImport).toHaveBeenCalledExactlyOnceWith(item); expect(removeExport).not.toHaveBeenCalled()
  })
  it('旁路草稿恢复到独立最终步，删除不会调用其他任务类型', async () => {
    const item={...draft('direct'),type:'OBLOADER_DIRECT_LOAD' as const,availableActions:['CONTINUE_EDIT','SUBMIT','DELETE'] as const},navigate=vi.fn(),remove=vi.fn(async()=>{}),removeOther=vi.fn(async()=>{})
    const scope=effectScope();scopes.push(scope)
    const model=scope.run(()=>useTaskDraftList(ref({}),vi.fn(),navigate,()=>({listTaskDrafts:vi.fn(async()=>({items:[item],totalPages:1})),deleteExportDraft:removeOther,deleteNormalImportDraft:removeOther,deleteDirectLoadDraft:remove})))!
    await settle();model.open(item,true);expect(navigate).toHaveBeenCalledWith({path:'/imports/direct/new',query:{draft:'direct',step:'5'}})
    model.requestDelete(item);await model.confirmDelete();expect(remove).toHaveBeenCalledExactlyOnceWith(item);expect(removeOther).not.toHaveBeenCalled()
  })
})
