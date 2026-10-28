<script setup lang="ts">
import { computed } from 'vue'
import { RouterView, useRoute, useRouter } from 'vue-router'
import { Tabs as ATabs, TabPane as ATabPane } from 'ant-design-vue'

const route = useRoute(), router = useRouter()
const tabs = [
  { key: 'platform-settings', label: '常规设置' },
  { key: 'scheduling-policy', label: '调度策略' },
  { key: 'security-policy', label: '安全策略' },
  { key: 'tool-compatibility', label: '工具与兼容' },
]
// 页签只投影已完成的路由；离开守卫拒绝切换时，当前内容和选中项保持一致。
const activeTab = computed(() => String(route.name))
function selectTab(key: string | number) {
  if (tabs.some(tab => tab.key === key) && key !== activeTab.value) void router.push({ name: String(key) })
}
</script>

<template>
  <div class="orch-ui">
    <header class="orch-page-header"><div class="orch-page-title"><h1>平台设置</h1></div></header>
    <section class="settings-main settings-workspace" aria-label="平台设置">
      <ATabs :active-key="activeTab" :animated="false" :destroy-inactive-tab-pane="true" @change="selectTab">
        <ATabPane v-for="tab in tabs" :key="tab.key" :tab="tab.label">
          <RouterView v-if="activeTab === tab.key" />
        </ATabPane>
      </ATabs>
    </section>
  </div>
</template>
