import { reactive } from 'vue'
import type { DirectLoadConfig } from '@/api/directLoad'
import { directLoadDefaultRpcPort } from './directLoadRules'

/** 旁路唯一表单；没有运行事实、秘密或浏览器持久存储。 */
export function createDirectLoadForm(): DirectLoadConfig {
  return reactive(createPlain())
}
/** 切换场景时只更新空值或原场景默认端口，人工自定义值与草稿恢复值不会被隐式覆盖。 */
export function applyDirectLoadScenario(form: DirectLoadConfig, scenario: DirectLoadConfig['scenario']) {
  if (form.scenario === scenario) return
  const useDefault = !form.rpcPort || form.rpcPort === directLoadDefaultRpcPort(form.scenario)
  form.scenario = scenario
  if (useDefault) form.rpcPort = directLoadDefaultRpcPort(scenario)
}
/** 只投影当前格式的活动配置，保留原路径且不生成任何命令。 */
export function directLoadInput(form: DirectLoadConfig): DirectLoadConfig {
  return { ...form, applicability: { ...form.applicability }, columnSeparator: form.format === 'CSV' ? form.columnSeparator : '', externalData: form.format === 'CSV' && form.externalData }
}
/** 逐字段恢复唯一表单；不把旧检查、确认或运行事实带入会话。 */
export function hydrateDirectLoad(form: DirectLoadConfig, config: DirectLoadConfig) { Object.assign(form, config, { applicability: { ...config.applicability } }) }
/** 配置对象的键顺序不参与语义，服务端序列化顺序变化不会造成误判。 */
export function directLoadSignature(config: DirectLoadConfig): string { return JSON.stringify(directLoadInput({ ...createPlain(), ...config, applicability: { ...createPlain().applicability, ...config.applicability } })) }
function createPlain(): DirectLoadConfig { return { dataSourceId: '', nodeId: '', database: '', scenario: '', rpcPort: 0, table: '', filePath: '', format: '', externalData: false, columnSeparator: '', thread: '', parallel: '', replaceData: false, truncateTable: false, applicability: { singleTable: false, wholeTableCommit: false, noCheckpoint: false, structureLimits: false } } }
