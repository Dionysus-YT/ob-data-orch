<script setup lang="ts">
import { computed } from 'vue'
import { Button, Tooltip } from 'ant-design-vue'
import { QuestionCircleOutlined } from '@ant-design/icons-vue'
import { normalImportParameterInfo } from '../normalImportParameterCatalog'

const props = defineProps<{ name: string }>()
const detail = computed(() => normalImportParameterInfo(props.name))
</script>

<template>
  <Tooltip v-if="detail" :trigger="['hover', 'focus']">
    <template #title>
      <div class="normal-import-parameter-content">
        <div class="normal-import-parameter-name">参数：<code>--{{ name }}</code></div>
        <div>含义：{{ detail.description }}</div>
      </div>
    </template>
    <Button type="text" size="small" class="normal-import-parameter-hint" :aria-label="`${detail.label}参数说明`">
      <template #icon><QuestionCircleOutlined aria-hidden="true" /></template>
    </Button>
  </Tooltip>
</template>

<style scoped>
.normal-import-parameter-hint { flex: none; color: var(--ob-color-secondary); }
.normal-import-parameter-content { overflow-wrap: anywhere; }
.normal-import-parameter-name { margin-block-end: var(--ob-foundation-space-2); }
</style>
