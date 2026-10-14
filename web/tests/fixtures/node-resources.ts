import { createApp, h, ref } from 'vue'
import { Button, ConfigProvider, Space } from 'ant-design-vue'
import NodeExecutionQuota from '../../src/workbench/nodes/NodeExecutionQuota.vue'
import { platformTheme } from '../../src/platform/theme'
import 'ant-design-vue/dist/reset.css'
import '../../src/platform/tokens.css'
import '../../src/platform/components.css'

// 隔离夹具仅验证真实组件的分段交互；这些预算不进入生产 API 或状态。
createApp({
  setup() {
    const mode = ref('正常')
    const quotas = {
      正常: { totalGiB: 20, usedGiB: 10, reservedGiB: 5 },
      空额度: { totalGiB: 20, usedGiB: 0, reservedGiB: 0 },
      满额度: { totalGiB: 20, usedGiB: 20, reservedGiB: 0 },
      未知额度: undefined,
    }
    return () => h(ConfigProvider, { theme: platformTheme }, () => h('main', { style: 'padding:24px;max-width:260px' }, [
      h('h1', '节点额度合成验证'),
      h(Space, { wrap: true }, () => Object.keys(quotas).map(label => h(Button, { onClick: () => { mode.value = label } }, () => label))),
      h(NodeExecutionQuota, { quota: quotas[mode.value as keyof typeof quotas] }),
    ]))
  },
}).mount('#app')
