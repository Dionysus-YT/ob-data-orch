<script setup lang="ts">
import { Form as AForm, FormItem as AFormItem } from 'ant-design-vue'
import { Button as AButton, Input as AInput, Select as ASelect, SelectOption as ASelectOption, Textarea as ATextarea } from 'ant-design-vue'
import { computed, nextTick, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useNodeForm } from '@/workbench/nodes/useNodeForm'

const route = useRoute()
const router = useRouter()
const errorSummary = ref<HTMLElement>()
const nodeID = computed(() => typeof route.params.id === 'string' ? route.params.id : '')
const isNew = computed(() => route.name === 'node-new')
const { node, loading, busy, failure, notice, formErrors, form, errorEntries, loadNode, save, clearError, validateField } = useNodeForm(
  nodeID, isNew,
  id => router.replace({ name: 'node-detail', params: { id }, query: { registration: 'created' } }),
  async active => { await nextTick(); if (active()) errorSummary.value?.focus() },
)
const rootPlaceholder = computed(() => form.platform === 'WINDOWS_AMD64' ? '/E:/ob-data/exports' : '/var/lib/ob-data-orch/exports')
const toolHomePlaceholder = computed(() => form.platform === 'WINDOWS_AMD64' ? 'E:\\tools\\ob-loader-dumper-4.3.5-RELEASE' : '/opt/ob-loader-dumper-4.3.5-RELEASE')
const javaPathPlaceholder = computed(() => form.platform === 'WINDOWS_AMD64' ? 'C:\\Program Files\\Java\\jdk8\\bin\\java.exe' : '/usr/lib/jvm/java-8/bin/java')
</script>

<template>
  <section class="page-heading node-form-heading">
    <div>
      <h1>{{ isNew ? '注册执行节点' : '编辑执行节点' }}</h1>
      <p>登记目标平台与本机路径。保存后由目标机器上的 Agent 回写运行事实，管理字段不能代替环境检查。</p>
    </div>
    <RouterLink v-slot="{ href, navigate }" to="/nodes" custom><AButton :href="href" @click="navigate">返回执行节点</AButton></RouterLink>
  </section>

  <p v-if="failure" class="feedback feedback-error" role="alert">{{ failure }}</p>
  <p v-else-if="notice" class="feedback feedback-notice" role="status">{{ notice }}</p>

  <section v-if="loading" class="content-card loading-state">正在加载执行节点…</section>
  <section v-else-if="!isNew && !node" class="content-card empty-state">
    <h2>无法打开执行节点</h2>
    <p>{{ failure || '节点详情不可用。' }}</p>
    <AButton @click="loadNode">重试</AButton>
  </section>

  <div v-else class="node-form-workspace">
    <AForm :model="form" :disabled="busy" layout="vertical" class="node-form-surface" @submit.prevent="save">
      <div v-if="errorEntries.length" ref="errorSummary" class="node-error-summary" role="alert" tabindex="-1">
        <strong>请检查以下字段</strong>
        <ul><li v-for="entry in errorEntries" :key="entry.field"><a :href="`#${entry.id}`">{{ entry.label }}：{{ entry.message }}</a></li></ul>
      </div>
      <section class="node-form-section" aria-labelledby="node-basics-title">
        <h2 id="node-basics-title">节点标识</h2>
        <p>名称用于任务和日志中识别节点；目标平台决定 Agent 安装包与路径格式。</p>
        <div class="node-form-grid">
          <AFormItem label="节点名称" name="displayName" html-for="node-display-name" required :validate-status="formErrors.displayName ? 'error' : undefined" :help="formErrors.displayName">
            <AInput id="node-display-name" v-model:value.trim="form.displayName" :aria-invalid="Boolean(formErrors.displayName)" :maxlength="200" @input="clearError('displayName')" @blur="validateField('displayName')" />
          </AFormItem>
          <AFormItem label="目标平台" name="platform" html-for="node-platform" required :validate-status="formErrors.platform ? 'error' : undefined" :help="formErrors.platform" extra="这是任务路由声明；实际操作系统与架构以后续 Agent 上报为准。">
            <ASelect id="node-platform" v-model:value="form.platform" :aria-invalid="Boolean(formErrors.platform)" @change="clearError('platform')">
              <ASelectOption value="WINDOWS_AMD64">Windows AMD64</ASelectOption>
              <ASelectOption value="LINUX_AMD64">Kylin Linux AMD64</ASelectOption>
              <ASelectOption value="LINUX_ARM64">Kylin Linux ARM64</ASelectOption>
            </ASelect>
          </AFormItem>
        </div>
      </section>
      <section class="node-form-section" aria-labelledby="node-runtime-title">
        <h2 id="node-runtime-title">工具运行时</h2>
        <p>路径只作为目标机器上的声明配置；Agent 同步后会在本机核验。</p>
        <div class="node-form-grid">
          <AFormItem label="OB Loader/Dumper 安装目录" name="toolHome" html-for="node-tool-home" required :validate-status="formErrors.toolHome ? 'error' : undefined" :help="formErrors.toolHome" extra="填写目标执行机上的工具安装根目录；首次关联后由 Agent 在本机核验。">
            <AInput id="node-tool-home" v-model:value.trim="form.toolHome" :aria-invalid="Boolean(formErrors.toolHome)" :placeholder="toolHomePlaceholder" @input="clearError('toolHome')" @blur="validateField('toolHome')" />
          </AFormItem>
          <AFormItem label="工具专用 Java 8 路径" name="javaPath" html-for="node-java-path" required :validate-status="formErrors.javaPath ? 'error' : undefined" :help="formErrors.javaPath" extra="填写 Java 可执行文件的绝对路径；不会读取系统 PATH，也不修改机器环境变量。">
            <AInput id="node-java-path" v-model:value.trim="form.javaPath" :aria-invalid="Boolean(formErrors.javaPath)" :placeholder="javaPathPlaceholder" @input="clearError('javaPath')" @blur="validateField('javaPath')" />
          </AFormItem>
        </div>
      </section>
      <section class="node-form-section" aria-labelledby="node-roots-title">
        <h2 id="node-roots-title">导出数据目录</h2>
        <p>只允许导出写入列出的节点侧目录；具体任务还会独立复核路径与空间。</p>
        <AFormItem label="导出数据目录白名单" name="allowedRoots" html-for="node-allowed-roots" required :validate-status="formErrors.allowedRoots ? 'error' : undefined" :help="formErrors.allowedRoots" extra="每行一个绝对目录。Windows 使用 /E:/exports 形式；Linux 使用 / 开头的绝对路径。">
          <ATextarea id="node-allowed-roots" v-model:value="form.allowedRootsText" :aria-invalid="Boolean(formErrors.allowedRoots)" :rows="4" :placeholder="rootPlaceholder" @input="clearError('allowedRoots')" @blur="validateField('allowedRoots')" />
        </AFormItem>
      </section>
      <div class="node-form-boundary" role="note">{{ isNew ? '新节点固定为已禁用、待关联。保存后进入一次性 Agent 关联。' : '修改配置后保留当前管理状态；Agent 空闲时同步配置并重新检查环境。' }}未取得有效环境事实前，节点不能接收新任务。</div>
      <div class="node-form-actions">
        <AButton :loading="busy" html-type="submit" type="primary">{{ isNew ? '保存并继续 Agent 关联' : '保存修改' }}</AButton>
        <RouterLink v-slot="{ href, navigate }" to="/nodes" custom><AButton :href="href" @click="navigate">取消</AButton></RouterLink>
      </div>
    </AForm>
  </div>
