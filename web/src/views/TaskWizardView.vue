<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

import WizardFrame from '@/components/WizardFrame.vue'

const route = useRoute()
const kind = computed(() => route.meta.kind as 'export' | 'normal' | 'direct')
const title = computed(() => kind.value === 'export' ? '新建导出任务' : kind.value === 'normal' ? '新建普通导入任务' : '新建旁路导入任务')
const intro = computed(() => kind.value === 'export' ? '选择已启用且最近一次基础连接测试成功的数据源。向导不会重复填写连接信息。' : kind.value === 'normal' ? '普通导入独立处理文件、内容格式、对象映射和检查点继续，不会引入旁路参数。' : '旁路导入是独立流程：先确认适用条件，再核对 SQL/RPC、版本和唯一目标表。')
</script>

<template>
  <section class="page-heading"><div><h1>{{ title }}</h1><p>{{ intro }}</p></div><span class="draft-status">草稿尚未创建</span></section>
  <WizardFrame :kind="kind" :active-step="1"><template #default><section class="form-section"><h2>{{ kind === 'direct' ? '适用条件确认' : '选择数据源' }}</h2><p v-if="kind === 'export'">只有已启用、当前配置至少一次成功测试且具备当前用户权限的数据源可以进入下一步。</p><p v-else-if="kind === 'normal'">文件来源、格式、对象和映射将在后续步骤按官方能力与已确认条件规则展示。</p><p v-else>旁路导入要求大批量单目标表、同表多分片、RPC 通道、表级整体提交和失败从头执行；不设置未经确认的数据量阈值。</p><div class="placeholder-control"><span>该步骤的真实候选项将随模块功能接入加载</span></div></section></template><template #summary><h2>配置总览</h2><dl class="summary-definition"><div><dt>数据源</dt><dd>尚未选择</dd></div><div><dt>任务类型</dt><dd>{{ kind === 'export' ? '导出' : kind === 'normal' ? '普通导入' : '旁路导入' }}</dd></div><div><dt>预检查</dt><dd>尚未执行</dd></div></dl><p class="aside-note">命令将在第 6 步根据实际配置生成；默认脱敏、只读且不可直接编辑。</p></template></WizardFrame>
</template>
