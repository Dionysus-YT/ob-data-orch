import { describe, expect, it, vi } from 'vitest'
import { createLogQueryApi, parseLogContext, parseLogPage } from './logQuery'

export function testLogItem() { return { token: 'record-1', taskId: 'task-1', source: 'OBDUMPER_STDOUT', streamId: 'a'.repeat(64), epoch: 1, sequence: 2, kind: 'LOG' as const, message: '<script>synthetic</script>\npassword=******', integrityCode: '', receivedAt: '2026-10-25T00:00:00Z' } }
export function testLogPage() { return { items: [testLogItem()], nextCursor: 'next', pageCursor: 'first', snapshotToken: 'snapshot', snapshotAt: '2026-10-25T00:00:01Z', sources: ['OBDUMPER_STDOUT'], availability: 'RETAINED' as const, expired: false, incomplete: false, truncated: false, scannedRecords: 1, scannedBytes: 200 } }
export function testLogContext() { return { items: [{ ...testLogItem(), token: '' }], selectedSequence: 2, snapshotAt: '2026-10-25T00:00:01Z', incomplete: false } }
describe('日志查询 API 投影', () => {
  it('保留原文换行并只编码固定过滤条件', async () => {
    const read = vi.fn().mockResolvedValue({ item: testLogPage() }), write = vi.fn().mockResolvedValue({ text: 'safe' }), download = vi.fn().mockResolvedValue(new Blob(['safe']))
    const api = createLogQueryApi({ read, write, download }), signal = new AbortController().signal
    expect(await api.queryTaskLogs({ from: '2026-10-25T00:00:00Z', to: '2026-10-25T01:00:00Z', taskId: 'task-1', source: '', keyword: 'a&b+[]', limit: 200 }, 'cursor', signal)).toEqual(testLogPage())
    const url = new URL(read.mock.calls[0]![0], 'http://synthetic.invalid'); expect(url.pathname).toBe('/api/v1/logs/tasks'); expect(url.searchParams.get('keyword')).toBe('a&b+[]'); expect(read.mock.calls[0]![1]).toBe(signal)
    read.mockResolvedValueOnce({ item: testLogContext() }); expect(await api.getLogContext('record+/&', signal)).toEqual(testLogContext()); expect(new URL(read.mock.calls[1]![0], 'http://synthetic.invalid').searchParams.get('token')).toBe('record+/&')
    await api.copyLogContext('record', signal); expect(write).toHaveBeenCalledWith('/api/v1/logs/task-context:copy', { token: 'record' }, signal)
    await api.downloadTaskLogs('snapshot', true, signal); expect(download).toHaveBeenCalledWith('/api/v1/logs/tasks:download', { token: 'snapshot', fullTask: true }, signal)
  })
  it('拒绝未知字段、无效时间、预算越界和伪造总数', () => {
    for (const value of [{ ...testLogPage(), rawPath: 'synthetic-private-only' }, { ...testLogPage(), snapshotAt: '2026-02-31T00:00:00Z' }, { ...testLogPage(), scannedRecords: 50001 }, { ...testLogPage(), scannedBytes: 64 * 1024 * 1024 + 1 }, { ...testLogPage(), truncated: true }, { ...testLogPage(), total: 99 }, { ...testLogPage(), sources: ['SSH'] }]) {
      try { parseLogPage(value); expect.fail('无效响应被接受') } catch (error) { expect(error).toMatchObject({ code: 'RESPONSE_INVALID' }); expect(JSON.stringify(error)).not.toContain('synthetic-private-only') }
    }
  })
  it('拒绝混入来源、错序、越窗和未取得当前行的上下文', () => {
    const c = testLogContext(), other = { ...testLogItem(), token: '', sequence: 3 }
    for (const value of [{ ...c, items: [...c.items, { ...other, streamId: 'b'.repeat(64) }] }, { ...c, items: [...c.items, { ...other, epoch: 2 }] }, { ...c, items: [...c.items, { ...other, sequence: 53 }] }, { ...c, items: [other, ...c.items] }, { ...c, selectedSequence: 99 }, { ...c, items: [] }]) expect(() => parseLogContext(value)).toThrow()
  })
  it('未提供可靠级别和来源时间时拒绝猜测字段', () => {
    for (const item of [{ ...testLogItem(), level: 'INFO' }, { ...testLogItem(), sourceTime: '2026-10-25T00:00:00Z' }, { ...testLogItem(), source: 'AGENT' }, { ...testLogItem(), token: '' }, { ...testLogItem(), sequence: 0 }, { ...testLogItem(), streamId: '../file' }]) expect(() => parseLogPage({ ...testLogPage(), items: [item] })).toThrow()
  })
})
