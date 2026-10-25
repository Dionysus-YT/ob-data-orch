import { afterEach, describe, expect, it, vi } from 'vitest'
import { computed, effectScope, ref } from 'vue'
import type { NormalImportApi, NormalImportCheck, NormalImportConfig, NormalImportDraft } from '@/api/normalImport'
import { createNormalImportForm } from './normalImportForm'
import { hydrateNormalImport, normalImportDraftInput } from './normalImportDraftInput'
import { projectNormalImport } from './normalImportRules'
import { useNormalImportDraft } from './useNormalImportDraft'
import { useNormalImportReview } from './useNormalImportReview'

const scopes: ReturnType<typeof effectScope>[] = []
afterEach(() => { scopes.splice(0).forEach(scope => scope.stop()); vi.useRealTimers(); vi.unstubAllGlobals() })
function deferred<T>() { let resolve!: (value: T) => void; let reject!: (error: unknown) => void; const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no }); return { resolve, reject, promise } }
function record(config: NormalImportConfig, id = 'draft-normal', revision = 1): NormalImportDraft { return { id, revision, configVersion: 'normal-import-v1', config, configFingerprint: 'a'.repeat(64), executionEnabled: false } }
function setup() {
  const scope = effectScope(); scopes.push(scope)
  return scope.run(() => {
    const form = createNormalImportForm()
    Object.assign(form, { dataSourceId: 'source', nodeId: 'node', database: 'synthetic_db', filePath: 'E:\\input', content: 'DATA_ONLY', format: 'CSV', scope: 'ALL', taskName: '合成任务' })
    const input = computed(() => normalImportDraftInput(projectNormalImport(form, {})))
    const api: NormalImportApi = {
      createNormalImportDraft: vi.fn(async config => record(config)), updateNormalImportDraft: vi.fn(async (draft, config) => record(config, draft.id, draft.revision + 1)),
      getNormalImportDraft: vi.fn(), listNormalImportDrafts: vi.fn(), deleteNormalImportDraft: vi.fn(), previewNormalImportCommand: vi.fn(),
      startNormalImportCheck: vi.fn(async () => 'check-normal'), getNormalImportCheck: vi.fn(), submitNormalImport: vi.fn(),
    }
    const bindRoute = vi.fn(async () => undefined), navigateTask = vi.fn(async () => undefined)
    const draft = useNormalImportDraft({ api, form, input, bindRoute })
    const binding = computed(() => JSON.stringify(form))
    const review = useNormalImportReview({ api, draft, binding, complete: ref(true), filesReady: ref(true), risky: ref(false), navigateTask })
    return { scope, form, input, api, draft, review, bindRoute, navigateTask }
  })!
}
function report(draft: NormalImportDraft, filesOnly = false): NormalImportCheck {
  return { id: 'check-normal', draftId: draft.id, draftRevision: draft.revision, configFingerprint: draft.configFingerprint, checkKind: filesOnly ? 'NORMAL_IMPORT_FILES' : 'NORMAL_IMPORT_PREFLIGHT', status: 'SUCCEEDED', integrityStatus: 'COMPLETE', executionEnabled: true, validUntil: new Date(Date.now() + 60000).toISOString(),
    results: (filesOnly ? ['INPUT_FILES'] : ['DATABASE_CONNECTIVITY', 'WRITE_ACCESS', 'TOOL_ENVIRONMENT', 'NODE_RESOURCE', 'INPUT_FILES', 'INPUT_MAPPING']).map(check => ({ check, status: 'PASSED' as const, evidenceCode: 'SYNTHETIC_OK' })),
    inputManifest: { files: [{ path: 'synthetic.csv', kind: 'DATA', object: 'synthetic', size: 2, modified: '1790000000000000000', sha256: 'b'.repeat(64) }], fileCount: 1, totalBytes: 2, digest: 'c'.repeat(64) } }
}

