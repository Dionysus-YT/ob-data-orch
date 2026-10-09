import { afterEach, describe, expect, it, vi } from 'vitest'
import { effectScope } from 'vue'
import type { StorageCredentialListItem } from '@/api/browser'
import { useCredentialList } from './useCredentialList'
import { useCredentialEditor } from './useCredentialEditor'
import { useCredentialDeletion } from './useCredentialDeletion'
function deferred<T>() { let resolve!: (value: T) => void; let reject!: (error: unknown) => void; const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no }); return { promise, resolve, reject } }
const item = (id = 'synthetic-a', revision = 1): StorageCredentialListItem => ({ id, displayName: id, provider: 'OSS', revision, currentRevision: revision, updatedAt: '2026-01-01T00:00:00Z' })
const scopes: ReturnType<typeof effectScope>[] = []
afterEach(() => { scopes.splice(0).forEach(scope => scope.stop()) })
function setup() {
  const api = { listStorageCredentials: vi.fn(async () => [item()]), createStorageCredential: vi.fn(async () => item('synthetic-created')), rotateStorageCredential: vi.fn(async () => item('synthetic-a', 2)), deleteStorageCredential: vi.fn(async () => {}) }
  const signals: AbortSignal[] = []; const scope = effectScope(); scopes.push(scope)
  const value = scope.run(() => { const list = useCredentialList(signal => { signals.push(signal); return api }); return { list, editor: useCredentialEditor(list), deletion: useCredentialDeletion(list) } })!
  return { ...value, api, scope, signals }
}
function fill(editor: ReturnType<typeof useCredentialEditor>) { editor.formName.value = '合成凭据'; editor.formAccessKey.value = 'synthetic-access'; editor.formSecretKey.value = 'synthetic-secret' }
function secretsEmpty(editor: ReturnType<typeof useCredentialEditor>) { return editor.formAccessKey.value === '' && editor.formSecretKey.value === '' }

