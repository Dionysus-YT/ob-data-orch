<script setup lang="ts">
defineProps<{ steps: readonly string[]; activeStep: number }>()
</script>

<template>
  <ol class="orch-task-step-rail" aria-label="任务创建步骤">
    <li v-for="(step, index) in steps" :key="step" :aria-current="index + 1 === activeStep ? 'step' : undefined" :class="{ 'is-current': index + 1 === activeStep, 'is-visited': index + 1 < activeStep }"><span aria-hidden="true">{{ index + 1 }}</span><div>{{ step }}<small>{{ index + 1 === activeStep ? '当前步骤' : index + 1 < activeStep ? '已访问' : '待配置' }}</small></div></li>
  </ol>
</template>