describe('普通导入草稿与冻结事务', () => {
  it('源兼容版本恢复原值，旧草稿不猜测版本，关闭后不恢复残留版本', () => {
    const form = createNormalImportForm()
    const base: NormalImportConfig = { dataSourceId: 'source', nodeId: 'node', database: 'synthetic_db', filePath: 'E:\\input', content: 'DDL_ONLY', scope: 'ALL', cleanup: 'NONE', parser: {}, mapping: { kind: 'POSITION' }, execution: {}, compatMysql: true }
    for (const compatMysqlVersion of ['mysql56', 'mysql57', 'mysql80'] as const) {
      hydrateNormalImport(form, { ...base, compatMysqlVersion })
      expect(form.compatMysqlVersion).toBe(compatMysqlVersion)
    }
    hydrateNormalImport(form, base)
    expect(form.compatMysqlVersion).toBe('')
    hydrateNormalImport(form, { ...base, compatMysql: false, compatMysqlVersion: 'mysql80' })
    expect(form.compatMysqlVersion).toBe('')
  })
  it('原配置重跑拒绝程序化改写，但允许同配置重新绑定当前引用事实', async () => {
    const s = setup(), original = structuredClone(s.input.value)
    const source = { sourceTaskId: 'source-task', sourceDerivation: 'RERUN_FROM_SCRATCH' as const }
    vi.mocked(s.api.getNormalImportDraft).mockResolvedValue({ ...record(original), ...source })
    vi.mocked(s.api.updateNormalImportDraft).mockImplementation(async (draft, config) => ({ ...record(config, draft.id, draft.revision + 1), ...source }))
    await s.draft.loadDraft('draft-normal')
    expect(s.draft.rerunLocked.value).toBe(true)
    s.form.filePath = 'E:\\changed'
    expect(await s.draft.saveDraft()).toBeNull()
    expect(s.api.updateNormalImportDraft).not.toHaveBeenCalled()
    expect(s.draft.currentDraft.value?.config.filePath).toBe(original.filePath)
    s.form.filePath = original.filePath
    s.draft.invalidateDraftBinding()
    expect(await s.draft.ensureSaved()).toMatchObject({ revision: 2, ...source })
    expect(s.draft.rerunLocked.value).toBe(true)
    expect(s.draft.draftNeedsRebind.value).toBe(false)
  })
  it('保存响应丢失派生来源时不接受新修订或解除原配置锁定', async () => {
    const s = setup(), source = { sourceTaskId: 'source-task', sourceDerivation: 'RERUN_FROM_SCRATCH' as const }
    vi.mocked(s.api.getNormalImportDraft).mockResolvedValue({ ...record(s.input.value), ...source })
    await s.draft.loadDraft('draft-normal')
    s.draft.invalidateDraftBinding()
    expect(await s.draft.ensureSaved()).toBeNull()
    expect(s.draft.currentDraft.value).toMatchObject({ revision: 1, ...source })
    expect(s.draft.rerunLocked.value).toBe(true)
    expect(s.draft.draftNeedsRebind.value).toBe(true)
    expect(s.draft.draftFailure.value).not.toBe('')
  })
  it('检查结果总览不把已通过外推为允许执行，过期后撤销通过提示', async () => {
    const s = setup(), draft = await s.draft.saveDraft()
    vi.mocked(s.api.getNormalImportCheck).mockResolvedValue(report(draft!))
    await s.review.startCheck()
    expect(s.review.precheckOverview.value).toMatchObject({ type: 'warning', message: '预检查通过，执行暂未开放' })
    s.draft.currentDraft.value!.executionEnabled = true
    expect(s.review.precheckOverview.value).toMatchObject({ type: 'success', message: '预检查通过，可以提交' })
    s.review.checkExpired.value = true
    expect(s.review.precheckOverview.value.type).toBe('warning')
    expect(s.review.canSubmit.value).toBe(false)
  })
  it('复制失败仅保留在复制操作区，重试清除错误且迟到失败不能回写新会话', async () => {
    const s = setup(), pending = deferred<void>(), writeText = vi.fn().mockRejectedValueOnce(new Error('合成剪贴板失败')).mockResolvedValueOnce(undefined).mockReturnValueOnce(pending.promise)
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    s.review.commandPreview.value = { redactedCommand: 'obloader -p ******', configFingerprint: 'a'.repeat(64), executionEnabled: false }
    await s.review.copyCommand()
    expect(s.review.copyFailure.value).toContain('手动选择复制')
    expect(s.review.reviewFailure.value).toBe('')
    await s.review.copyCommand()
    expect(s.review.copyFailure.value).toBe('')
    expect(s.review.copyNotice.value).toBe('脱敏命令已复制。')
    const copying = s.review.copyCommand()
    s.form.database = 'synthetic_new_session'
    pending.reject(new Error('迟到的合成失败'))
    await copying
    expect(s.review.copyFailure.value).toBe('')
    expect(s.review.copyNotice.value).toBe('')
    expect(s.review.copyingCommand.value).toBe(false)
  })
  it('保存防重，保存期间继续编辑不被旧响应覆盖或误标为已保存', async () => {
    const s = setup(), pending = deferred<NormalImportDraft>()
    vi.mocked(s.api.createNormalImportDraft).mockReturnValue(pending.promise)
    const saving = s.draft.saveDraft(); await s.draft.saveDraft()
    s.form.database = 'new_database'
    pending.resolve(record({ ...s.input.value, database: 'synthetic_db' }))
    expect(await saving).toBeNull()
    expect(s.form.database).toBe('new_database'); expect(s.draft.currentDraft.value?.revision).toBe(1); expect(s.draft.draftDirty.value).toBe(true)
    expect(s.api.createNormalImportDraft).toHaveBeenCalledTimes(1)
    const sent = vi.mocked(s.api.createNormalImportDraft).mock.calls[0]![0]
    expect(sent.database).toBe('synthetic_db'); expect(sent).not.toHaveProperty('compatibilityMode'); expect(sent).not.toHaveProperty('credentialRevision')
  })
  it('A → B → A 与卸载隔离迟到草稿、错误及 finally', async () => {
    const s = setup(), first = deferred<NormalImportDraft>(), second = deferred<NormalImportDraft>(), third = deferred<NormalImportDraft>()
    vi.mocked(s.api.getNormalImportDraft).mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise).mockReturnValueOnce(third.promise)
    const a = s.draft.loadDraft('A'); s.draft.resetDraft(); const b = s.draft.loadDraft('B'); s.draft.resetDraft(); const last = s.draft.loadDraft('A')
    first.resolve(record({ ...s.input.value, database: 'stale' }, 'A')); second.reject(new Error('旧错误')); await Promise.all([a, b])
    expect(s.draft.loadingDraft.value).toBe(true); expect(s.draft.draftFailure.value).toBe('')
    third.resolve(record({ ...s.input.value, database: 'current' }, 'A')); await last
    expect(s.form.database).toBe('current'); expect(s.draft.draftDirty.value).toBe(false)
    const stale = deferred<NormalImportDraft>(); vi.mocked(s.api.getNormalImportDraft).mockReturnValue(stale.promise)
    const loading = s.draft.loadDraft('B'); s.scope.stop(); stale.reject(new Error('卸载后错误')); await loading
    expect(s.draft.draftFailure.value).toBe(''); expect(s.draft.currentDraft.value).toBeNull()
  })
  it('保存响应不得换成另一个活动配置或覆盖读取对象绑定', async () => {
    const s = setup()
    vi.mocked(s.api.createNormalImportDraft).mockResolvedValue(record({ ...s.input.value, database: 'other_database' }))
    expect(await s.draft.saveDraft()).toBeNull(); expect(s.draft.currentDraft.value).toBeNull(); expect(s.draft.draftFailure.value).not.toBe('')
    vi.mocked(s.api.getNormalImportDraft).mockResolvedValue(record(s.input.value, 'other-draft'))
    await s.draft.loadDraft('wanted-draft'); expect(s.draft.currentDraft.value).toBeNull(); expect(s.draft.draftLoadFailure.value).not.toBe('')
  })
  it('命令预览在配置 A → B → A 后拒绝旧结果，复制也不能补放成功提示', async () => {
    const s = setup(), pending = deferred<{ redactedCommand: string; configFingerprint: string; executionEnabled: boolean }>()
    await s.draft.saveDraft(); vi.mocked(s.api.previewNormalImportCommand).mockReturnValue(pending.promise)
    const preview = s.review.previewCommand(); await Promise.resolve()
    s.form.database = 'B'; s.form.database = 'synthetic_db'
    pending.resolve({ redactedCommand: 'obloader --no-sys', configFingerprint: 'a'.repeat(64), executionEnabled: false }); await preview
    expect(s.review.commandPreview.value).toBeNull(); expect(s.review.previewingCommand.value).toBe(false)
  })
  it('文件发现不能替代完整预检查；未开放的执行门禁持续阻断提交', async () => {
    const s = setup(); const draft = await s.draft.saveDraft(); expect(draft).not.toBeNull()
    vi.mocked(s.api.getNormalImportCheck).mockResolvedValue(report(draft!, true))
    await s.review.startCheck(true)
    expect(s.review.discoveredFiles.value?.inputManifest?.fileCount).toBe(1); expect(s.review.activePrecheck.value).toBeNull(); expect(s.review.canSubmit.value).toBe(false)
    vi.mocked(s.api.getNormalImportCheck).mockResolvedValue(report(draft!))
    await s.review.startCheck(false)
    expect(s.review.canSubmit.value).toBe(false)
    expect(s.api.submitNormalImport).not.toHaveBeenCalled()
  })
  it('完整检查过期或配置变化立即撤销资格和确认，重复发起只产生一条检查', async () => {
    vi.useFakeTimers(); const s = setup(); const draft = await s.draft.saveDraft(); draft!.executionEnabled = true; s.draft.currentDraft.value = draft
    const pending = deferred<string>(); vi.mocked(s.api.startNormalImportCheck).mockReturnValue(pending.promise)
    vi.mocked(s.api.getNormalImportCheck).mockResolvedValue(report(draft!))
    const check = s.review.startCheck(); await Promise.resolve(); await s.review.startCheck(); expect(s.api.startNormalImportCheck).toHaveBeenCalledTimes(1)
    pending.resolve('check-normal'); await check; expect(s.review.canSubmit.value).toBe(true)
    s.review.submitConfirmationOpen.value = true; await vi.advanceTimersByTimeAsync(60001)
    expect(s.review.canSubmit.value).toBe(false); expect(s.review.submitConfirmationOpen.value).toBe(false)
    s.form.filePath = 'E:\\other'; expect(s.review.activePrecheck.value).toBeNull()
  })
  it('迟到提交响应在新配置下不导航，检查绑定错误保持失败关闭', async () => {
    const s = setup(), draft = await s.draft.saveDraft(); draft!.executionEnabled = true; s.draft.currentDraft.value = draft
    vi.mocked(s.api.getNormalImportCheck).mockResolvedValue(report(draft!)); await s.review.startCheck()
    const pending = deferred<string>(); vi.mocked(s.api.submitNormalImport).mockReturnValue(pending.promise)
    await s.review.submitTask(); const submitting = s.review.submitTask(true); s.form.database = 'changed'; pending.resolve('task-normal'); await submitting
    expect(s.navigateTask).not.toHaveBeenCalled()
    vi.mocked(s.api.getNormalImportCheck).mockResolvedValue({ ...report(draft!), draftRevision: 999 }); await s.review.startCheck()
    expect(s.review.canSubmit.value).toBe(false); expect(s.review.reviewFailure.value).not.toBe('')
  })
  it('提交前弹窗可选命名，名称不改变检查绑定；确认防重且无效名称不提交', async () => {
    const s = setup(), draft = await s.draft.saveDraft(); draft!.executionEnabled = true; s.draft.currentDraft.value = draft
    vi.mocked(s.api.getNormalImportCheck).mockResolvedValue(report(draft!)); await s.review.startCheck()
    await s.review.submitTask(); expect(s.review.submitConfirmationOpen.value).toBe(true); expect(s.api.submitNormalImport).not.toHaveBeenCalled()
    s.review.taskNameInput.value = '名'.repeat(201); await s.review.submitTask(true)
    expect(s.review.taskNameError.value).not.toBe(''); expect(s.api.submitNormalImport).not.toHaveBeenCalled()
    s.review.taskNameInput.value = '  自定义合成任务  '
    const pending = deferred<string>(); vi.mocked(s.api.submitNormalImport).mockReturnValue(pending.promise)
    const submitting = s.review.submitTask(true); await s.review.submitTask(true)
    expect(s.api.submitNormalImport).toHaveBeenCalledExactlyOnceWith(draft, 'check-normal', undefined, '自定义合成任务')
    expect(s.review.activePrecheck.value?.id).toBe('check-normal'); expect(s.draft.draftDirty.value).toBe(false)
    pending.resolve('task-normal'); await submitting; expect(s.navigateTask).toHaveBeenCalledWith('task-normal')
  })
  it('名称留空由服务端生成，配置变更会清除提交名称并关闭弹窗', async () => {
    const s = setup(), draft = await s.draft.saveDraft(); draft!.executionEnabled = true; s.draft.currentDraft.value = draft
    vi.mocked(s.api.getNormalImportCheck).mockResolvedValue(report(draft!)); await s.review.startCheck()
    vi.mocked(s.api.submitNormalImport).mockRejectedValue(new Error('合成提交失败'))
    await s.review.submitTask(); s.review.taskNameInput.value = '   '; await s.review.submitTask(true)
    expect(s.api.submitNormalImport).toHaveBeenCalledWith(draft, 'check-normal', undefined, undefined)
    expect(s.review.submitConfirmationOpen.value).toBe(true)
    s.form.filePath = 'E:\\changed'; expect(s.review.submitConfirmationOpen.value).toBe(false); expect(s.review.taskNameInput.value).toBe('')
  })
  it('显式重存撤销旧修订事实，引用事实变化会在检查前更新绑定', async () => {
    const s = setup(), draft = await s.draft.saveDraft()
    vi.mocked(s.api.getNormalImportCheck).mockResolvedValue(report(draft!)); await s.review.startCheck()
    expect(s.review.activePrecheck.value?.status).toBe('SUCCEEDED')
    await s.draft.saveDraft()
    expect(s.review.activePrecheck.value).toBeNull(); expect(s.review.commandPreview.value).toBeNull()
    s.draft.invalidateDraftBinding(); expect(s.draft.draftDirty.value).toBe(false); expect(s.draft.draftNeedsRebind.value).toBe(true)
    const revision = s.draft.currentDraft.value!.revision
    await s.draft.ensureSaved(); expect(s.draft.currentDraft.value!.revision).toBe(revision + 1); expect(s.draft.draftNeedsRebind.value).toBe(false)
  })
  it('检查读取期间并行保存的新修订不能接纳旧报告或恢复轮询', async () => {
    const s = setup(), draft = await s.draft.saveDraft(), pending = deferred<NormalImportCheck>()
    vi.mocked(s.api.getNormalImportCheck).mockReturnValue(pending.promise)
    const checking = s.review.startCheck(); await vi.waitFor(() => expect(s.api.getNormalImportCheck).toHaveBeenCalledTimes(1))
    await s.draft.saveDraft(); pending.resolve(report(draft!)); await checking
    expect(s.review.activePrecheck.value).toBeNull(); expect(s.review.checking.value).toBe(false); expect(s.review.canSubmit.value).toBe(false)
  })
})