describe('存储凭据列表和短时输入生命周期', () => {
  it.each([401, 403])('列表 %s 同步撤销删除确认，阻断旧目标直到授权重读成功', async status => {
    const { list, deletion, api } = setup(); await list.loadCredentials()
    deletion.requestDelete(item()); expect(deletion.pendingDelete.value).toEqual(item())
    api.listStorageCredentials.mockRejectedValueOnce({ status }); await list.loadCredentials()
    expect(list.credentials.value).toEqual([]); expect(deletion.pendingDelete.value).toBeUndefined()
    deletion.requestDelete(item()); await deletion.confirmDelete()
    expect(deletion.pendingDelete.value).toBeUndefined(); expect(api.deleteStorageCredential).not.toHaveBeenCalled()
    api.listStorageCredentials.mockRejectedValueOnce({ status: 503 }); await list.loadCredentials()
    deletion.requestDelete(item()); await deletion.confirmDelete()
    expect(deletion.pendingDelete.value).toBeUndefined(); expect(api.deleteStorageCredential).not.toHaveBeenCalled()
    await list.loadCredentials(); await deletion.confirmDelete()
    expect(deletion.pendingDelete.value).toBeUndefined(); expect(api.deleteStorageCredential).not.toHaveBeenCalled()
    deletion.requestDelete(list.credentials.value[0]!); await deletion.confirmDelete()
    expect(api.deleteStorageCredential).toHaveBeenCalledExactlyOnceWith('synthetic-a', 1)
  })
  it('普通列表故障保留删除确认，过期授权拒绝不得撤销最新授权会话', async () => {
    const { list, deletion, api } = setup(); await list.loadCredentials(); deletion.requestDelete(item())
    api.listStorageCredentials.mockRejectedValueOnce({ status: 503 }); await list.loadCredentials()
    expect(deletion.pendingDelete.value).toEqual(item()); expect(list.accessDenied.value).toBe(false)
    const old = deferred<StorageCredentialListItem[]>(); api.listStorageCredentials.mockImplementationOnce(() => old.promise)
    const pending = list.loadCredentials(); await list.loadCredentials(); old.reject({ status: 403 }); await pending
    expect(deletion.pendingDelete.value).toEqual(item()); expect(list.credentials.value).toEqual([item()]); expect(list.accessDenied.value).toBe(false)
  })
  it('最新读取胜出，刷新失败保留事实，权限失败清除列表及输入', async () => {
    const { list, editor, api } = setup(); const old = deferred<StorageCredentialListItem[]>(); api.listStorageCredentials.mockImplementationOnce(() => old.promise)
    const pending = list.loadCredentials(); await list.loadCredentials(); old.resolve([item('synthetic-old')]); await pending; expect(list.credentials.value[0]?.id).toBe('synthetic-a')
    editor.openCreate(); fill(editor); api.listStorageCredentials.mockRejectedValueOnce({ status: 503 }); await list.loadCredentials(); expect(list.credentials.value).toHaveLength(1); expect(secretsEmpty(editor)).toBe(false)
    api.listStorageCredentials.mockRejectedValueOnce({ status: 403 }); await list.loadCredentials(); expect(list.credentials.value).toHaveLength(0); expect(editor.editorOpen.value).toBe(false); expect(secretsEmpty(editor)).toBe(true)
    api.listStorageCredentials.mockRejectedValueOnce({ status: 503 }); await list.loadCredentials(); editor.openCreate(); expect(editor.editorOpen.value).toBe(false); expect(list.beginWrite('synthetic-a')).toBeUndefined()
    await list.loadCredentials(); editor.openCreate(); expect(editor.editorOpen.value).toBe(true)
  })
  it('创建方法防重，共享锁阻止删除，成功清理输入和请求栈中的材料', async () => {
    const { list, editor, deletion, api } = setup(); await list.loadCredentials(); editor.openCreate(); fill(editor)
    const write = deferred<StorageCredentialListItem>(); api.createStorageCredential.mockImplementationOnce(() => write.promise)
    const pending = editor.submitForm(); await editor.submitForm(); deletion.requestDelete(item()); await deletion.confirmDelete(); expect(api.createStorageCredential).toHaveBeenCalledTimes(1); expect(api.deleteStorageCredential).not.toHaveBeenCalled()
    write.resolve(item('synthetic-created')); await pending; expect(list.credentials.value).toHaveLength(2); expect(secretsEmpty(editor)).toBe(true); expect(editor.editorOpen.value).toBe(false); expect(list.writeBusy.value).toBe(false)
  })
  it('失败保留当前输入便于重试；关闭、轮换对象切换均清除秘密', async () => {
    const { editor, api } = setup(); editor.openCreate(); fill(editor); api.createStorageCredential.mockRejectedValueOnce({ status: 409 }); await editor.submitForm()
    expect(secretsEmpty(editor)).toBe(false); expect(editor.saving.value).toBe(false); editor.openRotate(item()); expect(secretsEmpty(editor)).toBe(true); fill(editor); editor.closeEditor(); expect(secretsEmpty(editor)).toBe(true)
  })
  it('轮换捕获打开时修订，刷新不静默提升版本，旧读取不能覆盖操作结果', async () => {
    const { list, editor, api } = setup(); await list.loadCredentials(); editor.openRotate(item()); fill(editor)
    const read = deferred<StorageCredentialListItem[]>(); api.listStorageCredentials.mockImplementationOnce(() => read.promise); const pending = list.loadCredentials()
    await editor.submitForm(); read.resolve([item()]); await pending; expect(api.rotateStorageCredential.mock.calls[0]?.slice(0, 2)).toEqual(['synthetic-a', 1]); expect(list.credentials.value[0]?.revision).toBe(2)
  })
  it('关闭后的迟到成功不恢复编辑器、事实或提示，也不解除新的事务锁', async () => {
    const { list, editor, api } = setup(); const write = deferred<StorageCredentialListItem>(); api.createStorageCredential.mockImplementationOnce(() => write.promise)
    editor.openCreate(); fill(editor); const pending = editor.submitForm(); editor.closeEditor(); editor.openRotate(item('synthetic-b')); expect(editor.editorOpen.value).toBe(false)
    write.resolve(item()); await pending; expect(editor.editorOpen.value).toBe(false); expect(secretsEmpty(editor)).toBe(true); expect(list.notice.value).toBe(''); expect(list.credentials.value).toHaveLength(0); expect(list.writeBusy.value).toBe(false)
  })
  it.each(['read', 'create', 'rotate', 'delete'] as const)('卸载后 %s 响应和错误不得回写，取消信号及清秘密有效', async kind => {
    const { list, editor, deletion, api, scope, signals } = setup(); await list.loadCredentials()
    let pending: Promise<void>; const response = deferred<StorageCredentialListItem>(); const reading = deferred<StorageCredentialListItem[]>(); const deleting = deferred<void>()
    if (kind === 'read') { api.listStorageCredentials.mockImplementationOnce(() => reading.promise); editor.openCreate(); fill(editor); pending = list.loadCredentials() }
    else if (kind === 'delete') { api.deleteStorageCredential.mockImplementationOnce(() => deleting.promise); deletion.requestDelete(item()); pending = deletion.confirmDelete() }
    else { if (kind === 'create') { editor.openCreate(); api.createStorageCredential.mockImplementationOnce(() => response.promise) } else { editor.openRotate(item()); api.rotateStorageCredential.mockImplementationOnce(() => response.promise) }; fill(editor); pending = editor.submitForm() }
    scope.stop(); const previous = list.credentials.value; if (kind === 'read') reading.resolve([item('synthetic-late')]); else if (kind === 'delete') deleting.reject({ status: 409 }); else response.resolve(item('synthetic-late'))
    await pending; expect(list.credentials.value).toBe(previous); expect(list.notice.value).toBe(''); expect(list.feedback.value).toBe(''); expect(secretsEmpty(editor)).toBe(true); expect(signals.every(signal => signal.aborted)).toBe(true)
  })
  it('删除入口防重且版本保留，取消确认使迟到错误失效', async () => {
    const { list, deletion, api } = setup(); await list.loadCredentials(); const request = deferred<void>(); api.deleteStorageCredential.mockImplementationOnce(() => request.promise)
    deletion.requestDelete(item()); const pending = deletion.confirmDelete(); await deletion.confirmDelete(); expect(api.deleteStorageCredential).toHaveBeenCalledTimes(1); expect(api.deleteStorageCredential).toHaveBeenCalledWith('synthetic-a', 1)
    deletion.cancelDelete(); request.reject({ status: 409 }); await pending; expect(list.feedback.value).toBe(''); expect(deletion.pendingDelete.value).toBeUndefined()
    deletion.requestDelete(item()); await deletion.confirmDelete(); expect(list.credentials.value).toHaveLength(0)
  })
})