</template>

<style scoped>
.node-form-workspace { max-width: 960px; }
.node-form-surface { border: 1px solid var(--ob-color-border); border-radius: var(--ob-component-table-radius); background: var(--ob-color-surface); }
.node-form-section { padding: var(--ob-foundation-space-6); }
.node-form-section + .node-form-section { border-top: 1px solid var(--ob-color-border); }
.node-form-section h2 { margin: 0 0 var(--ob-foundation-space-1); color: var(--ob-color-form-text); font-size: var(--ob-product-typography-section-size); }
.node-form-section > p { margin: 0 0 var(--ob-foundation-space-4); color: var(--ob-color-form-secondary); font-size: var(--ob-component-field-label-size); line-height: 1.6; }
.node-form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 var(--ob-foundation-space-4); }
.node-form-boundary { margin: 0 var(--ob-foundation-space-6) var(--ob-foundation-space-4); padding: var(--ob-foundation-space-3); color: var(--ob-color-form-secondary); background: var(--ob-color-subtle); font-size: var(--ob-component-field-label-size); line-height: 1.6; }
.node-form-actions { display: flex; flex-wrap: wrap; gap: var(--ob-foundation-space-2); padding: var(--ob-foundation-space-4) var(--ob-foundation-space-6); border-top: 1px solid var(--ob-color-border); }
.node-error-summary { margin: var(--ob-foundation-space-6) var(--ob-foundation-space-6) 0; padding: var(--ob-foundation-space-3); border: 1px solid var(--ob-foundation-danger-border); color: var(--ob-color-danger); background: var(--ob-foundation-danger-surface); }
.node-error-summary ul { margin: var(--ob-foundation-space-2) 0 0; padding-left: var(--ob-foundation-space-6); }
.node-error-summary a { color: inherit; }
@media (max-width: 720px) { .node-form-grid { grid-template-columns: 1fr; }.node-form-section { padding: var(--ob-foundation-space-4); }.node-form-boundary { margin-inline: var(--ob-foundation-space-4); }.node-form-actions { padding-inline: var(--ob-foundation-space-4); } }
</style>
