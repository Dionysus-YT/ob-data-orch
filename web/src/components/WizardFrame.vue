<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{ kind: 'export' | 'normal' | 'direct'; activeStep?: number }>()

const steps = computed(() => {
  if (props.kind === 'export') return ['选择数据源', '导出对象', '导出内容', '数据格式', '执行与输出', '预检查与命令']
  if (props.kind === 'normal') return ['选择数据源', '文件来源', '内容与格式', '对象与映射', '执行参数', '预检查与命令']
  return ['适用条件', '连接与版本', '单表文件', '表与格式', '执行参数', '预检查与命令']
})
</script>

<template>
  <div class="wizard-frame">
    <ol class="stepper" aria-label="任务创建步骤">
      <li v-for="(step, index) in steps" :key="step" :class="{ active: index + 1 === (activeStep ?? 1), complete: index + 1 < (activeStep ?? 1) }"><span>{{ index + 1 }}</span><p>{{ step }}</p></li>
    </ol>
    <div class="wizard-columns"><section class="wizard-main"><slot /></section><aside class="wizard-summary"><slot name="summary" /></aside></div>
    <footer class="wizard-footer"><button type="button" class="button button-secondary">上一步</button><span class="footer-grow" /><button type="button" class="button button-tertiary">保存草稿</button><button type="button" class="button button-primary">下一步</button></footer>
  </div>
</template>
