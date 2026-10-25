<script setup lang="ts">
import { computed, ref, useId } from 'vue'
import { Alert, Button, Drawer, Input, Spin, Tag } from 'ant-design-vue'
import { useAntDrawerDialog } from '@/composables/useAntDrawerDialog'
import type { DirectLoadStepModel } from '../directLoadStepModels'
const props = defineProps<{ open: boolean; model: DirectLoadStepModel }>()
const emit = defineEmits<{ close: []; locate: [step: number] }>()
const keyword = ref(''), titleId = useId()
const { heading, afterOpenChange } = useAntDrawerDialog(titleId, () => emit('close'), undefined, () => props.open)
void heading
const rows = computed(() => { const query = keyword.value.trim().toLocaleLowerCase(); return props.model.metadata.filter(item => `${item.longName} ${item.description} ${item.disposition}`.toLocaleLowerCase().includes(query)) })
const nodeKeys = ['direct_path_load.task_timeout_ms', 'direct_path_load.heartbeat_timeout_ms', 'direct_path_load.heartbeat_interval_ms']
function step(name: string) { return ['host','port','rpc-port','tenant','cluster','public-cloud','user','password'].includes(name) ? 1 : ['database','table'].includes(name) ? 2 : ['file-path','csv','sql','par','orc','column-separator','external-data'].includes(name) ? 3 : 4 }
function locate(name: string) { emit('close'); emit('locate', step(name)) }
</script>
<template><Drawer :open="open" width="min(100vw, 720px)" :destroy-on-close="true" @close="emit('close')" @after-open-change="afterOpenChange"><template #title><h2 :id="titleId" ref="heading" class="ant-drawer-title" tabindex="-1">旁路导入全部参数</h2></template><div class="direct-load-catalog"><label :for="titleId + '-search'">搜索参数、含义或旁路适用状态</label><Input :id="titleId + '-search'" v-model:value="keyword" placeholder="例如 rpc-port、重试、CSV" allow-clear /><Spin v-if="model.loadingMetadata" /><Alert v-if="model.metadataFailure" type="error" show-icon :message="model.metadataFailure"><template #action><Button type="link" @click="model.loadMetadata">重试</Button></template></Alert><p role="status">共 {{ model.metadata.length }} 个 CLI 参数，当前显示 {{ rows.length }} 项；另含 3 个只读节点配置键。目录覆盖不授予执行资格。</p><ul><li v-for="row in rows" :key="row.longName"><div><code>--{{ row.longName }}</code><Tag>{{ row.supportState === 'ENABLED' ? '旁路已确认' : row.supportState === 'VALIDATION_GATED' ? '待验证 / 不开放' : '不适用 / 不生成' }}</Tag><Button v-if="row.supportState === 'ENABLED'" type="link" @click="locate(row.longName)">定位步骤 {{ step(row.longName) }}</Button></div><p>{{ row.description }}</p><p>{{ row.disposition }}</p></li></ul><h3>执行节点配置（只读）</h3><p v-for="key in nodeKeys.filter(key => !keyword || key.includes(keyword))" :key="key"><code>{{ key }}</code></p><p>实际值、配置来源及指纹由受控节点取证；向导不编辑配置文件，也不补造默认值。</p></div></Drawer></template>
