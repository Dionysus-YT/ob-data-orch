<script setup lang="ts">
import { Collapse, CollapsePanel, ConfigProvider } from 'ant-design-vue'
import type { ThemeConfig } from 'ant-design-vue/es/config-provider/context'
import { computed, ref } from 'vue'
import { component, foundation, semantic } from '@/platform/tokens'

defineProps<{ title: string; configuredCount: number }>()
const activeKey = ref<string | number | Array<string | number>>([])
const expanded = computed(() => Array.isArray(activeKey.value) ? activeKey.value.includes('advanced') : activeKey.value === 'advanced')

// 导出高级区仅收窄 Collapse 主题，复用既有颜色、间距和原生开合动画。
const advancedTheme: ThemeConfig = {
  components: {
    Collapse: {
      colorFillAlter: semantic.subtle,
      colorBgContainer: semantic.surface,
      colorBorder: semantic.border,
      borderRadiusLG: component.control.managementRadius,
      padding: foundation.space[4],
      paddingSM: foundation.space[3],
    },
  },
}
</script>

<template>
  <div class="export-advanced">
    <ConfigProvider :theme="advancedTheme">
      <Collapse v-model:active-key="activeKey" class="export-advanced-collapse" expand-icon-position="end">
        <CollapsePanel key="advanced">
          <template #header>
            <div class="advanced-heading"><span class="advanced-title">高级设置 · {{ title }}</span><span v-if="configuredCount" class="advanced-count">（已配置 {{ configuredCount }} 项）</span></div>
          </template>
          <template #extra><span class="advanced-toggle" aria-hidden="true">{{ expanded ? '收起' : '展开' }}</span></template>
          <slot />
        </CollapsePanel>
      </Collapse>
    </ConfigProvider>
  </div>
</template>

<style scoped>
.export-advanced { min-inline-size: 0; }
.advanced-heading { display: flex; align-items: baseline; flex-wrap: wrap; gap: var(--ob-foundation-space-2); min-inline-size: 0; }
.advanced-title { font-weight: var(--ob-product-typography-weight); overflow-wrap: anywhere; }
.advanced-count { color: var(--ob-color-secondary); font-size: var(--ob-component-field-helper-size); white-space: nowrap; }
.advanced-toggle { color: var(--ob-color-secondary); font-size: var(--ob-component-field-helper-size); white-space: nowrap; }
</style>
