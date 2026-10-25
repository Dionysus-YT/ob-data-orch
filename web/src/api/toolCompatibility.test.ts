import { describe, expect, it, vi } from 'vitest'
import { createToolCompatibilityApi, parseToolCompatibility, type ToolCompatibility } from './toolCompatibility'

function baseline(): ToolCompatibility {
  return { capturedAt: '2026-10-24T01:00:00Z', loadedAt: '2026-10-24T00:00:00Z', source: 'BUILT_IN_CATALOGS', tools: [
    { tool: 'OBDUMPER', supportedVersions: ['4.3.5-RELEASE'], defaultTargetVersion: '4.3.5-RELEASE', modules: [{ taskType: 'OBDUMPER_EXPORT', metadataSelection: 'TASK_CONFIGURATION', catalogs: [{ metadataVersion: 'obdumper-4.3.5-slice-v11-expert-parameters', usage: 'TASK_CONFIGURATION' }, { metadataVersion: 'obdumper-4.3.5-slice-v6', usage: 'HISTORICAL_REPLAY' }] }] },
    { tool: 'OBLOADER', supportedVersions: ['4.3.5-RELEASE'], defaultTargetVersion: '4.3.5-RELEASE', modules: [{ taskType: 'OBLOADER_NORMAL_IMPORT', metadataSelection: 'FIXED', catalogs: [{ metadataVersion: 'obloader-4.3.5-normal-v1', usage: 'TASK_CONFIGURATION' }] }, { taskType: 'OBLOADER_DIRECT_LOAD', metadataSelection: 'FIXED', catalogs: [{ metadataVersion: 'obloader-4.3.5-direct-v1', usage: 'TASK_CONFIGURATION' }] }] },
  ] }
}

describe('工具基线严格投影', () => {
  it('使用固定 GET 传递取消信号并区分各模块目录', async () => {
    const read = vi.fn().mockResolvedValue({ requestId: 'synthetic', item: baseline() }), api = createToolCompatibilityApi({ read }), controller = new AbortController()
    expect(await api.getToolCompatibility(controller.signal)).toEqual(baseline()); expect(read).toHaveBeenCalledWith('/api/v1/settings/tools', controller.signal); expect(Object.keys(api)).toEqual(['getToolCompatibility'])
  })
  it('拒绝未知字段、坏日期、未来载入或缺失目录且不回显', () => {
    const missing = { ...baseline() } as Record<string, unknown>; delete missing.tools
    for (const input of [{ ...baseline(), filePath: 'synthetic-private-only' }, { ...baseline(), loadedAt: '2026-02-31T00:00:00Z' }, { ...baseline(), loadedAt: '2027-01-01T00:00:00Z' }, { ...baseline(), tools: [] }, missing, Object.create(baseline())]) {
      try { parseToolCompatibility(input); expect.fail('非法工具状态未拒绝') } catch (error) { expect(error).toMatchObject({ code: 'RESPONSE_INVALID' }); expect(JSON.stringify(error)).not.toContain('synthetic-private-only') }
    }
  })
  it('拒绝未知工具、默认越界、模块混用及伪造安装状态', () => {
    const input = () => structuredClone(baseline()) as unknown as { tools: { tool: string; defaultTargetVersion: string; installedVersion?: string; modules: { taskType: string; metadataSelection: string; catalogs: { metadataVersion: string; usage: string }[] }[] }[] }
    const wrongTool = input(); wrongTool.tools[0]!.tool = 'OTHER'
    const target = input(); target.tools[1]!.defaultTargetVersion = 'synthetic-other'
    const modules = input(); modules.tools[1]!.modules.reverse()
    const mixed = input(); mixed.tools[1]!.modules[0]!.catalogs[0]!.metadataVersion = 'obloader-4.3.5-direct-v1'
    const history = input(); history.tools[1]!.modules[0]!.catalogs[0]!.usage = 'HISTORICAL_REPLAY'
    const duplicate = input(); duplicate.tools[0]!.modules[0]!.catalogs.push(duplicate.tools[0]!.modules[0]!.catalogs[0]!)
    const active = input(); active.tools[0]!.modules[0]!.metadataSelection = 'FIXED'
    const installed = input(); installed.tools[0]!.installedVersion = 'synthetic-guessed'
    for (const value of [wrongTool, target, modules, mixed, history, duplicate, active, installed]) expect(() => parseToolCompatibility(value)).toThrow()
  })
  it('返回独立副本，调用者不能改写传输响应', () => {
    const input = baseline(), output = parseToolCompatibility(input)
    expect(output).toEqual(input); expect(output.tools).not.toBe(input.tools); expect(output.tools[0]!.modules[0]!.catalogs).not.toBe(input.tools[0]!.modules[0]!.catalogs)
  })
})
