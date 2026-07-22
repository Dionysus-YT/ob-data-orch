<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

import WizardFrame from '@/components/WizardFrame.vue'

const route = useRoute()
const activeStep = computed(() => {
  const value = Number(route.query.step ?? '1')
  return Number.isInteger(value) && value >= 1 && value <= 6 ? value : 1
})

const titles = ['选择数据源', '选择导出对象', '选择导出内容', '选择数据格式', '执行与输出', '预检查与命令']
</script>

<template>
  <section class="page-heading"><div><h1>新建导出任务</h1><p>OBDUMPER 4.3.5 导出向导。未验证的参数或执行事实不会在页面中伪装成可用能力。</p></div><span class="draft-status">草稿尚未创建</span></section>
  <WizardFrame kind="export" :active-step="activeStep">
    <template #default>
      <section v-if="activeStep === 1" class="form-section"><h2>选择已有数据源</h2><p>向导只选择已启用、当前配置至少一次基础测试成功的数据源；不重复填写地址、用户名或密码。</p><div class="placeholder-control"><span>数据源候选将在连接测试结果持久化后加载</span></div><p class="section-hint">没有可选数据源？请先到数据源管理新增并完成基础连接测试。</p></section>
      <section v-else-if="activeStep === 2" class="form-section"><h2>选择导出对象</h2><div class="option-grid"><label class="option-card selected"><input checked type="radio" name="scope" />全部对象<span>严格生成全部对象范围；与具体对象互斥。</span></label><label class="option-card"><input type="radio" name="scope" />指定对象<span>按官方对象类型和表达式选择；至少包含一项。</span></label></div><div class="form-line"><label class="field-label">默认数据库 / Schema <b>*</b><input disabled placeholder="选择数据源后可回填" /></label></div><p class="section-hint">对象类型、版本和兼容模式的条件显示将按已确认映射基线处理；未知条件不开放。</p></section>
      <section v-else-if="activeStep === 3" class="form-section"><h2>选择导出内容</h2><div class="option-grid three"><label class="option-card"><input type="radio" name="content" />仅对象定义<span>生成 DDL；后续数据格式不生效。</span></label><label class="option-card selected"><input checked type="radio" name="content" />仅数据<span>需要选择一种官方支持的数据格式。</span></label><label class="option-card"><input type="radio" name="content" />对象定义和数据<span>分别展示 DDL 与数据输出风险。</span></label></div><p class="section-hint">仅 DDL 时跳过数据格式步骤；仅 DDL 对象不能进入仅数据任务。</p></section>
      <section v-else-if="activeStep === 4" class="form-section"><h2>选择数据格式</h2><div class="format-grid"><label class="format-card selected"><input checked type="radio" name="format" />CSV<span>推荐格式；可配置已映射的文本序列化字段。</span></label><label class="format-card"><input type="radio" name="format" />CUT<span>字符串分隔字段格式。</span></label><label class="format-card pending"><input type="radio" name="format" />POS<span>待官方参数映射确认</span></label><label class="format-card"><input type="radio" name="format" />Insert SQL<span>输出 INSERT SQL 数据文件。</span></label><label class="format-card"><input type="radio" name="format" />Parquet<span>列式格式；文件拆分字段不生效。</span></label><label class="format-card"><input type="radio" name="format" />ORC<span>列式格式；文件拆分字段不生效。</span></label><label class="format-card"><input type="radio" name="format" />Avro<span>官方支持的格式。</span></label></div><p class="section-hint">同一任务只允许一种数据格式。切换格式时保留草稿值，但非活动字段不会生成命令或进入提交快照。</p></section>
      <section v-else-if="activeStep === 5" class="form-section"><h2>执行与输出配置</h2><div class="section-block"><h3>1. 输出位置</h3><div class="radio-row"><label><input checked type="radio" name="output" />执行节点本地绝对路径</label><label><input disabled type="radio" name="output" />OSS / S3 / COS / OBS（待安全凭据设计）</label></div><label class="field-label wide-label">输出路径 <b>*</b><input placeholder="Windows 或 Linux 执行节点上的完整绝对路径" /></label><p class="section-hint">平台不追加子目录，也不转换 Windows 或 Linux 路径格式。</p></div><div class="section-block"><h3>2. 执行节点</h3><div class="placeholder-control short"><span>执行节点候选将在节点模块接入后按真实可接收状态加载</span></div></div><div class="section-block"><h3>3. 常规配置</h3><div class="parameter-list"><div><strong>导出线程</strong><span>高级配置 · 未设置时沿用官方默认</span><input disabled placeholder="未设置" /></div><div><strong>JVM 内存</strong><span>高级配置 · 未设置时沿用官方默认</span><input disabled placeholder="未设置" /></div><div><strong>压缩</strong><span>仅 CSV/CUT/POS/SQL · 未设置</span><select disabled><option>沿用官方默认</option></select></div></div></div><details class="parameter-fold"><summary>高级配置 <span>已映射字段将在功能接入后按条件显示</span></summary><p>输出、序列化、压缩、文件布局、性能和节点资源参数遵循官方默认、条件显示和互斥规则。</p></details><details class="parameter-fold"><summary>专家配置 <span>需要权限；未映射或高风险字段保持阻断</span></summary><p>DDL、筛选、一致性、日期时间和高风险选项只开放已确认映射，不能编辑命令片段。</p></details></section>
      <section v-else class="form-section"><h2>参数预检查与完整命令</h2><section class="configuration-summary"><h3>任务配置摘要</h3><dl><div><dt>数据源</dt><dd>尚未选择</dd></div><div><dt>对象与内容</dt><dd>尚未配置</dd></div><div><dt>输出与节点</dt><dd>尚未配置</dd></div></dl></section><section class="precheck-list"><h3>预检查结果</h3><div v-for="item in ['数据源连接', '导出对象', '输出配置', '执行节点', '参数约束']" :key="item" class="precheck-item"><span class="status-dot neutral" /><strong>{{ item }}</strong><span>尚未执行；完成当前配置后由 Agent 预检查。</span></div></section><section class="command-empty"><div><h3>完整命令（脱敏）</h3><button type="button" class="button button-secondary" disabled>复制脱敏命令</button></div><pre><code>完成配置并通过预检查后，由控制面生成只读、脱敏的完整 OBDUMPER 命令。</code></pre><p>参数来源将区分用户配置、平台派生、官方默认和未生效字段。</p></section></section>
    </template>
    <template #summary><h2>配置总览</h2><dl class="summary-definition"><div><dt>当前步骤</dt><dd>{{ titles[activeStep - 1] }}</dd></div><div><dt>数据源</dt><dd>尚未选择</dd></div><div><dt>导出范围</dt><dd>尚未配置</dd></div><div><dt>导出内容</dt><dd>尚未配置</dd></div><div><dt>数据格式</dt><dd>尚未配置</dd></div><div><dt>预检查</dt><dd>尚未执行</dd></div></dl><p class="aside-note">第 6 步只显示当前活动配置的脱敏完整命令。任何影响命令的变更都会使预检查失效。</p></template>
  </WizardFrame>
</template>
