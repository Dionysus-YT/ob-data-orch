<script setup lang="ts">
import { type UnwrapNestedRefs } from 'vue'
import { Alert as AAlert, Button as AButton, Form as AForm, FormItem as AFormItem, Input as AInput } from 'ant-design-vue'
import OrchDangerConfirm from '@/components/OrchDangerConfirm.vue'
import type { useTemplateActions } from './useTemplateActions'

defineProps<{ model: UnwrapNestedRefs<ReturnType<typeof useTemplateActions>>; busy: boolean }>()
const emit = defineEmits<{ refresh: [] }>()
</script>

<template>
  <OrchDangerConfirm :open="Boolean(model.renameTarget)" title="改名" confirm-label="保存" :busy="busy" :confirm-disabled="model.renameConflict" @confirm="model.confirmRename()" @cancel="!busy && model.cancelRename()">
    <p>{{ model.renameTarget?.displayName }}</p>
    <AForm layout="vertical" @finish="model.confirmRename()"><AFormItem label="模板名称" required :validate-status="model.renameError ? 'error' : undefined" :help="model.renameError || undefined"><AInput :value="model.renameValue" aria-label="模板名称" :disabled="busy || model.renameConflict" @update:value="value => model.updateRenameValue(String(value))" @blur="model.validateRename()" /></AFormItem></AForm>
    <AAlert v-if="model.renameFailure" type="error" :message="model.renameConflict ? '模板已变更' : '改名未完成'" :description="model.renameFailure" show-icon><template v-if="model.renameConflict" #action><AButton @click="emit('refresh')">刷新列表</AButton></template></AAlert>
  </OrchDangerConfirm>
</template>
