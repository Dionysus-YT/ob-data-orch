<script setup lang="ts">
withDefaults(defineProps<{
  label: string
  required?: boolean
  optional?: string
  helper?: string
  error?: string
  errorId?: string
  as?: 'label' | 'div'
}>(), { required: false, optional: '', helper: '', error: '', errorId: '', as: 'label' })
</script>

<template>
  <component :is="as" class="workbench-form-field" :aria-invalid="error ? 'true' : undefined">
    <span class="workbench-form-field-label">
      {{ label }}<b v-if="required">*</b><span v-if="optional" class="workbench-form-field-optional">{{ optional }}</span>
    </span>
    <slot />
    <span v-if="error" :id="errorId || undefined" class="workbench-form-field-error" role="alert">{{ error }}</span>
    <span v-else-if="helper" class="workbench-form-field-helper">{{ helper }}</span>
  </component>
</template>

<style scoped>
.workbench-form-field {
  display: grid;
  min-width: 0;
  align-content: start;
  gap: var(--space-1);
  color: #4a525c;
  font-size: var(--text-label-table-size);
  font-weight: var(--font-weight-medium);
  line-height: var(--text-label-table-line-height);
}

.workbench-form-field-label {
  display: inline-flex;
  width: fit-content;
  min-width: 0;
  align-items: baseline;
  gap: var(--space-1);
  white-space: nowrap;
}

.workbench-form-field-label b {
  flex: none;
  color: #c53b32;
  font-weight: var(--font-weight-semibold);
}

.workbench-form-field-optional,
.workbench-form-field-helper,
.workbench-form-field-error {
  font-size: var(--text-metadata-size);
  font-weight: var(--font-weight-regular);
  line-height: var(--text-metadata-line-height);
}

.workbench-form-field-optional { margin-left: var(--space-1); color: var(--color-text-tertiary); }
.workbench-form-field-helper { color: var(--color-text-tertiary); }
.workbench-form-field-error { color: #b42318; }
</style>
